package pricehistory

import (
	"strings"
	"testing"
	"time"
)

func ptr(v float64) *float64 { return &v }

func chartDays(t *testing.T, n int) []time.Time {
	t.Helper()
	days := make([]time.Time, n)
	for i := range days {
		days[i] = time.Date(2026, 9, 1+i, 0, 0, 0, 0, time.UTC)
	}
	return days
}

// TestRenderChartLabelsAxesWithRealValues is the direct regression test for
// the bug report's "unreadable ASCII... no axis labels, no legend, no
// visible price values on the chart itself" complaint: the rendered chart
// must carry the actual min and max dollar values on its Y-axis and the
// first/last dates on its X-axis, not just a wall of bar characters.
func TestRenderChartLabelsAxesWithRealValues(t *testing.T) {
	days := chartDays(t, 4)
	values := []*float64{ptr(3), ptr(2.5), ptr(2.1), ptr(1.5)}
	chart := RenderChart("Input $/M", days, values, 40)
	if !strings.HasPrefix(chart, "Input $/M\n") {
		t.Fatalf("chart is missing its series label:\n%s", chart)
	}
	if !strings.Contains(chart, "$3") {
		t.Errorf("chart Y-axis is missing the max value label $3:\n%s", chart)
	}
	if !strings.Contains(chart, "$1.5") {
		t.Errorf("chart Y-axis is missing the min value label $1.5:\n%s", chart)
	}
	if !strings.Contains(chart, "2026-09-01") {
		t.Errorf("chart X-axis is missing the first date:\n%s", chart)
	}
	if !strings.Contains(chart, "2026-09-04") {
		t.Errorf("chart X-axis is missing the last date:\n%s", chart)
	}
	lines := strings.Split(strings.TrimRight(chart, "\n"), "\n")
	// label line + chartRows plotted rows + 1 baseline row + 1 x-axis row.
	if want := 1 + chartRows + 1 + 1; len(lines) != want {
		t.Fatalf("chart has %d lines, want %d:\n%s", len(lines), want, chart)
	}
	for _, marker := range []string{"┤", "└", "─"} {
		if !strings.Contains(chart, marker) {
			t.Errorf("chart is missing axis marker %q:\n%s", marker, chart)
		}
	}
}

func TestRenderChartConstantSeriesReportsTheFlatValueInstead(t *testing.T) {
	days := chartDays(t, 3)
	values := []*float64{ptr(2), ptr(2), ptr(2)}
	chart := RenderChart("Output $/M", days, values, 40)
	if !strings.Contains(chart, "constant at $2 across 3 observed day(s)") {
		t.Errorf("chart = %q, want the flat-series message", chart)
	}
}

func TestRenderChartAllGapsAndEmptyInput(t *testing.T) {
	if got := RenderChart("Input $/M", chartDays(t, 2), []*float64{nil, nil}, 40); got != "Input $/M: n/a\n" {
		t.Errorf("all-gap chart = %q", got)
	}
	if got := RenderChart("Input $/M", nil, nil, 40); got != "Input $/M: n/a\n" {
		t.Errorf("empty chart = %q", got)
	}
}

func TestRenderChartGapRendersAsBlankColumn(t *testing.T) {
	days := chartDays(t, 3)
	values := []*float64{ptr(1), nil, ptr(3)}
	chart := RenderChart("Input $/M", days, values, 3)
	lines := strings.Split(chart, "\n")
	// The middle column (the gap) must be a space in every plotted row.
	for i := 1; i <= chartRows; i++ {
		axisEnd := strings.Index(lines[i], "┤") + len("┤")
		plotted := []rune(lines[i][axisEnd:])
		if len(plotted) < 2 || plotted[1] != ' ' {
			t.Fatalf("row %q: gap column not blank (plotted=%q)", lines[i], string(plotted))
		}
	}
}

func TestRenderChartDownsamplesLongerSeriesToWidth(t *testing.T) {
	n := 200
	days := chartDays(t, n)
	values := make([]*float64, n)
	for i := range values {
		values[i] = ptr(float64(i))
	}
	chart := RenderChart("Input $/M", days, values, 40)
	lines := strings.Split(chart, "\n")
	axisEnd := strings.Index(lines[1], "┤") + len("┤")
	plottedWidth := len([]rune(lines[1])) - len([]rune(lines[1][:axisEnd]))
	if plottedWidth != 40 {
		t.Errorf("plotted width = %d, want 40 (downsampled)", plottedWidth)
	}
	if !strings.Contains(chart, "$199") {
		t.Errorf("chart is missing the max value after downsampling:\n%s", chart)
	}
}

func TestResampleColumnsKeepsLastNonGapValuePerBucketWhenDownsampling(t *testing.T) {
	days := chartDays(t, 6)
	// bucket 0 = indices [0,1] (1, gap); bucket 1 = indices [2,3] (3, 4);
	// bucket 2 = indices [4,5] (gap, 6) — "last non-gap in the bucket"
	// means the most recent value, not the first one written.
	values := []*float64{ptr(1), nil, ptr(3), ptr(4), nil, ptr(6)}
	outDays, outValues := resampleColumns(days, values, 3)
	if len(outValues) != 3 || len(outDays) != 3 {
		t.Fatalf("resampleColumns returned %d values, %d days, want 3/3", len(outValues), len(outDays))
	}
	if outValues[0] == nil || *outValues[0] != 1 {
		t.Errorf("bucket 0 = %v, want 1 (only non-nil value)", outValues[0])
	}
	if outValues[1] == nil || *outValues[1] != 4 {
		t.Errorf("bucket 1 = %v, want 4 (the LAST non-nil value in [3,4], not the first)", outValues[1])
	}
	if outValues[2] == nil || *outValues[2] != 6 {
		t.Errorf("bucket 2 = %v, want 6", outValues[2])
	}
}

// TestResampleColumnsStretchesAShorterSeriesToFillTheWidth is the fix for a
// short series (fewer points than the target chart width) collapsing into
// a chart only as wide as the point count — too narrow to show real
// X-axis date labels. Each point should instead widen into its own
// multi-column bar spanning the full requested width.
func TestResampleColumnsStretchesAShorterSeriesToFillTheWidth(t *testing.T) {
	days := chartDays(t, 3)
	values := []*float64{ptr(1), ptr(2), ptr(3)}
	outDays, outValues := resampleColumns(days, values, 9)
	if len(outValues) != 9 || len(outDays) != 9 {
		t.Fatalf("resampleColumns = %d values, %d days, want 9/9 (stretched to width)", len(outValues), len(outDays))
	}
	// 3 points stretched across 9 columns: 3 columns per point, in order.
	want := []float64{1, 1, 1, 2, 2, 2, 3, 3, 3}
	for i, w := range want {
		if outValues[i] == nil || *outValues[i] != w {
			t.Fatalf("outValues = %v, want each source point repeated 3x: %v", derefAll(outValues), want)
		}
	}
}

func derefAll(values []*float64) []any {
	out := make([]any, len(values))
	for i, v := range values {
		if v == nil {
			out[i] = nil
			continue
		}
		out[i] = *v
	}
	return out
}
