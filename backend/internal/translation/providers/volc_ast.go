package providers

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"

	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/translation"
	volcevent "github.com/allcallall/backend/internal/translation/providers/volcproto/common/event"
	volcrpcmeta "github.com/allcallall/backend/internal/translation/providers/volcproto/common/rpcmeta"
	volcast "github.com/allcallall/backend/internal/translation/providers/volcproto/products/understanding/ast"
	volcbase "github.com/allcallall/backend/internal/translation/providers/volcproto/products/understanding/base"
)

const (
	defaultVolcASTWSURL     = "wss://openspeech.bytedance.com/api/v4/ast/v2/translate"
	defaultVolcASTResource  = "volc.service_type.10053"
	volcGatewayStatusOKCode = int32(20000000)
	volcStartWriteTimeout   = 5 * time.Second
	volcAudioWriteTimeout   = 3 * time.Second
	volcStopWriteTimeout    = 3 * time.Second
	volcDefaultSampleRate   = 16000
	volcDefaultChannelCount = 1
)

// VolcASTProvider 火山 AST 同传 Provider（V4 protobuf 协议）
// VolcASTProvider adapts Volcengine AST V4 websocket stream.
type VolcASTProvider struct {
	logger zerolog.Logger
	cfg    config.VolcASTConfig
	dialer *websocket.Dialer
}

// NewVolcASTProvider 创建 Volc AST Provider
// NewVolcASTProvider creates Volc AST provider.
func NewVolcASTProvider(log zerolog.Logger, cfg config.VolcASTConfig) *VolcASTProvider {
	return &VolcASTProvider{
		logger: log.With().Str("component", "translation_provider_volc_ast").Logger(),
		cfg:    cfg,
		dialer: websocket.DefaultDialer,
	}
}

// Name 返回供应商名称
// Name returns provider name.
func (p *VolcASTProvider) Name() string {
	return "volc_ast"
}

// Start 建立供应商会话
// Start opens provider websocket and begins receiving events.
func (p *VolcASTProvider) Start(
	ctx context.Context,
	sessionID string,
	req translation.StartRequest,
	onEvent func(translation.Event),
) (translation.ProviderSession, error) {
	if strings.TrimSpace(p.cfg.AppKey) == "" {
		return nil, errors.New("volc ast app_key is required")
	}
	if strings.TrimSpace(p.cfg.AccessKey) == "" {
		return nil, errors.New("volc ast access_key is required")
	}

	wsURL := strings.TrimSpace(p.cfg.WSURL)
	if wsURL == "" {
		wsURL = defaultVolcASTWSURL
	}
	resourceID := strings.TrimSpace(p.cfg.ResourceID)
	if resourceID == "" {
		resourceID = defaultVolcASTResource
	}

	parsed, err := url.Parse(wsURL)
	if err != nil {
		return nil, fmt.Errorf("invalid ws_url: %w", err)
	}

	connectionID := uuid.NewString()
	headers := make(http.Header)
	headers.Set("X-Api-App-Key", strings.TrimSpace(p.cfg.AppKey))
	headers.Set("X-Api-Access-Key", strings.TrimSpace(p.cfg.AccessKey))
	headers.Set("X-Api-Resource-Id", resourceID)
	headers.Set("X-Api-Connect-Id", connectionID)
	if appID := strings.TrimSpace(p.cfg.AppID); appID != "" {
		headers.Set("X-Api-App-Id", appID)
	}

	conn, resp, err := p.dialer.DialContext(ctx, parsed.String(), headers)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return nil, fmt.Errorf("dial volc ast websocket failed (status=%d): %w", statusCode, err)
	}

	session := &volcASTSession{
		logger:          p.logger,
		conn:            conn,
		onEvent:         onEvent,
		sessionID:       sessionID,
		connectionID:    connectionID,
		appID:           strings.TrimSpace(p.cfg.AppID),
		appKey:          strings.TrimSpace(p.cfg.AppKey),
		resourceID:      resourceID,
		closed:          make(chan struct{}),
		revisionBySeg:   make(map[string]int),
		sourceBySeg:     make(map[string]string),
		translatedBySeg: make(map[string]string),
		finalizedSeg:    make(map[string]bool),
	}

	if err := session.sendStart(req); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			p.logger.Warn().Err(closeErr).Msg("failed to close websocket after failed start")
		}
		return nil, fmt.Errorf("send provider start message: %w", err)
	}

	go session.readLoop()
	return session, nil
}

type volcASTSession struct {
	logger       zerolog.Logger
	conn         *websocket.Conn
	onEvent      func(translation.Event)
	sessionID    string
	connectionID string
	appID        string
	appKey       string
	resourceID   string

	closeOnce sync.Once
	closed    chan struct{}

	stateMu          sync.Mutex
	fallbackSegIDSeq int64
	lastActiveSegID  string
	revisionBySeg    map[string]int
	sourceBySeg      map[string]string
	translatedBySeg  map[string]string
	finalizedSeg     map[string]bool
}

func (s *volcASTSession) sendStart(req translation.StartRequest) error {
	payload := &volcast.TranslateRequest{
		RequestMeta: &volcrpcmeta.RequestMeta{
			AppKey:       s.appKey,
			AppID:        s.appID,
			ResourceID:   s.resourceID,
			ConnectionID: s.connectionID,
			SessionID:    s.sessionID,
		},
		Event: volcevent.Type_StartSession,
		User: &volcbase.User{
			Uid: "allcallall",
			Did: "allcallall",
		},
		SourceAudio: &volcbase.Audio{
			Format:  "wav",
			Codec:   "raw",
			Rate:    volcDefaultSampleRate,
			Bits:    16,
			Channel: volcDefaultChannelCount,
		},
		Request: &volcast.ReqParams{
			Mode:           "s2t",
			SourceLanguage: req.SourceLang,
			TargetLanguage: req.TargetLang,
		},
	}

	frame, err := proto.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal start request failed: %w", err)
	}

	s.conn.SetWriteDeadline(time.Now().Add(volcStartWriteTimeout))
	if err := s.conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("send start frame failed: %w", err)
	}
	return nil
}

// SendAudio 上送音频分片
// SendAudio sends one audio chunk to provider.
func (s *volcASTSession) SendAudio(ctx context.Context, chunk translation.AudioChunk) error {
	select {
	case <-s.closed:
		return errors.New("volc ast session closed")
	default:
	}

	pcmRaw, err := base64.StdEncoding.DecodeString(chunk.PCM16Base64)
	if err != nil {
		return fmt.Errorf("invalid pcm16_base64: %w", err)
	}

	pcm16Mono16k, err := normalizePCM16LE(pcmRaw, chunk.SampleRate, chunk.Channels)
	if err != nil {
		return err
	}

	payload := &volcast.TranslateRequest{
		RequestMeta: &volcrpcmeta.RequestMeta{
			SessionID: s.sessionID,
			Sequence:  clampInt64ToInt32(chunk.Seq),
		},
		Event: volcevent.Type_TaskRequest,
		SourceAudio: &volcbase.Audio{
			BinaryData: pcm16Mono16k,
		},
	}

	frame, err := proto.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal audio request failed: %w", err)
	}

	if deadline, ok := ctx.Deadline(); ok {
		s.conn.SetWriteDeadline(deadline)
	} else {
		s.conn.SetWriteDeadline(time.Now().Add(volcAudioWriteTimeout))
	}

	if err := s.conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("write audio chunk failed: %w", err)
	}
	return nil
}

// Stop 停止供应商会话
// Stop gracefully closes provider stream.
func (s *volcASTSession) Stop(ctx context.Context) error {
	var closeErr error
	s.closeOnce.Do(func() {
		payload := &volcast.TranslateRequest{
			RequestMeta: &volcrpcmeta.RequestMeta{SessionID: s.sessionID},
			Event:       volcevent.Type_FinishSession,
		}
		if frame, err := proto.Marshal(payload); err == nil {
			if deadline, ok := ctx.Deadline(); ok {
				s.conn.SetWriteDeadline(deadline)
			} else {
				s.conn.SetWriteDeadline(time.Now().Add(volcStopWriteTimeout))
			}
			if writeErr := s.conn.WriteMessage(websocket.BinaryMessage, frame); writeErr != nil {
				s.logger.Warn().Err(writeErr).Msg("failed to send finish session frame")
			}
		}

		if ctrlErr := s.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(volcStopWriteTimeout),
		); ctrlErr != nil {
			s.logger.Warn().Err(ctrlErr).Msg("failed to send close control frame")
		}
		closeErr = s.conn.Close()
		close(s.closed)
	})
	return closeErr
}

func (s *volcASTSession) readLoop() {
	defer func() {
		s.closeOnce.Do(func() {
			if closeErr := s.conn.Close(); closeErr != nil {
				s.logger.Warn().Err(closeErr).Msg("failed to close websocket in read loop")
			}
			close(s.closed)
		})
	}()

	for {
		messageType, data, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			s.emitError("PROVIDER_ERROR", err.Error(), true)
			return
		}

		if messageType != websocket.BinaryMessage && messageType != websocket.TextMessage {
			continue
		}

		evt, ok := s.parseProviderMessage(data)
		if !ok {
			continue
		}
		s.onEvent(evt)
	}
}

func (s *volcASTSession) parseProviderMessage(data []byte) (translation.Event, bool) {
	var resp volcast.TranslateResponse
	if err := proto.Unmarshal(data, &resp); err != nil {
		return translation.Event{
			Error: &translation.ProviderError{
				Code:        "PROVIDER_BAD_PAYLOAD",
				Message:     err.Error(),
				Recoverable: true,
			},
		}, true
	}

	meta := resp.GetResponseMeta()
	statusCode := int32(0)
	statusMsg := ""
	if meta != nil {
		statusCode = meta.GetStatusCode()
		statusMsg = strings.TrimSpace(meta.GetMessage())
	}

	switch resp.GetEvent() {
	case volcevent.Type_SessionFailed, volcevent.Type_ConnectionFailed:
		providerErr := providerErrorFromStatus(statusCode, statusMsg)
		if providerErr.Code == "PROVIDER_ERROR_0" {
			providerErr.Code = "PROVIDER_ERROR"
		}
		return translation.Event{Error: &providerErr}, true

	case volcevent.Type_SourceSubtitleResponse:
		if isFailureStatus(statusCode, statusMsg) {
			providerErr := providerErrorFromStatus(statusCode, statusMsg)
			return translation.Event{Error: &providerErr}, true
		}
		segID := s.segmentIDFromSequence(meta.GetSequence())
		s.updateSource(segID, resp.GetText())
		return translation.Event{}, false

	case volcevent.Type_TranslationSubtitleResponse:
		if isFailureStatus(statusCode, statusMsg) {
			providerErr := providerErrorFromStatus(statusCode, statusMsg)
			return translation.Event{Error: &providerErr}, true
		}
		segID := s.segmentIDFromSequence(meta.GetSequence())
		result := s.buildResult(segID, resp.GetText(), false, resp.GetStartTime(), resp.GetEndTime())
		if result == nil {
			return translation.Event{}, false
		}
		return translation.Event{Result: result}, true

	case volcevent.Type_TranslationSubtitleEnd:
		if isFailureStatus(statusCode, statusMsg) {
			providerErr := providerErrorFromStatus(statusCode, statusMsg)
			return translation.Event{Error: &providerErr}, true
		}
		segID := s.segmentIDFromSequence(meta.GetSequence())
		result := s.buildResult(segID, resp.GetText(), true, resp.GetStartTime(), resp.GetEndTime())
		if result == nil {
			return translation.Event{}, false
		}
		return translation.Event{Result: result}, true

	case volcevent.Type_SessionFinished:
		if tail := s.flushTailFinal(); tail != nil {
			return translation.Event{Result: tail}, true
		}
		return translation.Event{}, false

	case volcevent.Type_SessionStarted,
		volcevent.Type_UsageResponse,
		volcevent.Type_SourceSubtitleStart,
		volcevent.Type_SourceSubtitleEnd,
		volcevent.Type_TranslationSubtitleStart,
		volcevent.Type_AudioMuted:
		return translation.Event{}, false

	default:
		if isFailureStatus(statusCode, statusMsg) {
			providerErr := providerErrorFromStatus(statusCode, statusMsg)
			return translation.Event{Error: &providerErr}, true
		}
		return translation.Event{}, false
	}
}

func (s *volcASTSession) segmentIDFromSequence(sequence int32) string {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	if sequence < 0 {
		sequence = -sequence
	}
	if sequence > 0 {
		segID := fmt.Sprintf("seg-%d", sequence)
		s.lastActiveSegID = segID
		return segID
	}

	if s.lastActiveSegID != "" && !s.finalizedSeg[s.lastActiveSegID] {
		return s.lastActiveSegID
	}

	s.fallbackSegIDSeq++
	segID := fmt.Sprintf("seg-fb-%d", s.fallbackSegIDSeq)
	s.lastActiveSegID = segID
	return segID
}

func (s *volcASTSession) updateSource(segID string, sourceText string) {
	trimmed := strings.TrimSpace(sourceText)
	if trimmed == "" {
		return
	}

	s.stateMu.Lock()
	s.sourceBySeg[segID] = trimmed
	s.stateMu.Unlock()
}

func (s *volcASTSession) buildResult(segID, text string, isFinal bool, startTime, endTime int32) *translation.Result {
	trimmed := strings.TrimSpace(text)

	s.stateMu.Lock()
	if trimmed != "" {
		s.translatedBySeg[segID] = trimmed
	}
	translated := strings.TrimSpace(s.translatedBySeg[segID])
	if translated == "" {
		s.stateMu.Unlock()
		return nil
	}

	revision := s.revisionBySeg[segID] + 1
	s.revisionBySeg[segID] = revision
	if isFinal {
		s.finalizedSeg[segID] = true
	}
	original := strings.TrimSpace(s.sourceBySeg[segID])
	s.lastActiveSegID = segID
	s.stateMu.Unlock()

	latencyMS := int64(0)
	if endTime > startTime {
		latencyMS = int64(endTime - startTime)
	}

	return &translation.Result{
		SegmentID:      segID,
		Revision:       revision,
		IsFinal:        isFinal,
		OriginalText:   original,
		TranslatedText: translated,
		TimestampMS:    time.Now().UnixMilli(),
		LatencyMS:      latencyMS,
		Source:         "online",
	}
}

func (s *volcASTSession) flushTailFinal() *translation.Result {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	segID := strings.TrimSpace(s.lastActiveSegID)
	if segID == "" || s.finalizedSeg[segID] {
		return nil
	}
	translated := strings.TrimSpace(s.translatedBySeg[segID])
	if translated == "" {
		return nil
	}

	revision := s.revisionBySeg[segID] + 1
	s.revisionBySeg[segID] = revision
	s.finalizedSeg[segID] = true

	return &translation.Result{
		SegmentID:      segID,
		Revision:       revision,
		IsFinal:        true,
		OriginalText:   strings.TrimSpace(s.sourceBySeg[segID]),
		TranslatedText: translated,
		TimestampMS:    time.Now().UnixMilli(),
		LatencyMS:      0,
		Source:         "online",
	}
}

func (s *volcASTSession) emitError(code, msg string, recoverable bool) {
	s.onEvent(translation.Event{
		Error: &translation.ProviderError{
			Code:        code,
			Message:     msg,
			Recoverable: recoverable,
		},
	})
}

func isFailureStatus(code int32, message string) bool {
	return !isSuccessStatus(code, message)
}

func isSuccessStatus(code int32, message string) bool {
	switch code {
	case 0, int32(volcbase.Code_SUCCESS), volcGatewayStatusOKCode:
		return true
	}

	trimmed := strings.TrimSpace(message)
	if strings.EqualFold(trimmed, "ok") {
		return true
	}
	return false
}

func providerErrorFromStatus(code int32, message string) translation.ProviderError {
	message = strings.TrimSpace(message)
	if message == "" {
		if name, ok := volcbase.Code_name[code]; ok {
			message = name
		} else {
			message = "provider returned non-zero status"
		}
	}

	return translation.ProviderError{
		Code:        "PROVIDER_ERROR_" + strconv.Itoa(int(code)),
		Message:     message,
		Recoverable: isRecoverableStatus(code),
	}
}

func isRecoverableStatus(code int32) bool {
	switch code {
	case int32(volcbase.Code_LIMIT_QPS),
		int32(volcbase.Code_SERVER_BUSY),
		int32(volcbase.Code_TIMEOUT_WAITING),
		int32(volcbase.Code_TIMEOUT_PROCESSING),
		int32(volcbase.Code_INTERRUPTED):
		return true
	case int32(volcbase.Code_PERMISSION_DENIED),
		int32(volcbase.Code_INVALID_REQUEST),
		int32(volcbase.Code_INVALID_FORMAT),
		int32(volcbase.Code_ERROR_PARAMS):
		return false
	default:
		return true
	}
}

func normalizePCM16LE(raw []byte, sampleRate, channels int) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("pcm payload is empty")
	}
	if len(raw)%2 != 0 {
		return nil, errors.New("pcm payload must be int16 little-endian")
	}
	if sampleRate <= 0 {
		sampleRate = volcDefaultSampleRate
	}
	if channels <= 0 {
		channels = volcDefaultChannelCount
	}
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("unsupported channel count: %d", channels)
	}
	if sampleRate != 16000 && sampleRate != 48000 {
		return nil, fmt.Errorf("unsupported sample rate: %d", sampleRate)
	}

	totalSamples := len(raw) / 2
	frameCount := totalSamples / channels
	if frameCount <= 0 {
		return nil, errors.New("invalid pcm frame count")
	}

	mono := make([]int16, frameCount)
	for i := 0; i < frameCount; i++ {
		idx := (i * channels) * 2
		if idx+1 >= len(raw) {
			break
		}
		mono[i] = int16(binary.LittleEndian.Uint16(raw[idx : idx+2]))
	}

	resampled := mono
	if sampleRate == 48000 {
		outLen := frameCount / 3
		if outLen <= 0 {
			return nil, errors.New("invalid 48k pcm payload")
		}
		resampled = make([]int16, outLen)
		for i := 0; i < outLen; i++ {
			resampled[i] = mono[i*3]
		}
	}

	out := make([]byte, len(resampled)*2)
	for i, sample := range resampled {
		binary.LittleEndian.PutUint16(out[i*2:i*2+2], uint16(sample))
	}
	return out, nil
}

func clampInt64ToInt32(v int64) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}
