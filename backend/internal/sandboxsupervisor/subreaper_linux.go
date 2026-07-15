//go:build linux

package sandboxsupervisor

import (
	"time"

	"golang.org/x/sys/unix"
)

func enableSubreaper() error {
	return unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0)
}

func reapAdoptedChildren() {
	deadline := time.Now().Add(2 * DefaultTermGrace)
	for {
		var status unix.WaitStatus
		pid, err := unix.Wait4(-1, &status, unix.WNOHANG, nil)
		if err == unix.ECHILD {
			return
		}
		if err != nil {
			return
		}
		if pid > 0 {
			continue
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
