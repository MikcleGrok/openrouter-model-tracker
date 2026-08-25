package main

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sboborikin/openrouter-model-tracker/internal/model"
	"github.com/sboborikin/openrouter-model-tracker/internal/refresh"
)

// TestTUIRuntimeCaptureAcrossRealSession прогоняет настоящую tea.Program
// (через пропатченный форк Bubble Tea — тот же самый, что в проде,
// подключён прозрачно через replace в go.mod) и проверяет РЕАЛЬНЫЕ байты,
// которые пишет рендерер, через сессионный эмулятор терминала — то, что
// ни один тест на основе m.View() в принципе не может проверить, потому
// что View() — самодостаточная логическая строка без курсорных
// последовательностей и без состояния между кадрами.
func TestTUIRuntimeCaptureAcrossRealSession(t *testing.T) {
	rows := []model.Model{
		{Slug: "first/model", DisplayName: "First model", Provider: "First", License: "Apache-2.0", Tier: "sonnet", ClaudeRef: "≈ Sonnet", TaskFit: []string{"implement", "audit"}, Context: 128000, InPerM: 0.5, OutPerM: 2, OpenWeights: "yes", CanonicalSlug: "first/model", MetadataSourceURL: "https://meta.example/first", Description: strings.Repeat("first long description ", 12), Note: "first note"},
		{Slug: "second/model", DisplayName: "Second model", Provider: "Second", License: "MIT", Tier: "haiku", ClaudeRef: "≈ Haiku", TaskFit: []string{"review"}, Context: 32768, InPerM: 0.1, OutPerM: 0.4, OpenWeights: "no", CanonicalSlug: "second/model", MetadataSourceURL: "https://meta.example/second", Description: "short description", Note: "short note", Score: &model.ScoreInfo{Value: 91.2, Metric: "SWE-bench Verified", Unit: "%", VariantMeasured: "second/model", SourceURL: "https://bench.example/second", Checked: "2026-08-20"}},
		{Slug: "third/model", DisplayName: "Third model", Provider: "Third", License: "MIT", Tier: "opus", ClaudeRef: "≈ Opus", TaskFit: []string{"plan", "review", "audit"}, Context: 200000, InPerM: 3, OutPerM: 15, OpenWeights: "no", CanonicalSlug: "third/model", MetadataSourceURL: "https://meta.example/third", Description: "third description", Note: "third note"},
	}
	m := newTUIModel(context.Background(), "", refresh.Options{}, 0, rows)
	m.sortKey = "name"
	m.rebuild()

	// startRuntimeProgram (runtime_program_test.go) drives m the same way
	// production does — tea.WithAltScreen(), no tea.WithANSICompressor() —
	// against an in-memory writer, and hands back real per-frame boundaries
	// instead of a sleep-then-drain guess. Unlike teatest.NewTestModel (which
	// hardcodes tea.WithANSICompressor() into its internal tea.NewProgram
	// call with no override), this exercises the exact renderer code path
	// production takes: tea.WithAltScreen() is a startup option here, not a
	// runtime message, so there is exactly one initial frame — no separate
	// "send EnterAltScreen and hope its flush already landed" priming step,
	// and no risk of that flush silently folding into the first captured one.
	const frameTimeout = 2 * time.Second
	rp := startRuntimeProgram(m, 120, 30)

	sess := newRuntimeSession(120, 30)
	check := func(label string, width, height int, chunk string) {
		t.Helper()
		if len(chunk) == 0 {
			t.Logf("%s: (пусто — в это окно ничего не флашилось)", label)
			return
		}
		rowsOut, err := sess.Frame(chunk)
		if err != nil {
			t.Errorf("%s FAILED at %dx%d: %v", label, width, height, err)
			return
		}
		t.Logf("%s OK at %dx%d, %d строк", label, width, height, len(rowsOut))
	}

	check("начальный рендер", 120, 30, rp.NextFrame(t, frameTimeout))

	rp.Send(tea.KeyMsg{Type: tea.KeyEnter})
	check("открыт detail", 120, 30, rp.NextFrame(t, frameTimeout))

	rp.Send(tea.WindowSizeMsg{Width: 90, Height: 24})
	sess.Resize(90, 24)
	check("resize при открытом detail", 90, 24, rp.NextFrame(t, frameTimeout))

	rp.Send(tea.KeyMsg{Type: tea.KeyEscape})
	check("overlay закрыт", 90, 24, rp.NextFrame(t, frameTimeout))

	rp.Send(tea.WindowSizeMsg{Width: 120, Height: 30})
	sess.Resize(120, 30)
	check("resize обратно к исходному размеру", 120, 30, rp.NextFrame(t, frameTimeout))

	rp.Send(tea.KeyMsg{Type: tea.KeyEnter})
	check("повторно открыт detail", 120, 30, rp.NextFrame(t, frameTimeout))

	rp.Send(tea.KeyMsg{Type: tea.KeyEscape})
	check("overlay закрыт повторно", 120, 30, rp.NextFrame(t, frameTimeout))

	// Ctrl+C reaches tuiModel.Update with no overlay open (it was already
	// closed by the Escape above), so it returns straight to tea.Quit
	// without changing the model — no new visible content is ever rendered
	// for this step, so unlike every step above there is no frame to wait
	// for here via NextFrame. What the original test asserted at this point
	// (via teatest's WaitFinished + FinalOutput) was simply "whatever bytes
	// the shutdown path emits parse without error" — Quit confirms Run()
	// actually returns (shutdown() runs fully synchronously beforehand), and
	// DrainRemaining collects whatever shutdown-only control sequences
	// (EraseEntireLine, cursor show, exit-alt-screen, ...) landed in the
	// meantime, which is the same tolerant "may be content-free" check as
	// before.
	rp.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	rp.Quit(t, frameTimeout)
	check("завершение программы", 120, 30, rp.DrainRemaining())
}
