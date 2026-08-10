// Package sources fetches every external data source the comparison document
// depends on. One file per source; each score source exposes a Fetch function
// taking a slug -> exact-name-on-this-source map built from model-map.tsv, so
// no source ever guesses which model a leaderboard row belongs to.
package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/sboborikin/openrouter-model-tracker/internal/httpcache"
)

// CatalogURL is the public OpenRouter catalogue endpoint. It needs no auth.
// It is a var rather than a const so tests can point it at an httptest server.
var CatalogURL = "https://openrouter.ai/api/v1/models"

// PriceInfo is one model's entry in the OpenRouter catalogue.
type PriceInfo struct {
	Slug    string
	InPerM  float64
	OutPerM float64
	Context int
	Free    bool
	Found   bool

	// Created is the catalogue's publication timestamp (Unix seconds) and
	// Description is the vendor's prose about the model. They are catalogue
	// data exactly like Context and the prices: same response, same entry,
	// no derivation and no normalisation. PriceInfo's name is a little
	// narrower than its job, which its own doc comment already admits — it
	// is this model's entry in the OpenRouter catalogue.
	Created     int64
	Description string
	Name        string

	// CanonicalSlug is the catalogue's own stable identifier for this
	// entry — OpenRouter builds its own links.details URLs out of it rather
	// than out of id — and HuggingFaceID is the bare "<org>/<repo>" path of
	// the model's HuggingFace repository. Both are carried verbatim and are
	// never derived: canonical_slug disagrees with id for most of the
	// catalogue, and hugging_face_id routinely sits under a different
	// organisation than the slug does, so a guess would link to a 404 or,
	// worse, to a different model's page.
	CanonicalSlug string
	HuggingFaceID string

	// HasOverride and the three fields below surface the catalogue's
	// long-context pricing tier: a model whose prompt exceeds
	// OverrideMinTokens bills at OverrideInPerM/OverrideOutPerM instead of
	// InPerM/OutPerM.
	HasOverride bool
	// OverrideMinTokens is the min_prompt_tokens of the override with the
	// SMALLEST threshold. The catalogue's overrides array is parsed in full,
	// but only the lowest threshold is surfaced today: every override we've
	// observed in the wild is a single long-context tier, and picking the
	// smallest is the one choice that stays correct if a second, higher tier
	// ever shows up.
	OverrideMinTokens int
	OverrideInPerM    float64
	OverrideOutPerM   float64
}

type catalogResponse struct {
	Data []catalogModel `json:"data"`
}

// catalogModel deliberately declares only the fields we use. Everything else in
// the payload is skipped by encoding/json, which keeps us immune to new or
// polymorphic fields appearing upstream.
type catalogModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Created       int64  `json:"created"`
	Description   string `json:"description"`
	CanonicalSlug string `json:"canonical_slug"`
	HuggingFaceID string `json:"hugging_face_id"`
	ContextLength int    `json:"context_length"`
	Pricing       struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
		// Overrides is the long-context pricing tier: alternate prompt/
		// completion prices that kick in once the request's prompt exceeds
		// MinPromptTokens. Not every model has one. Cache-price fields
		// inside each entry are deliberately not declared here — this tool
		// tracks no cache pricing anywhere.
		Overrides []struct {
			MinPromptTokens int    `json:"min_prompt_tokens"`
			Prompt          string `json:"prompt"`
			Completion      string `json:"completion"`
		} `json:"overrides"`
	} `json:"pricing"`
}

func fetchCatalog(ctx context.Context, c *httpcache.Client) ([]catalogModel, error) {
	body, err := c.Get(ctx, CatalogURL)
	if err != nil {
		return nil, fmt.Errorf("openrouter: fetch catalogue: %w", err)
	}
	var resp catalogResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("openrouter: decode catalogue: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("openrouter: catalogue at %s returned no models", CatalogURL)
	}
	return resp.Data, nil
}

// perMillion converts an OpenRouter per-token price string into $/M tokens,
// rounded to four decimals — the same conversion the retired
// openrouter-pricing.sh did.
func perMillion(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return math.Round(v*1e6*10000) / 10000, nil
}

// LookupPrices returns one PriceInfo per requested slug. A slug missing from
// the catalogue comes back with Found == false rather than being omitted.
func LookupPrices(ctx context.Context, c *httpcache.Client, slugs []string) (map[string]PriceInfo, error) {
	models, err := fetchCatalog(ctx, c)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]catalogModel, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}

	out := make(map[string]PriceInfo, len(slugs))
	for _, slug := range slugs {
		m, ok := byID[slug]
		if !ok {
			out[slug] = PriceInfo{Slug: slug}
			continue
		}
		in, err := perMillion(m.Pricing.Prompt)
		if err != nil {
			return nil, fmt.Errorf("openrouter: %s: parse prompt price %q: %w", slug, m.Pricing.Prompt, err)
		}
		outPrice, err := perMillion(m.Pricing.Completion)
		if err != nil {
			return nil, fmt.Errorf("openrouter: %s: parse completion price %q: %w", slug, m.Pricing.Completion, err)
		}
		info := PriceInfo{
			Slug:          slug,
			InPerM:        in,
			OutPerM:       outPrice,
			Context:       m.ContextLength,
			Free:          m.Pricing.Prompt == "0" && m.Pricing.Completion == "0",
			Found:         true,
			Created:       m.Created,
			Description:   m.Description,
			Name:          m.Name,
			CanonicalSlug: m.CanonicalSlug,
			HuggingFaceID: m.HuggingFaceID,
		}
		for _, ov := range m.Pricing.Overrides {
			// A zero/absent threshold is not a usable long-context tier —
			// skip it rather than let it win the tie-break and render a
			// bogus "от 0K+" claim.
			if ov.MinPromptTokens <= 0 {
				continue
			}
			if info.HasOverride && ov.MinPromptTokens >= info.OverrideMinTokens {
				continue
			}
			ovIn, err := perMillion(ov.Prompt)
			if err != nil {
				return nil, fmt.Errorf("openrouter: %s: parse override prompt price %q: %w", slug, ov.Prompt, err)
			}
			ovOut, err := perMillion(ov.Completion)
			if err != nil {
				return nil, fmt.Errorf("openrouter: %s: parse override completion price %q: %w", slug, ov.Completion, err)
			}
			info.HasOverride = true
			info.OverrideMinTokens = ov.MinPromptTokens
			info.OverrideInPerM = ovIn
			info.OverrideOutPerM = ovOut
		}
		out[slug] = info
	}
	return out, nil
}

// CatalogSlugs returns every model id in the catalogue, sorted.
func CatalogSlugs(ctx context.Context, c *httpcache.Client) ([]string, error) {
	models, err := fetchCatalog(ctx, c)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(models))
	for _, m := range models {
		slugs = append(slugs, m.ID)
	}
	sort.Strings(slugs)
	return slugs, nil
}
