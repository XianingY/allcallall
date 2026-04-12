package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/signaling"
	"github.com/allcallall/backend/internal/translation"
)

// SubtitleDispatcher 对端字幕分发接口
// SubtitleDispatcher forwards final subtitles to remote peer.
type SubtitleDispatcher interface {
	DispatchSubtitle(ctx context.Context, fromEmail, toEmail, callID string, result translation.Result) error
}

// TranslationWSHandler 翻译 WebSocket 处理器
// TranslationWSHandler handles client translation websocket requests.
type TranslationWSHandler struct {
	logger     zerolog.Logger
	service    *translation.Service
	dispatcher SubtitleDispatcher
	upgrader   websocket.Upgrader
}

// NewTranslationWSHandler 构造函数
// NewTranslationWSHandler creates handler with signaling-hub dispatcher.
func NewTranslationWSHandler(log zerolog.Logger, service *translation.Service, hub *signaling.Hub) *TranslationWSHandler {
	var dispatcher SubtitleDispatcher
	if hub != nil {
		dispatcher = &hubSubtitleDispatcher{hub: hub}
	}
	return NewTranslationWSHandlerWithDispatcher(log, service, dispatcher)
}

// NewTranslationWSHandlerWithDispatcher 构造函数（可注入自定义分发器）
// NewTranslationWSHandlerWithDispatcher creates handler with custom dispatcher.
func NewTranslationWSHandlerWithDispatcher(log zerolog.Logger, service *translation.Service, dispatcher SubtitleDispatcher) *TranslationWSHandler {
	return &TranslationWSHandler{
		logger:     log.With().Str("component", "translation_ws_handler").Logger(),
		service:    service,
		dispatcher: dispatcher,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Handle 升级并处理翻译 websocket 会话
// Handle upgrades websocket and processes translation lifecycle.
func (h *TranslationWSHandler) Handle(c *gin.Context) {
	if h.service == nil {
		JSONError(c, http.StatusServiceUnavailable, "translation service not available")
		return
	}

	claims, err := auth.GetClaimsFromContext(c)
	if err != nil {
		JSONError(c, http.StatusUnauthorized, "unauthorized")
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Warn().Err(err).Msg("failed to upgrade translation websocket")
		return
	}
	defer conn.Close()

	outbound := make(chan []byte, 64)
	writerDone := make(chan struct{})
	go h.writeLoop(conn, outbound, writerDone)

	sendJSON := func(v any) {
		data, err := json.Marshal(v)
		if err != nil {
			h.logger.Warn().Err(err).Msg("marshal outbound translation payload failed")
			return
		}
		select {
		case outbound <- data:
		default:
			// 队列拥塞时优先保留最新消息。
			// Prefer latest messages when queue is congested.
			select {
			case <-outbound:
			default:
			}
			select {
			case outbound <- data:
			default:
			}
		}
	}

	var currentSession *translation.Session
	var eventCancel context.CancelFunc

	stopSession := func() {
		if eventCancel != nil {
			eventCancel()
			eventCancel = nil
		}
		if currentSession != nil {
			_ = currentSession.Stop(context.Background())
			currentSession = nil
		}
	}
	defer stopSession()
	defer close(outbound)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				h.logger.Debug().Err(err).Str("email", claims.Email).Msg("translation websocket read closed")
			}
			break
		}

		var msg translationClientMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			sendJSON(translationErrorMessage("BAD_REQUEST", "invalid message format", false))
			continue
		}

		switch strings.TrimSpace(msg.Type) {
		case "translation.start":
			stopSession()

			req := translation.StartRequest{
				OwnerID:     claims.UserID,
				CallID:     strings.TrimSpace(msg.CallID),
				To:         strings.TrimSpace(msg.To),
				SourceLang: strings.TrimSpace(msg.SourceLang),
				TargetLang: strings.TrimSpace(msg.TargetLang),
				ChunkMS:    msg.ChunkMS,
			}
			if req.ChunkMS <= 0 {
				req.ChunkMS = 400
			}

			session, err := h.service.StartSession(c.Request.Context(), claims.Email, req)
			if err != nil {
				h.logger.Warn().Err(err).Str("email", claims.Email).Msg("start translation session failed")
				sendJSON(translationErrorMessage("START_FAILED", err.Error(), isRecoverableStartError(err)))
				continue
			}

			currentSession = session
			evtCtx, cancel := context.WithCancel(context.Background())
			eventCancel = cancel
			go h.forwardSessionEvents(evtCtx, claims.Email, currentSession, sendJSON)

			sendJSON(map[string]any{
				"type":       "translation.ack",
				"session_id": currentSession.ID,
			})

		case "translation.audio":
			if currentSession == nil {
				sendJSON(translationErrorMessage("SESSION_NOT_STARTED", "translation session not started", true))
				continue
			}

			chunk := translation.AudioChunk{
				Seq:         msg.Seq,
				PCM16Base64: msg.PCM16Base64,
				SampleRate:  msg.SampleRate,
				Channels:    msg.Channels,
				TimestampMS: msg.TimestampMS,
			}
			if chunk.SampleRate <= 0 {
				chunk.SampleRate = 16000
			}
			if chunk.Channels <= 0 {
				chunk.Channels = 1
			}
			if chunk.TimestampMS <= 0 {
				chunk.TimestampMS = time.Now().UnixMilli()
			}
			if strings.TrimSpace(chunk.PCM16Base64) == "" {
				sendJSON(translationErrorMessage("BAD_AUDIO", "pcm16_base64 is required", true))
				continue
			}

			if err := currentSession.SendAudio(c.Request.Context(), chunk); err != nil {
				h.logger.Warn().Err(err).Str("session_id", currentSession.ID).Msg("send translation audio failed")
				sendJSON(translationErrorMessage("PROVIDER_ERROR", err.Error(), true))
			}

		case "translation.stop":
			stopSession()

		default:
			sendJSON(translationErrorMessage("BAD_REQUEST", "unknown message type", false))
		}
	}

	<-writerDone
}

func (h *TranslationWSHandler) writeLoop(conn *websocket.Conn, outbound <-chan []byte, done chan<- struct{}) {
	defer close(done)
	for msg := range outbound {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func (h *TranslationWSHandler) forwardSessionEvents(
	ctx context.Context,
	fromEmail string,
	session *translation.Session,
	sendJSON func(v any),
) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-session.Events():
			if !ok {
				return
			}

			if evt.Error != nil {
				sendJSON(translationErrorMessage(evt.Error.Code, evt.Error.Message, evt.Error.Recoverable))
				continue
			}
			if evt.Result == nil {
				continue
			}

			eventType := "translation.partial"
			if evt.Result.IsFinal {
				eventType = "translation.final"
			}

			sendJSON(map[string]any{
				"type":            eventType,
				"session_id":      session.ID,
				"segment_id":      evt.Result.SegmentID,
				"revision":        evt.Result.Revision,
				"is_final":        evt.Result.IsFinal,
				"original_text":   evt.Result.OriginalText,
				"translated_text": evt.Result.TranslatedText,
				"timestamp_ms":    evt.Result.TimestampMS,
				"latency_ms":      evt.Result.LatencyMS,
			})

			if evt.Result.IsFinal && h.dispatcher != nil {
				if err := h.dispatcher.DispatchSubtitle(ctx, fromEmail, session.To, session.CallID, *evt.Result); err != nil {
					h.logger.Warn().
						Err(err).
						Str("session_id", session.ID).
						Str("call_id", session.CallID).
						Str("to", session.To).
						Msg("dispatch call.subtitle failed")
				}
			}
		}
	}
}

func isRecoverableStartError(err error) bool {
	if errors.Is(err, translation.ErrSessionLimitExceeded) {
		return true
	}
	if errors.Is(err, translation.ErrBadStartRequest) {
		return false
	}
	return true
}

func translationErrorMessage(code, message string, recoverable bool) map[string]any {
	if strings.TrimSpace(code) == "" {
		code = "PROVIDER_ERROR"
	}
	if strings.TrimSpace(message) == "" {
		message = "translation error"
	}
	return map[string]any{
		"type":        "translation.error",
		"code":        code,
		"message":     message,
		"recoverable": recoverable,
	}
}

type translationClientMessage struct {
	Type       string `json:"type"`
	CallID     string `json:"call_id"`
	To         string `json:"to"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
	ChunkMS    int    `json:"chunk_ms"`

	Seq         int64  `json:"seq"`
	PCM16Base64 string `json:"pcm16_base64"`
	SampleRate  int    `json:"sample_rate"`
	Channels    int    `json:"channels"`
	TimestampMS int64  `json:"timestamp_ms"`
}

type hubSubtitleDispatcher struct {
	hub *signaling.Hub
}

func (d *hubSubtitleDispatcher) DispatchSubtitle(
	ctx context.Context,
	fromEmail, toEmail, callID string,
	result translation.Result,
) error {
	if d.hub == nil {
		return nil
	}

	payloadBytes, err := json.Marshal(map[string]any{
		"segment_id":      result.SegmentID,
		"revision":        result.Revision,
		"is_final":        true,
		"original_text":   result.OriginalText,
		"translated_text": result.TranslatedText,
		"timestamp_ms":    result.TimestampMS,
		"source":          "online",
	})
	if err != nil {
		return err
	}

	msg := signaling.SignalMessage{
		Type:    "call.subtitle",
		CallID:  callID,
		To:      toEmail,
		Payload: payloadBytes,
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return d.hub.HandleHTTPMessage(ctx, fromEmail, body)
}
