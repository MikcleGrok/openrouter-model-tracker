// Package ranking compiles and evaluates the safe mixed-utility expression tree.
package ranking

import (
	"fmt"
	"math"
	"strings"

	tierpkg "github.com/sboborikin/openrouter-model-tracker/internal/tier"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPriceWeight = 10.0
	MaxDepth           = 16
	MaxNodes           = 64
)

type PriceConfig struct {
	InputWeight  *float64 `yaml:"input_weight"`
	OutputWeight *float64 `yaml:"output_weight"`
}

type Config struct {
	PriceWeight *float64           `yaml:"price_weight"`
	Price       PriceConfig        `yaml:"price"`
	TierFactors map[string]float64 `yaml:"tier_factors"`
	Formula     *Expr              `yaml:"formula"`
}

type Expr struct {
	Var   string   `yaml:"var"`
	Const *float64 `yaml:"const"`
	Op    string   `yaml:"op"`
	Args  []Expr   `yaml:"args"`
}

func (e *Expr) UnmarshalYAML(value *yaml.Node) error {
	type plain Expr
	var decoded plain
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("ranking: formula node must be a mapping")
	}
	for i := 0; i < len(value.Content); i += 2 {
		key := value.Content[i].Value
		if key != "var" && key != "const" && key != "op" && key != "args" {
			return fmt.Errorf("ranking: unknown formula key %q", key)
		}
	}
	if err := value.Decode(&decoded); err != nil {
		return fmt.Errorf("ranking: invalid formula node: %w", err)
	}
	*e = Expr(decoded)
	return nil
}

type Context struct {
	Score, InputPrice, OutputPrice, PriceMix, QualityPrice, TierValue, TierFactor float64
}

type Compiled struct {
	inputWeight, outputWeight float64
	tierFactors               map[string]float64
	formula                   *Expr
	priceWeight               float64
}

func DefaultConfig() Config { return Config{PriceWeight: ptr(DefaultPriceWeight)} }

func ptr(value float64) *float64 { return &value }

func Compile(c Config) (Compiled, error) {
	if c.Formula != nil && c.PriceWeight != nil {
		return Compiled{}, fmt.Errorf("ranking: formula cannot be combined with price_weight")
	}
	input, output := 3.0, 1.0
	if c.Price.InputWeight != nil {
		input = *c.Price.InputWeight
	}
	if c.Price.OutputWeight != nil {
		output = *c.Price.OutputWeight
	}
	if !finiteNonNegative(input) || !finiteNonNegative(output) || input+output <= 0 {
		return Compiled{}, fmt.Errorf("ranking: price weights must be finite, non-negative, and have a positive sum")
	}
	weight := DefaultPriceWeight
	if c.PriceWeight != nil {
		weight = *c.PriceWeight
	}
	if !finiteNonNegative(weight) {
		return Compiled{}, fmt.Errorf("ranking: price_weight must be finite and non-negative")
	}
	factors := map[string]float64{"opus": 1, "sonnet": 1, "haiku": 0.5, "free": 0, "default": 0}
	for tier, factor := range c.TierFactors {
		if !finite(factor) {
			return Compiled{}, fmt.Errorf("ranking: tier factor %q must be finite", tier)
		}
		if tier != "default" && !tierpkg.IsValid(tier) {
			return Compiled{}, fmt.Errorf("ranking: unknown tier factor %q", tier)
		}
		factors[tier] = factor
	}
	if c.Formula != nil {
		if err := validateExpr(c.Formula, 1, new(int)); err != nil {
			return Compiled{}, err
		}
	}
	return Compiled{inputWeight: input, outputWeight: output, tierFactors: factors, formula: c.Formula, priceWeight: weight}, nil
}

func (c Compiled) Evaluate(ctx Context) (float64, error) {
	if !allFinite(ctx) {
		return 0, fmt.Errorf("ranking: context contains non-finite number")
	}
	if c.formula == nil {
		if ctx.QualityPrice <= -1 {
			return 0, fmt.Errorf("ranking: log1p domain error")
		}
		value := ctx.Score + c.priceWeight*ctx.TierFactor*math.Log1p(ctx.QualityPrice)
		if !finite(value) {
			return 0, fmt.Errorf("ranking: formula returned non-finite number")
		}
		return value, nil
	}
	value, err := eval(*c.formula, ctx)
	if err != nil {
		return 0, err
	}
	if !finite(value) {
		return 0, fmt.Errorf("ranking: formula returned non-finite number")
	}
	return value, nil
}

// QualityUtility evaluates the ranking expression with every price-dependent
// input disabled. Non-price terms such as score and tier factor remain active;
// this is the base_quality term of the canonical utility model.
func (c Compiled) QualityUtility(score, input, output float64, tier string) (float64, error) {
	ctx := c.Context(score, input, output, tier)
	ctx.InputPrice, ctx.OutputPrice, ctx.PriceMix = 0, 0, 0
	ctx.QualityPrice = 0
	return c.Evaluate(ctx)
}

// FullUtility evaluates the current ranking model with base_qp as its
// price-dependent input. The caller must derive base_qp from QualityUtility;
// passing full q/p here would make the formula circular.
func (c Compiled) FullUtility(score, input, output, baseQP float64, tier string) (float64, error) {
	ctx := c.Context(score, input, output, tier)
	ctx.QualityPrice = baseQP
	return c.Evaluate(ctx)
}

func (c Compiled) Context(score, input, output float64, tier string) Context {
	mix := (input*c.inputWeight + output*c.outputWeight) / (c.inputWeight + c.outputWeight)
	tierValue := tierValue(tier)
	factor, ok := c.tierFactors[strings.ToLower(tier)]
	if !ok {
		factor = c.tierFactors["default"]
	}
	quality := 0.0
	if mix > 0 {
		quality = score / mix
	}
	return Context{Score: score, InputPrice: input, OutputPrice: output, PriceMix: mix, QualityPrice: quality, TierValue: tierValue, TierFactor: factor}
}

func validateExpr(expr *Expr, depth int, nodes *int) error {
	if depth > MaxDepth {
		return fmt.Errorf("ranking: formula exceeds maximum depth %d", MaxDepth)
	}
	(*nodes)++
	if *nodes > MaxNodes {
		return fmt.Errorf("ranking: formula exceeds maximum node count %d", MaxNodes)
	}
	if expr.Var != "" {
		if expr.Const != nil || expr.Op != "" || len(expr.Args) != 0 {
			return fmt.Errorf("ranking: var node cannot have other fields")
		}
		if !allowedVar(expr.Var) {
			return fmt.Errorf("ranking: unknown variable %q", expr.Var)
		}
		return nil
	}
	if expr.Const != nil {
		if expr.Op != "" || len(expr.Args) != 0 || !finite(*expr.Const) {
			return fmt.Errorf("ranking: const must be a finite number")
		}
		return nil
	}
	arity, ok := map[string]int{"add": 2, "sub": 2, "mul": 2, "div": 2, "neg": 1, "log": 1, "log1p": 1, "min": 2, "max": 2, "clamp": 3}[expr.Op]
	if !ok {
		return fmt.Errorf("ranking: unknown operation %q", expr.Op)
	}
	if len(expr.Args) != arity {
		return fmt.Errorf("ranking: operation %q requires %d args", expr.Op, arity)
	}
	for i := range expr.Args {
		if err := validateExpr(&expr.Args[i], depth+1, nodes); err != nil {
			return err
		}
	}
	return nil
}

func allowedVar(name string) bool {
	switch name {
	case "score", "input_price", "output_price", "price_mix", "quality_price", "tier_value", "tier_factor":
		return true
	}
	return false
}
func finite(value float64) bool            { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func finiteNonNegative(value float64) bool { return finite(value) && value >= 0 }
func allFinite(c Context) bool {
	return finite(c.Score) && finite(c.InputPrice) && finite(c.OutputPrice) && finite(c.PriceMix) && finite(c.QualityPrice) && finite(c.TierValue) && finite(c.TierFactor)
}
func tierValue(tier string) float64 {
	switch strings.ToLower(tier) {
	case "opus":
		return 3
	case "sonnet":
		return 2
	case "haiku":
		return 1
	case "free":
		return 0
	}
	return 0
}

func eval(expr Expr, c Context) (float64, error) {
	if expr.Var != "" {
		switch expr.Var {
		case "score":
			return c.Score, nil
		case "input_price":
			return c.InputPrice, nil
		case "output_price":
			return c.OutputPrice, nil
		case "price_mix":
			return c.PriceMix, nil
		case "quality_price":
			return c.QualityPrice, nil
		case "tier_value":
			return c.TierValue, nil
		case "tier_factor":
			return c.TierFactor, nil
		}
	}
	if expr.Const != nil {
		return *expr.Const, nil
	}
	values := make([]float64, len(expr.Args))
	for i := range expr.Args {
		value, err := eval(expr.Args[i], c)
		if err != nil {
			return 0, err
		}
		values[i] = value
	}
	var result float64
	switch expr.Op {
	case "add":
		result = values[0] + values[1]
	case "sub":
		result = values[0] - values[1]
	case "mul":
		result = values[0] * values[1]
	case "div":
		if values[1] == 0 {
			return 0, fmt.Errorf("ranking: division by zero")
		}
		result = values[0] / values[1]
	case "neg":
		result = -values[0]
	case "log":
		if values[0] <= 0 {
			return 0, fmt.Errorf("ranking: log domain error")
		}
		result = math.Log(values[0])
	case "log1p":
		if values[0] <= -1 {
			return 0, fmt.Errorf("ranking: log1p domain error")
		}
		result = math.Log1p(values[0])
	case "min":
		result = math.Min(values[0], values[1])
	case "max":
		result = math.Max(values[0], values[1])
	case "clamp":
		result = math.Max(values[1], math.Min(values[0], values[2]))
	}
	if !finite(result) {
		return 0, fmt.Errorf("ranking: operation %q returned non-finite number", expr.Op)
	}
	return result, nil
}

// NormalizeMinMax rescales raw values onto 0–100 by the observed minimum and
// maximum of the set itself, so a source with its own scale (LMArena Elo,
// roughly 950–1550) can enter the same formula, with the same price_weight
// and the same tier factors, as a source already expressed in percent.
// Without it the score term would dwarf the price term and the mixed-utility
// order would collapse into a plain sort by raw Elo.
//
// The mapping is deliberately relative: it moves whenever the set's own
// extremes move, which is the documented trade-off of not hand-tuning a
// separate weight for every source. A set with no spread — one element, or
// all values equal — maps to 100, because there is nothing to rank against
// and 0 would read as "worst possible".
func NormalizeMinMax(raw []float64) []float64 {
	out := make([]float64, len(raw))
	if len(raw) == 0 {
		return out
	}
	low, high := raw[0], raw[0]
	for _, value := range raw {
		if value < low {
			low = value
		}
		if value > high {
			high = value
		}
	}
	if high == low {
		for i := range out {
			out[i] = 100
		}
		return out
	}
	for i, value := range raw {
		out[i] = (value - low) * 100 / (high - low)
	}
	return out
}
