package main

import (
	"testing"

	"github.com/pion/webrtc/v3"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultWebRTCConfig()
	if config.HTTPAddr != ":8082" {
		t.Errorf("default HTTPAddr = %q, want %q", config.HTTPAddr, ":8082")
	}
	if len(config.ICEServers) == 0 {
		t.Error("default ICEServers should not be empty")
	}
	if config.ICEServers[0] != "stun:stun.l.google.com:19302" {
		t.Errorf("default first ICE server = %q, want stun:stun.l.google.com:19302", config.ICEServers[0])
	}
}

func TestConnectionStateConstants(t *testing.T) {
	if webrtc.PeerConnectionStateClosed.String() != "closed" {
		t.Errorf("unexpected state string")
	}
}
