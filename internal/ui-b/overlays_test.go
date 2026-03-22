package uib

import "testing"

func TestOverlayOpenClose(t *testing.T) {
	o := newOverlayModel(DefaultTheme())
	if o.Active() {
		t.Fatal("overlay should start inactive")
	}
	items := []OverlayItem{
		{Name: "New conversation", Command: "/new"},
		{Name: "Exit", Command: "/exit"},
	}
	o.Open("palette", items)
	if !o.Active() {
		t.Fatal("overlay should be active after Open")
	}
	if o.Kind() != "palette" {
		t.Fatalf("expected kind 'palette', got %q", o.Kind())
	}
	o.Close()
	if o.Active() {
		t.Fatal("overlay should be inactive after Close")
	}
}

func TestOverlayFilter(t *testing.T) {
	o := newOverlayModel(DefaultTheme())
	items := []OverlayItem{
		{Name: "New conversation", Command: "/new"},
		{Name: "Exit", Command: "/exit"},
		{Name: "Clear history", Command: "/clear"},
	}
	o.Open("palette", items)
	o.SetQuery("ex")
	filtered := o.Filtered()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 match, got %d", len(filtered))
	}
	if filtered[0].Command != "/exit" {
		t.Fatalf("expected /exit, got %q", filtered[0].Command)
	}
}

func TestOverlayFilterEmpty(t *testing.T) {
	o := newOverlayModel(DefaultTheme())
	items := []OverlayItem{{Name: "A", Command: "/a"}}
	o.Open("palette", items)
	o.SetQuery("")
	if len(o.Filtered()) != 1 {
		t.Fatal("empty query should return all items")
	}
}

func TestOverlaySelected(t *testing.T) {
	o := newOverlayModel(DefaultTheme())
	items := []OverlayItem{
		{Name: "A", Command: "/a"},
		{Name: "B", Command: "/b"},
	}
	o.Open("palette", items)
	o.idx = 1
	sel := o.Selected()
	if sel.Command != "/b" {
		t.Fatalf("expected /b selected, got %q", sel.Command)
	}
}
