package translation

import "context"

// StartRequest 翻译会话启动参数
// StartRequest defines session-level translation options.
type StartRequest struct {
	OwnerID    uint64
	CallID     string
	To         string
	SourceLang string
	TargetLang string
	ChunkMS    int
}

// AudioChunk 上行音频分片
// AudioChunk carries an encoded PCM frame from client to provider.
type AudioChunk struct {
	Seq         int64
	PCM16Base64 string
	SampleRate  int
	Channels    int
	TimestampMS int64
}

// Result 单条翻译结果
// Result is a normalized translation result emitted by provider.
type Result struct {
	SegmentID      string
	Revision       int
	IsFinal        bool
	OriginalText   string
	TranslatedText string
	TimestampMS    int64
	LatencyMS      int64
	Source         string
}

// ProviderError 供应商错误
// ProviderError carries provider-level error classification.
type ProviderError struct {
	Code        string
	Message     string
	Recoverable bool
}

// Event 会话事件
// Event is emitted by translation session.
type Event struct {
	Result *Result
	Error  *ProviderError
}

// Provider 抽象供应商接口
// Provider abstracts realtime translation providers.
type Provider interface {
	Name() string
	Start(ctx context.Context, sessionID string, req StartRequest, onEvent func(Event)) (ProviderSession, error)
}

// ProviderSession 单个供应商会话
// ProviderSession is a live provider-backed stream session.
type ProviderSession interface {
	SendAudio(ctx context.Context, chunk AudioChunk) error
	Stop(ctx context.Context) error
}
