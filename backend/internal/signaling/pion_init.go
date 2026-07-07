package signaling

import (
	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/media"
)

// InitPionMediaEngine 初始化 Pion 媒体引擎
// InitPionMediaEngine initializes the Pion WebRTC media engine
func InitPionMediaEngine(logger zerolog.Logger, rtcCfg config.WebRTCConfig) (*media.Engine, error) {
	iceServers := make([]webrtc.ICEServer, 0, len(rtcCfg.ICEServers))
	for _, srv := range rtcCfg.ICEServers {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:       srv.URLs,
			Username:   srv.Username,
			Credential: srv.Credential,
		})
	}

	// WebRTC 高并发网络调优 (High Concurrency Network Tuning)
	settingEngine := webrtc.SettingEngine{}
	// 划定专门的高并发 UDP 端口区间，避免耗尽临时端口池
	_ = settingEngine.SetEphemeralUDPPortRange(40000, 60000)

	// 创建 API 对象并注入 SettingEngine
	m := &webrtc.MediaEngine{}
	_ = m.RegisterDefaultCodecs()
	api := webrtc.NewAPI(
		webrtc.WithSettingEngine(settingEngine),
		webrtc.WithMediaEngine(m),
	)

	cfg := &media.Config{
		WebRTCConfig: webrtc.Configuration{
			ICEServers: iceServers,
		},
		API: api,
	}

	engine, err := media.NewEngine(logger, cfg)
	if err != nil {
		return nil, err
	}

	logger.Info().Msg("pion media engine initialized")
	return engine, nil
}
