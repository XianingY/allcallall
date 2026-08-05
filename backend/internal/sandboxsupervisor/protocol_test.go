package sandboxsupervisor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestFrameRoundTripUsesBigEndianHeader(t *testing.T) {
	payload := []byte(`{"version":1}`)
	var encoded bytes.Buffer
	if err := WriteFrame(&encoded, KindStart, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if encoded.Bytes()[0] != KindStart {
		t.Fatalf("unexpected kind byte: %x", encoded.Bytes()[0])
	}
	if got := binary.BigEndian.Uint32(encoded.Bytes()[1:5]); got != uint32(len(payload)) {
		t.Fatalf("unexpected encoded length: %d", got)
	}
	frame, err := ReadFrame(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if frame.Kind != KindStart || !bytes.Equal(frame.Payload, payload) {
		t.Fatalf("unexpected frame: %+v", frame)
	}
}

func TestFrameRejectsOversizedAndTruncatedPayloads(t *testing.T) {
	if err := WriteFrame(&bytes.Buffer{}, KindStdin, make([]byte, MaxFrameSize+1)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("expected write size rejection, got %v", err)
	}
	var oversized bytes.Buffer
	oversized.WriteByte(KindStdin)
	_ = binary.Write(&oversized, binary.BigEndian, uint32(MaxFrameSize+1))
	if _, err := ReadFrame(&oversized); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("expected read size rejection, got %v", err)
	}
	var truncated bytes.Buffer
	truncated.WriteByte(KindStdin)
	_ = binary.Write(&truncated, binary.BigEndian, uint32(10))
	truncated.WriteString("short")
	if _, err := ReadFrame(&truncated); !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("expected truncated payload rejection, got %v", err)
	}
}

func TestDecodeStartRequestAcceptsStructuredCommands(t *testing.T) {
	for _, command := range []string{"python", "bin/python", "/usr/local/bin/python"} {
		payload := []byte(`{"version":1,"command":"` + command + `","args":["-m","server"],"env":{"PATH":"/usr/bin","TOKEN":"redacted"},"timeout_ms":30000}`)
		request, err := DecodeStartRequest(payload)
		if err != nil {
			t.Fatalf("command %q rejected: %v", command, err)
		}
		if request.Command != command || request.TimeoutMS != 30_000 {
			t.Fatalf("unexpected request: %+v", request)
		}
	}
}

func TestDecodeStartRequestRejectsUnsafeOrUnboundedFields(t *testing.T) {
	tests := map[string]string{
		"unknown field":       `{"version":1,"command":"python","args":[],"env":{},"timeout_ms":1,"shell":true}`,
		"wrong version":       `{"version":2,"command":"python","args":[],"env":{},"timeout_ms":1}`,
		"zero timeout":        `{"version":1,"command":"python","args":[],"env":{},"timeout_ms":0}`,
		"excess timeout":      `{"version":1,"command":"python","args":[],"env":{},"timeout_ms":30001}`,
		"traversal":           `{"version":1,"command":"bin/../python","args":[],"env":{},"timeout_ms":1}`,
		"unclean path":        `{"version":1,"command":"./python","args":[],"env":{},"timeout_ms":1}`,
		"nul argument":        `{"version":1,"command":"python","args":["x\u0000y"],"env":{},"timeout_ms":1}`,
		"invalid env name":    `{"version":1,"command":"python","args":[],"env":{"A-B":"x"},"timeout_ms":1}`,
		"trailing JSON value": `{"version":1,"command":"python","args":[],"env":{},"timeout_ms":1} {}`,
	}
	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeStartRequest([]byte(payload)); !errors.Is(err, ErrInvalidStart) {
				t.Fatalf("expected invalid start, got %v", err)
			}
		})
	}

	request := StartRequest{Version: 1, Command: "python", TimeoutMS: 1}
	request.Args = make([]string, maxArgumentCount+1)
	if err := ValidateStartRequest(request); !errors.Is(err, ErrInvalidStart) {
		t.Fatalf("expected argument count rejection, got %v", err)
	}
	request.Args = nil
	request.Env = map[string]string{"VALUE": strings.Repeat("x", maxEnvironmentValueBytes+1)}
	if err := ValidateStartRequest(request); !errors.Is(err, ErrInvalidStart) {
		t.Fatalf("expected environment size rejection, got %v", err)
	}
}

func TestValidateJSONRPCObject(t *testing.T) {
	valid := [][]byte{
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`),
		[]byte(` {"jsonrpc":"2.0","result":{}} `),
	}
	for _, payload := range valid {
		if err := ValidateJSONRPCObject(payload); err != nil {
			t.Fatalf("valid JSON-RPC rejected: %q: %v", payload, err)
		}
	}
	invalid := [][]byte{
		nil,
		[]byte(`[]`),
		[]byte(`{"jsonrpc":"1.0","id":1}`),
		[]byte("{\"jsonrpc\":\"2.0\"}\n"),
		[]byte(`{"jsonrpc":"2.0"} {}`),
		{0xff},
	}
	for _, payload := range invalid {
		if err := ValidateJSONRPCObject(payload); !errors.Is(err, ErrInvalidJSONRPC) {
			t.Fatalf("invalid JSON-RPC accepted: %q", payload)
		}
	}
}

func TestEnvironmentListIsDeterministic(t *testing.T) {
	got := environmentList(map[string]string{"Z": "last", "A": "first"})
	want := []string{"A=first", "Z=last"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("environment order = %v, want %v", got, want)
	}
}
