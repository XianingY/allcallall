package diagnostics

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func freeListenAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port failed: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release port failed: %v", err)
	}
	return addr
}

func TestServeExposesPprofIndex(t *testing.T) {
	logger := zerolog.Nop()
	addr := freeListenAddr(t)
	server := Serve(addr, logger)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		Shutdown(ctx, server, logger)
	})

	var resp *http.Response
	var err error
	for i := 0; i < 20; i++ {
		resp, err = http.Get("http://" + addr + "/debug/pprof/")
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /debug/pprof/ failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /debug/pprof/ status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	if !strings.Contains(string(body), "Types of profiles available") {
		t.Fatalf("pprof index body did not contain profile listing: %s", string(body)[:min(200, len(body))])
	}
}

func TestServeSupportsCPUProfileSampling(t *testing.T) {
	logger := zerolog.Nop()
	addr := freeListenAddr(t)
	server := Serve(addr, logger)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		Shutdown(ctx, server, logger)
	})

	// The listener starts asynchronously; on slow CI runners the first
	// request can race Serve before the socket accepts connections.
	var resp *http.Response
	var err error
	for i := 0; i < 40; i++ {
		resp, err = http.Get("http://" + addr + "/debug/pprof/profile?seconds=1")
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET profile failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET profile status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read profile failed: %v", err)
	}
	// protobuf profiles start with a compressed-1 wire-format byte; any
	// non-empty payload means the sampler actually ran.
	if len(body) == 0 {
		t.Fatalf("profile body is empty")
	}
}

func TestServeOnlyServesPprofPaths(t *testing.T) {
	// The listener mounts a private mux: anything outside /debug/pprof/*
	// must 404. (Importing net/http/pprof also registers handlers on
	// http.DefaultServeMux via its init(); that is inert as long as no
	// component serves the default mux, and none does.)
	logger := zerolog.Nop()
	addr := freeListenAddr(t)
	server := Serve(addr, logger)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		Shutdown(ctx, server, logger)
	})

	var resp *http.Response
	var err error
	for i := 0; i < 20; i++ {
		resp, err = http.Get("http://" + addr + "/")
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET / status = %d, want 404 (private mux must not serve anything else)", resp.StatusCode)
	}
}

func TestShutdownIsNilSafe(t *testing.T) {
	Shutdown(context.Background(), nil, zerolog.Nop())
}
