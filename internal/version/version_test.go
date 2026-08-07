package version

import "testing"

func TestValidateTag(t *testing.T) {
	tests := []struct {
		tag  string
		want string
		ok   bool
	}{
		{tag: "v1.2.3", want: "1.2.3", ok: true},
		{tag: "v1.2.3-rc.1", want: "1.2.3-rc.1", ok: true},
		{tag: "v1.2.3-rc.01", ok: false},
		{tag: "v01.2.3", ok: false},
		{tag: "v1.2", ok: false},
		{tag: "v1.2.3+build.1", ok: false},
		{tag: "v1.2.3-rc.0", want: "1.2.3-rc.0", ok: true},
		{tag: "v1.2.3-rc..1", ok: false},
		{tag: "v1.2.3-rc.1+build", ok: false},
		{tag: "1.2.3", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			got, err := ValidateTag(tt.tag)
			if tt.ok {
				if err != nil || got != tt.want {
					t.Fatalf("ValidateTag(%q) = %q, %v; want %q", tt.tag, got, err, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateTag(%q) succeeded; want error", tt.tag)
			}
		})
	}
}

func TestValidateVersion(t *testing.T) {
	valid := []string{"0.0.0", "1.2.3", "1.2.3-rc.1", "1.2.3-rc.0"}
	for _, value := range valid {
		if got, err := ValidateVersion(value); err != nil || got != value {
			t.Errorf("ValidateVersion(%q) = %q, %v", value, got, err)
		}
	}
	invalid := []string{"v1.2.3", "1.2", "01.2.3", "1.2.3-01", "1.2.3-rc..1", "1.2.3+build.1", "1.2.3-rc.1+build"}
	for _, value := range invalid {
		if _, err := ValidateVersion(value); err == nil {
			t.Errorf("ValidateVersion(%q) succeeded; want error", value)
		}
	}
}

func TestNormalize(t *testing.T) {
	if got := Normalize("v1.2.3-1-gabc123"); got != "1.2.3-1-gabc123" {
		t.Errorf("Normalize() = %q", got)
	}
}
