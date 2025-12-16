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

	// 创建媒体引擎
	// Create media engine
	cfg := &media.Config{
		WebRTCConfig: webrtc.Configuration{
			ICEServers: iceServers,
		},
	}

	engine, err := media.NewEngine(logger, cfg)
	if err != nil {
		return nil, err
	}

	logger.Info().Msg("pion media engine initialized")
	return engine, nil
}
