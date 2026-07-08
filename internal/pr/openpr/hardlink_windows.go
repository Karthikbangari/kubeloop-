//go:build windows

package openpr

import (
	"fmt"
	"os"
	"syscall"
)

// hasMultipleHardLinks reports whether path has more than one directory entry
// pointing at the same file. NTFS supports hard links, so this guard matters on
// Windows too — but os.FileInfo.Sys() there is a *syscall.Win32FileAttributeData,
// which carries no link count. The count only comes from
// GetFileInformationByHandle, so the file must actually be opened.
//
// The handle is opened with zero desired access: enough to query metadata,
// never enough to read or write the contents. FILE_SHARE_* keeps the open from
// disturbing anything else holding the file.
//
// ⚠ Compile-verified only. This has never been executed on Windows — there is no
// Windows machine in the build environment. It fails closed (an error refuses the
// patch), so the worst case is a false refusal, never a silent overwrite of a
// hard-linked file outside the checkout.
func hasMultipleHardLinks(path string, _ os.FileInfo) (bool, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, fmt.Errorf("cannot determine hard-link count: %w", err)
	}
	h, err := syscall.CreateFile(
		p,
		0, // query metadata only: no read, no write
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return false, fmt.Errorf("cannot determine hard-link count: %w", err)
	}
	defer syscall.CloseHandle(h)

	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return false, fmt.Errorf("cannot determine hard-link count: %w", err)
	}
	return info.NumberOfLinks > 1, nil
}
