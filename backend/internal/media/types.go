package media

import (
	"github.com/pion/webrtc/v4"
)

// CallState 表示通话的状态
// CallState represents the state of an active call
type CallState int

const (
	// CallStateOffering 表示正在等待 offer
	// CallStateOffering indicates waiting for offer
	CallStateOffering CallState = iota
	// CallStateAnswering 表示正在等待 answer
	// CallStateAnswering indicates waiting for answer
	CallStateAnswering
	// CallStateActive 表示通话已激活
	// CallStateActive indicates call is active
	CallStateActive
	// CallStateEnded 表示通话已结束
	// CallStateEnded indicates call is ended
	CallStateEnded
)

// PeerConnection 包装 Pion WebRTC PeerConnection
// PeerConnection wraps Pion WebRTC PeerConnection with extensible management
type PeerConnection struct {
	// 底层 Pion PeerConnection
	// Underlying Pion WebRTC PeerConnection
	PC *webrtc.PeerConnection

	// 通话 ID
	// Call identifier
	CallID string

	// 本地用户邮箱
	// Local user email
	LocalEmail string

	// 远程用户邮箱
	// Remote user email
	RemoteEmail string

	// 通话状态
	// Call state
	State CallState

	// 媒体轨道处理器
	// Media track handlers
	Handlers *MediaHandlers
}

// MediaHandlers 包含所有媒体相关的回调处理
// MediaHandlers contains all media-related event handlers
type MediaHandlers struct {
	// 当音频轨道被添加时调用
	// Called when audio track is added
	OnAudioTrack func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver)

	// 当视频轨道被添加时调用
	// Called when video track is added
	OnVideoTrack func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver)

	// 当 ICE 候选被生成时调用
	// Called when ICE candidate is generated
	OnICECandidate func(candidate *webrtc.ICECandidate)

	// 当 ICE 连接状态改变时调用
	// Called when ICE connection state changes
	OnICEConnectionStateChange func(connectionState webrtc.ICEConnectionState)

	// 当信令状态改变时调用
	// Called when signaling state changes
	OnSignalingStateChange func(signalingState webrtc.SignalingState)

	// 当连接状态改变时调用
	// Called when connection state changes
	OnConnectionStateChange func(connectionState webrtc.PeerConnectionState)
}

// OfferAnswer 包含 SDP offer 或 answer
// OfferAnswer contains SDP offer or answer
type OfferAnswer struct {
	Type string `json:"type"` // "offer" or "answer"
	SDP  string `json:"sdp"`
}

// ICECandidateInit 包含 ICE 候选信息
// ICECandidateInit contains ICE candidate information
type ICECandidateInit struct {
	Candidate        string  `json:"candidate"`
	SDPMLineIndex    *uint16 `json:"sdpMLineIndex"`
	SDPMid           *string `json:"sdpMid"`
	UsernameFragment string  `json:"usernameFragment,omitempty"`
}
