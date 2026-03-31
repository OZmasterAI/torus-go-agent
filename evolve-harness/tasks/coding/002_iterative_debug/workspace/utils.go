package stringutils

// Add returns the sum of two integers.
// BUG: off-by-one, returns a+b-1 instead of a+b.
func Add(a, b int) int {
	return a + b - 1
}

// Reverse returns the string with characters in reverse order.
// BUG: index calculation uses len(s)-1 instead of len(s)-i-1.
func Reverse(s string) string {
	runes := []rune(s)
	n := len(runes)
	result := make([]rune, n)
	for i := 0; i < n; i++ {
		result[i] = runes[n-1]
	}
	return string(result)
}

// Filter returns a new slice containing only elements where predicate returns true.
// BUG: logic is inverted — keeps items where predicate is false.
func Filter(items []string, predicate func(string) bool) []string {
	var result []string
	for _, item := range items {
		if !predicate(item) {
			result = append(result, item)
		}
	}
	return result
}

// Contains checks if a string is in the slice.
func Contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// Max returns the larger of two integers.
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Min returns the smaller of two integers.
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CountOccurrences counts how many times substr appears in s.
func CountOccurrences(s, substr string) int {
	count := 0
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}
	return count
}

// Truncate shortens a string to maxLen characters, adding "..." if truncated.
func Truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// IsPalindrome checks if a string reads the same forwards and backwards.
func IsPalindrome(s string) bool {
	rev := Reverse(s)
	return s == rev
}

// Repeat returns the string repeated n times.
func Repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
