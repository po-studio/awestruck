package session

import (
	"log"
	"sync/atomic"
	"time"

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

	// Stop synth first to prevent new audio data
	if as.Synth != nil {
		log.Printf("[%s] Stopping synth engine", as.Id)
		if err := as.Synth.Stop(); err != nil {
			log.Printf("[%s] Error stopping synth: %v", as.Id, err)
		}
		as.Synth = nil
	}

	// Clean up WebRTC resources
	if as.PeerConnection != nil {
		log.Printf("[%s] Closing WebRTC peer connection", as.Id)

		// Stop the pipeline before cleaning up WebRTC
		if as.GStreamerPipeline != nil {
			log.Printf("[%s] Stopping GStreamer pipeline", as.Id)
			as.GStreamerPipeline.Stop()
			as.GStreamerPipeline = nil
		}

		// Get current connection state
		connState := as.PeerConnection.ConnectionState()
		log.Printf("[%s] Current connection state before cleanup: %s", as.Id, connState)

		// Only attempt to clean up tracks if not already closed/failed
		if connState != webrtc.PeerConnectionStateClosed &&
			connState != webrtc.PeerConnectionStateFailed {

			// Stop transceivers first
			for _, t := range as.PeerConnection.GetTransceivers() {
				if t.Sender() != nil {
					// Stop sending before closing
					t.Sender().ReplaceTrack(nil)
				}
				if err := t.Stop(); err != nil {
					log.Printf("[%s] Error stopping transceiver: %v", as.Id, err)
				}
			}

			// Small delay to allow transceiver changes to propagate
			time.Sleep(100 * time.Millisecond)

			// Then remove tracks
			for _, sender := range as.PeerConnection.GetSenders() {
				if err := as.PeerConnection.RemoveTrack(sender); err != nil {
					log.Printf("[%s] Error removing track: %v", as.Id, err)
				}
			}
		}

		// Finally close the connection
		if err := as.PeerConnection.Close(); err != nil {
			log.Printf("[%s] Error closing peer connection: %v", as.Id, err)
		} else {
			log.Printf("[%s] Peer connection closed successfully", as.Id)
		}
		as.PeerConnection = nil
	}

	// Reset monitoring state
	as.monitorClosed.Store(false)

	log.Printf("[%s] Cleanup completed - Resources freed: Synth=%v, PeerConnection=%v, GStreamer=%v",
		as.Id,
		as.Synth == nil,
		as.PeerConnection == nil,
		as.GStreamerPipeline == nil)
}
