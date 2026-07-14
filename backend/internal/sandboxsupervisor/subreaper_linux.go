//go:build linux

package sandboxsupervisor

import "golang.org/x/sys/unix"

func enableSubreaper() error {
	return unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0)
}

func reapAdoptedChildren() {
	for {
		var status unix.WaitStatus
		pid, err := unix.Wait4(-1, &status, unix.WNOHANG, nil)
		if err != nil || pid <= 0 {
			return
		}
	}
}
