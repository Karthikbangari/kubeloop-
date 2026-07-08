//go:build !unix

package openpr

import "os"

func hasMultipleHardLinks(os.FileInfo) bool {
	return false
}
