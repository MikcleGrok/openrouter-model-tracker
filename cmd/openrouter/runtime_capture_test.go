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

	const (
		// stepTimeout — сколько ждём, пока экран придёт в ожидаемое шагом
		// состояние.
		stepTimeout = 2 * time.Second
		// quietPeriod — сколько рендерер должен молчать, чтобы шаг
		// считался завершённым. Это НЕ способ дождаться кадра: кадр
		// дожидается детерминированно, по want() ниже. Это дренаж
		// «хвоста» шага, потому что одно действие законно порождает
		// больше одной записи: tea.ClearScreen() шлёт ESC[2J/ESC[H
		// отдельными write() и форсирует repaint, а на старте рендерер
		// может успеть флашнуть ещё до того, как event loop разберёт
		// WindowSizeMsg (см. startRuntimeProgram). Недобранная запись
		// уехала бы в СЛЕДУЮЩИЙ шаг, а через Resize это означало бы
		// 120-колоночные байты на 90-колоночном экране. Шесть тиков
		// рендерера при 60fps.
		quietPeriod = 100 * time.Millisecond
	)

	// startRuntimeProgram (runtime_program_test.go) драйвит m ровно теми же
	// опциями, что и прод — tea.WithAltScreen(), tea.WithMouseCellMotion(),
	// без tea.WithANSICompressor() — в in-memory writer. В отличие от
	// teatest.NewTestModel (который жёстко зашивает tea.WithANSICompressor()
	// в свой внутренний tea.NewProgram без возможности переопределить), это
	// проходит ровно тот путь рендерера, который проходит прод:
	// tea.WithAltScreen() здесь стартовая опция, а не runtime-сообщение, так
	// что нет отдельного шага «пошлём EnterAltScreen и понадеемся, что его
	// flush уже долетел».
	rp := startRuntimeProgram(t, m, 120, 30)
	sess := newRuntimeSession(120, 30)

	// screen — текущее состояние сессионного экрана: персистентная сетка
	// ячеек, которую эмулятор ведёт между кадрами, как настоящий терминал.
	var screen []string

	// step проигрывает один шаг сценария до проверенного состояния экрана.
	//
	// В эмулятор скармливается КАЖДАЯ запись рендерера — управляющие
	// последовательности наравне с кадрами, в том порядке, в каком они были
	// записаны, по одному вызову Frame() на каждый вызов Write(), ровно как
	// их получил бы настоящий терминал. Отдельный Frame() на запись важен:
	// проверка «повторная запись в ячейку внутри одного кадра» должна
	// оставаться на границах flush(), а не размазываться по нескольким.
	//
	// Условие остановки — want(экран), а не «пришёл первый кадр с видимым
	// текстом»: рендерер не обязан отдавать ровно один кадр на действие, и
	// счёт кадров — это ровно то, на чём тест начинает молча проверять не
	// тот кадр. После совпадения шаг дочитывает хвост до тишины и проверяет
	// want ещё раз — чтобы совпадение на промежуточном кадре, который
	// следующая запись тут же отменяет, не проходило как успех.
	step := func(label string, want func(rows []string) bool) {
		t.Helper()
		deadline := time.Now().Add(stepTimeout)
		written := 0
		feed := func(raw string) {
			written++
			out, err := sess.Frame(raw)
			if err != nil {
				t.Fatalf("%s: запись рендерера #%d не разобралась эмулятором: %v\nбайты: %q", label, written, err, raw)
			}
			screen = out
		}
		for {
			raw, ok := rp.NextWrite(time.Until(deadline))
			if !ok {
				t.Fatalf("%s: за %s экран так и не пришёл в ожидаемое состояние (%d записей рендерера)\nэкран:\n%s", label, stepTimeout, written, strings.Join(screen, "\n"))
			}
			feed(raw)
			if want(screen) {
				break
			}
		}
		for {
			raw, ok := rp.NextWrite(quietPeriod)
			if !ok {
				break
			}
			feed(raw)
		}
		if !want(screen) {
			t.Fatalf("%s: экран разъехался при дорисовке хвоста шага (%d записей рендерера)\nэкран:\n%s", label, written, strings.Join(screen, "\n"))
		}
		t.Logf("%s OK: %d записей рендерера, %d строк, контент до колонки %d", label, written, len(screen), lastContentColumn(screen))
	}

	// tableAt — главный экран со списком моделей, свёрстанный именно под
	// ширину width. Проверка ширины здесь не косметика: дефолт модели до
	// первого WindowSizeMsg — 100 колонок (newTUIModel в tui.go), так что
	// таблица, обрывающаяся на 100-й колонке в 120-колоночной сессии, —
	// это не «узкий кадр», а чужой кадр.
	tableAt := func(width int) func([]string) bool {
		return func(rows []string) bool {
			return containsPhysicalRow(rows, "OpenRouter models") &&
				!containsPhysicalRow(rows, "Esc close") &&
				contentFillsWidth(rows, width)
		}
	}
	// detailWithFooter — оверлей карточки модели. footer содержит счётчик
	// видимых строк, который пересчитывается от размера окна, так что он же
	// служит доказательством, что кадр перевёрстан под текущий размер.
	detailWithFooter := func(footer string) func([]string) bool {
		return func(rows []string) bool {
			return containsPhysicalRow(rows, "Second model (second/model)") &&
				containsPhysicalRow(rows, footer) &&
				!containsPhysicalRow(rows, "OpenRouter models")
		}
	}

	step("начальный рендер", func(rows []string) bool {
		// data:unknown — хвост мета-строки длиной ~117 колонок: на 100
		// колонках она обрезается и этого хвоста на экране нет.
		return tableAt(120)(rows) && containsPhysicalRow(rows, "data:unknown")
	})

	rp.Send(tea.KeyMsg{Type: tea.KeyEnter})
	step("открыт detail", detailWithFooter("Detail 1-28/46"))

	sess.Resize(90, 24)
	rp.Send(tea.WindowSizeMsg{Width: 90, Height: 24})
	step("resize при открытом detail", detailWithFooter("Detail 1-22/48"))

	rp.Send(tea.KeyMsg{Type: tea.KeyEscape})
	step("overlay закрыт", tableAt(90))

	sess.Resize(120, 30)
	rp.Send(tea.WindowSizeMsg{Width: 120, Height: 30})
	step("resize обратно к исходному размеру", func(rows []string) bool {
		return tableAt(120)(rows) && containsPhysicalRow(rows, "data:unknown")
	})

	rp.Send(tea.KeyMsg{Type: tea.KeyEnter})
	step("повторно открыт detail", detailWithFooter("Detail 1-28/46"))

	rp.Send(tea.KeyMsg{Type: tea.KeyEscape})
	step("overlay закрыт повторно", tableAt(120))

	// Поиск ("/") — пример реального пользовательского действия в этой модели,
	// которое способно укоротить содержимое строки экрана БЕЗ полного
	// tea.ClearScreen() (help-overlay search typing и in-place list changes
	// like sort toggles — другие примеры). Открытие/закрытие любого
	// оверлея (как detail выше) каждый раз меняет m.screenIdentity() и
	// потому проходит через Controller.Transition ->
	// InvalidateOnTransition (internal/tui/screen/controller.go:70), которая
	// форсирует tea.ClearScreen() на КАЖДОМ таком переключении — ни один из
	// шагов выше ни разу не добирается до детектора с живым нетронутым
	// хвостом старого кадра. Набор и бэкспейс в строке поиска, наоборот, не
	// трогают m.overlay: screenIdentity() остаётся "list" всё время, пока
	// активен m.inputMode == "search" (см. screenIdentity() и case "/" в
	// key(), cmd/openrouter/tui.go) — Transition() поэтому не срабатывает,
	// и укорачивание строки поиска на бэкспейсе доходит до экрана
	// исключительно через собственную построчную diff/erase-to-end-of-line
	// логику патченного рендерера (standard_renderer.go flush(): дописывает
	// ansi.EraseLineRight к каждой укоротившейся строке). Это ровно тот
	// путь, который detectStaleContent (runtime_session_test.go) существует
	// проверять: если бы рендерер когда-нибудь перестал дописывать erase к
	// укоротившейся строке поиска, sess.Frame ниже упал бы с ошибкой раньше,
	// чем скрипт дошёл бы до собственной проверки содержимого.
	const searchFull, searchShort = "second-model-lookup", "second"
	rp.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	step("поиск открыт", func(rows []string) bool {
		return tableAt(120)(rows) && containsPhysicalRow(rows, "/ _")
	})

	rp.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(searchFull)})
	step("текст поиска введён", func(rows []string) bool {
		return containsPhysicalRow(rows, "/ "+searchFull+"_")
	})

	// searchFull[len(searchShort):] — вычитаемый хвост "-model-lookup";
	// range по нему даёт ровно один бэкспейс на руну хвоста (в этой строке
	// ASCII, так что руна == байт), без ручного подсчёта количества.
	for range searchFull[len(searchShort):] {
		rp.Send(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	step("текст поиска укорочен без ClearScreen", func(rows []string) bool {
		// searchFull[len(searchShort)+1:], а не [len(searchShort):]: символ
		// сразу после searchShort (тот самый "-") сидит ровно в колонке
		// нового курсора и потому переписывается новым кадром ("_")
		// независимо от того, отработал erase-to-EOL или нет — контроль на
		// нём самом никогда бы не сработал (72e060f). Настоящий утёкший
		// хвост при сломанном erase-to-EOL начинается со следующего символа.
		return containsPhysicalRow(rows, "/ "+searchShort+"_") &&
			!containsPhysicalRow(rows, searchFull[len(searchShort)+1:])
	})

	rp.Send(tea.KeyMsg{Type: tea.KeyEscape})
	// tableAt(120) само по себе не различает "поиск открыт" и "поиск
	// закрыт" — оно уже было true во время "поиск открыт" выше (см. ту
	// проверку: tableAt(120)(rows) && containsPhysicalRow(rows, "/ _")),
	// потому что строка поиска рисуется НАД той же самой таблицей, а не
	// вместо неё. Без отдельного условия на отсутствие строки поиска этот
	// шаг прошёл бы даже если бы Escape не закрывал поиск вовсе — контроль
	// на "/ " (с пробелом сразу после слэша — именно так строится строка
	// поиска, "/ " + текст + "_") доказывает, что она действительно ушла с
	// экрана.
	step("поиск закрыт", func(rows []string) bool {
		return tableAt(120)(rows) && !containsPhysicalRow(rows, "/ ")
	})

	// Ctrl+C доходит до tuiModel.Update при закрытом оверлее (его закрыл
	// Escape выше) и возвращает tea.Quit напрямую (case "ctrl+c" в key(),
	// cmd/openrouter/tui.go) — модель не меняется, так что View() не
	// перерисовывается заново и нового видимого кадра на этот шаг не
	// рождается; ждать здесь нечего. Что проверял на этом месте исходный
	// тест (teatest WaitFinished + FinalOutput) — это «байты пути
	// завершения разбираются без ошибки»: Quit подтверждает, что Run()
	// действительно вернулся (shutdown() до этого отрабатывает полностью
	// синхронно), а DrainRemaining отдаёт всё, что успело записаться
	// (EraseEntireLine, show cursor, выход из alt-screen, отключение мыши и
	// bracketed paste).
	//
	// Это НЕ «ни одна из этих последовательностей не пишет в ячейки»:
	// patched-рендереровский stop()
	// (internal/thirdparty/bubbletea-patched/standard_renderer.go:115) шлёт
	// ansi.EraseEntireLine — ESC[2K с mode=2, который по eraseLine(mode=2)
	// выше стирает СТРОКУ ЦЕЛИКОМ, а не ничего. Стирается ровно одна
	// строка — та, где курсор оставил последний кадр: flush() в
	// alt-screen режиме паркует курсор в CursorPosition(0, len(newLines))
	// после каждой отрисовки (standard_renderer.go), а последний кадр
	// перед Ctrl+C — полная перерисовка на все 30 строк, так что это
	// последняя (нижняя) строка экрана. В фикстуре этого теста та строка
	// уже пустая (нижняя строка списка — вертикальный отступ), так что сам
	// erase здесь визуально no-op — но проверка не должна опираться на это
	// совпадение: она должна ловить регресс, при котором erase вообще
	// перестанет чистить строку, или (что опаснее) начнёт задевать не ту
	// строку и стирать реальный контент. Отсюда — сравнение экрана
	// построчно с состоянием до Ctrl+C: каждая строка, КРОМЕ последней,
	// обязана остаться побайтово той же, а последняя обязана оказаться
	// пустой.
	rp.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	rp.Quit(t, stepTimeout)
	beforeShutdown := append([]string(nil), screen...)
	shutdown := rp.DrainRemaining()
	if len(shutdown) == 0 {
		// shutdown() (standard_renderer.go stop() + restoreTerminalState())
		// отрабатывает полностью синхронно до возврата Run(), которое уже
		// подтвердил rp.Quit() выше — так что к этому моменту в канале
		// frameWriter гарантированно лежат как минимум EraseEntireLine и
		// "\r" из stop(). Пустой DrainRemaining() здесь значит не «путь
		// завершения ничего не пишет», а что проверка ниже вообще не
		// увидела ни одного байта пути завершения и потому не проверяет
		// ничего — это и есть vacuous pass, который эта проверка исключает.
		t.Fatalf("завершение программы: DrainRemaining не вернул ни одной записи рендерера — путь завершения не был проверен вообще")
	}
	for i, raw := range shutdown {
		out, err := sess.Frame(raw)
		if err != nil {
			t.Fatalf("завершение программы: запись рендерера #%d не разобралась эмулятором: %v\nбайты: %q", i+1, err, raw)
		}
		screen = out
	}
	if len(screen) != len(beforeShutdown) {
		t.Fatalf("завершение программы: число строк экрана изменилось на пути завершения: было %d, стало %d", len(beforeShutdown), len(screen))
	}
	lastRow := len(screen) - 1
	for y := range screen {
		if y == lastRow {
			continue
		}
		if screen[y] != beforeShutdown[y] {
			t.Fatalf("завершение программы: строка %d изменилась на пути завершения, хотя shutdown трогает только последнюю строку экрана\nбыло:  %q\nстало: %q", y, beforeShutdown[y], screen[y])
		}
	}
	if trimmed := strings.TrimRight(screen[lastRow], " "); trimmed != "" {
		t.Fatalf("завершение программы: ожидали, что EraseEntireLine очистит последнюю строку экрана (%d), но там осталось: %q", lastRow, trimmed)
	}
	t.Logf("завершение программы OK: %d записей рендерера", len(shutdown))
}

// lastContentColumn возвращает 1-based номер самой правой непустой колонки
// по всем строкам сетки — то есть фактическую ширину отрисованного кадра на
// экране. Строки сетки — это ячейки, по руне на ячейку (двухширинная руна
// занимает — и хранится в — обеих своих ячейках, см. runtimeTerminal.write),
// так что счёт рун здесь и есть счёт колонок.
func lastContentColumn(rows []string) int {
	last := 0
	for _, row := range rows {
		if n := len([]rune(strings.TrimRight(row, " "))); n > last {
			last = n
		}
	}
	return last
}

// contentFillsWidth проверяет, что кадр свёрстан именно под ширину width:
// контент не вылезает за неё и при этом доходит почти до неё. Нижняя
// граница нарочно с запасом — она отделяет «свёрстано под этот размер» от
// «свёрстано под другой», а не фиксирует конкретную вёрстку колонок.
func contentFillsWidth(rows []string, width int) bool {
	last := lastContentColumn(rows)
	return last <= width && last > width-15
}
