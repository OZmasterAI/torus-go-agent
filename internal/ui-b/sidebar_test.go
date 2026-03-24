package uib

import (
	"strings"
	"testing"

	"torus_go_agent/internal/config"
)

func TestSidebarRender(t *testing.T) {
	s := newSidebarModel(DefaultTheme(), config.AgentConfig{})
	s.modifiedFiles["main.go"] = 3
	view := s.View(30)
	if !strings.Contains(view, "main.go") {
		t.Fatal("should show modified file")
	}
}

func TestSidebarTrackTool(t *testing.T) {
	s := newSidebarModel(DefaultTheme(), config.AgentConfig{})
	s.TrackTool(ToolEvent{Name: "write", FilePath: "/tmp/foo.go"})
	s.TrackTool(ToolEvent{Name: "edit", FilePath: "/tmp/foo.go"})
	s.TrackTool(ToolEvent{Name: "read", FilePath: "/tmp/bar.go"})
	if s.modifiedFiles["/tmp/foo.go"] != 2 {
		t.Fatalf("expected 2 edits for foo.go, got %d", s.modifiedFiles["/tmp/foo.go"])
	}
	if _, ok := s.modifiedFiles["/tmp/bar.go"]; ok {
		t.Fatal("read should not count as modified")
	}
}

func TestSidebarEmpty(t *testing.T) {
	s := newSidebarModel(DefaultTheme(), config.AgentConfig{})
	view := s.View(20)
	if !strings.Contains(view, "Session") {
		t.Fatal("sidebar should show Session title")
	}
	if !strings.Contains(view, "(none)") {
		t.Fatal("sidebar should show (none) for empty files")
	}
}

func TestSidebarSteerPlusFlag(t *testing.T) {
	s := newSidebarModel(DefaultTheme(), config.AgentConfig{})
	// With steerAggressive = false, Steer+ should show with hollow dot.
	s.steerAggressive = false
	view := s.View(30)
	if !strings.Contains(view, "Steer+") {
		t.Fatal("sidebar should show Steer+ flag")
	}

	// With steerAggressive = true, Steer+ should show with filled dot.
	s.steerAggressive = true
	view = s.View(30)
	if !strings.Contains(view, "Steer+") {
		t.Fatal("sidebar should show Steer+ flag when aggressive")
	}
	// The filled dot (U+25CF) should appear for aggressive mode.
	if !strings.Contains(view, "\u25cf") {
		t.Fatal("sidebar should show filled dot for aggressive Steer+")
	}
}

func TestSidebarFiveFlags(t *testing.T) {
	s := newSidebarModel(DefaultTheme(), config.AgentConfig{
		SmartRouting:          true,
		ContinuousCompression: true,
		ZoneBudgeting:         true,
		Compaction:             "llm",
	})
	s.steerAggressive = true
	view := s.View(30)

	flags := []string{"Smart", "Compress", "Zones", "Compact", "Steer+"}
	for _, flag := range flags {
		if !strings.Contains(view, flag) {
			t.Fatalf("sidebar should show %s flag", flag)
		}
	}
}
