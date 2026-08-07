package main

import (
	"fmt"
	"os"

	"github.com/sboborikin/openrouter-model-tracker/internal/version"
)

func main() {
	normalized, err := validateTag(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Println(normalized)
}

func validateTag(args []string) (string, error) {
	if len(args) == 2 && args[0] == "--version" {
		return version.ValidateVersion(args[1])
	}
	if len(args) != 1 {
		return "", fmt.Errorf("usage: versioncheck TAG | versioncheck --version VERSION")
	}
	return version.ValidateTag(args[0])
}
