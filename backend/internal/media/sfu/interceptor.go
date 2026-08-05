package sfu

import (
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/webrtc/v4"
)

// BuildInterceptorRegistry assembles the interceptor chain used by meeting-room
// peer connections:
//
//   - RTCP sender/receiver reports (via pion defaults)
//   - NACK generator + responder for loss recovery (via pion defaults)
//   - TWCC sender so the remote can emit transport-wide congestion feedback
//     (via pion defaults; this also registers the transport-cc header extension
//     on the supplied MediaEngine so it is negotiated in the SDP)
//   - GCC send-side bandwidth estimator (Google Congestion Control) whose
//     per-connection estimator is bound to controller.
//
// The SAME mediaEngine instance passed here must be the one handed to
// webrtc.WithMediaEngine when the API is built, otherwise the transport-cc
// header extension will not appear in the SDP and TWCC feedback will not flow.
//
// controller may be nil; in that case the registry contains only pion's defaults
// and callers should simply use webrtc.NewAPI without WithInterceptorRegistry to
// keep identical behaviour. When non-nil, the registry must be supplied to
// webrtc.WithInterceptorRegistry.
func BuildInterceptorRegistry(m *webrtc.MediaEngine, controller *BandwidthController) (*interceptor.Registry, error) {
	reg := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, reg); err != nil {
		return nil, err
	}
	if controller != nil {
		ccFactory, err := cc.NewInterceptor(controller.Factory())
		if err != nil {
			return nil, err
		}
		reg.Add(ccFactory)
	}
	return reg, nil
}
