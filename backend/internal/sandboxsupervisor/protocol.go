package sandboxsupervisor

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	ProtocolVersion = 1
	MaxFrameSize    = 1 << 20

	KindStart      byte = 0x01
	KindReady      byte = 0x02
	KindStdin      byte = 0x03
	KindStdout     byte = 0x04
	KindError      byte = 0x05
	KindExit       byte = 0x06
	KindCloseStdin byte = 0x07
	KindCancel     byte = 0x08
)

const (
	maxCommandBytes          = 4 << 10
	maxArgumentCount         = 256
	maxArgumentBytes         = 64 << 10
	maxArgumentsBytes        = 256 << 10
	maxEnvironmentCount      = 128
	maxEnvironmentValueBytes = 64 << 10
	maxEnvironmentBytes      = 256 << 10
	maxTimeoutMilliseconds   = 30_000
)

var (
	ErrFrameTooLarge  = errors.New("supervisor frame exceeds maximum size")
	ErrInvalidFrame   = errors.New("invalid supervisor frame")
	ErrInvalidStart   = errors.New("invalid supervisor start request")
	ErrInvalidJSONRPC = errors.New("invalid JSON-RPC object")
)

// Frame is one length-delimited supervisor protocol message.
type Frame struct {
	Kind    byte
	Payload []byte
}

// StartRequest describes one structured child process invocation.
type StartRequest struct {
	Version   int               `json:"version"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	TimeoutMS int64             `json:"timeout_ms"`
}

// ErrorPayload is deliberately generic so child stderr and environment values
// can never cross the supervisor protocol boundary.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ExitPayload reports the conventional process exit code. Signal exits use
// 128+signal, matching common Unix process conventions.
type ExitPayload struct {
	ExitCode int `json:"exit_code"`
}

func ReadFrame(reader io.Reader) (Frame, error) {
	var header [5]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return Frame{}, err
	}
	length := binary.BigEndian.Uint32(header[1:])
	if length > MaxFrameSize {
		return Frame{}, ErrFrameTooLarge
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return Frame{}, fmt.Errorf("%w: incomplete payload", ErrInvalidFrame)
	}
	return Frame{Kind: header[0], Payload: payload}, nil
}

func WriteFrame(writer io.Writer, kind byte, payload []byte) error {
	if len(payload) > MaxFrameSize {
		return ErrFrameTooLarge
	}
	var header [5]byte
	header[0] = kind
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if err := writeAll(writer, header[:]); err != nil {
		return err
	}
	return writeAll(writer, payload)
}

func DecodeStartRequest(payload []byte) (StartRequest, error) {
	if len(payload) == 0 || len(payload) > MaxFrameSize || !utf8.Valid(payload) {
		return StartRequest{}, ErrInvalidStart
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var request StartRequest
	if err := decoder.Decode(&request); err != nil {
		return StartRequest{}, ErrInvalidStart
	}
	if err := requireJSONEOF(decoder); err != nil {
		return StartRequest{}, ErrInvalidStart
	}
	if err := ValidateStartRequest(request); err != nil {
		return StartRequest{}, err
	}
	return request, nil
}

func ValidateStartRequest(request StartRequest) error {
	if request.Version != ProtocolVersion || !validCommand(request.Command) {
		return ErrInvalidStart
	}
	if len(request.Args) > maxArgumentCount {
		return ErrInvalidStart
	}
	argumentBytes := 0
	for _, argument := range request.Args {
		if !validString(argument, maxArgumentBytes) {
			return ErrInvalidStart
		}
		argumentBytes += len(argument)
		if argumentBytes > maxArgumentsBytes {
			return ErrInvalidStart
		}
	}
	if len(request.Env) > maxEnvironmentCount {
		return ErrInvalidStart
	}
	environmentBytes := 0
	for key, value := range request.Env {
		if !validEnvironmentName(key) || !validString(value, maxEnvironmentValueBytes) {
			return ErrInvalidStart
		}
		environmentBytes += len(key) + len(value)
		if environmentBytes > maxEnvironmentBytes {
			return ErrInvalidStart
		}
	}
	if request.TimeoutMS <= 0 || request.TimeoutMS > maxTimeoutMilliseconds {
		return ErrInvalidStart
	}
	return nil
}

func ValidateJSONRPCObject(payload []byte) error {
	if len(payload) == 0 || len(payload) > MaxFrameSize || !utf8.Valid(payload) {
		return ErrInvalidJSONRPC
	}
	if bytes.IndexByte(payload, '\n') >= 0 || bytes.IndexByte(payload, '\r') >= 0 {
		return ErrInvalidJSONRPC
	}
	var object map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&object); err != nil || object == nil {
		return ErrInvalidJSONRPC
	}
	if err := requireJSONEOF(decoder); err != nil {
		return ErrInvalidJSONRPC
	}
	var version string
	if raw, ok := object["jsonrpc"]; !ok || json.Unmarshal(raw, &version) != nil || version != "2.0" {
		return ErrInvalidJSONRPC
	}
	return nil
}

func environmentList(environmentMap map[string]string) []string {
	keys := make([]string, 0, len(environmentMap))
	for key := range environmentMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+environmentMap[key])
	}
	return environment
}

func validCommand(command string) bool {
	if !validString(command, maxCommandBytes) || command == "." || command == ".." {
		return false
	}
	if strings.ContainsAny(command, "\r\n") || filepath.Clean(command) != command {
		return false
	}
	for _, component := range strings.Split(filepath.ToSlash(command), "/") {
		if component == ".." {
			return false
		}
	}
	return true
}

func validString(value string, maxBytes int) bool {
	return len(value) <= maxBytes && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func validEnvironmentName(name string) bool {
	if name == "" || len(name) > 256 {
		return false
	}
	for index, character := range name {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || character == '_' {
			continue
		}
		if index > 0 && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return true
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ErrInvalidFrame
	}
	return nil
}

func writeAll(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		payload = payload[written:]
	}
	return nil
}
