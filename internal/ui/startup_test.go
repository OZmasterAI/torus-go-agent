package ui

import "testing"

func TestClampScrollOffset(t *testing.T) {
	tests := []struct {
		name         string
		cursor       int
		scrollOffset int
		total        int
		want         int
	}{
		// Short list: always returns 0
		{"short list no scroll", 2, 0, 5, 0},
		{"short list even with offset", 2, 3, 5, 0},

		// Exactly visibleItems: no scroll needed
		{"exact fit", 5, 0, visibleItems, 0},

		// Long list: cursor in view, no change
		{"cursor in view", 5, 3, 20, 3},

		// Long list: cursor above viewport, scroll up
		{"cursor above viewport", 2, 5, 20, 2},

		// Long list: cursor below viewport, scroll down
		{"cursor below viewport", 15, 3, 20, 15 - visibleItems + 1},

		// Wrap from top to bottom: cursor at last, offset was 0
		{"wrap to bottom", 19, 0, 20, 19 - visibleItems + 1},

		// Wrap from bottom to top: cursor at 0, offset was at end
		{"wrap to top", 0, 10, 20, 0},

		// Cursor exactly at viewport boundary (last visible)
		{"cursor at last visible", 12, 3, 20, 3},

		// Cursor one past viewport boundary
		{"cursor one past visible", 13, 3, 20, 13 - visibleItems + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampScrollOffset(tt.cursor, tt.scrollOffset, tt.total)
			if got != tt.want {
				t.Errorf("clampScrollOffset(%d, %d, %d) = %d, want %d",
					tt.cursor, tt.scrollOffset, tt.total, got, tt.want)
			}
		})
	}
}
