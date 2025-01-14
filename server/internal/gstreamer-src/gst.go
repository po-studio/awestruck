// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package gst provides an easy API to create an appsink pipeline
package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0

#include "gst.h"

*/
import "C"

import (
	"fmt"
	"log"
	"sync"
	"time"
	"unsafe"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

// nolint
func init() {
	go C.gstreamer_send_start_mainloop()
}

// Pipeline is a wrapper for a GStreamer Pipeline
type Pipeline struct {
	Pipeline  *C.GstElement
	tracks    []*webrtc.TrackLocalStaticSample
	id        int
	codecName string
	clockRate float32
}

// nolint
var (
	pipelines     = make(map[int]*Pipeline)
	pipelinesLock sync.Mutex
)

const (
	videoClockRate = 90000
	audioClockRate = 48000
	pcmClockRate   = 8000
)

// why we need focused gstreamer logging:
// - track critical pipeline events
// - monitor audio buffer flow
// - identify potential bottlenecks
func logWithTime(format string, v ...interface{}) {
	log.Printf("[%s] %s", time.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), fmt.Sprintf(format, v...))
}

// CreatePipeline creates a GStreamer Pipeline
func CreatePipeline(codecName string, tracks []*webrtc.TrackLocalStaticSample, pipelineSrc string) *Pipeline {
	logWithTime("[GST] Creating pipeline: codec=%s tracks=%d", codecName, len(tracks))

	pipelineStr := "appsink name=appsink"
	var clockRate float32

	switch codecName {
	case "vp8":
		pipelineStr = pipelineSrc + " ! vp8enc error-resilient=partitions keyframe-max-dist=10 auto-alt-ref=true cpu-used=5 deadline=1 ! " + pipelineStr
		clockRate = videoClockRate

	case "vp9":
		pipelineStr = pipelineSrc + " ! vp9enc ! " + pipelineStr
		clockRate = videoClockRate

	case "h264":
		pipelineStr = pipelineSrc + " ! video/x-raw,format=I420 ! x264enc speed-preset=ultrafast tune=zerolatency key-int-max=20 ! video/x-h264,stream-format=byte-stream ! " + pipelineStr
		clockRate = videoClockRate

	case "opus":
		pipelineStr = pipelineSrc + " ! opusenc frame-size=20 complexity=10 bitrate=128000 ! " + pipelineStr
		clockRate = audioClockRate
		logWithTime("[GST] Configured Opus encoder: rate=%f", clockRate)

	case "g722":
		pipelineStr = pipelineSrc + " ! avenc_g722 ! " + pipelineStr
		clockRate = audioClockRate

	case "pcmu":
		pipelineStr = pipelineSrc + " ! audio/x-raw, rate=8000 ! mulawenc ! " + pipelineStr
		clockRate = pcmClockRate

	case "pcma":
		pipelineStr = pipelineSrc + " ! audio/x-raw, rate=8000 ! alawenc ! " + pipelineStr
		clockRate = pcmClockRate

	default:
		logWithTime("[GST][ERROR] Unsupported codec: %s", codecName)
		panic("Unhandled codec " + codecName)
	}

	pipelineStrUnsafe := C.CString(pipelineStr)
	defer C.free(unsafe.Pointer(pipelineStrUnsafe))

	pipelinesLock.Lock()
	defer pipelinesLock.Unlock()

	pipeline := &Pipeline{
		Pipeline:  C.gstreamer_send_create_pipeline(pipelineStrUnsafe),
		tracks:    tracks,
		id:        len(pipelines),
		codecName: codecName,
		clockRate: clockRate,
	}

	if pipeline.Pipeline == nil {
		logWithTime("[GST][ERROR] Pipeline creation failed")
		return nil
	}

	pipelines[pipeline.id] = pipeline
	return pipeline
}

// Start starts the GStreamer Pipeline
func (p *Pipeline) Start() {
	logWithTime("[GST] Starting pipeline %d", p.id)
	C.gstreamer_send_start_pipeline(p.Pipeline, C.int(p.id))
}

// Stop stops the GStreamer Pipeline
func (p *Pipeline) Stop() {
	logWithTime("[GST] Stopping pipeline %d", p.id)
	C.gstreamer_send_stop_pipeline(p.Pipeline)
}

//export goHandlePipelineBuffer
func goHandlePipelineBuffer(buffer unsafe.Pointer, bufferLen C.int, duration C.int, pipelineID C.int) {
	pipelinesLock.Lock()
	pipeline, ok := pipelines[int(pipelineID)]
	pipelinesLock.Unlock()

	if ok {
		data := C.GoBytes(buffer, bufferLen)
		dur := time.Duration(duration)

		// Log buffer details only every 10000 samples to reduce noise
		// if int(pipelineID)%(48000*600) == 0 {
		// 	logWithTime("[GST] Buffer stats: pipeline=%d size=%d duration=%v",
		// 		pipelineID, len(data), dur)
		// }

		for i, t := range pipeline.tracks {
			if err := t.WriteSample(media.Sample{Data: data, Duration: dur}); err != nil {
				logWithTime("[GST][ERROR] Track %d write failed: %v", i, err)
				panic(err)
			}
		}
	} else {
		logWithTime("[GST][ERROR] No pipeline found for id %d", int(pipelineID))
	}
	C.free(buffer)
}
