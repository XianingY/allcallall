//go:build !linux

package sandboxsupervisor

func enableSubreaper() error {
	return nil
}

func reapAdoptedChildren() {}
