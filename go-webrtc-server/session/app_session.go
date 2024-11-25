package session

import (
	"fmt"

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
	if as.Synth != nil {
		as.Synth.Stop()
	}

	if err := jack.DisconnectJackPorts(as.Id); err != nil {
		fmt.Println("Error disconnecting JACK ports:", err)
	} else {
		fmt.Println("JACK ports disconnected successfully.")
	}

	if as.PeerConnection != nil {
		if err := as.PeerConnection.Close(); err != nil {
			fmt.Println("Error closing peer connection:", err)
		} else {
			fmt.Println("Peer connection closed successfully.")
		}
		as.PeerConnection = nil
	}

	if as.GStreamerPipeline != nil {
		as.GStreamerPipeline.Stop()
		as.GStreamerPipeline = nil
	}

	fmt.Println("All processes have been stopped.")
}
