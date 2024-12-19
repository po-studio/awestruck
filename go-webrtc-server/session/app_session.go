package session

import (
	"log"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v3"

	gst "github.com/po-studio/go-webrtc-server/internal/gstreamer-src"
	"github.com/po-studio/go-webrtc-server/synth"
	"github.com/po-studio/go-webrtc-server/types"
)

type AppSession struct {
	Id                string
	PeerConnection    *webrtc.PeerConnection
	GStreamerPipeline *gst.Pipeline
	Synth             synth.Synth
	AudioSrc          *string
	SynthPort         int
	TURNCredentials   *types.TURNCredentials
	JackClientName    string
	MonitorDone       chan struct{}
	monitorClosed     atomic.Value
	cleanupInProgress atomic.Value
}

func (as *AppSession) StopAllProcesses() {
	// Prevent multiple simultaneous cleanups
	if !as.cleanupInProgress.CompareAndSwap(false, true) {
		log.Printf("[%s] Cleanup already in progress, skipping", as.Id)
		return
	}

	log.Printf("Starting cleanup for session %s", as.Id)

	// Stop monitoring first - safely handle multiple closes
	if as.MonitorDone != nil {
		if closed, _ := as.monitorClosed.Load().(bool); !closed {
			as.monitorClosed.Store(true)
			close(as.MonitorDone)
		}
	}

	// First close WebRTC to prevent new data flow
	if as.PeerConnection != nil {
		log.Printf("[%s] Closing WebRTC peer connection", as.Id)
		if err := as.PeerConnection.Close(); err != nil {
			log.Printf("[%s] Error closing peer connection: %v", as.Id, err)
		} else {
			log.Printf("[%s] Peer connection closed successfully", as.Id)
		}
		as.PeerConnection = nil
	}

	// Then stop GStreamer pipeline
	if as.GStreamerPipeline != nil {
		log.Printf("[%s] Stopping GStreamer pipeline", as.Id)
		as.GStreamerPipeline.Stop()
		as.GStreamerPipeline = nil
	}

	// Finally stop SuperCollider
	if as.Synth != nil {
		log.Printf("[%s] Stopping synth engine", as.Id)
		if err := as.Synth.Stop(); err != nil {
			log.Printf("[%s] Error stopping synth: %v", as.Id, err)
		}
		as.Synth = nil
	}

	// Small delay to ensure JACK ports are properly cleaned up
	time.Sleep(100 * time.Millisecond)

	log.Printf("[%s] Cleanup completed - Resources freed: Synth=%v, PeerConnection=%v, GStreamer=%v",
		as.Id,
		as.Synth == nil,
		as.PeerConnection == nil,
		as.GStreamerPipeline == nil)

	as.cleanupInProgress.Store(false)
}
