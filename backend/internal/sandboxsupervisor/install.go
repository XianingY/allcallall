package sandboxsupervisor

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const selfExecutablePath = "/proc/self/exe"

// Install atomically copies the currently running supervisor binary into the
// requested sandbox path with read-and-execute-only permissions.
func Install(target string) error {
	return installFrom(selfExecutablePath, target)
}

func installFrom(source, target string) error {
	if !filepath.IsAbs(target) || filepath.Clean(target) != target || strings.ContainsRune(target, '\x00') {
		return errors.New("supervisor install target must be a clean absolute path")
	}
	parent := filepath.Dir(target)
	sourceFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open supervisor source: %w", err)
	}
	defer sourceFile.Close()

	temporary, err := os.CreateTemp(parent, ".sandbox-supervisor-*")
	if err != nil {
		return fmt.Errorf("create temporary supervisor: %w", err)
	}
	temporaryName := temporary.Name()
	keepTemporary := true
	defer func() {
		_ = temporary.Close()
		if keepTemporary {
			_ = os.Remove(temporaryName)
		}
	}()

	if _, err := io.Copy(temporary, sourceFile); err != nil {
		return fmt.Errorf("copy supervisor: %w", err)
	}
	if err := temporary.Chmod(0o555); err != nil {
		return fmt.Errorf("set supervisor permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync supervisor: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close supervisor: %w", err)
	}
	if err := os.Rename(temporaryName, target); err != nil {
		return fmt.Errorf("activate supervisor: %w", err)
	}
	keepTemporary = false
	directory, err := os.Open(parent)
	if err != nil {
		return fmt.Errorf("open supervisor directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync supervisor directory: %w", err)
	}
	return nil
}
