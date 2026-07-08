//go:build unix

package openpr

import (
	"os"
	"syscall"
)

func hasMultipleHardLinks(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink > 1
}
