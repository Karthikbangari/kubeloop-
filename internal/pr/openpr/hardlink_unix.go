//go:build unix

package openpr

import (
	"fmt"
	"os"
	"syscall"
)

// hasMultipleHardLinks reports whether path has more than one directory entry
// pointing at its inode. On unix the stat the caller already performed carries
// the link count, so no extra syscall is needed and path is unused.
//
// An error is returned rather than swallowed: this is a security guard, and
// "I could not tell" must never read as "it is safe".
func hasMultipleHardLinks(_ string, info os.FileInfo) (bool, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false, fmt.Errorf("cannot determine hard-link count on this filesystem")
	}
	return st.Nlink > 1, nil
}
