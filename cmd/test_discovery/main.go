package main

import (
	"fmt"
	"torus_go_agent/internal/core"
)

func main() {
	files := core.LoadAndParseAll("/home/crab/projects/go_sdk_agent", core.LoadReasonSessionStart)
	fmt.Printf("Discovered %d files:\n", len(files))
	totalChars := 0
	for _, f := range files {
		chars := len(f.Content)
		toks := chars / 4
		totalChars += chars
		fmt.Printf("  [%s] %s (%d chars ~%d tok) paths=%v\n", f.MemType, f.Path, chars, toks, f.Paths)
	}
	fmt.Printf("\nTotal: %d chars ~%d tokens\n", totalChars, totalChars/4)

	base := core.BuildPrompt(files, nil)
	fmt.Printf("\nBase prompt (no active files): %d chars ~%d tokens\n", len(base), len(base)/4)

	full := core.BuildPrompt(files, []string{"internal/core/loop.go"})
	fmt.Printf("Full prompt (loop.go active):  %d chars ~%d tokens\n", len(full), len(full)/4)

	minimal := core.BuildPrompt(files, []string{"README.md"})
	fmt.Printf("Minimal (README.md active):    %d chars ~%d tokens\n", len(minimal), len(minimal)/4)
}
