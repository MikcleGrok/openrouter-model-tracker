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
