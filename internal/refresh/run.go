package refresh

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/httpcache"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

// cacheTTL is how long a fetched page stays fresh on disk.
const cacheTTL = 12 * time.Hour

// Options configures one run.
type Options struct {
	DataDir    string
	OutputPath string
	DryRun     bool
}

// scoreSource is one benchmark source, identified by the column name it uses in
// model-map.tsv.
type scoreSource struct {
	id string
	fn func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error)
}

// deps is the seam that lets run be tested without touching the network.
type deps struct {
	prices  func(ctx context.Context, slugs []string) (map[string]sources.PriceInfo, error)
	catalog func(ctx context.Context) ([]string, error)
	sources []scoreSource
	now     func() time.Time
}

func liveDeps(opts Options) deps {
	c := httpcache.New(filepath.Join(opts.DataDir, "cache", "http"), cacheTTL)
	return deps{
		prices: func(ctx context.Context, slugs []string) (map[string]sources.PriceInfo, error) {
			return sources.LookupPrices(ctx, c, slugs)
		},
		catalog: func(ctx context.Context) ([]string, error) {
			return sources.CatalogSlugs(ctx, c)
		},
		// Order is priority: the first source with a row for a slug wins, and
		// two numbers are never averaged.
		sources: []scoreSource{
			{id: "swebench", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
				return sources.FetchSWEBenchVerified(ctx, c, names)
			}},
			{id: "vals", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
				return sources.FetchValsSWEBench(ctx, c, names)
			}},
		},
		now: time.Now,
	}
}

// Run performs one refresh against the live sources.
func Run(ctx context.Context, opts Options) (Report, error) {
	return run(ctx, opts, liveDeps(opts))
}

func run(ctx context.Context, opts Options, d deps) (Report, error) {
	entries, err := modelmap.Load(filepath.Join(opts.DataDir, "model-map.tsv"))
	if err != nil {
		return Report{}, err
	}
	nt, err := notes.Load(filepath.Join(opts.DataDir, "notes.yaml"))
	if err != nil {
		return Report{}, err
	}
	snapshotPath := filepath.Join(opts.DataDir, "cache", "last-run-snapshot.json")
	snap, err := LoadSnapshot(snapshotPath)
	if err != nil {
		return Report{}, err
	}

	var (
		mu       sync.Mutex
		prices   map[string]sources.PriceInfo
		pricesOK bool
		catalog  []string
		rows     = make(map[string][]sources.ScoreRow, len(d.sources))
		warnings []string
	)
	warn := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	// One goroutine per source. A source that fails warns and returns; it never
	// aborts the run and never wipes the other sources' results.
	var wg sync.WaitGroup

	// Catalogue and prices share the same underlying OpenRouter URL and the same
	// on-disk HTTP cache entry, so they run sequentially in one goroutine —
	// catalog first to warm the cache, then prices reads the now-fresh entry —
	// instead of as two goroutines that could both miss a cold/expired cache at
	// once and race writing the same cache file.
	wg.Add(1)
	go func() {
		defer wg.Done()
		c, err := d.catalog(ctx)
		if err != nil {
			warn("openrouter: каталог не получен (%v) — новые и снятые модели в этом прогоне не искались", err)
		} else {
			mu.Lock()
			catalog = c
			mu.Unlock()
		}

		p, err := d.prices(ctx, modelmap.Slugs(entries))
		if err != nil {
			warn("openrouter: цены не получены (%v) — берутся из снимка прошлого прогона", err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		prices, pricesOK = p, true
	}()

	for _, s := range d.sources {
		wg.Add(1)
		go func(s scoreSource) {
			defer wg.Done()
			r, err := s.fn(ctx, modelmap.NamesFor(entries, s.id))
			if err != nil {
				warn("%s: источник недоступен или изменил структуру (%v) — оценки берутся из снимка", s.id, err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			rows[s.id] = r
		}(s)
	}
	wg.Wait()

	var scores []sources.ScoreRow
	for _, s := range d.sources {
		scores = append(scores, rows[s.id]...)
	}

	today := d.now().Format("2006-01-02")
	prices, scores, stalePrices, staleScores := applyFallback(entries, prices, pricesOK, scores, snap)

	// A whole-catalogue failure with no snapshot fallback for some tracked slug
	// would make model.Merge silently drop it — never overwrite the document
	// with fewer rows than are actually tracked.
	if !pricesOK {
		var missing []string
		for _, e := range entries {
			if _, ok := prices[e.Slug]; !ok {
				missing = append(missing, e.Slug)
			}
		}
		if len(missing) > 0 {
			return Report{Warnings: warnings}, fmt.Errorf(
				"refresh: OpenRouter catalogue unreachable and %d model(s) have no snapshot fallback: %v — refusing to overwrite the document",
				len(missing), missing)
		}
	}

	models := model.Merge(entries, prices, scores, nt)
	report := BuildReport(entries, catalog, prices, models)
	report.Warnings = warnings
	// markStale runs after BuildReport: it appends to Note/ScoreLabel, and
	// BuildReport's NeedsReview check needs to see the original, unmutated Note.
	markStale(models, stalePrices, staleScores, today)
	if opts.DryRun {
		return report, nil
	}

	var buf bytes.Buffer
	if err := Render(&buf, BuildRenderData(models, nt, today)); err != nil {
		return report, err
	}
	if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
		return report, fmt.Errorf("refresh: create output directory: %w", err)
	}
	if err := os.WriteFile(opts.OutputPath, buf.Bytes(), 0o644); err != nil {
		return report, fmt.Errorf("refresh: write %s: %w", opts.OutputPath, err)
	}
	if err := NewSnapshot(models, today).Save(snapshotPath); err != nil {
		return report, err
	}
	return report, nil
}

// applyFallback fills the gaps a failed source left, from the previous run's
// snapshot, and reports which slugs were filled so markStale can label them.
func applyFallback(entries []modelmap.Entry, prices map[string]sources.PriceInfo, pricesOK bool, scores []sources.ScoreRow, snap *Snapshot) (map[string]sources.PriceInfo, []sources.ScoreRow, map[string]bool, map[string]bool) {
	stalePrices := map[string]bool{}
	staleScores := map[string]bool{}
	if prices == nil {
		prices = map[string]sources.PriceInfo{}
	}

	// Prices fall back only when the catalogue lookup failed as a whole. On a
	// successful lookup, Found == false means the model really is gone, and
	// resurrecting it from the snapshot would hide a retirement.
	if !pricesOK {
		for _, e := range entries {
			se, ok := snap.Models[e.Slug]
			if !ok {
				continue
			}
			prices[e.Slug] = sources.PriceInfo{
				Slug:    e.Slug,
				InPerM:  se.InPerM,
				OutPerM: se.OutPerM,
				Context: se.Context,
				Free:    se.InPerM == 0 && se.OutPerM == 0,
				Found:   true,
			}
			stalePrices[e.Slug] = true
		}
	}

	scored := make(map[string]bool, len(scores))
	for _, r := range scores {
		scored[r.Slug] = true
	}
	for _, e := range entries {
		// A model that declares no source has nothing that could have failed:
		// its number, if any, comes from notes.yaml and is never stale.
		if scored[e.Slug] || len(e.Names) == 0 {
			continue
		}
		se, ok := snap.Models[e.Slug]
		if !ok || se.Score == nil {
			continue
		}
		scores = append(scores, sources.ScoreRow{
			Slug:            e.Slug,
			Metric:          se.Score.Metric,
			Value:           se.Score.Value,
			VariantMeasured: se.Score.VariantMeasured,
			SourceURL:       se.Score.SourceURL,
			Checked:         se.Score.Checked,
		})
		staleScores[e.Slug] = true
	}
	return prices, scores, stalePrices, staleScores
}

// markStale labels the rows whose values came from the snapshot rather than
// from this run, in the document itself.
func markStale(models []model.Model, stalePrices, staleScores map[string]bool, date string) {
	for i := range models {
		m := &models[i]
		if stalePrices[m.Slug] {
			m.PriceStale = true
			m.Note = strings.TrimSpace(m.Note + " Цену не удалось проверить на " + date + " — значение из прошлого прогона.")
		}
		if staleScores[m.Slug] && m.Score != nil {
			m.Score.Stale = true
			m.ScoreLabel += " (не удалось проверить на " + date + ")"
		}
	}
}
