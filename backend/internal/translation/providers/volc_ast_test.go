package providers

import (
	"encoding/binary"
	"testing"

	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	"github.com/allcallall/backend/internal/translation/providers/volcproto/common/event"
	"github.com/allcallall/backend/internal/translation/providers/volcproto/common/rpcmeta"
	"github.com/allcallall/backend/internal/translation/providers/volcproto/products/understanding/ast"
	"github.com/allcallall/backend/internal/translation/providers/volcproto/products/understanding/base"
)

func TestParseProviderMessageSourceAndTranslation(t *testing.T) {
	s := &volcASTSession{
		logger:          zerolog.Nop(),
		revisionBySeg:   map[string]int{},
		sourceBySeg:     map[string]string{},
		translatedBySeg: map[string]string{},
		finalizedSeg:    map[string]bool{},
	}

	sourceResp := &ast.TranslateResponse{
		ResponseMeta: &rpcmeta.ResponseMeta{SessionID: "s1", Sequence: 1},
		Event:        event.Type_SourceSubtitleResponse,
		Text:         "你好",
	}
	sourceFrame, err := proto.Marshal(sourceResp)
	if err != nil {
		t.Fatalf("marshal source resp failed: %v", err)
	}
	if evt, ok := s.parseProviderMessage(sourceFrame); ok || evt.Result != nil || evt.Error != nil {
		t.Fatalf("source subtitle should not emit user-visible event, got ok=%v evt=%+v", ok, evt)
	}

	partialResp := &ast.TranslateResponse{
		ResponseMeta: &rpcmeta.ResponseMeta{SessionID: "s1", Sequence: 1},
		Event:        event.Type_TranslationSubtitleResponse,
		Text:         "hello",
	}
	partialFrame, err := proto.Marshal(partialResp)
	if err != nil {
		t.Fatalf("marshal translation resp failed: %v", err)
	}
	partialEvt, ok := s.parseProviderMessage(partialFrame)
	if !ok || partialEvt.Result == nil {
		t.Fatalf("expected partial result, got ok=%v evt=%+v", ok, partialEvt)
	}
	if partialEvt.Result.SegmentID != "seg-1" {
		t.Fatalf("unexpected segment id: %s", partialEvt.Result.SegmentID)
	}
	if partialEvt.Result.Revision != 1 {
		t.Fatalf("unexpected revision: %d", partialEvt.Result.Revision)
	}
	if partialEvt.Result.IsFinal {
		t.Fatal("partial result should not be final")
	}
	if partialEvt.Result.OriginalText != "你好" {
		t.Fatalf("unexpected original text: %s", partialEvt.Result.OriginalText)
	}
	if partialEvt.Result.TranslatedText != "hello" {
		t.Fatalf("unexpected translated text: %s", partialEvt.Result.TranslatedText)
	}

	finalResp := &ast.TranslateResponse{
		ResponseMeta: &rpcmeta.ResponseMeta{SessionID: "s1", Sequence: 1},
		Event:        event.Type_TranslationSubtitleEnd,
	}
	finalFrame, err := proto.Marshal(finalResp)
	if err != nil {
		t.Fatalf("marshal translation end failed: %v", err)
	}
	finalEvt, ok := s.parseProviderMessage(finalFrame)
	if !ok || finalEvt.Result == nil {
		t.Fatalf("expected final result, got ok=%v evt=%+v", ok, finalEvt)
	}
	if !finalEvt.Result.IsFinal {
		t.Fatal("expected final result")
	}
	if finalEvt.Result.Revision != 2 {
		t.Fatalf("unexpected final revision: %d", finalEvt.Result.Revision)
	}
	if finalEvt.Result.TranslatedText != "hello" {
		t.Fatalf("unexpected final translated text: %s", finalEvt.Result.TranslatedText)
	}
}

func TestParseProviderMessageStatusErrorClassification(t *testing.T) {
	s := &volcASTSession{}

	permissionDenied := &ast.TranslateResponse{
		ResponseMeta: &rpcmeta.ResponseMeta{SessionID: "s1", StatusCode: int32(base.Code_PERMISSION_DENIED), Message: "permission denied"},
		Event:        event.Type_SessionFailed,
	}
	permissionFrame, err := proto.Marshal(permissionDenied)
	if err != nil {
		t.Fatalf("marshal permission denied failed: %v", err)
	}
	evt, ok := s.parseProviderMessage(permissionFrame)
	if !ok || evt.Error == nil {
		t.Fatalf("expected provider error event, got ok=%v evt=%+v", ok, evt)
	}
	if evt.Error.Recoverable {
		t.Fatal("permission denied should be non-recoverable")
	}

	timeoutResp := &ast.TranslateResponse{
		ResponseMeta: &rpcmeta.ResponseMeta{SessionID: "s1", StatusCode: int32(base.Code_TIMEOUT_PROCESSING), Message: "timeout"},
		Event:        event.Type_SessionFailed,
	}
	timeoutFrame, err := proto.Marshal(timeoutResp)
	if err != nil {
		t.Fatalf("marshal timeout failed: %v", err)
	}
	evt, ok = s.parseProviderMessage(timeoutFrame)
	if !ok || evt.Error == nil {
		t.Fatalf("expected timeout error event, got ok=%v evt=%+v", ok, evt)
	}
	if !evt.Error.Recoverable {
		t.Fatal("timeout should be recoverable")
	}
}

func TestParseProviderMessageGatewayOKStatusIsNotError(t *testing.T) {
	s := &volcASTSession{
		logger:          zerolog.Nop(),
		revisionBySeg:   map[string]int{},
		sourceBySeg:     map[string]string{},
		translatedBySeg: map[string]string{},
		finalizedSeg:    map[string]bool{},
	}

	resp := &ast.TranslateResponse{
		ResponseMeta: &rpcmeta.ResponseMeta{
			SessionID:  "s1",
			Sequence:   7,
			StatusCode: 20000000,
			Message:    "OK",
		},
		Event: event.Type_TranslationSubtitleResponse,
		Text:  "hello",
	}
	frame, err := proto.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response failed: %v", err)
	}

	evt, ok := s.parseProviderMessage(frame)
	if !ok {
		t.Fatalf("expected event to be emitted")
	}
	if evt.Error != nil {
		t.Fatalf("expected no provider error, got %+v", evt.Error)
	}
	if evt.Result == nil {
		t.Fatalf("expected translation result")
	}
	if evt.Result.TranslatedText != "hello" {
		t.Fatalf("unexpected translated text: %s", evt.Result.TranslatedText)
	}
}

func TestParseProviderMessageBadPayload(t *testing.T) {
	s := &volcASTSession{}
	evt, ok := s.parseProviderMessage([]byte("not protobuf"))
	if !ok || evt.Error == nil {
		t.Fatalf("expected bad payload error, got ok=%v evt=%+v", ok, evt)
	}
	if evt.Error.Code != "PROVIDER_BAD_PAYLOAD" {
		t.Fatalf("unexpected error code: %s", evt.Error.Code)
	}
}

func TestNormalizePCM16LE(t *testing.T) {
	// 48k stereo, channel-0 values: [100,300,500,700,900,1100]
	// Downsample by 3 => [100,700]
	raw := make([]byte, 12*2)
	samples := []int16{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000, 1100, 1200}
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(raw[i*2:], uint16(sample))
	}

	out, err := normalizePCM16LE(raw, 48000, 2)
	if err != nil {
		t.Fatalf("normalize pcm failed: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("expected 4 bytes output, got %d", len(out))
	}
	first := int16(binary.LittleEndian.Uint16(out[0:2]))
	second := int16(binary.LittleEndian.Uint16(out[2:4]))
	if first != 100 || second != 700 {
		t.Fatalf("unexpected normalized samples: [%d, %d]", first, second)
	}

	if _, err := normalizePCM16LE(raw, 44100, 1); err == nil {
		t.Fatal("expected unsupported sample rate error")
	}
}
