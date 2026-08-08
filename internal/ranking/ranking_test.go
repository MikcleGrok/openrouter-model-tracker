package ranking

import (
	"math"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultCompatibility(t *testing.T) {
	c, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	ctx := c.Context(93, 3, 1, "haiku")
	got, err := c.Evaluate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := 93 + 10*0.5*math.Log1p(93/2.5)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("utility = %v, want %v", got, want)
	}
}

func TestCustomFormulaAndPriceMix(t *testing.T) {
	var expr Expr
	if err := yaml.Unmarshal([]byte("op: add\nargs:\n  - var: score\n  - op: mul\n    args:\n      - var: tier_factor\n      - op: log1p\n        args:\n          - var: quality_price\n"), &expr); err != nil {
		t.Fatal(err)
	}
	c, err := Compile(Config{Price: PriceConfig{InputWeight: ptr(1), OutputWeight: ptr(1)}, TierFactors: map[string]float64{"opus": 2}, Formula: &expr})
	if err != nil {
		t.Fatal(err)
	}
	ctx := c.Context(10, 1, 3, "opus")
	got, err := c.Evaluate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := 10 + 2*math.Log1p(5)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("utility = %v, want %v", got, want)
	}
}

func TestRejectsUnsafeFormula(t *testing.T) {
	for _, source := range []string{"op: add\nargs: []\n", "var: exec\n", "op: pow\nargs: []\n"} {
		var expr Expr
		if err := yaml.Unmarshal([]byte(source), &expr); err != nil {
			t.Fatal(err)
		}
		if _, err := Compile(Config{Formula: &expr}); err == nil {
			t.Fatalf("accepted %q", source)
		}
	}
}

func TestRuntimeDomainErrors(t *testing.T) {
	zero := Expr{Op: "div", Args: []Expr{{Const: ptr(1)}, {Const: ptr(0)}}}
	c, err := Compile(Config{Formula: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Evaluate(c.Context(1, 1, 1, "sonnet")); err == nil {
		t.Fatal("division by zero accepted")
	}
}

func TestNormalizeMinMax(t *testing.T) {
	got := NormalizeMinMax([]float64{1315.4, 1453, 1497.2})
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0] != 0 {
		t.Errorf("min mapped to %v, want 0", got[0])
	}
	if got[2] != 100 {
		t.Errorf("max mapped to %v, want 100", got[2])
	}
	if want := (1453 - 1315.4) * 100 / (1497.2 - 1315.4); math.Abs(got[1]-want) > 1e-9 {
		t.Errorf("middle mapped to %v, want %v", got[1], want)
	}
}

func TestNormalizeMinMaxWithoutSpread(t *testing.T) {
	if got := NormalizeMinMax([]float64{1400}); len(got) != 1 || got[0] != 100 {
		t.Errorf("single value = %v, want [100]: there is nothing to rank against, and 0 would read as worst possible", got)
	}
	if got := NormalizeMinMax([]float64{1400, 1400, 1400}); len(got) != 3 || got[0] != 100 || got[2] != 100 {
		t.Errorf("flat set = %v, want all 100", got)
	}
}

func TestNormalizeMinMaxEmpty(t *testing.T) {
	if got := NormalizeMinMax(nil); len(got) != 0 {
		t.Errorf("nil input = %v, want an empty slice and no panic", got)
	}
}

func TestNormalizedArenaScoreFeedsTheSameFormula(t *testing.T) {
	compiled, err := Compile(DefaultConfig())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	normalized := NormalizeMinMax([]float64{1315.4, 1453, 1497.2})
	low, err := compiled.Evaluate(compiled.Context(normalized[0], 1, 3, "sonnet"))
	if err != nil {
		t.Fatalf("Evaluate(low): %v", err)
	}
	high, err := compiled.Evaluate(compiled.Context(normalized[2], 1, 3, "sonnet"))
	if err != nil {
		t.Fatalf("Evaluate(high): %v", err)
	}
	if !(high > low) {
		t.Errorf("utility(max)=%v is not above utility(min)=%v at identical prices", high, low)
	}
}
