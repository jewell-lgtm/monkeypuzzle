package adapters

import (
	"os"
	"syscall"
)

// ProcessAlive reports whether a process with the given pid exists. Signal 0
// performs the existence check without delivering anything; EPERM still means
// the process exists (it just isn't ours).
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}
