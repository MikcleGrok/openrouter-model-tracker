package version

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
)

func Normalize(value string) string {
	return strings.TrimPrefix(value, "v")
}

func ValidateTag(tag string) (string, error) {
	if !strings.HasPrefix(tag, "v") {
		return "", fmt.Errorf("release tag %q must start with v", tag)
	}
	return validateVersion(strings.TrimPrefix(tag, "v"), tag)
}

func ValidateVersion(value string) (string, error) {
	return validateVersion(value, value)
}

func validateVersion(value, display string) (string, error) {
	if strings.HasPrefix(value, "v") {
		return "", fmt.Errorf("version %q must not have a v prefix", display)
	}
	parsed, err := semver.NewVersion(value)
	if err != nil || parsed.String() != value || parsed.Metadata() != "" {
		return "", fmt.Errorf("version %q must be SemVer 2.0.0 without build metadata", display)
	}
	if strings.Contains(value, "+") {
		return "", fmt.Errorf("version %q must not contain build metadata", display)
	}
	return value, nil
}
