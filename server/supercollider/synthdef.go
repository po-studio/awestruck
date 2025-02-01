package supercollider

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const SuperColliderSynthTemplate = `
SynthDef.new("%s", { |out=0, amp=0.5|
    var sound;
    %s
    Out.ar(out, sound * amp);
}).writeDefFile("/app/supercollider/synthdefs");
`

func SaveSynthDef(id, provider, model, coreLogic string) error {
	log.Printf("[SYNTHDEF] Starting to save synthdef for id=%s", id)

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %v", err)
	}

	// Create all necessary directories
	dirs := []string{
		filepath.Join(cwd, "supercollider", "synthdefs"),
		filepath.Join(cwd, "supercollider", "src", provider, model),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0777); err != nil {
			log.Printf("[SYNTHDEF][ERROR] Failed to create directory %s: %v", dir, err)
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
		// Ensure directory has correct permissions
		if err := os.Chmod(dir, 0777); err != nil {
			log.Printf("[SYNTHDEF][WARNING] Failed to set directory permissions %s: %v", dir, err)
		}
	}

	// Write the .scd file
	outputPath := getSynthPath(cwd, provider, model)
	synthdefDir := filepath.Join(cwd, "supercollider", "synthdefs")
	log.Printf("[SYNTHDEF] Writing .scd file to: %s", outputPath)

	synthCode := fmt.Sprintf(SuperColliderSynthTemplate, id, coreLogic)
	if err := os.WriteFile(outputPath, []byte(synthCode), 0666); err != nil {
		log.Printf("[SYNTHDEF][ERROR] Failed to write scd file: %v", err)
		return fmt.Errorf("failed to write scd file: %v", err)
	}

	// Compile the synthdef using compile_synthdef.sh
	scriptPath := filepath.Join(cwd, "supercollider", "compile_synthdef.sh")
	cmd := exec.Command("bash", scriptPath, outputPath, synthdefDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[SYNTHDEF][ERROR] Failed to compile synthdef: %v\nOutput: %s", err, output)
		return fmt.Errorf("failed to compile synthdef: %v", err)
	}

	// Verify the synthdef was created
	synthdefPath := filepath.Join(synthdefDir, id+".scsyndef")
	if _, err := os.Stat(synthdefPath); os.IsNotExist(err) {
		log.Printf("[SYNTHDEF][ERROR] Synthdef file was not created at %s", synthdefPath)
		return fmt.Errorf("synthdef file was not created")
	}

	log.Printf("[SYNTHDEF] Successfully compiled synthdef to %s", synthdefPath)
	return nil
}

func formatTimestamp() string {
	return time.Now().Format("2006_01_02_15_04_05")
}

func getSynthPath(cwd, provider, model string) string {
	timestamp := formatTimestamp()
	return filepath.Join(cwd, "supercollider", "src", provider, model, timestamp+".scd")
}
