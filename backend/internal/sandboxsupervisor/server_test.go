package sandboxsupervisor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const helperProcessEnvironment = "ALLCALLALL_SUPERVISOR_HELPER"

func TestSupervisorHelperProcess(t *testing.T) {
	if os.Getenv(helperProcessEnvironment) != "1" {
		return
	}
	mode := os.Getenv("HELPER_MODE")
	switch mode {
	case "echo":
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 64<<10), MaxFrameSize+1)
		for scanner.Scan() {
			_, _ = fmt.Fprintln(os.Stdout, scanner.Text())
		}
		os.Exit(0)
	case "sleep":
		writeHelperPID()
		signal.Ignore(syscall.SIGTERM)
		for {
			time.Sleep(time.Hour)
		}
	case "invalid-output":
		_, _ = fmt.Fprintln(os.Stdout, os.Getenv("SECRET_VALUE"))
		signal.Ignore(syscall.SIGTERM)
		for {
			time.Sleep(time.Hour)
		}
	case "missing-newline":
		_, _ = fmt.Fprint(os.Stdout, `{"jsonrpc":"2.0","id":1}`)
		os.Exit(0)
	case "stderr":
		amount, _ := strconv.Atoi(os.Getenv("HELPER_BYTES"))
		_, _ = fmt.Fprint(os.Stderr, strings.Repeat(os.Getenv("SECRET_VALUE"), amount))
		signal.Ignore(syscall.SIGTERM)
		for {
			time.Sleep(time.Hour)
		}
	case "stdout-budget":
		line := `{"jsonrpc":"2.0","id":1,"result":"xxxxxxxxxxxxxxxx"}`
		for {
			_, _ = fmt.Fprintln(os.Stdout, line)
		}
	case "environment":
		_, present := os.LookupEnv("UNSAFE_PARENT_SECRET")
		payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": present})
		_, _ = fmt.Fprintln(os.Stdout, string(payload))
		os.Exit(0)
	case "spawn":
		child := exec.Command(os.Args[0], "-test.run=^TestSupervisorHelperProcess$")
		child.Env = append(os.Environ(), helperProcessEnvironment+"=1", "HELPER_MODE=sleep", "PID_FILE="+os.Getenv("CHILD_PID_FILE"))
		if err := child.Start(); err != nil {
			os.Exit(9)
		}
		payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": child.Process.Pid})
		_, _ = fmt.Fprintln(os.Stdout, string(payload))
		signal.Ignore(syscall.SIGTERM)
		for {
			time.Sleep(time.Hour)
		}
	default:
		os.Exit(8)
	}
}

func TestHandleConnBridgesJSONRPCWithoutNewlines(t *testing.T) {
	session := startHelperSession(t, NewServer(), "echo", 2_000, nil)
	request := []byte(`{"jsonrpc":"2.0","id":7,"method":"ping"}`)
	session.write(t, KindStdin, request)
	session.write(t, KindCloseStdin, nil)
	stdout := session.read(t)
	if stdout.Kind != KindStdout || !bytes.Equal(stdout.Payload, request) {
		t.Fatalf("unexpected stdout frame: kind=%x payload=%q", stdout.Kind, stdout.Payload)
	}
	exit := session.read(t)
	assertExitCode(t, exit, 0)
	session.wait(t, nil)
}

func TestHandleConnRejectsInvalidInputWithSingleTerminalError(t *testing.T) {
	session := startHelperSession(t, testServer(), "sleep", 2_000, nil)
	session.write(t, KindStdin, []byte(`[{"jsonrpc":"2.0"}]`))
	terminal := session.read(t)
	assertErrorCode(t, terminal, "protocol_error")
	session.assertEOF(t)
	session.wait(t, nil)
}

func TestHandleConnEnforcesCumulativeInputBudget(t *testing.T) {
	server := testServer()
	server.StdinBudget = 79
	session := startHelperSession(t, server, "sleep", 2_000, nil)
	request := []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	session.write(t, KindStdin, request)
	session.write(t, KindStdin, request)
	terminal := session.read(t)
	assertErrorCode(t, terminal, "input_limit_exceeded")
	session.assertEOF(t)
	session.wait(t, nil)
}

func TestHandleConnRejectsInvalidAndUnterminatedStdoutWithoutLeakingIt(t *testing.T) {
	for _, mode := range []string{"invalid-output", "missing-newline"} {
		t.Run(mode, func(t *testing.T) {
			secret := "secret-must-not-cross-boundary"
			session := startHelperSession(t, testServer(), mode, 2_000, map[string]string{"SECRET_VALUE": secret})
			terminal := session.read(t)
			assertErrorCode(t, terminal, "invalid_output")
			if bytes.Contains(terminal.Payload, []byte(secret)) {
				t.Fatalf("terminal error leaked child output: %s", terminal.Payload)
			}
			session.assertEOF(t)
			session.wait(t, nil)
		})
	}
}

func TestHandleConnEnforcesStdoutAndStderrBudgets(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		configure  func(*Server)
		requestEnv map[string]string
		code       string
	}{
		{
			name:      "stdout",
			mode:      "stdout-budget",
			configure: func(server *Server) { server.StdoutBudget = 100 },
			code:      "output_limit_exceeded",
		},
		{
			name:       "stderr",
			mode:       "stderr",
			configure:  func(server *Server) { server.StderrBudget = 32 },
			requestEnv: map[string]string{"SECRET_VALUE": "diagnostic-secret", "HELPER_BYTES": "100"},
			code:       "stderr_limit_exceeded",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := testServer()
			test.configure(server)
			session := startHelperSession(t, server, test.mode, 2_000, test.requestEnv)
			var terminal Frame
			for {
				terminal = session.read(t)
				if terminal.Kind != KindStdout {
					break
				}
			}
			assertErrorCode(t, terminal, test.code)
			if bytes.Contains(terminal.Payload, []byte("diagnostic-secret")) {
				t.Fatalf("terminal error leaked stderr: %s", terminal.Payload)
			}
			session.assertEOF(t)
			session.wait(t, nil)
		})
	}
}

func TestHandleConnTimeoutAndCancelAreSingleTerminalErrors(t *testing.T) {
	tests := []struct {
		name      string
		timeoutMS int64
		cancel    bool
		code      string
	}{
		{name: "timeout", timeoutMS: 30, code: "timeout"},
		{name: "cancel", timeoutMS: 2_000, cancel: true, code: "canceled"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := startHelperSession(t, testServer(), "sleep", test.timeoutMS, map[string]string{"SECRET_VALUE": "never-return-this"})
			if test.cancel {
				session.write(t, KindCancel, nil)
			}
			terminal := session.read(t)
			assertErrorCode(t, terminal, test.code)
			if bytes.Contains(terminal.Payload, []byte("never-return-this")) {
				t.Fatalf("terminal error leaked environment: %s", terminal.Payload)
			}
			session.assertEOF(t)
			session.wait(t, nil)
		})
	}
}

func TestHandleConnDisconnectTerminatesProcess(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	session := startHelperSession(t, testServer(), "sleep", 2_000, map[string]string{"PID_FILE": pidFile})
	pid := waitForPIDFile(t, pidFile)
	_ = session.connection.Close()
	session.wait(t, nil)
	waitForProcessExit(t, pid)
}

func TestHandleConnCancelTerminatesCompleteProcessGroup(t *testing.T) {
	childPIDFile := filepath.Join(t.TempDir(), "child-pid")
	session := startHelperSession(t, testServer(), "spawn", 2_000, map[string]string{"CHILD_PID_FILE": childPIDFile})
	spawnFrame := session.read(t)
	if spawnFrame.Kind != KindStdout {
		t.Fatalf("expected child PID stdout, got kind %x", spawnFrame.Kind)
	}
	childPID := waitForPIDFile(t, childPIDFile)
	session.write(t, KindCancel, nil)
	assertErrorCode(t, session.read(t), "canceled")
	session.assertEOF(t)
	session.wait(t, nil)
	waitForProcessExit(t, childPID)
}

func TestChildReceivesOnlySafeBaseAndRequestEnvironment(t *testing.T) {
	t.Setenv("UNSAFE_PARENT_SECRET", "must-not-be-inherited")
	session := startHelperSession(t, NewServer(), "environment", 2_000, nil)
	frame := session.read(t)
	if frame.Kind != KindStdout || bytes.Contains(frame.Payload, []byte("true")) {
		t.Fatalf("unsafe parent environment reached child: kind=%x payload=%s", frame.Kind, frame.Payload)
	}
	assertExitCode(t, session.read(t), 0)
	session.wait(t, nil)
}

func TestResolveCommandUsesEffectivePath(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "tool")
	if err := os.WriteFile(executable, []byte("binary"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	resolved, err := resolveCommand("tool", map[string]string{"PATH": directory})
	if err != nil || resolved != executable {
		t.Fatalf("resolveCommand = %q, %v; want %q", resolved, err, executable)
	}
	if _, err := resolveCommand("tool", map[string]string{"PATH": ".:" + directory}); err != nil {
		t.Fatalf("absolute entry after unsafe relative entry should resolve: %v", err)
	}
}

func TestServeCreatesMode0600SingleConnectionSocket(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "aca-supervisor-")
	if err != nil {
		t.Fatalf("create short socket directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	socketPath := filepath.Join(directory, "supervisor.sock")
	done := make(chan error, 1)
	go func() {
		done <- testServer().Serve(context.Background(), socketPath)
	}()
	waitForSocket(t, socketPath)
	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket permissions = %o, want 600", got)
	}
	connection, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial supervisor socket: %v", err)
	}
	request := helperStartRequest(t, "echo", 2_000, nil)
	writeStart(t, connection, request)
	if frame := readFrameWithDeadline(t, connection); frame.Kind != KindReady {
		t.Fatalf("expected READY, got kind %x", frame.Kind)
	}
	waitForListenerClose(t, socketPath)
	if second, err := net.DialTimeout("unix", socketPath, 20*time.Millisecond); err == nil {
		_ = second.Close()
		t.Fatal("second supervisor connection unexpectedly succeeded")
	}
	if err := WriteFrame(connection, KindCloseStdin, nil); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	assertExitCode(t, readFrameWithDeadline(t, connection), 0)
	_ = connection.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return")
	}
}

type helperSession struct {
	connection net.Conn
	done       <-chan error
}

func testServer() *Server {
	server := NewServer()
	server.TermGrace = 25 * time.Millisecond
	server.WriteTimeout = time.Second
	return server
}

func startHelperSession(t *testing.T, server *Server, mode string, timeoutMS int64, environment map[string]string) helperSession {
	t.Helper()
	serverConnection, clientConnection := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- server.HandleConn(context.Background(), serverConnection)
	}()
	writeStart(t, clientConnection, helperStartRequest(t, mode, timeoutMS, environment))
	ready := readFrameWithDeadline(t, clientConnection)
	if ready.Kind != KindReady || len(ready.Payload) != 0 {
		t.Fatalf("expected empty READY frame, got kind=%x payload=%q", ready.Kind, ready.Payload)
	}
	return helperSession{connection: clientConnection, done: done}
}

func helperStartRequest(t *testing.T, mode string, timeoutMS int64, environment map[string]string) StartRequest {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	childEnvironment := map[string]string{
		helperProcessEnvironment: "1",
		"HELPER_MODE":            mode,
	}
	for key, value := range environment {
		childEnvironment[key] = value
	}
	return StartRequest{
		Version:   ProtocolVersion,
		Command:   executable,
		Args:      []string{"-test.run=^TestSupervisorHelperProcess$"},
		Env:       childEnvironment,
		TimeoutMS: timeoutMS,
	}
}

func writeStart(t *testing.T, connection net.Conn, request StartRequest) {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal START: %v", err)
	}
	if err := WriteFrame(connection, KindStart, payload); err != nil {
		t.Fatalf("write START: %v", err)
	}
}

func (session helperSession) write(t *testing.T, kind byte, payload []byte) {
	t.Helper()
	if err := WriteFrame(session.connection, kind, payload); err != nil {
		t.Fatalf("write frame %x: %v", kind, err)
	}
}

func (session helperSession) read(t *testing.T) Frame {
	t.Helper()
	return readFrameWithDeadline(t, session.connection)
}

func (session helperSession) assertEOF(t *testing.T) {
	t.Helper()
	if err := session.connection.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return
	}
	if frame, err := ReadFrame(session.connection); err == nil {
		t.Fatalf("expected EOF after terminal ERROR, got frame %+v", frame)
	}
}

func (session helperSession) wait(t *testing.T, expected error) {
	t.Helper()
	select {
	case err := <-session.done:
		if !errors.Is(err, expected) {
			t.Fatalf("HandleConn error = %v, want %v", err, expected)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HandleConn did not return")
	}
}

func readFrameWithDeadline(t *testing.T, connection net.Conn) Frame {
	t.Helper()
	if err := connection.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	frame, err := ReadFrame(connection)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	return frame
}

func assertErrorCode(t *testing.T, frame Frame, code string) {
	t.Helper()
	if frame.Kind != KindError {
		t.Fatalf("expected ERROR, got kind=%x payload=%q", frame.Kind, frame.Payload)
	}
	var payload ErrorPayload
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		t.Fatalf("decode ERROR: %v", err)
	}
	if payload.Code != code || payload.Message == "" {
		t.Fatalf("ERROR payload = %+v, want code %q", payload, code)
	}
}

func assertExitCode(t *testing.T, frame Frame, code int) {
	t.Helper()
	if frame.Kind != KindExit {
		t.Fatalf("expected EXIT, got kind=%x payload=%q", frame.Kind, frame.Payload)
	}
	var payload ExitPayload
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		t.Fatalf("decode EXIT: %v", err)
	}
	if payload.ExitCode != code {
		t.Fatalf("exit code = %d, want %d", payload.ExitCode, code)
	}
}

func writeHelperPID() {
	if path := os.Getenv("PID_FILE"); path != "" {
		_ = os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600)
	}
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		payload, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(payload)))
			if parseErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("PID file %s was not written", path)
	return 0
}

func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d survived supervisor termination", pid)
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Mode()&os.ModeSocket != 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("socket %s was not created", path)
}

func waitForListenerClose(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("unix", path, 10*time.Millisecond)
		if err != nil {
			return
		}
		_ = connection.Close()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("listener %s remained available", path)
}
