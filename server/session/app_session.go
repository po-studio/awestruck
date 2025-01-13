package session

import (
	"log"
	"sync/atomic"

	"github.com/pion/webrtc/v3"

	gst "github.com/po-studio/server/internal/gstreamer-src"
	"github.com/po-studio/server/synth"
)

type AppSession struct {
	Id                string
	PeerConnection    *webrtc.PeerConnection
	GStreamerPipeline *gst.Pipeline
	Synth             synth.Synth
	AudioSrc          *string
	SynthPort         int
	JackClientName    string
	MonitorDone       chan struct{}
	monitorClosed     atomic.Value
}

func (as *AppSession) StopAllProcesses() {
	log.Printf("[CLEANUP] Starting cleanup for session %s", as.Id)

	// Stop monitoring first - safely handle multiple closes
	if as.MonitorDone != nil {
		if closed, _ := as.monitorClosed.Load().(bool); !closed {
			as.monitorClosed.Store(true)
			close(as.MonitorDone)
		}
		as.MonitorDone = nil
	}

	// Stop GStreamer before SuperCollider to prevent port disconnection race
	if as.GStreamerPipeline != nil {
		log.Printf("[%s] Stopping GStreamer pipeline", as.Id)
		as.GStreamerPipeline.Stop()
		as.GStreamerPipeline = nil
	}

	// Stop synth for this session only
	if as.Synth != nil {
		log.Printf("[%s] Stopping synth engine", as.Id)
		if err := as.Synth.Stop(); err != nil {
			log.Printf("[%s] Error stopping synth: %v", as.Id, err)
		}
		as.Synth = nil
	}

	// Clean up WebRTC resources for this session
	if as.PeerConnection != nil {
		log.Printf("[%s] Closing WebRTC peer connection", as.Id)
		// Close all transceivers
		for _, t := range as.PeerConnection.GetTransceivers() {
			if err := t.Stop(); err != nil {
				log.Printf("[%s] Error stopping transceiver: %v", as.Id, err)
			}
		}

		// Remove all tracks
		for _, sender := range as.PeerConnection.GetSenders() {
			if err := as.PeerConnection.RemoveTrack(sender); err != nil {
				log.Printf("[%s] Error removing track: %v", as.Id, err)
			}
		}

		if err := as.PeerConnection.Close(); err != nil {
			log.Printf("[%s] Error closing peer connection: %v", as.Id, err)
		} else {
			log.Printf("[%s] Peer connection closed successfully", as.Id)
		}
		as.PeerConnection = nil
	}

	// Reset monitoring state for this session
	as.monitorClosed.Store(false)

	log.Printf("[%s] Cleanup completed - Resources freed: Synth=%v, PeerConnection=%v, GStreamer=%v",
		as.Id,
		as.Synth == nil,
		as.PeerConnection == nil,
		as.GStreamerPipeline == nil)
}
