package stringutils

import (
	"strings"
	"testing"
)

func TestAdd(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{2, 3, 5},
		{0, 0, 0},
		{-1, 1, 0},
		{100, 200, 300},
	}
	for _, tc := range tests {
		got := Add(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("Add(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestReverse(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"abc", "cba"},
		{"hello", "olleh"},
		{"a", "a"},
		{"", ""},
		{"racecar", "racecar"},
	}
	for _, tc := range tests {
		got := Reverse(tc.input)
		if got != tc.want {
			t.Errorf("Reverse(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFilter(t *testing.T) {
	items := []string{"apple", "banana", "avocado", "blueberry", "apricot"}
	startsWithA := func(s string) bool { return strings.HasPrefix(s, "a") }

	got := Filter(items, startsWithA)
	want := []string{"apple", "avocado", "apricot"}

	if len(got) != len(want) {
		t.Fatalf("Filter() returned %d items, want %d: got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Filter()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestContains(t *testing.T) {
	slice := []string{"foo", "bar", "baz"}
	if !Contains(slice, "bar") {
		t.Error("Contains should find 'bar'")
	}
	if Contains(slice, "qux") {
		t.Error("Contains should not find 'qux'")
	}
}

func TestMax(t *testing.T) {
	if Max(3, 7) != 7 {
		t.Error("Max(3, 7) should be 7")
	}
	if Max(10, 2) != 10 {
		t.Error("Max(10, 2) should be 10")
	}
}

func TestMin(t *testing.T) {
	if Min(3, 7) != 3 {
		t.Error("Min(3, 7) should be 3")
	}
	if Min(10, 2) != 2 {
		t.Error("Min(10, 2) should be 2")
	}
}

func TestCountOccurrences(t *testing.T) {
	if CountOccurrences("hello world hello", "hello") != 2 {
		t.Error("should find 2 occurrences of hello")
	}
	if CountOccurrences("aaa", "aa") != 2 {
		t.Error("should find 2 overlapping occurrences of aa in aaa")
	}
}

func TestTruncate(t *testing.T) {
	if Truncate("hello", 10) != "hello" {
		t.Error("short string should not be truncated")
	}
	got := Truncate("hello world", 8)
	if got != "hello..." {
		t.Errorf("Truncate(\"hello world\", 8) = %q, want \"hello...\"", got)
	}
}

func TestRepeat(t *testing.T) {
	if Repeat("ab", 3) != "ababab" {
		t.Error("Repeat('ab', 3) should be 'ababab'")
	}
	if Repeat("x", 0) != "" {
		t.Error("Repeat('x', 0) should be empty")
	}
}
