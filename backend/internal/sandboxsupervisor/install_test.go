package sandboxsupervisor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallFromAtomicallyReplacesTargetWithExecutable(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	target := filepath.Join(directory, "supervisor")
	if err := os.WriteFile(source, []byte("new-static-binary"), 0o700); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte("old-binary"), 0o600); err != nil {
		t.Fatalf("write old target: %v", err)
	}
	if err := installFrom(source, target); err != nil {
		t.Fatalf("installFrom: %v", err)
	}
	payload, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if string(payload) != "new-static-binary" {
		t.Fatalf("installed payload = %q", payload)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat installed binary: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o555 {
		t.Fatalf("installed permissions = %o, want 555", got)
	}
	matches, err := filepath.Glob(filepath.Join(directory, ".sandbox-supervisor-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary install files remain: %v, err=%v", matches, err)
	}
}

func TestInstallFromRejectsNonAbsoluteAndUncleanTargets(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, []byte("binary"), 0o700); err != nil {
		t.Fatalf("write source: %v", err)
	}
	uncleanTarget := filepath.Join(t.TempDir(), "nested") + string(filepath.Separator) + ".." + string(filepath.Separator) + "target"
	for _, target := range []string{"relative", uncleanTarget} {
		if err := installFrom(source, target); err == nil {
			t.Fatalf("expected target %q to be rejected", target)
		}
	}
}
