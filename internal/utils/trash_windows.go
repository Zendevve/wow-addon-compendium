//go:build windows

package utils

import (
	"syscall"
	"unsafe"
)

var (
	shell32   = syscall.NewLazyDLL("shell32.dll")
	shFileOpW = shell32.NewProc("SHFileOperationW")
)

// SHFILEOPSTRUCT as consumed by SHFileOperationW. Field types match
// the C declaration exactly (UINT/FILEOP_FLAGS are 32-bit, BOOL is
// 32-bit) so the struct is layout-compatible on 64-bit Windows.
type shfileop struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint32
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

const (
	fofDelete    = 3
	fofAllowUndo = 0x0040
	fofNoConfirm = 0x0010
	fofSilent    = 0x0004
	fofNoErrorUI = 0x0400
)

// NativeTrashProbe exposes nativeTrash for diagnostics/tests.
func NativeTrashProbe(path string) error {
	return nativeTrash(path)
}

// nativeTrash sends a path to the Windows Recycle Bin using
// SHFileOperation with FO_DELETE|FOF_ALLOWUNDO. This is the same
// operation the Explorer "Delete" command performs.
func nativeTrash(path string) error {
	// pFrom must be a double-null-terminated list of paths.
	from, err := syscall.UTF16PtrFromString(path + "\x00")
	if err != nil {
		return err
	}
	op := shfileop{
		wFunc:  fofDelete,
		pFrom:  from,
		fFlags: fofAllowUndo | fofNoConfirm | fofSilent | fofNoErrorUI,
	}
	r1, _, callErr := shFileOpW.Call(uintptr(unsafe.Pointer(&op)), 0)
	if r1 != 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return callErr
		}
		return syscall.Errno(r1)
	}
	return nil
}
