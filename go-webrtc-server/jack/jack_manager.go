package jack

import (
	"fmt"
	"os/exec"
	"strings"
)

func DisconnectJackPorts(appSessionId string) error {
	// Get all current connections
	cmd := exec.Command("jack_lsp", "-c")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error getting JACK connections: %w", err)
	}

	// Look for connections to our session's webrtc ports
	connections := strings.Split(string(output), "\n")
	var disconnectErrors []string

	for i, line := range connections {
		// Skip empty lines
		if line == "" {
			continue
		}

		// If we find a webrtc port for our session
		if strings.Contains(line, fmt.Sprintf("webrtc-server:in_%s", appSessionId)) ||
			strings.Contains(line, fmt.Sprintf("webrtc-server-\\d+:in_%s", appSessionId)) {
			// Look at the previous lines for connected ports
			for j := i - 1; j >= 0; j-- {
				if strings.HasPrefix(connections[j], "   ") {
					// This is a connected port
					connectedPort := strings.TrimSpace(connections[j])
					webrtcPort := strings.TrimSpace(line)

					if err := disconnectPort(connectedPort, webrtcPort); err != nil {
						disconnectErrors = append(disconnectErrors, err.Error())
					}
				} else {
					// We've reached the previous main port entry
					break
				}
			}
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
