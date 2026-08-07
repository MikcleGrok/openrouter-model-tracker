package pricing

import "testing"

func TestMixedPrice(t *testing.T) {
	if got := MixedPrice(0.5, 3.0); got != 1.125 {
		t.Fatalf("MixedPrice(0.5, 3.0) = %v, want 1.125", got)
	}
	if got := MixedPrice(5.0, 30.0); got != 11.25 {
		t.Fatalf("MixedPrice(5.0, 30.0) = %v, want 11.25", got)
	}
}

func TestQualityPrice(t *testing.T) {
	got := QualityPrice(93.0, 1.125)
	if got < 82.66 || got > 82.67 {
		t.Fatalf("QualityPrice(93.0, 1.125) = %v, want ~82.667", got)
	}
	if got := QualityPrice(93.0, 0); got != 0 {
		t.Fatalf("QualityPrice with zero price = %v, want 0 (free models are not ranked by this metric)", got)
	}
}

func TestTokens10(t *testing.T) {
	if got := Tokens10(0.5); got != 20 {
		t.Fatalf("Tokens10(0.5) = %v, want 20", got)
	}
	if got := Tokens10(0); got != 0 {
		t.Fatalf("Tokens10(0) = %v, want 0", got)
	}
}

func TestFormatQualityPrice(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{8.6, "8.6"},
		{82.667, "82.7"},
		{99.94, "99.9"},
		{99.96, "100"}, // округляется до 100 -> уже целое
		{100, "100"},
		{153.3, "153"},
		{436.2, "436"},
	}
	for _, c := range cases {
		if got := FormatQualityPrice(c.in); got != c.want {
			t.Errorf("FormatQualityPrice(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatTokens10(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{0.4, "0.40"}, // < 1: три значащие цифры дали бы 0.400, но потолок — 2 знака
		{0.33333, "0.33"},
		{0.88889, "0.89"},
		{1.0, "1.00"},
		{2.0, "2.00"},
		{3.33333, "3.33"},
		{8.88889, "8.89"},
		{12.5, "12.5"}, // >= 10: три значащие цифры = 1 знак после запятой
		{20.0, "20.0"},
		{57.14286, "57.1"},
		{83.33333, "83.3"},
		{146.4, "146"}, // >= 100: три значащие цифры = целое
		{436.2, "436"},
		{1452.3, "1452"}, // > 999: округлять некуда, печатаем целое
	}
	for _, c := range cases {
		if got := FormatTokens10(c.in); got != c.want {
			t.Errorf("FormatTokens10(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatPrice(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0.00"},
		{0.5, "0.50"},
		{3, "3.00"},
		{30, "30.00"},
		{0.14, "0.14"},
		{0.132, "0.13"},
		{0.435, "0.44"},
		{1.475, "1.48"},
		{0.6818, "0.68"},
	}
	for _, c := range cases {
		if got := FormatPrice(c.in); got != c.want {
			t.Errorf("FormatPrice(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatPriceSubCentAndHalfCentBoundaries(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0.00"},
		{0.0049, ""},
		{0.0099, ""},
		{0.01, "0.01"},
		{1.004, "1.00"},
		{1.0049999995, "1.00"},
		{1.005, "1.01"},
		{1.006, "1.01"},
		{12.3, "12.30"},
	}
	for _, c := range cases {
		if got := FormatPrice(c.in); got != c.want {
			t.Errorf("FormatPrice(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatDollarOmitsSubCentCurrencyMarker(t *testing.T) {
	if got := FormatDollar(0.009); got != "" {
		t.Fatalf("FormatDollar(0.009) = %q, want empty", got)
	}
	if got := FormatDollar(0); got != "$0.00" {
		t.Fatalf("FormatDollar(0) = %q, want $0.00", got)
	}
}

func TestFormatContext(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{1000000, "1M"},
		{1048576, "1M"},
		{2000000, "2M"},
		{500000, "500K"},
		{262144, "262K"},
		{256000, "256K"},
		{200000, "200K"},
		{163840, "164K"},
		{131072, "131K"},
		{128000, "128K"},
	}
	for _, c := range cases {
		if got := FormatContext(c.in); got != c.want {
			t.Errorf("FormatContext(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
