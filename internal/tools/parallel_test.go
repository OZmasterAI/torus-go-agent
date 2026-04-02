package tools_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"torus_go_agent/internal/tools"
	types "torus_go_agent/internal/types"
)

func TestParallelRead(tt *testing.T) {
	allTools := tools.BuildDefaultTools()
	var readTool *types.Tool
	for i := range allTools {
		if allTools[i].Name == "read" {
			readTool = &allTools[i]
			break
		}
	}
	if readTool == nil {
		tt.Fatal("read tool not found")
	}

	files := []string{
		"../../cmd/main.go",
		"../../internal/core/loop.go",
		"../../internal/tools/tools.go",
	}

	results := make([]*types.ToolResult, 3)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)

	start := time.Now()

	for i, fp := range files {
		wg.Add(1)
		go func(idx int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r, err := readTool.Execute(map[string]any{
				"file_path": path,
				"limit":     float64(5),
			})
			if err != nil {
				results[idx] = &types.ToolResult{Content: "error: " + err.Error(), IsError: true}
			} else {
				results[idx] = r
			}
		}(i, fp)
	}

	wg.Wait()
	elapsed := time.Since(start)

	fmt.Printf("3 parallel reads completed in %v\n", elapsed)

	for i, fp := range files {
		base := fp[strings.LastIndex(fp, "/")+1:]
		if results[i] == nil {
			tt.Errorf("[%d] %s: result is nil", i+1, base)
			continue
		}
		if results[i].IsError {
			tt.Errorf("[%d] %s: error: %s", i+1, base, results[i].Content)
			continue
		}
		lines := strings.Count(results[i].Content, "\n")
		firstLine := strings.SplitN(results[i].Content, "\n", 2)[0]
		fmt.Printf("  [%d] %s — %d lines, first: %s\n", i+1, base, lines, firstLine)
	}
}
