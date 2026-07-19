package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/allcallall/backend/internal/sandboxsupervisor"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stderr))
}

func run(args []string, getenv func(string) string, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: sandbox-supervisor <install|serve>")
		return 2
	}
	switch args[0] {
	case "install":
		if len(args) != 2 {
			_, _ = fmt.Fprintln(stderr, "usage: sandbox-supervisor install <destination>")
			return 2
		}
		if err := sandboxsupervisor.Install(args[1]); err != nil {
			_, _ = fmt.Fprintln(stderr, "sandbox-supervisor: install failed")
			return 1
		}
		return 0
	case "serve":
		flags := flag.NewFlagSet("serve", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		defaultSocket := strings.TrimSpace(getenv("SANDBOX_SUPERVISOR_SOCKET"))
		if defaultSocket == "" {
			defaultSocket = sandboxsupervisor.DefaultSocketPath
		}
		socketPath := flags.String("socket", defaultSocket, "supervisor Unix socket path")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			_, _ = fmt.Fprintln(stderr, "usage: sandbox-supervisor serve [--socket <path>]")
			return 2
		}
		if err := sandboxsupervisor.NewServer().Serve(context.Background(), *socketPath); err != nil {
			_, _ = fmt.Fprintln(stderr, "sandbox-supervisor: serve failed")
			return 1
		}
		return 0
	default:
		_, _ = fmt.Fprintln(stderr, "usage: sandbox-supervisor <install|serve>")
		return 2
	}
}
