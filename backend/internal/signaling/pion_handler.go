package signaling

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pion/webrtc/v4"

	"github.com/allcallall/backend/internal/media"
)

// PionSignalMessage 扩展的信令消息，支持 Pion WebRTC
// PionSignalMessage extends SignalMessage to support Pion WebRTC operations
type PionSignalMessage struct {
	*SignalMessage

	// SDP offer/answer 数据
	// SDP offer or answer data
	SDP string `json:"sdp,omitempty"`

	// ICE candidate 数据
	// ICE candidate data
	Candidate *ICECandidatePayload `json:"candidate,omitempty"`

	// 媒体引擎状态或控制命令
	// Media engine status or control command
	MediaCommand string `json:"media_command,omitempty"`
}

// ICECandidatePayload 包含 ICE 候选的信息
// ICECandidatePayload contains ICE candidate information
type ICECandidatePayload struct {
	Candidate        string  `json:"candidate"`
	SDPMLineIndex    *uint16 `json:"sdpMLineIndex"`
	SDPMid           *string `json:"sdpMid"`
	UsernameFragment string  `json:"usernameFragment,omitempty"`
}

// MediaCommandType 定义媒体命令类型
// MediaCommandType defines types of media commands
type MediaCommandType string

const (
	// MediaCommandStartAudio 开始音频传输
	// MediaCommandStartAudio indicates starting audio transmission
	MediaCommandStartAudio MediaCommandType = "start_audio"

	// MediaCommandStopAudio 停止音频传输
	// MediaCommandStopAudio indicates stopping audio transmission
	MediaCommandStopAudio MediaCommandType = "stop_audio"

	// MediaCommandStartVideo 开始视频传输
	// MediaCommandStartVideo indicates starting video transmission
	MediaCommandStartVideo MediaCommandType = "start_video"

	// MediaCommandStopVideo 停止视频传输
	// MediaCommandStopVideo indicates stopping video transmission
	MediaCommandStopVideo MediaCommandType = "stop_video"

	// MediaCommandGetStats 获取媒体统计信息
	// MediaCommandGetStats requests media statistics
	MediaCommandGetStats MediaCommandType = "get_stats"
)

// WithMediaEngine 附加媒体引擎到 Hub
// WithMediaEngine attaches a Pion media engine to the signaling hub
func (h *Hub) WithMediaEngine(engine *media.Engine) {
	h.mediaEngine = engine
	h.logger.Info().Msg("media engine attached to signaling hub")
}

// HandlePionMessage 处理 Pion 媒体相关的信令消息
// HandlePionMessage processes Pion WebRTC-related signaling messages
func (h *Hub) HandlePionMessage(ctx context.Context, callID, localEmail, remoteEmail string, msgType string, payload json.RawMessage) error {
	if h.mediaEngine == nil {
		return fmt.Errorf("media engine not attached")
	}

	// 解析负载
	// Parse message payload
	var pionMsg PionSignalMessage
	if err := json.Unmarshal(payload, &pionMsg); err != nil {
		return fmt.Errorf("unmarshal pion message: %w", err)
	}

	// 获取或创建对等连接
	// Get or create peer connection
	peerConn, err := h.mediaEngine.GetPeerConnection(callID, localEmail, remoteEmail)
	if err != nil {
		// 如果连接不存在，尝试创建
		// If connection doesn't exist, try to create it
		peerConn, err = h.mediaEngine.CreatePeerConnection(
			ctx,
			callID,
			localEmail,
			remoteEmail,
			nil, // use engine default ICE config
		)
		if err != nil {
			return fmt.Errorf("create peer connection: %w", err)
		}
	}

	// 根据消息类型处理
	// Handle based on message type
	switch msgType {
	case "offer":
		return h.handleOffer(ctx, peerConn, pionMsg.SDP)

	case "answer":
		return h.handleAnswer(ctx, peerConn, pionMsg.SDP)

	case "ice_candidate":
		return h.handleICECandidate(ctx, peerConn, pionMsg.Candidate)

	case "media_command":
		return h.handleMediaCommand(ctx, peerConn, MediaCommandType(pionMsg.MediaCommand))

	default:
		return fmt.Errorf("unknown pion message type: %s", msgType)
	}
}

// handleOffer 处理 WebRTC offer
// handleOffer processes a WebRTC offer
func (h *Hub) handleOffer(ctx context.Context, peerConn *media.PeerConnection, sdp string) error {
	// 设置远程描述为 offer
	// Set remote description as offer
	err := peerConn.PC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	})
	if err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	peerConn.State = media.CallStateAnswering

	// 创建答案
	// Create answer
	answer, err := peerConn.PC.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("create answer: %w", err)
	}

	// 设置本地描述为答案
	// Set local description as answer
	err = peerConn.PC.SetLocalDescription(answer)
	if err != nil {
		return fmt.Errorf("set local description: %w", err)
	}

	h.logger.Debug().
		Str("call_id", peerConn.CallID).
		Msg("offer handled, answer created")

	return nil
}

// handleAnswer 处理 WebRTC answer
// handleAnswer processes a WebRTC answer
func (h *Hub) handleAnswer(ctx context.Context, peerConn *media.PeerConnection, sdp string) error {
	// 设置远程描述为 answer
	// Set remote description as answer
	err := peerConn.PC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdp,
	})
	if err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	peerConn.State = media.CallStateActive

	h.logger.Debug().
		Str("call_id", peerConn.CallID).
		Msg("answer handled, connection established")

	return nil
}

// handleICECandidate 处理 ICE 候选
// handleICECandidate processes an ICE candidate
func (h *Hub) handleICECandidate(ctx context.Context, peerConn *media.PeerConnection, candidate *ICECandidatePayload) error {
	if candidate == nil {
		return fmt.Errorf("candidate payload is nil")
	}

	// 转换为 Pion ICECandidateInit
	// Convert to Pion ICECandidateInit
	var usernameFragment *string
	if candidate.UsernameFragment != "" {
		usernameFragment = &candidate.UsernameFragment
	}

	init := webrtc.ICECandidateInit{
		Candidate:        candidate.Candidate,
		SDPMLineIndex:    candidate.SDPMLineIndex,
		SDPMid:           candidate.SDPMid,
		UsernameFragment: usernameFragment,
	}

	// 添加 ICE 候选
	// Add ICE candidate
	err := peerConn.PC.AddICECandidate(init)
	if err != nil {
		return fmt.Errorf("add ice candidate: %w", err)
	}

	h.logger.Debug().
		Str("call_id", peerConn.CallID).
		Msg("ice candidate added")

	return nil
}

// handleMediaCommand 处理媒体控制命令
// handleMediaCommand processes media control commands
func (h *Hub) handleMediaCommand(ctx context.Context, peerConn *media.PeerConnection, command MediaCommandType) error {
	h.logger.Debug().
		Str("call_id", peerConn.CallID).
		Str("command", string(command)).
		Msg("media command received")

	switch command {
	case MediaCommandStartAudio, MediaCommandStartVideo:
		// 媒体启动命令 - 由前端客户端处理
		// Media start commands - handled by frontend client
		return nil

	case MediaCommandStopAudio, MediaCommandStopVideo:
		// 媒体停止命令 - 由前端客户端处理
		// Media stop commands - handled by frontend client
		return nil

	case MediaCommandGetStats:
		// 获取统计信息 - 将在未来实现
		// Get statistics - to be implemented in future
		return nil

	default:
		return fmt.Errorf("unknown media command: %s", command)
	}
}

// CreateOffer 创建 WebRTC offer
// CreateOffer creates a WebRTC offer for a peer connection
func (h *Hub) CreateOffer(ctx context.Context, callID, localEmail, remoteEmail string) (string, error) {
	peerConn, err := h.mediaEngine.GetPeerConnection(callID, localEmail, remoteEmail)
	if err != nil {
		return "", fmt.Errorf("get peer connection: %w", err)
	}

	offer, err := peerConn.PC.CreateOffer(nil)
	if err != nil {
		return "", fmt.Errorf("create offer: %w", err)
	}

	err = peerConn.PC.SetLocalDescription(offer)
	if err != nil {
		return "", fmt.Errorf("set local description: %w", err)
	}

	peerConn.State = media.CallStateOffering

	return offer.SDP, nil
}

// GetConnectionStats 获取连接统计信息（可扩展功能）
// GetConnectionStats retrieves connection statistics for monitoring
func (h *Hub) GetConnectionStats(callID, localEmail, remoteEmail string) (*ConnectionStats, error) {
	peerConn, err := h.mediaEngine.GetPeerConnection(callID, localEmail, remoteEmail)
	if err != nil {
		return nil, err
	}

	// 基本状态信息
	// Basic status information
	stats := &ConnectionStats{
		CallID:         callID,
		LocalEmail:     localEmail,
		RemoteEmail:    remoteEmail,
		State:          peerConn.State,
		PCState:        peerConn.PC.ConnectionState().String(),
		SignalingState: peerConn.PC.SignalingState().String(),
	}

	return stats, nil
}

// ConnectionStats 包含连接统计信息
// ConnectionStats contains connection statistics information
type ConnectionStats struct {
	CallID         string
	LocalEmail     string
	RemoteEmail    string
	State          media.CallState
	PCState        string
	SignalingState string
}
