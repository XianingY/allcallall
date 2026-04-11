package handlers

import (
	"testing"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/mail"
	"github.com/allcallall/backend/internal/signaling"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func TestWebRTCHandler(t *testing.T) {
	t.Run("empty config", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		handler := NewWebRTCHandler(zerolog.Nop(), config.WebRTCConfig{})
		router := gin.New()
		handler.RegisterRoutes(router.Group("/api/v1"))

		rec := performRequest(t, router, "GET", "/api/v1/webrtc/config", nil)
		expectHandlerStatus(t, rec, 200)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if servers, ok := got["ice_servers"].([]any); !ok || len(servers) != 0 {
			t.Fatalf("expected empty ICE server list, got=%v", got)
		}
	})

	t.Run("configured servers", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		handler := NewWebRTCHandler(zerolog.Nop(), config.WebRTCConfig{
			ICEServers: []config.ICEServer{{URLs: []string{"stun:stun.example.com:19302"}}},
		})
		router := gin.New()
		handler.RegisterRoutes(router.Group("/api/v1"))

		rec := performRequest(t, router, "GET", "/api/v1/webrtc/config", nil)
		expectHandlerStatus(t, rec, 200)

		var got map[string]any
		decodeBody(t, rec.Body.Bytes(), &got)
		if servers, ok := got["ice_servers"].([]any); !ok || len(servers) != 1 {
			t.Fatalf("expected one ICE server, got=%v", got)
		}
	})
}

func TestSignalingPollHandler(t *testing.T) {
	hub := &signaling.Hub{}
	claims := &auth.Claims{UserID: 1, Email: "alice@example.com"}

	t.Run("send ping success", func(t *testing.T) {
		handler := NewSignalingPollHandler(zerolog.Nop(), hub)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		rec := performRequest(t, router, "POST", "/api/v1/signaling/send", []byte(`{"type":"client.ping"}`))
		expectHandlerStatus(t, rec, 200)
	})

	t.Run("send missing target", func(t *testing.T) {
		handler := NewSignalingPollHandler(zerolog.Nop(), hub)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		rec := performRequest(t, router, "POST", "/api/v1/signaling/send", []byte(`{"type":"call.invite"}`))
		expectHandlerStatus(t, rec, 400)
	})

	t.Run("send invalid payload", func(t *testing.T) {
		handler := NewSignalingPollHandler(zerolog.Nop(), hub)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		rec := performRequest(t, router, "POST", "/api/v1/signaling/send", []byte(`not-json`))
		expectHandlerStatus(t, rec, 400)
	})

	t.Run("poll default timeout", func(t *testing.T) {
		handler := NewSignalingPollHandler(zerolog.Nop(), hub)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		rec := performRequest(t, router, "GET", "/api/v1/signaling/poll", nil)
		expectHandlerStatus(t, rec, 204)
	})

	t.Run("poll invalid timeout", func(t *testing.T) {
		handler := NewSignalingPollHandler(zerolog.Nop(), hub)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		rec := performRequest(t, router, "GET", "/api/v1/signaling/poll?timeout_ms=bad", nil)
		expectHandlerStatus(t, rec, 204)
	})

	t.Run("poll capped timeout", func(t *testing.T) {
		handler := NewSignalingPollHandler(zerolog.Nop(), hub)
		router := newRouterWithClaims(claims, handler.RegisterRoutes)

		rec := performRequest(t, router, "GET", "/api/v1/signaling/poll?timeout_ms=70000", nil)
		expectHandlerStatus(t, rec, 204)
	})

	t.Run("unauthorized", func(t *testing.T) {
		handler := NewSignalingPollHandler(zerolog.Nop(), hub)
		router := newRouterWithClaims(nil, handler.RegisterRoutes)

		rec := performRequest(t, router, "GET", "/api/v1/signaling/poll", nil)
		expectHandlerStatus(t, rec, 401)
	})
}

func TestSignalingHandler(t *testing.T) {
	hub := &signaling.Hub{}
	claims := &auth.Claims{UserID: 1, Email: "alice@example.com"}

	t.Run("unauthorized", func(t *testing.T) {
		handler := NewSignalingHandler(zerolog.Nop(), hub)
		router := newRouterWithClaims(nil, func(rg *gin.RouterGroup) {
			rg.GET("/ws", handler.Handle)
		})

		rec := performRequest(t, router, "GET", "/api/v1/ws", nil)
		expectHandlerStatus(t, rec, 401)
	})

	t.Run("upgrade failure", func(t *testing.T) {
		handler := NewSignalingHandler(zerolog.Nop(), hub)
		router := newRouterWithClaims(claims, func(rg *gin.RouterGroup) {
			rg.GET("/ws", handler.Handle)
		})

		rec := performRequest(t, router, "GET", "/api/v1/ws", nil)
		expectHandlerStatus(t, rec, 400)
	})
}

func TestEmailHandlerValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewEmailHandler(zerolog.Nop(), &mail.VerificationCodeService{})
	router := gin.New()
	handler.RegisterRoutes(router.Group("/api/v1"))

	t.Run("send invalid payload", func(t *testing.T) {
		rec := performRequest(t, router, "POST", "/api/v1/email/send-verification-code", []byte(`{"email":"bad"}`))
		expectHandlerStatus(t, rec, 400)
	})

	t.Run("verify invalid payload", func(t *testing.T) {
		rec := performRequest(t, router, "POST", "/api/v1/email/verify-code", []byte(`{"email":"bad","code":"1"}`))
		expectHandlerStatus(t, rec, 400)
	})
}
