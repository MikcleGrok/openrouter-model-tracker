package refresh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/httpcache"
	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/modelmap"
	"github.com/sboborikin/openrouter-model-tracker/internal/notes"
	"github.com/sboborikin/openrouter-model-tracker/internal/pricehistory"
	"github.com/sboborikin/openrouter-model-tracker/internal/sources"
)

// Options configures one run.
type Options struct {
	DataDir           string
	OutputPath        string
	CacheDir          string
	CacheTTL          time.Duration
	CacheTTLSet       bool
	RequestTimeout    time.Duration
	RequestTimeoutSet bool
	DryRun            bool
}

// scoreSource is one benchmark source, identified by the column name it uses in
// model-map.tsv.
type scoreSource struct {
	id string
	fn func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error)
}

// deps is the seam that lets run be tested without touching the network.
type deps struct {
	prices      func(ctx context.Context, slugs []string) (map[string]sources.PriceInfo, error)
	catalog     func(ctx context.Context) ([]string, error)
	sources     []scoreSource
	now         func() time.Time
	saveHistory func(*pricehistory.History, string) error
	rename      func(string, string) error
	remove      func(string) error
}

func liveDeps(opts Options) deps {
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir = "cache"
	}
	if !filepath.IsAbs(cacheDir) {
		cacheDir = filepath.Join(opts.DataDir, cacheDir)
	}
	ttl := opts.CacheTTL
	if !opts.CacheTTLSet && ttl <= 0 {
		ttl = 12 * time.Hour
	}
	timeout := opts.RequestTimeout
	if !opts.RequestTimeoutSet && timeout <= 0 {
		timeout = 30 * time.Second
	}
	c := httpcache.NewWithTimeout(filepath.Join(cacheDir, "http"), ttl, timeout)
	return deps{
		prices: func(ctx context.Context, slugs []string) (map[string]sources.PriceInfo, error) {
			return sources.LookupPrices(ctx, c, slugs)
		},
		catalog: func(ctx context.Context) ([]string, error) {
			return sources.CatalogSlugs(ctx, c)
		},
		// Order is priority *within one family*: the first source of a
		// family with a row for a slug wins, and two numbers are never
		// averaged. Sources of different families feed different views and
		// never compete at all — see model.SourceFamily.
		sources: []scoreSource{
			{id: "swebench", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
				return sources.FetchSWEBenchVerified(ctx, c, names)
			}},
			{id: "vals", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
				return sources.FetchValsSWEBench(ctx, c, names)
			}},
			{id: "arena", fn: func(ctx context.Context, names map[string]string) ([]sources.ScoreRow, error) {
				return sources.FetchArenaElo(ctx, c, names)
			}},
		},
		now:         time.Now,
		saveHistory: func(history *pricehistory.History, path string) error { return history.Save(path) },
		rename:      os.Rename,
		remove:      os.Remove,
	}
}

// Run performs one refresh against the live sources.
func Run(ctx context.Context, opts Options) (Report, error) {
	return run(ctx, opts, liveDeps(opts))
}

func run(ctx context.Context, opts Options, d deps) (Report, error) {
	if d.saveHistory == nil {
		d.saveHistory = func(history *pricehistory.History, path string) error { return history.Save(path) }
	}
	if d.rename == nil {
		d.rename = os.Rename
	}
	if d.remove == nil {
		d.remove = os.Remove
	}
	entries, err := modelmap.Load(filepath.Join(opts.DataDir, "model-map.tsv"))
	if err != nil {
		return Report{}, err
	}
	nt, err := notes.Load(filepath.Join(opts.DataDir, "notes.yaml"))
	if err != nil {
		return Report{}, err
	}
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cacheDir = "cache"
	}
	if !filepath.IsAbs(cacheDir) {
		cacheDir = filepath.Join(opts.DataDir, cacheDir)
	}
	snapshotPath := filepath.Join(cacheDir, "last-run-snapshot.json")
	snap, err := LoadSnapshot(snapshotPath)
	if err != nil {
		return Report{}, err
	}
	historyPath := pricehistory.Path(opts.DataDir)
	history, err := pricehistory.Load(historyPath)
	if err != nil {
		return Report{}, err
	}

	var (
		mu        sync.Mutex
		prices    map[string]sources.PriceInfo
		pricesOK  bool
		catalog   []string
		catalogOK bool
		rows      = make(map[string][]sources.ScoreRow, len(d.sources))
		warnings  []string
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
			catalogOK = true
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

	// Rows are split by family, never concatenated into one slice: Merge takes
	// the first row per slug, so a single shared slice would let an Arena Elo
	// silently win the SWE-bench column of a model that has no SWE-bench row.
	// The switch is exhaustive and explicit on purpose: an id with no
	// model.SourceFamily entry — a typo, or a new source registered in
	// liveDeps before its family is wired up — must never fall through to
	// the SWE-bench branch by default. It is dropped instead, with a warning,
	// which is the same "safe default" model.SourceFamily itself documents
	// for an unknown id.
	var scores, arenaScores []sources.ScoreRow
	for _, s := range d.sources {
		switch model.SourceFamily[s.id] {
		case model.ScoreSourceSWEBench:
			scores = append(scores, rows[s.id]...)
		case model.ScoreSourceArena:
			arenaScores = append(arenaScores, rows[s.id]...)
		default:
			if n := len(rows[s.id]); n > 0 {
				warn("%s: у источника нет записи в model.SourceFamily — %d строк(и) отброшены, а не объединены ни с одним представлением", s.id, n)
			}
		}
	}

	// A source "succeeded" if its rows entry was ever set — even to an empty
	// slice — since the goroutine only reaches that assignment on a nil error.
	sourceOK := make(map[string]bool, len(d.sources))
	for _, s := range d.sources {
		_, ok := rows[s.id]
		sourceOK[s.id] = ok
	}

	today := d.now().Format("2006-01-02")
	prices, scores, stalePrices, staleScores := applyFallback(entries, prices, pricesOK, scores, sourceOK, nt, snap)
	arenaScores, staleArena := applyArenaFallback(entries, arenaScores, sourceOK, snap)

	models := model.MergeWithArena(entries, prices, scores, arenaScores, nt)
	report := BuildReport(entries, catalog, prices, pricesOK, models)
	report.PriceChanges = priceChanges(history, prices, pricesOK)
	if catalogOK && len(snap.CatalogSlugs) > 0 {
		report.CatalogAdded, report.CatalogRemoved = catalogDelta(snap.CatalogSlugs, catalog)
	}
	sort.Strings(warnings)
	report.Warnings = append(report.Warnings, warnings...)

	// Most of the raw NewCandidates list is preview/dated/distilled variants
	// of already-tracked families nobody will realistically add — filter
	// those out so the section stays reviewable. Runs after report.Warnings
	// is set above so a load failure here is never lost.
	patterns, err := loadIgnorePatterns(filepath.Join(opts.DataDir, "ignore-candidates.txt"))
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("ignore-candidates.txt: %v", err))
		sort.Strings(report.Warnings)
	} else {
		report.NewCandidates = filterIgnored(report.NewCandidates, patterns)
	}

	// markStale runs after BuildReport: it appends to Note/ScoreLabel, and
	// BuildReport's NeedsReview check needs to see the original, unmutated Note.
	markStale(models, stalePrices, staleScores, staleArena, today)

	// Report generation above is a cheap, useful diagnostic even on a dry run
	// (`openrouter check`) — a pure read-only report must never hard-fail, so
	// every guard below that blocks an actual WRITE runs only past this point.
	if opts.DryRun {
		return report, nil
	}

	// A whole-catalogue failure with no snapshot fallback for some tracked slug
	// would make model.MergeWithArena silently drop it — never overwrite the
	// document with fewer rows than are actually tracked.
	if !pricesOK {
		var missing []string
		for _, e := range entries {
			if _, ok := prices[e.Slug]; !ok {
				missing = append(missing, e.Slug)
			}
		}
		if len(missing) > 0 {
			return report, fmt.Errorf(
				"refresh: OpenRouter catalogue unreachable and %d model(s) have no snapshot fallback: %v — refusing to overwrite the document",
				len(missing), missing)
		}
	}

	// A live price lookup that succeeds but reports every tracked slug as gone
	// (e.g. OpenRouter renamed its slug scheme) would make MergeWithArena drop
	// every entry — never overwrite the document with an empty document.
	if len(models) == 0 {
		return report, fmt.Errorf("refresh: no tracked model has usable price data this run — refusing to overwrite the document")
	}

	var buf bytes.Buffer
	if err := Render(&buf, BuildRenderData(models, nt, today)); err != nil {
		return report, err
	}
	newSnapshot := NewSnapshotWithPrices(models, prices, today)
	newSnapshot.UpdatedAt = d.now().UTC().Format(time.RFC3339)
	if catalogOK {
		newSnapshot.CatalogSlugs = append([]string(nil), catalog...)
	} else {
		// Keep a known-good baseline when this refresh could not fetch the catalogue.
		newSnapshot.CatalogSlugs = append([]string(nil), snap.CatalogSlugs...)
	}
	files := []publishFile{{path: opts.OutputPath, data: buf.Bytes()}, {path: snapshotPath, save: func(path string) error { return newSnapshot.Save(path) }}}
	if pricesOK {
		history.Add(d.now(), prices)
		files = append(files, publishFile{path: historyPath, save: func(path string) error { return d.saveHistory(history, path) }, errPrefix: "save price history"})
	}
	if err := publish(files, d.rename, d.remove); err != nil {
		return report, err
	}
	return report, nil
}

type publishFile struct {
	path      string
	data      []byte
	save      func(string) error
	errPrefix string
}

type publishedFile struct {
	publishFile
	temp        string
	backup      string
	hadOriginal bool
	published   bool
}

// PostCommitCleanupError means all targets were published, but old backups
// could not be removed. The published generation must not be rolled back.
type PostCommitCleanupError struct {
	Err error
}

func (e *PostCommitCleanupError) Error() string {
	return fmt.Sprintf("refresh: publication completed, but cleanup did not complete: %v", e.Err)
}

func (e *PostCommitCleanupError) Unwrap() error { return e.Err }

func IsPostCommitCleanupError(err error) bool {
	var cleanupErr *PostCommitCleanupError
	return errors.As(err, &cleanupErr)
}

// publish prepares every file before replacing any target, then rolls back
// earlier replacements if a later rename fails. A process crash between two
// renames can still leave a mixed generation; ordinary write errors recover.
func publish(files []publishFile, rename func(string, string) error, remove func(string) error) error {
	prepared := make([]publishedFile, len(files))
	for i, file := range files {
		if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
			return preparationError(fmt.Errorf("refresh: create directory: %w", err), prepared, remove)
		}
		tmp, err := os.CreateTemp(filepath.Dir(file.path), ".refresh-*.tmp")
		if err != nil {
			return preparationError(fmt.Errorf("refresh: create temporary file: %w", err), prepared, remove)
		}
		tmpName := tmp.Name()
		prepared[i] = publishedFile{publishFile: file, temp: tmpName}
		if err := tmp.Chmod(0o644); err != nil {
			tmp.Close()
			return preparationError(fmt.Errorf("refresh: chmod temporary file: %w", err), prepared, remove)
		}
		if file.data != nil {
			_, err = tmp.Write(file.data)
		}
		if err == nil {
			err = tmp.Close()
		} else {
			tmp.Close()
		}
		if err != nil {
			return preparationError(fmt.Errorf("refresh: prepare %s: %w", file.path, err), prepared, remove)
		}
		if file.save != nil {
			if err := file.save(tmpName); err != nil {
				if file.errPrefix != "" {
					return preparationError(fmt.Errorf("refresh: %s: %w", file.errPrefix, err), prepared, remove)
				}
				return preparationError(fmt.Errorf("refresh: prepare %s: %w", file.path, err), prepared, remove)
			}
		}
	}

	var cleanupErrs []error
	for i := range prepared {
		backup, err := os.CreateTemp(filepath.Dir(prepared[i].path), ".refresh-backup-*")
		if err != nil {
			return rollback(prepared, rename, remove, fmt.Errorf("refresh: create backup: %w", err))
		}
		backupName := backup.Name()
		prepared[i].backup = backupName
		if err := backup.Close(); err != nil {
			return rollback(prepared, rename, remove, fmt.Errorf("refresh: close backup: %w", err))
		}
		if err := remove(backupName); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("refresh: cleanup backup placeholder %s: %w", prepared[i].path, err))
		}
		if err := rename(prepared[i].path, backupName); err == nil {
			prepared[i].hadOriginal = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return rollback(prepared, rename, remove, fmt.Errorf("refresh: backup %s: %w", prepared[i].path, err))
		}
		if err := rename(prepared[i].temp, prepared[i].path); err != nil {
			return rollback(prepared, rename, remove, fmt.Errorf("refresh: publish %s: %w", prepared[i].path, err))
		}
		prepared[i].published = true
	}
	for _, file := range prepared {
		if file.backup != "" {
			if err := remove(file.backup); err != nil && !errors.Is(err, os.ErrNotExist) {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("refresh: cleanup backup %s: %w", file.path, err))
			}
		}
	}
	if len(cleanupErrs) > 0 {
		return &PostCommitCleanupError{Err: errors.Join(cleanupErrs...)}
	}
	return nil
}

func preparationError(cause error, files []publishedFile, remove func(string) error) error {
	return errors.Join(cause, cleanupPrepared(files, remove))
}

func cleanupPrepared(files []publishedFile, remove func(string) error) error {
	var errs []error
	for _, file := range files {
		if file.temp != "" {
			if err := remove(file.temp); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("refresh: cleanup temporary file %s: %w", file.path, err))
			}
		}
		if file.backup != "" {
			if err := remove(file.backup); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("refresh: cleanup backup %s: %w", file.path, err))
			}
		}
	}
	return errors.Join(errs...)
}

func rollback(files []publishedFile, rename func(string, string) error, remove func(string) error, cause error) error {
	errs := []error{cause}
	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		if file.published {
			if err := remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("refresh: rollback remove %s: %w", file.path, err))
			}
		}
		if file.hadOriginal {
			if err := rename(file.backup, file.path); err != nil {
				errs = append(errs, fmt.Errorf("refresh: rollback restore %s: %w", file.path, err))
			}
		}
		if file.temp != "" {
			if err := remove(file.temp); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("refresh: rollback cleanup temporary file %s: %w", file.path, err))
			}
		}
		if file.backup != "" {
			if err := remove(file.backup); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("refresh: rollback cleanup backup %s: %w", file.path, err))
			}
		}
	}
	return errors.Join(errs...)
}

// applyFallback fills the gaps a failed source left, from the previous run's
// snapshot, and reports which slugs were filled so markStale can label them.
func applyFallback(entries []modelmap.Entry, prices map[string]sources.PriceInfo, pricesOK bool, scores []sources.ScoreRow, sourceOK map[string]bool, nt *notes.Notes, snap *Snapshot) (map[string]sources.PriceInfo, []sources.ScoreRow, map[string]bool, map[string]bool) {
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
			prices[e.Slug] = sources.PriceInfo{Slug: e.Slug, InPerM: se.InPerM, OutPerM: se.OutPerM, Context: se.Context, Free: se.InPerM == 0 && se.OutPerM == 0, Found: true, Created: se.Created, Description: se.Description, CanonicalSlug: se.CanonicalSlug, HuggingFaceID: se.HuggingFaceID, Provider: se.Provider, ReleaseVariant: se.ReleaseVariant, ModelVariant: se.ModelVariant, Reasoning: se.Reasoning, Configuration: se.Configuration, HasOverride: se.HasOverride, OverrideMinTokens: se.OverrideMinTokens, OverrideInPerM: se.OverrideInPerM, OverrideOutPerM: se.OverrideOutPerM}
			stalePrices[e.Slug] = true
		}
	}

	scored := make(map[string]bool, len(scores))
	for _, r := range scores {
		scored[r.Slug] = true
	}
	for _, e := range entries {
		if scored[e.Slug] {
			continue
		}
		// Only consider a slug for score-fallback if at least one of its
		// declared SWE-bench-family sources actually failed this run. If every
		// such source succeeded but simply had no row for this slug, that is a
		// genuine absence (model dropped off the leaderboard, or the
		// model-map name is wrong) — not a failure, and injecting a stale
		// snapshot value here would hide both. A model that declares no
		// SWE-bench-family source at all — including one that declares only
		// arena= — has nothing that could have failed for this column: its
		// number, if any, comes from notes.yaml and is never stale.
		anySourceFailed := false
		for sourceID := range e.Names {
			if model.SourceFamily[sourceID] != model.ScoreSourceSWEBench {
				continue
			}
			if !sourceOK[sourceID] {
				anySourceFailed = true
				break
			}
		}
		if !anySourceFailed {
			continue
		}
		// A manual notes.yaml override outranks a stale fallback: Merge
		// already falls back to nt.ScoreOverride when no live row is
		// present, so simply not injecting a stale row here lets that
		// existing path take over with the correct provenance label.
		if _, has := nt.ScoreOverride(e.Slug); has {
			continue
		}
		se, ok := snap.Models[e.Slug]
		if !ok || se.Score == nil {
			continue
		}
		identity := se.Score.IdentityStatus
		if snapshotIdentityUnavailable(se) {
			identity = model.IdentityLegacyUnknown
		}
		scores = append(scores, sources.ScoreRow{
			Slug:               e.Slug,
			SourceFamily:       se.Score.SourceFamily,
			ConfiguredIdentity: se.Score.ConfiguredIdentity,
			IdentityAmbiguous:  se.Score.IdentityAmbiguous,
			Metric:             se.Score.Metric,
			Value:              se.Score.Value,
			Unit:               se.Score.Unit,
			VariantMeasured:    se.Score.VariantMeasured,
			SourceURL:          se.Score.SourceURL,
			Checked:            se.Score.Checked,
			IdentityStatus:     identity,
			CanonicalID:        se.Score.CanonicalID,
			ReleaseVariant:     se.Score.ReleaseVariant,
			ModelVariant:       se.Score.ModelVariant,
			Reasoning:          se.Score.Reasoning,
			Configuration:      se.Score.Configuration,
			Provider:           se.Score.Provider,
			Uncertainty:        se.Score.Uncertainty,
			SampleSize:         se.Score.SampleSize,
			Harness:            se.Score.Harness,
			Scaffold:           se.Score.Scaffold,
		})
		staleScores[e.Slug] = true
	}
	return prices, scores, stalePrices, staleScores
}

// applyArenaFallback does for the Arena column what applyFallback does for
// the SWE-bench one, and stays a separate function for the same reason the
// two columns are separate: one source's outage must never put a number into
// the other's view. There is no notes.yaml equivalent to consult here —
// manual overrides describe SWE-bench Verified only.
func applyArenaFallback(entries []modelmap.Entry, arena []sources.ScoreRow, sourceOK map[string]bool, snap *Snapshot) ([]sources.ScoreRow, map[string]bool) {
	staleArena := map[string]bool{}
	scored := make(map[string]bool, len(arena))
	for _, r := range arena {
		scored[r.Slug] = true
	}
	for _, e := range entries {
		if scored[e.Slug] {
			continue
		}
		failed := false
		for sourceID := range e.Names {
			if model.SourceFamily[sourceID] == model.ScoreSourceArena && !sourceOK[sourceID] {
				failed = true
				break
			}
		}
		if !failed {
			continue
		}
		se, ok := snap.Models[e.Slug]
		if !ok || se.ArenaScore == nil {
			continue
		}
		identity := se.ArenaScore.IdentityStatus
		if snapshotIdentityUnavailable(se) {
			identity = model.IdentityLegacyUnknown
		}
		arena = append(arena, sources.ScoreRow{
			Slug:               e.Slug,
			SourceFamily:       se.ArenaScore.SourceFamily,
			ConfiguredIdentity: se.ArenaScore.ConfiguredIdentity,
			IdentityAmbiguous:  se.ArenaScore.IdentityAmbiguous,
			Metric:             se.ArenaScore.Metric,
			Value:              se.ArenaScore.Value,
			Unit:               se.ArenaScore.Unit,
			VariantMeasured:    se.ArenaScore.VariantMeasured,
			SourceURL:          se.ArenaScore.SourceURL,
			Checked:            se.ArenaScore.Checked,
			IdentityStatus:     identity,
			CanonicalID:        se.ArenaScore.CanonicalID,
			ReleaseVariant:     se.ArenaScore.ReleaseVariant,
			ModelVariant:       se.ArenaScore.ModelVariant,
			Reasoning:          se.ArenaScore.Reasoning,
			Configuration:      se.ArenaScore.Configuration,
			Provider:           se.ArenaScore.Provider,
			License:            se.License,
			ModelURL:           se.ModelURL,
			MetadataSourceURL:  se.MetadataSourceURL,
			Uncertainty:        se.ArenaScore.Uncertainty,
			SampleSize:         se.ArenaScore.SampleSize,
			Harness:            se.ArenaScore.Harness,
			Scaffold:           se.ArenaScore.Scaffold,
		})
		staleArena[e.Slug] = true
	}
	return arena, staleArena
}

func snapshotIdentityUnavailable(se SnapshotEntry) bool {
	return se.CanonicalSlug == "" && se.Provider == "" && se.ReleaseVariant == "" && se.ModelVariant == "" && se.Reasoning == "" && se.Configuration == ""
}

// markStale labels the rows whose values came from the snapshot rather than
// from this run, in the document itself. The two score columns are labelled
// independently, because they can go stale independently.
func markStale(models []model.Model, stalePrices, staleScores, staleArena map[string]bool, date string) {
	for i := range models {
		m := &models[i]
		if stalePrices[m.Slug] {
			m.PriceStale = true
			m.Note = strings.TrimSpace(m.Note + " Цену не удалось проверить на " + date + " — значение из прошлого прогона.")
		}
		if staleScores[m.Slug] && m.Score != nil {
			m.Score.Stale = true
			m.Score.Provenance = strings.TrimSpace(m.Score.Provenance + " [snapshot fallback]")
			m.ScoreLabel += " (не удалось проверить на " + date + ")"
		}
		if staleArena[m.Slug] && m.ArenaScore != nil {
			m.ArenaScore.Stale = true
			m.ArenaScore.Provenance = strings.TrimSpace(m.ArenaScore.Provenance + " [snapshot fallback]")
			m.ArenaLabel += " (не удалось проверить на " + date + ")"
		}
	}
}
