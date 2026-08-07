// Package pricing implements the arithmetic and the number formatting rules of
// docs/openrouter-model-comparison.md. It has no dependencies beyond the
// standard library and knows nothing about models, HTTP or rendering.
package pricing

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// MixedPrice returns the 3:1 input:output blended price in $/M tokens.
func MixedPrice(inPerM, outPerM float64) float64 {
	return (3*inPerM + outPerM) / 4
}

// QualityPrice returns a benchmark score (in percent) divided by the blended
// price. A non-positive price yields 0: free models are ranked by score, not by
// this metric.
func QualityPrice(scorePct, mixedPerM float64) float64 {
	if mixedPerM <= 0 {
		return 0
	}
	return scorePct / mixedPerM
}

// Tokens10 returns how many millions of tokens $10 buys at pricePerM.
func Tokens10(pricePerM float64) float64 {
	if pricePerM <= 0 {
		return 0
	}
	return 10 / pricePerM
}

// FormatQualityPrice prints the Качество/цена column: one decimal below 100,
// an integer at 100 and above. Values that round up to 100 are printed as
// integers, because that is what the rounded value is.
func FormatQualityPrice(v float64) string {
	if s := strconv.FormatFloat(v, 'f', 1, 64); s != "100.0" && v < 100 {
		return s
	}
	return strconv.FormatFloat(v, 'f', 0, 64)
}

// FormatTokens10 prints a token count for the "Сколько токенов даст $10"
// tables: three significant digits, capped at two decimal places. The cap is
// what makes 0.4 print as "0.40" rather than "0.400".
func FormatTokens10(v float64) string {
	if v == 0 {
		return "0"
	}
	digits := 3 - int(math.Floor(math.Log10(math.Abs(v)))) - 1
	if digits > 2 {
		digits = 2
	}
	if digits < 0 {
		digits = 0
	}
	return strconv.FormatFloat(v, 'f', digits, 64)
}

// FormatPrice prints a $/M price with exactly two decimals. Positive values
// below one cent are omitted because they are not meaningful at this precision.
func FormatPrice(v float64) string {
	if v > 0 && v < 0.01 {
		return ""
	}
	decimal := strconv.FormatFloat(v, 'f', -1, 64)
	rounded, ok := new(big.Rat).SetString(decimal)
	if !ok {
		return ""
	}
	return rounded.FloatString(2)
}

// FormatDollar formats a displayed $/M price, omitting the currency marker
// together with prices that are too small to display.
func FormatDollar(v float64) string {
	if price := FormatPrice(v); price != "" {
		return "$" + price
	}
	return ""
}

// FormatContext prints a context window: millions above 1M, thousands below.
func FormatContext(n int) string {
	if n >= 1_000_000 {
		s := strconv.FormatFloat(float64(n)/1_000_000, 'f', 1, 64)
		return strings.TrimSuffix(s, ".0") + "M"
	}
	return strconv.Itoa(int(math.Round(float64(n)/1000))) + "K"
}
