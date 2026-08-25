package main

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

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

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 30))
	const tick = 60 * time.Millisecond

	// Production always runs with tea.WithAltScreen() (cmd/openrouter/tui.go),
	// which routes the patched renderer into a different code path than the
	// default teatest session: alt-screen mode re-homes the cursor absolutely
	// every frame (ESC[H + absolute positioning) and never emits relative
	// CursorUp moves, while non-alt-screen mode does emit CursorUp(n) between
	// frames. teatest.NewTestModel does not enable alt-screen on its own, so
	// without this the test would exercise a renderer code path production
	// never actually takes.
	tm.Send(tea.EnterAltScreen())
	time.Sleep(tick) // let the alt-screen entry (and its own repaint) flush before scripting begins

	drain := func() []byte {
		b, err := io.ReadAll(tm.Output())
		if err != nil {
			t.Fatalf("draining Output(): %v", err)
		}
		return b
	}

	sess := newRuntimeSession(120, 30)
	check := func(label string, width, height int, chunk []byte) {
		t.Helper()
		if len(chunk) == 0 {
			t.Logf("%s: (пусто — в это окно ничего не флашилось)", label)
			return
		}
		rowsOut, err := sess.Frame(string(chunk))
		if err != nil {
			t.Errorf("%s FAILED at %dx%d: %v", label, width, height, err)
			return
		}
		t.Logf("%s OK at %dx%d, %d строк", label, width, height, len(rowsOut))
	}

	check("начальный рендер", 120, 30, drain())

	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	time.Sleep(tick)
	check("открыт detail", 120, 30, drain())

	tm.Send(tea.WindowSizeMsg{Width: 90, Height: 24})
	time.Sleep(tick)
	sess.Resize(90, 24)
	check("resize при открытом detail", 90, 24, drain())

	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(tick)
	check("overlay закрыт", 90, 24, drain())

	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 30})
	time.Sleep(tick)
	sess.Resize(120, 30)
	check("resize обратно к исходному размеру", 120, 30, drain())

	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	time.Sleep(tick)
	check("повторно открыт detail", 120, 30, drain())

	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(tick)
	check("overlay закрыт повторно", 120, 30, drain())

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
	final, err := io.ReadAll(tm.FinalOutput(t))
	if err != nil {
		t.Fatalf("reading FinalOutput: %v", err)
	}
	check("завершение программы", 120, 30, final)
}
