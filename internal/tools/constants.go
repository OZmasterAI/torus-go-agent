package tools

import (
	"os"
	"time"
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

	// DirPerm is the permission mode used when creating directories.
	DirPerm os.FileMode = 0755

	// FilePerm is the permission mode used when writing files.
	FilePerm os.FileMode = 0644

	// LineNumFormat is the format string used to prefix line numbers in read output.
	LineNumFormat = "%6d %s"
)
