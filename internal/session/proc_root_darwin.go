//go:build darwin

package session

import (
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	procPIDVnodePathInfo = 9 // PROC_PIDVNODEPATHINFO
	// vnodeInfoSize is sizeof(struct vnode_info): a 136-byte vinfo_stat,
	// vi_type, vi_pad and a 64-bit fsid_t.
	vnodeInfoSize = 152
	maxPathLen    = 1024 // MAXPATHLEN
)

// procVnodePathInfo mirrors struct proc_vnodepathinfo from
// <sys/proc_info.h>: the current and root directories, each a vnode_info
// followed by its path.
type procVnodePathInfo struct {
	cdirInfo [vnodeInfoSize]byte
	cdirPath [maxPathLen]byte
	rdirInfo [vnodeInfoSize]byte
	rdirPath [maxPathLen]byte
}

// rootProcess returns the executable base name and working directory of pid.
// Either is empty when the process is gone or not readable by this user.
// The kernel reports the directory with symbolic links resolved.
func rootProcess(pid int) (exe, cwd string) {
	if path := procPath(pid); path != "" {
		exe = filepath.Base(path)
	}
	var info procVnodePathInfo
	n, _, errno := unix.Syscall6(unix.SYS_PROC_INFO, procInfoCallPIDInfo, uintptr(pid), procPIDVnodePathInfo, 0, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info)) //nolint:staticcheck // see processes
	if errno == 0 && n == unsafe.Sizeof(info) {
		cwd = cString(info.cdirPath[:])
	}
	return exe, cwd
}
