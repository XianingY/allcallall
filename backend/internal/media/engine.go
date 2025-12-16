package media

import (
	"context"
	"fmt"
	"sync"

	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"
)

// Engine 是 Pion WebRTC 媒体引擎
// Engine is the Pion WebRTC media engine that manages peer connections
type Engine struct {
	mu              sync.RWMutex
	logger          zerolog.Logger
	peerConnections map[string]*PeerConnection
	defaultConfig   webrtc.Configuration
}

// Config 包含 Pion 媒体引擎的配置
// Config contains configuration for Pion media engine
type Config struct {
	// WebRTC 配置
	// WebRTC configuration
	WebRTCConfig webrtc.Configuration
}

// NewEngine 创建新的媒体引擎
// NewEngine creates a new Pion media engine
func NewEngine(logger zerolog.Logger, cfg *Config) (*Engine, error) {
	return &Engine{
		logger:          logger.With().Str("component", "media_engine").Logger(),
		peerConnections: make(map[string]*PeerConnection),
		defaultConfig:   cfg.WebRTCConfig,
	}, nil
}

// CreatePeerConnection 创建新的对等连接
// CreatePeerConnection creates a new peer connection
func (e *Engine) CreatePeerConnection(ctx context.Context, callID, localEmail, remoteEmail string, cfg *webrtc.Configuration) (*PeerConnection, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	pcCfg := e.defaultConfig
	if cfg != nil {
		pcCfg = *cfg
	}

	// 创建 PeerConnection
	// Create peer connection
	pc, err := webrtc.NewPeerConnection(pcCfg)
	if err != nil {
		return nil, fmt.Errorf("create peer connection: %w", err)
	}

	peerConn := &PeerConnection{
		PC:          pc,
		CallID:      callID,
		LocalEmail:  localEmail,
		RemoteEmail: remoteEmail,
		State:       CallStateOffering,
		Handlers:    &MediaHandlers{},
	}

	// 设置事件处理器
	// Setup event handlers
	e.setupPeerConnectionHandlers(pc, peerConn)

	connectionID := fmt.Sprintf("%s-%s-%s", callID, localEmail, remoteEmail)
	e.peerConnections[connectionID] = peerConn

	e.logger.Info().
		Str("call_id", callID).
		Str("local", localEmail).
		Str("remote", remoteEmail).
		Msg("peer connection created")

	return peerConn, nil
}

// setupPeerConnectionHandlers 设置 PeerConnection 的事件处理器
// setupPeerConnectionHandlers configures event handlers for a peer connection
func (e *Engine) setupPeerConnectionHandlers(pc *webrtc.PeerConnection, peerConn *PeerConnection) {
	// ICE candidate 处理
	// Handle ICE candidates
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil && peerConn.Handlers.OnICECandidate != nil {
			peerConn.Handlers.OnICECandidate(candidate)
		}
	})

	// ICE 连接状态改变
	// Handle ICE connection state changes
	pc.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		e.logger.Debug().
			Str("call_id", peerConn.CallID).
			Str("state", connectionState.String()).
			Msg("ice connection state changed")

		if peerConn.Handlers.OnICEConnectionStateChange != nil {
			peerConn.Handlers.OnICEConnectionStateChange(connectionState)
		}
	})

	// 信令状态改变
	// Handle signaling state changes
	pc.OnSignalingStateChange(func(signalingState webrtc.SignalingState) {
		e.logger.Debug().
			Str("call_id", peerConn.CallID).
			Str("state", signalingState.String()).
			Msg("signaling state changed")

		if peerConn.Handlers.OnSignalingStateChange != nil {
			peerConn.Handlers.OnSignalingStateChange(signalingState)
		}
	})

	// 连接状态改变
	// Handle connection state changes
	pc.OnConnectionStateChange(func(connectionState webrtc.PeerConnectionState) {
		e.logger.Info().
			Str("call_id", peerConn.CallID).
			Str("state", connectionState.String()).
			Msg("connection state changed")

		if peerConn.Handlers.OnConnectionStateChange != nil {
			peerConn.Handlers.OnConnectionStateChange(connectionState)
		}

		// 自动清理已关闭的连接
		// Auto cleanup closed connections
		if connectionState == webrtc.PeerConnectionStateClosed ||
			connectionState == webrtc.PeerConnectionStateFailed {
			_ = e.ClosePeerConnection(peerConn.CallID, peerConn.LocalEmail, peerConn.RemoteEmail)
		}
	})

	// 媒体轨道处理
	// Handle media tracks
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		switch track.Kind() {
		case webrtc.RTPCodecTypeAudio:
			e.logger.Debug().
				Str("call_id", peerConn.CallID).
				Msg("audio track received")
			if peerConn.Handlers.OnAudioTrack != nil {
				peerConn.Handlers.OnAudioTrack(track, receiver)
			}

		case webrtc.RTPCodecTypeVideo:
			e.logger.Debug().
				Str("call_id", peerConn.CallID).
				Msg("video track received")
			if peerConn.Handlers.OnVideoTrack != nil {
				peerConn.Handlers.OnVideoTrack(track, receiver)
			}
		}
	})
}

// ClosePeerConnection 关闭对等连接
// ClosePeerConnection closes and removes a peer connection
func (e *Engine) ClosePeerConnection(callID, localEmail, remoteEmail string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	connectionID := fmt.Sprintf("%s-%s-%s", callID, localEmail, remoteEmail)
	peerConn, exists := e.peerConnections[connectionID]
	if !exists {
		return fmt.Errorf("peer connection not found: %s", connectionID)
	}

	peerConn.State = CallStateEnded

	if err := peerConn.PC.Close(); err != nil {
		e.logger.Warn().Err(err).Str("call_id", callID).Msg("error closing peer connection")
	}

	delete(e.peerConnections, connectionID)

	e.logger.Info().
		Str("call_id", callID).
		Str("local", localEmail).
		Str("remote", remoteEmail).
		Msg("peer connection closed")

	return nil
}

// GetPeerConnection 获取对等连接
// GetPeerConnection retrieves a peer connection by ID
func (e *Engine) GetPeerConnection(callID, localEmail, remoteEmail string) (*PeerConnection, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	connectionID := fmt.Sprintf("%s-%s-%s", callID, localEmail, remoteEmail)
	peerConn, exists := e.peerConnections[connectionID]
	if !exists {
		return nil, fmt.Errorf("peer connection not found: %s", connectionID)
	}

	return peerConn, nil
}

// Shutdown 关闭媒体引擎并清理所有连接
// Shutdown closes the media engine and all peer connections
func (e *Engine) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var lastErr error
	for id, peerConn := range e.peerConnections {
		peerConn.State = CallStateEnded
		if err := peerConn.PC.Close(); err != nil {
			e.logger.Warn().Err(err).Str("connection_id", id).Msg("error closing peer connection")
			lastErr = err
		}
	}

	e.peerConnections = make(map[string]*PeerConnection)

	if lastErr != nil {
		return fmt.Errorf("shutdown with errors: %w", lastErr)
	}

	e.logger.Info().Msg("media engine shutdown complete")
	return nil
}

// ListPeerConnections 列出所有活跃的对等连接
// ListPeerConnections returns all active peer connections (for monitoring/debugging)
func (e *Engine) ListPeerConnections() []*PeerConnection {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*PeerConnection, 0, len(e.peerConnections))
	for _, pc := range e.peerConnections {
		result = append(result, pc)
	}
	return result
}
