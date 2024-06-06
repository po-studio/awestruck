package jack

import (
	"fmt"
	"os/exec"
	"strings"
)

func DisconnectJackPorts(appSessionId string) error {
	gstJackPorts, err := GetGStreamerJackPorts(appSessionId)
	if err != nil {
		return fmt.Errorf("error finding JACK ports: %w", err)
	}

	var disconnectErrors []string
	for _, gstJackPort := range gstJackPorts {
		// NOTE 1: this is hardcoded for SuperCollider, but move away from the
		// coupling with SuperCollider so that we can configure for multiple synths
		//
		// NOTE 2: this is a bug – this is the auto-generated channel name for only
		// the first SC connected output port
		if err := disconnectPort("SuperCollider:out_1", gstJackPort); err != nil {
			disconnectErrors = append(disconnectErrors, err.Error())
		}
	}

	if len(disconnectErrors) > 0 {
		return fmt.Errorf("failed to disconnect some ports: %s", strings.Join(disconnectErrors, "; "))
	}
	return nil
}

func GetGStreamerJackPorts(appSessionId string) ([]string, error) {
	cmd := exec.Command("jack_lsp")
	var out strings.Builder
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("error listing JACK ports: %w", err)
	}

	ports := strings.Split(out.String(), "\n")
	var gstJackPorts []string
	prefix := "webrtc-server"

	for _, port := range ports {
		if strings.HasPrefix(port, prefix) && strings.Contains(port, appSessionId) {
			gstJackPorts = append(gstJackPorts, port)
		}
	}

	return gstJackPorts, nil
}

func disconnectPort(outputPort, inputPort string) error {
	cmd := exec.Command("jack_disconnect", outputPort, inputPort)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to disconnect %s from %s: %w", outputPort, inputPort, err)
	}
	return nil
}
