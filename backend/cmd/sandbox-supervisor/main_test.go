package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsInvalidCommandsAndFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code int
	}{
		{name: "missing command", code: 2},
		{name: "unknown command", args: []string{"unknown"}, code: 2},
		{name: "install missing destination", args: []string{"install"}, code: 2},
		{name: "serve missing socket value", args: []string{"serve", "--socket"}, code: 2},
		{name: "serve extra argument", args: []string{"serve", "extra"}, code: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := run(test.args, func(string) string { return "" }, &stderr); code != test.code {
				t.Fatalf("run code = %d, want %d; stderr=%q", code, test.code, stderr.String())
			}
		})
	}
}

func TestRunSupportsServeSocketFlagAndEnvironmentDefault(t *testing.T) {
	for _, test := range []struct {
		name   string
		args   []string
		getenv func(string) string
	}{
		{
			name:   "flag",
			args:   []string{"serve", "--socket", "relative-invalid"},
			getenv: func(string) string { return "" },
		},
		{
			name: "environment",
			args: []string{"serve"},
			getenv: func(name string) string {
				if name == "SANDBOX_SUPERVISOR_SOCKET" {
					return "relative-invalid"
				}
				return ""
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := run(test.args, test.getenv, &stderr); code != 1 {
				t.Fatalf("run code = %d, want 1; stderr=%q", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "serve failed") || strings.Contains(stderr.String(), "relative-invalid") {
				t.Fatalf("serve error is not generic: %q", stderr.String())
			}
		})
	}
}

func TestRunInstallFailureDoesNotEchoDestination(t *testing.T) {
	var stderr bytes.Buffer
	destination := "secret-relative-destination"
	if code := run([]string{"install", destination}, func(string) string { return "" }, &stderr); code != 1 {
		t.Fatalf("run code = %d, want 1", code)
	}
	if strings.Contains(stderr.String(), destination) || !strings.Contains(stderr.String(), "install failed") {
		t.Fatalf("install error is not generic: %q", stderr.String())
	}
}
