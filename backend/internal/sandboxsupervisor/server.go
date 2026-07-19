package sandboxsupervisor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	DefaultSocketPath   = "/run/allcallall/supervisor.sock"
	DefaultStdinBudget  = 8 << 20
	DefaultStdoutBudget = 8 << 20
	DefaultStderrBudget = 64 << 10
	DefaultTermGrace    = 500 * time.Millisecond
	DefaultWriteTimeout = 5 * time.Second
	maxUnixSocketBytes  = 100
)

var (
	ErrInvalidSocketPath = errors.New("invalid supervisor socket path")
	ErrSessionRejected   = errors.New("supervisor session rejected")
	errMissingNewline    = errors.New("child stdout line is not newline terminated")
)

// Server executes exactly one child process for exactly one Unix socket
// connection. Limits are configurable to keep fault-path tests fast; zero
// values receive the production defaults below.
type Server struct {
	StdoutBudget int64
	StdinBudget  int64
	StderrBudget int64
	TermGrace    time.Duration
	WriteTimeout time.Duration
}

type eventKind int

const (
	eventDisconnected eventKind = iota
	eventCanceled
	eventProtocolViolation
	eventInputLimit
	eventInvalidOutput
	eventOutputLimit
	eventStderrLimit
	eventProcessIO
)

type sessionEvent struct {
	kind eventKind
}

type childInput struct {
	mu     sync.Mutex
	writer io.WriteCloser
	closed bool
}

type frameWriter struct {
	mu      sync.Mutex
	conn    net.Conn
	timeout time.Duration
}

func NewServer() *Server {
	return &Server{
		StdoutBudget: DefaultStdoutBudget,
		StdinBudget:  DefaultStdinBudget,
		StderrBudget: DefaultStderrBudget,
		TermGrace:    DefaultTermGrace,
		WriteTimeout: DefaultWriteTimeout,
	}
}

// Serve creates a mode-0600 UDS, accepts one connection, closes the listener,
// and runs that connection to completion.
func (server *Server) Serve(ctx context.Context, socketPath string) error {
	if err := validateSocketPath(socketPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return ErrInvalidSocketPath
	}
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return ErrInvalidSocketPath
		}
		if err := os.Remove(socketPath); err != nil {
			return ErrInvalidSocketPath
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrInvalidSocketPath
	}

	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		return errors.New("supervisor socket could not be created")
	}
	listener.SetUnlinkOnClose(true)
	defer listener.Close()
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return errors.New("supervisor socket permissions could not be set")
	}
	stopClose := context.AfterFunc(ctx, func() {
		_ = listener.Close()
	})
	defer stopClose()
	connection, err := listener.AcceptUnix()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("supervisor connection could not be accepted")
	}
	// There must never be a second queued session in the same sandbox.
	_ = listener.Close()
	return server.HandleConn(ctx, connection)
}

// HandleConn implements the framed supervisor protocol for one owned
// connection. It always closes the connection before returning.
func (server *Server) HandleConn(ctx context.Context, connection net.Conn) error {
	defer connection.Close()
	limits := server.withDefaults()
	writer := &frameWriter{conn: connection, timeout: limits.WriteTimeout}

	startFrame, err := ReadFrame(connection)
	if err != nil || startFrame.Kind != KindStart {
		_ = writer.writeError("invalid_start", "start request rejected")
		return ErrSessionRejected
	}
	request, err := DecodeStartRequest(startFrame.Payload)
	if err != nil {
		_ = writer.writeError("invalid_start", "start request rejected")
		return ErrSessionRejected
	}
	if err := enableSubreaper(); err != nil {
		_ = writer.writeError("process_setup_failed", "process could not be prepared")
		return ErrSessionRejected
	}

	command, stdin, stdout, stderr, err := prepareCommand(request)
	if err != nil {
		_ = writer.writeError("process_setup_failed", "process could not be prepared")
		return ErrSessionRejected
	}
	if err := command.Start(); err != nil {
		_ = writer.writeError("process_start_failed", "process could not be started")
		return ErrSessionRejected
	}
	input := &childInput{writer: stdin}
	if err := writer.write(KindReady, nil); err != nil {
		cleanupUnattachedProcess(command, input, stdout, stderr, limits.TermGrace)
		return ErrSessionRejected
	}

	events := make(chan sessionEvent, 8)
	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})
	processDone := make(chan struct{})
	waitResult := make(chan error, 1)
	go readClientFrames(connection, input, limits.StdinBudget, events)
	go func() {
		defer close(stdoutDone)
		streamStdout(stdout, writer, limits.StdoutBudget, events)
	}()
	go func() {
		defer close(stderrDone)
		consumeStderr(stderr, limits.StderrBudget, events)
	}()
	go func() {
		// StdoutPipe and StderrPipe require all reads to finish before Wait.
		// Otherwise Wait may close a pipe before an unterminated final frame is validated.
		<-stdoutDone
		<-stderrDone
		waitResult <- command.Wait()
		close(processDone)
	}()

	timer := time.NewTimer(time.Duration(request.TimeoutMS) * time.Millisecond)
	defer timer.Stop()
	clientAvailable := true
	shutdownStarted := false
	terminalErrorSent := false
	processExited := false
	beginShutdown := func(code, message string, sendError bool) {
		if shutdownStarted {
			return
		}
		shutdownStarted = true
		if sendError && clientAvailable {
			if err := writer.writeError(code, message); err != nil {
				clientAvailable = false
			} else {
				terminalErrorSent = true
			}
		}
		input.close()
		if processExited {
			return
		}
		signalProcessGroup(command.Process.Pid, syscall.SIGTERM)
		go func() {
			killTimer := time.NewTimer(limits.TermGrace)
			defer killTimer.Stop()
			select {
			case <-processDone:
			case <-killTimer.C:
				signalProcessGroup(command.Process.Pid, syscall.SIGKILL)
			}
		}()
	}
	handleEvent := func(event sessionEvent) {
		switch event.kind {
		case eventDisconnected:
			clientAvailable = false
			beginShutdown("", "", false)
		case eventCanceled:
			beginShutdown("canceled", "process canceled", true)
		case eventProtocolViolation:
			beginShutdown("protocol_error", "protocol violation", true)
		case eventInputLimit:
			beginShutdown("input_limit_exceeded", "process input limit exceeded", true)
		case eventInvalidOutput:
			beginShutdown("invalid_output", "process output rejected", true)
		case eventOutputLimit:
			beginShutdown("output_limit_exceeded", "process output limit exceeded", true)
		case eventStderrLimit:
			beginShutdown("stderr_limit_exceeded", "process diagnostic limit exceeded", true)
		case eventProcessIO:
			clientAvailable = false
			beginShutdown("", "", false)
		}
	}

	var waitErr error
waitLoop:
	for {
		select {
		case waitErr = <-waitResult:
			processExited = true
			break waitLoop
		case event := <-events:
			handleEvent(event)
		case <-timer.C:
			beginShutdown("timeout", "process timed out", true)
		case <-ctx.Done():
			clientAvailable = false
			beginShutdown("", "", false)
		}
	}
	input.close()
	<-stdoutDone
	<-stderrDone
	for {
		select {
		case event := <-events:
			handleEvent(event)
		default:
			goto eventsDrained
		}
	}

eventsDrained:
	reapAdoptedChildren()
	if clientAvailable && !terminalErrorSent {
		payload, _ := json.Marshal(ExitPayload{ExitCode: processExitCode(waitErr)})
		if err := writer.write(KindExit, payload); err != nil {
			return ErrSessionRejected
		}
	}
	return nil
}

func (server *Server) withDefaults() Server {
	limits := *server
	if limits.StdoutBudget <= 0 {
		limits.StdoutBudget = DefaultStdoutBudget
	}
	if limits.StdinBudget <= 0 {
		limits.StdinBudget = DefaultStdinBudget
	}
	if limits.StderrBudget <= 0 {
		limits.StderrBudget = DefaultStderrBudget
	}
	if limits.TermGrace <= 0 {
		limits.TermGrace = DefaultTermGrace
	}
	if limits.WriteTimeout <= 0 {
		limits.WriteTimeout = DefaultWriteTimeout
	}
	return limits
}

func prepareCommand(request StartRequest) (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	effectiveEnvironment := safeBaseEnvironment()
	for key, value := range request.Env {
		effectiveEnvironment[key] = value
	}
	commandPath, err := resolveCommand(request.Command, effectiveEnvironment)
	if err != nil {
		return nil, nil, nil, nil, ErrSessionRejected
	}
	command := exec.Command(commandPath, request.Args...)
	command.Args[0] = request.Command
	command.Env = environmentList(effectiveEnvironment)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, ErrSessionRejected
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, nil, nil, ErrSessionRejected
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, nil, nil, nil, ErrSessionRejected
	}
	return command, stdin, stdout, stderr, nil
}

func safeBaseEnvironment() map[string]string {
	environment := make(map[string]string, 5)
	for _, key := range []string{"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL"} {
		if value, ok := os.LookupEnv(key); ok {
			environment[key] = value
		}
	}
	return environment
}

func resolveCommand(command string, environment map[string]string) (string, error) {
	if strings.ContainsRune(command, filepath.Separator) {
		return command, nil
	}
	pathValue, supplied := environment["PATH"]
	if !supplied {
		pathValue = os.Getenv("PATH")
	}
	for _, directory := range filepath.SplitList(pathValue) {
		if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
			continue
		}
		candidate := filepath.Join(directory, command)
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", ErrSessionRejected
}

func readClientFrames(connection net.Conn, input *childInput, budget int64, events chan<- sessionEvent) {
	var total int64
	for {
		frame, err := ReadFrame(connection)
		if err != nil {
			events <- sessionEvent{kind: eventDisconnected}
			return
		}
		switch frame.Kind {
		case KindStdin:
			if ValidateJSONRPCObject(frame.Payload) != nil {
				events <- sessionEvent{kind: eventProtocolViolation}
				return
			}
			total += int64(len(frame.Payload))
			if total > budget {
				events <- sessionEvent{kind: eventInputLimit}
				return
			}
			if input.writeRPC(frame.Payload) != nil {
				events <- sessionEvent{kind: eventProtocolViolation}
				return
			}
		case KindCloseStdin:
			if len(frame.Payload) != 0 {
				events <- sessionEvent{kind: eventProtocolViolation}
				return
			}
			input.close()
		case KindCancel:
			if len(frame.Payload) != 0 {
				events <- sessionEvent{kind: eventProtocolViolation}
				return
			}
			events <- sessionEvent{kind: eventCanceled}
			return
		default:
			events <- sessionEvent{kind: eventProtocolViolation}
			return
		}
	}
}

func streamStdout(stdout io.Reader, writer *frameWriter, budget int64, events chan<- sessionEvent) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), MaxFrameSize+2)
	scanner.Split(scanNewlineTerminated)
	var total int64
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) > MaxFrameSize || ValidateJSONRPCObject(line) != nil {
			events <- sessionEvent{kind: eventInvalidOutput}
			return
		}
		total += int64(len(line))
		if total > budget {
			events <- sessionEvent{kind: eventOutputLimit}
			return
		}
		if err := writer.write(KindStdout, line); err != nil {
			events <- sessionEvent{kind: eventProcessIO}
			return
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
		events <- sessionEvent{kind: eventInvalidOutput}
	}
}

func scanNewlineTerminated(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if index := bytes.IndexByte(data, '\n'); index >= 0 {
		return index + 1, data[:index], nil
	}
	if atEOF && len(data) > 0 {
		return 0, nil, errMissingNewline
	}
	return 0, nil, nil
}

func consumeStderr(stderr io.Reader, budget int64, events chan<- sessionEvent) {
	buffer := make([]byte, 32<<10)
	var total int64
	reported := false
	for {
		count, err := stderr.Read(buffer)
		total += int64(count)
		if total > budget && !reported {
			reported = true
			events <- sessionEvent{kind: eventStderrLimit}
		}
		if err != nil {
			return
		}
	}
}

func cleanupUnattachedProcess(command *exec.Cmd, input *childInput, stdout, stderr io.Reader, grace time.Duration) {
	input.close()
	go io.Copy(io.Discard, stdout)
	go io.Copy(io.Discard, stderr)
	signalProcessGroup(command.Process.Pid, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_ = command.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(grace):
		signalProcessGroup(command.Process.Pid, syscall.SIGKILL)
		<-done
	}
	reapAdoptedChildren()
}

func (input *childInput) writeRPC(payload []byte) error {
	input.mu.Lock()
	if input.closed {
		input.mu.Unlock()
		return io.ErrClosedPipe
	}
	writer := input.writer
	input.mu.Unlock()
	line := make([]byte, len(payload)+1)
	copy(line, payload)
	line[len(payload)] = '\n'
	return writeAll(writer, line)
}

func (input *childInput) close() {
	input.mu.Lock()
	defer input.mu.Unlock()
	if input.closed {
		return
	}
	input.closed = true
	_ = input.writer.Close()
}

func (writer *frameWriter) write(kind byte, payload []byte) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if err := writer.conn.SetWriteDeadline(time.Now().Add(writer.timeout)); err != nil {
		return err
	}
	return WriteFrame(writer.conn, kind, payload)
}

func (writer *frameWriter) writeError(code, message string) error {
	payload, _ := json.Marshal(ErrorPayload{Code: code, Message: message})
	return writer.write(KindError, payload)
}

func signalProcessGroup(pid int, signal syscall.Signal) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, signal)
}

func processExitCode(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	var exitError *exec.ExitError
	if !errors.As(waitErr, &exitError) {
		return -1
	}
	status, ok := exitError.Sys().(syscall.WaitStatus)
	if !ok {
		return exitError.ExitCode()
	}
	if status.Signaled() {
		return 128 + int(status.Signal())
	}
	return status.ExitStatus()
}

func validateSocketPath(socketPath string) error {
	if !filepath.IsAbs(socketPath) || filepath.Clean(socketPath) != socketPath || len(socketPath) > maxUnixSocketBytes || strings.ContainsRune(socketPath, '\x00') {
		return ErrInvalidSocketPath
	}
	return nil
}
