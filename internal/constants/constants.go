package constants

import "os"

const (
	// DirPerm is the permission mode used when creating directories.
	DirPerm os.FileMode = 0755

	// FilePerm is the permission mode used when writing files.
	FilePerm os.FileMode = 0644
)
