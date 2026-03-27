package tools

import (
	"time"

	"torus_go_agent/internal/constants"
)

const (
	// BashTimeout is the maximum time a bash command may run before being killed.
	BashTimeout = 30 * time.Second

	// BashTimeoutMsg is returned when a bash command times out.
	BashTimeoutMsg = "Timed out (30s)"

	// NoOutputMsg is returned when a bash command produces no output.
	NoOutputMsg = "(no output)"

	// NoMatchesMsg is returned when glob or grep finds no results.
	NoMatchesMsg = "(no matches)"

	// DirPerm re-exports constants.DirPerm for use within the tools package.
	DirPerm = constants.DirPerm

	// FilePerm re-exports constants.FilePerm for use within the tools package.
	FilePerm = constants.FilePerm

	// LineNumFormat is the format string used to prefix line numbers in read output.
	LineNumFormat = "%6d %s"
)
