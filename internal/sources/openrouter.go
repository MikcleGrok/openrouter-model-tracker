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
	ContextLength int    `json:"context_length"`
	Pricing       struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
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
		out[slug] = PriceInfo{
			Slug:    slug,
			InPerM:  in,
			OutPerM: outPrice,
			Context: m.ContextLength,
			Free:    m.Pricing.Prompt == "0" && m.Pricing.Completion == "0",
			Found:   true,
		}
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
