package signaling

import (
	"os"

	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/media"
	"github.com/allcallall/backend/internal/media/sfu"
)

// roomBandwidthEstimationEnabled gates the GCC bandwidth estimation + bandwidth
// aware forwarding chain. It is off by default: when disabled the media engine
// keeps pion's auto-registered default interceptors (NACK/RTCP/TWCC) and the
// SDP/behaviour is unchanged. Enabling it adds the GCC send-side bandwidth
// estimator and feeds per-participant estimates into the forwarding policy.
func roomBandwidthEstimationEnabled() bool {
	return os.Getenv("ROOM_BANDWIDTH_ESTIMATION") == "true" ||
		os.Getenv("ROOM_BANDWIDTH_ESTIMATION") == "1"
}

// InitPionMediaEngine 初始化 Pion 媒体引擎
// InitPionMediaEngine initializes the Pion media engine
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

	var controller *sfu.BandwidthController
	var api *webrtc.API

	if roomBandwidthEstimationEnabled() {
		// Build the interceptor chain (TWCC sender + NACK + RTCP reports via
		// pion defaults, plus GCC bandwidth estimation) on the SAME media
		// engine so the transport-cc header extension is negotiated in SDP.
		controller = sfu.NewBandwidthController()
		reg, err := sfu.BuildInterceptorRegistry(m, controller)
		if err != nil {
			return nil, err
		}
		api = webrtc.NewAPI(
			webrtc.WithSettingEngine(settingEngine),
			webrtc.WithMediaEngine(m),
			webrtc.WithInterceptorRegistry(reg),
		)
		logger.Info().Msg("pion media engine initialized with bandwidth estimation (GCC)")
	} else {
		api = webrtc.NewAPI(
			webrtc.WithSettingEngine(settingEngine),
			webrtc.WithMediaEngine(m),
		)
	}

	cfg := &media.Config{
		WebRTCConfig: webrtc.Configuration{
			ICEServers: iceServers,
		},
		API:       api,
		Bandwidth: controller,
	}

	engine, err := media.NewEngine(logger, cfg)
	if err != nil {
		return nil, err
	}

	logger.Info().Msg("pion media engine initialized")
	return engine, nil
}
