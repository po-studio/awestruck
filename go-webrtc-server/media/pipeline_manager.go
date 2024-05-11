package media

import (
	"log"

	"github.com/pion/webrtc/v3"
	gst "github.com/po-studio/go-webrtc-server/internal/gstreamer-src"
)

// PipelineManager manages the lifecycle of GStreamer pipelines
type PipelineManager struct {
	Pipelines map[*webrtc.TrackLocalStaticSample]*gst.Pipeline
}

// NewPipelineManager creates a new instance of a PipelineManager
func NewPipelineManager() *PipelineManager {
	return &PipelineManager{
		Pipelines: make(map[*webrtc.TrackLocalStaticSample]*gst.Pipeline),
	}
}

// CreatePipeline sets up a new GStreamer pipeline for the given track
func (pm *PipelineManager) CreatePipeline(track *webrtc.TrackLocalStaticSample, codec string, src string) error {
	log.Println("Creating pipeline...")
	pipeline, err := gst.CreatePipeline(codec, []*webrtc.TrackLocalStaticSample{track}, src)
	if err != nil {
		log.Printf("Failed to create pipeline: %v\n", err)
		return err
	}
	pm.Pipelines[track] = pipeline
	return nil
}

// StartPipeline starts the pipeline associated with the given track
func (pm *PipelineManager) StartPipeline(track *webrtc.TrackLocalStaticSample) error {
	pipeline, exists := pm.Pipelines[track]
	if !exists {
		log.Println("No pipeline exists for this track")
		return nil // Or return an error depending on how you want to handle this case
	}

	log.Println("Starting pipeline...")
	err := pipeline.Start()
	if err != nil {
		log.Printf("Failed to start pipeline: %v\n", err)
		return err
	}
	return nil
}

// StopPipeline stops the pipeline associated with the given track
func (pm *PipelineManager) StopPipeline(track *webrtc.TrackLocalStaticSample) error {
	pipeline, exists := pm.Pipelines[track]
	if !exists {
		log.Println("No pipeline exists for this track to stop")
		return nil // Or return an error depending on how you want to handle this case
	}

	log.Println("Stopping pipeline...")
	err := pipeline.Stop()
	if err != nil {
		log.Printf("Failed to stop pipeline: %v\n", err)
		return err
	}
	return nil
}

// StopAllPipelines stops all pipelines managed by this PipelineManager
func (pm *PipelineManager) StopAllPipelines() {
	for track, _ := range pm.Pipelines {
		err := pm.StopPipeline(track)
		if err != nil {
			log.Printf("Failed to stop pipeline for track: %v\n", err)
		}
	}
}
