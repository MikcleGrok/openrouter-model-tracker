// Command openrouter regenerates the OpenRouter model comparison document
// from live prices and benchmark leaderboards.
package main

import "fmt"

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	fmt.Printf("openrouter %s\n", version)
}
