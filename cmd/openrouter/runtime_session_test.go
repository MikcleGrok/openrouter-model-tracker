package main

import (
	"fmt"
	"strings"
	"testing"
)

// runtimeSession моделирует одну непрерывную терминальную сессию из
// нескольких кадров: курсор и содержимое ячеек персистентны между вызовами
// Frame, а проверка повторной записи в ячейку выполняется только в рамках
// одного кадра — легитимная перерисовка ячейки в следующем кадре (реальное
// поведение терминала при частичном redraw) не считается нарушением;
// повторная запись в ту же ячейку ВНУТРИ одного кадра — считается.
//
// poisoned фиксирует первую ошибку Frame: после неё курсор/состояние
// runtimeTerminal остаются там, где парсинг оборвался (Frame не откатывает
// изменения), так что дальнейшие вызовы Frame на уже повреждённом состоянии
// могут привести не к чистой ошибке, а к панике внутри csi()-обработчиков
// (например eraseLine/eraseDisplay индексируют cells по уже невалидному y).
// Poisoning превращает это в один понятный отказ вместо каскада и паники.
type runtimeSession struct {
	term     *runtimeTerminal
	poisoned error
}

func newRuntimeSession(width, height int) *runtimeSession {
	t := &runtimeTerminal{cells: make([][]rune, height), writes: make([][]bool, height), touched: make([][]bool, height), width: width, height: height}
	for y := range t.cells {
		t.cells[y] = []rune(strings.Repeat(" ", width))
		t.writes[y] = make([]bool, width)
		t.touched[y] = make([]bool, width)
	}
	return &runtimeSession{term: t}
}

// Resize переконфигурирует сессию под новый размер терминала. Реальный
// resize доставляется в программу как tea.WindowSizeMsg, а патченный
// renderer обрабатывает его вызовом repaint() (см.
// internal/thirdparty/bubbletea-patched/standard_renderer.go), который
// очищает lastRender/lastRenderedLines — состояние построчного diff'а,
// которым renderer решает, что можно не перерисовывать. Это не приводит к
// явному ESC[2J/ESC[H в потоке байт (такой последовательности в реальных
// resize-кадрах нет), но заставляет renderer перерисовать каждую строку
// заново при следующем flush — начать со свежей сетки и курсором в (0,0)
// корректно моделирует именно этот эффект.
//
// Resize НЕ сбрасывает poisoned: если сессия уже упала, она должна
// оставаться упавшей и после resize — это осознанно консервативный выбор.
func (s *runtimeSession) Resize(width, height int) {
	t := &runtimeTerminal{cells: make([][]rune, height), writes: make([][]bool, height), touched: make([][]bool, height), width: width, height: height}
	for y := range t.cells {
		t.cells[y] = []rune(strings.Repeat(" ", width))
		t.writes[y] = make([]bool, width)
		t.touched[y] = make([]bool, width)
	}
	s.term = t
}

// Frame проверяет один кадр реального вывода рендерера относительно
// персистентного состояния сессии.
func (s *runtimeSession) Frame(stream string) ([]string, error) {
	if s.poisoned != nil {
		return nil, fmt.Errorf("session already failed, refusing further frames: %w", s.poisoned)
	}
	for y := range s.term.writes {
		for x := range s.term.writes[y] {
			s.term.writes[y][x] = false
			s.term.touched[y][x] = false
		}
	}
	if err := runtimeFeed(s.term, stream); err != nil {
		s.poisoned = err
		return nil, err
	}
	if err := detectStaleContent(s.term); err != nil {
		s.poisoned = err
		return nil, err
	}
	rows := make([]string, s.term.height)
	for i := range s.term.cells {
		rows[i] = string(s.term.cells[i])
	}
	return rows, nil
}

// detectStaleContent catches the "blurred/duplicated text" symptom this
// whole test harness exists to eventually police: a line's new content is
// shorter than what was there before, the renderer writes the new (shorter)
// content but never erases the vacated tail of the old, longer content, and
// that tail keeps showing on screen looking like leftover/duplicated
// glyphs.
//
// It runs once per frame, after runtimeFeed has applied every byte of the
// frame to term. For each row, it finds the highest column touched (written
// OR erased — see touched's own doc comment on runtimeTerminal) THIS frame.
// A row nothing touched this frame is not a candidate at all — the
// renderer's own line-diff legitimately skips repainting an unchanged row
// (see standard_renderer.go flush()'s canSkip), and that is not a bug.
// Everything past the highest touched column on a touched row, though,
// holds whatever cells held before this frame started (by definition of
// "highest touched column" — nothing this frame reached further right), so
// checking today's content there directly is correct and sufficient: no
// separate "before this frame" snapshot is needed. If that untouched
// tail is non-blank, it is stale content left over from a previous,
// longer-lived frame.
//
// This bookkeeping is only correct under the harness contract runtime_program_test.go's
// frameWriter establishes: one renderer Write() call feeds exactly one Frame()
// call here, in order, unfiltered — so "this frame" above means exactly one
// flush. Anything that merges or splits writes before they reach Frame() (the
// concrete way to reintroduce this: teatest's WithANSICompressor(), which
// fragments a single Write() into one write per rune) would silently break
// this function's per-row touched-column tracking.
func detectStaleContent(term *runtimeTerminal) error {
	for y := 0; y < term.height; y++ {
		maxTouched := -1
		for x := 0; x < term.width; x++ {
			if term.touched[y][x] {
				maxTouched = x
			}
		}
		if maxTouched < 0 {
			continue
		}
		for x := maxTouched + 1; x < term.width; x++ {
			if term.cells[y][x] != ' ' {
				leftover := strings.TrimRight(string(term.cells[y][x:]), " ")
				return fmt.Errorf("stale content at (%d,%d): row %d touched only through column %d this frame, but column %d still holds leftover %q from a previous frame", x, y, y, maxTouched, x, leftover)
			}
		}
	}
	return nil
}

func TestRuntimeSessionAllowsLegitimateCrossFrameRepaint(t *testing.T) {
	s := newRuntimeSession(5, 1)
	if _, err := s.Frame("hi"); err != nil {
		t.Fatalf("frame 1: %v", err)
	}
	// Кадр 2: курсор возвращается на начало строки (CSI 2D) и те же самые
	// ячейки (0,0) и (1,0), уже помеченные как записанные в кадре 1,
	// перезаписываются снова — легитимная полная перерисовка после
	// чего-то вроде resize, не баг. Если бы сброс write-bitmap между
	// кадрами не работал, эта запись упала бы с "duplicate write".
	rows, err := s.Frame("\x1b[2Dhi")
	if err != nil {
		t.Fatalf("frame 2 (legitimate repaint) should not fail, got: %v", err)
	}
	if rows[0] != "hi   " {
		t.Fatalf("expected content preserved after repaint, got %q", rows[0])
	}
}

func TestRuntimeSessionCatchesDuplicateWriteWithinOneFrame(t *testing.T) {
	s := newRuntimeSession(5, 2)
	// В ОДНОМ кадре курсор пишет "ab", возвращается на исходную позицию
	// и пишет "cd" поверх — это и есть баг, который ловит эмулятор.
	_, err := s.Frame("ab\x1b[2Dcd")
	if err == nil {
		t.Fatal("expected duplicate-write error within a single frame, got nil")
	}
}

func TestRuntimeSessionAllowsShrinkWithEraseToEndOfLine(t *testing.T) {
	s := newRuntimeSession(6, 1)
	if _, err := s.Frame("hello!"); err != nil {
		t.Fatalf("frame 1: %v", err)
	}
	// Кадр 2: курсор возвращается на начало строки, пишет более короткий
	// текст ("hi" вместо "hello!") и явно стирает освободившийся хвост
	// через CSI K — ровно то, что делает патченный рендерер на каждой
	// укоротившейся строке (standard_renderer.go flush(): дописывает
	// ansi.EraseLineRight, когда новая строка короче ширины терминала).
	// Легитимная перерисовка — detectStaleContent не должен её зафлагать.
	rows, err := s.Frame("\x1b[6Dhi\x1b[K")
	if err != nil {
		t.Fatalf("frame 2 (shrink with erase-to-end) should not fail, got: %v", err)
	}
	if rows[0] != "hi    " {
		t.Fatalf("expected shrunk content with the vacated tail erased, got %q", rows[0])
	}
}

func TestRuntimeSessionCatchesStaleContentAfterShrinkWithoutErase(t *testing.T) {
	s := newRuntimeSession(6, 1)
	if _, err := s.Frame("hello!"); err != nil {
		t.Fatalf("frame 1: %v", err)
	}
	// Кадр 2: курсор возвращается на начало строки и пишет "hi" — короче
	// прежнего "hello!" — но НЕ стирает хвост. "llo!" остаётся видимым
	// после "hi": это и есть баг detectStaleContent обязан ловить.
	_, err := s.Frame("\x1b[6Dhi")
	if err == nil {
		t.Fatal("expected a stale-content error for the un-erased leftover tail, got nil")
	}
	if !strings.Contains(err.Error(), "stale content") || !strings.Contains(err.Error(), "(2,0)") {
		t.Fatalf("expected error identifying stale content at row/column (2,0), got: %v", err)
	}
}
