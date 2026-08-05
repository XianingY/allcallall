package sfu

import (
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestBuildInterceptorRegistryDefaultsOnly(t *testing.T) {
	m := &webrtc.MediaEngine{}
	_ = m.RegisterDefaultCodecs()
	reg, err := BuildInterceptorRegistry(m, nil)
	if err != nil {
		t.Fatalf("build with nil controller error: %v", err)
	}
	if reg == nil {
		t.Fatal("expected a non-nil registry")
	}
}

func TestBuildInterceptorRegistryWithController(t *testing.T) {
	m := &webrtc.MediaEngine{}
	_ = m.RegisterDefaultCodecs()
	reg, err := BuildInterceptorRegistry(m, NewBandwidthController())
	if err != nil {
		t.Fatalf("build with controller error: %v", err)
	}
	if reg == nil {
		t.Fatal("expected a non-nil registry with controller")
	}
}
