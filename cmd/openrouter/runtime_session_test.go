package main

import (
	"strings"
	"testing"
)

// runtimeSession моделирует одну непрерывную терминальную сессию из
// нескольких кадров: курсор и содержимое ячеек персистентны между вызовами
// Frame, а проверка повторной записи в ячейку выполняется только в рамках
// одного кадра — легитимная перерисовка ячейки в следующем кадре (реальное
// поведение терминала при частичном redraw) не считается нарушением;
// повторная запись в ту же ячейку ВНУТРИ одного кадра — считается.
type runtimeSession struct {
	term *runtimeTerminal
}

func newRuntimeSession(width, height int) *runtimeSession {
	t := &runtimeTerminal{cells: make([][]rune, height), writes: make([][]bool, height), width: width, height: height}
	for y := range t.cells {
		t.cells[y] = []rune(strings.Repeat(" ", width))
		t.writes[y] = make([]bool, width)
	}
	return &runtimeSession{term: t}
}

// Frame проверяет один кадр реального вывода рендерера относительно
// персистентного состояния сессии.
func (s *runtimeSession) Frame(stream string) ([]string, error) {
	for y := range s.term.writes {
		for x := range s.term.writes[y] {
			s.term.writes[y][x] = false
		}
	}
	if err := runtimeFeed(s.term, stream); err != nil {
		return nil, err
	}
	rows := make([]string, s.term.height)
	for i := range s.term.cells {
		rows[i] = string(s.term.cells[i])
	}
	return rows, nil
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
