//go:build !unix && !windows

package openpr

import (
	"fmt"
	"os"
)

// hasMultipleHardLinks has no implementation for this platform (plan9, js/wasm,
// …). It fails closed.
//
// The previous version silently returned false, which meant the hard-link guard
// quietly did nothing on every non-unix target — including Windows, which
// GoReleaser ships and whose NTFS supports hard links. A security guard that
// cannot check must refuse, not wave the write through.
func hasMultipleHardLinks(string, os.FileInfo) (bool, error) {
	return false, fmt.Errorf("hard-link detection is unsupported on this platform; refusing to overwrite the manifest")
}
