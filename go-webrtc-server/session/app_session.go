package session

import (
	"log"

	"github.com/pion/webrtc/v3"

	gst "github.com/po-studio/go-webrtc-server/internal/gstreamer-src"
	"github.com/po-studio/go-webrtc-server/jack"
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
}

func (as *AppSession) StopAllProcesses() {
	log.Printf("Starting cleanup for session %s", as.Id)

	if as.Synth != nil {
		log.Printf("[%s] Stopping synth engine", as.Id)
		as.Synth.Stop()
	}

	if err := jack.DisconnectJackPorts(as.Id); err != nil {
		log.Printf("[%s] Error disconnecting JACK ports: %v", as.Id, err)
	} else {
		log.Printf("[%s] JACK ports disconnected successfully", as.Id)
	}

	if as.PeerConnection != nil {
		log.Printf("[%s] Closing WebRTC peer connection", as.Id)
		if err := as.PeerConnection.Close(); err != nil {
			log.Printf("[%s] Error closing peer connection: %v", as.Id, err)
		} else {
			log.Printf("[%s] Peer connection closed successfully", as.Id)
		}
		as.PeerConnection = nil
	}

	if as.GStreamerPipeline != nil {
		log.Printf("[%s] Stopping GStreamer pipeline", as.Id)
		as.GStreamerPipeline.Stop()
		as.GStreamerPipeline = nil
	}

	log.Printf("[%s] Cleanup completed - Resources freed: Synth=%v, PeerConnection=%v, GStreamer=%v",
		as.Id,
		as.Synth == nil,
		as.PeerConnection == nil,
		as.GStreamerPipeline == nil)
}
