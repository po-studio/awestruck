package synth

import (
	sc "github.com/po-studio/go-webrtc-server/supercollider"
)

type Synth interface {
	Start() error
	Stop() error
	GetPort() int
	SendPlayMessage()
}

func NewSuperColliderSynth(id string) *sc.SuperColliderSynth {
	return &sc.SuperColliderSynth{Id: id}
}
