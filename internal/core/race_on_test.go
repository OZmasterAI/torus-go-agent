//go:build race

package core

// raceEnabled is true when the race detector is active; timing-sensitive
// tests use it to widen their thresholds (tolerates -race overhead).
const raceEnabled = true
