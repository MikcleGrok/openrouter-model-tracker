// Package keymap contains shared key binding normalization.
package keymap

import "strings"

// CanonicalBinding returns the representation used when comparing a binding
// from configuration with a key event.
func CanonicalBinding(binding string) string {
	if binding == " " {
		return "space"
	}
	binding = strings.ToLower(strings.TrimSpace(binding))
	if binding == " " {
		return "space"
	}
	if binding == "space" {
		return "space"
	}
	return binding
}
