package jack

import (
	"fmt"
	"os/exec"
	"strings"
)

func DisconnectJackPorts(appSessionId string, jackClientName string) error {
	// Get all current connections
	cmd := exec.Command("jack_lsp", "-c")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error getting JACK connections: %w", err)
	}

	// Track unique connections to avoid duplicates
	seenConnections := make(map[string]bool)
	connections := strings.Split(string(output), "\n")
	var currentPort string
	var currentConnections []string
	var disconnectErrors []string

	for _, line := range connections {
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "   ") {
			// Handle previous port's connections
			if len(currentConnections) > 0 {
				for _, conn := range currentConnections {
					// Create unique connection identifier
					connKey := fmt.Sprintf("%s->%s", currentPort, conn)
					reverseKey := fmt.Sprintf("%s->%s", conn, currentPort)

					if !seenConnections[connKey] && !seenConnections[reverseKey] {
						seenConnections[connKey] = true
						disconnectErrors = append(disconnectErrors,
							disconnectPortConnections(currentPort, []string{conn}, appSessionId, jackClientName)...)
					}
				}
			}
			currentPort = strings.TrimSpace(line)
			currentConnections = nil
		} else {
			currentConnections = append(currentConnections, strings.TrimSpace(line))
		}
	}

	// Handle the last port's connections
	if len(currentConnections) > 0 {
		for _, conn := range currentConnections {
			connKey := fmt.Sprintf("%s->%s", currentPort, conn)
			reverseKey := fmt.Sprintf("%s->%s", conn, currentPort)

			if !seenConnections[connKey] && !seenConnections[reverseKey] {
				seenConnections[connKey] = true
				disconnectErrors = append(disconnectErrors,
					disconnectPortConnections(currentPort, []string{conn}, appSessionId, jackClientName)...)
			}
		}
	}

	if len(disconnectErrors) > 0 {
		return fmt.Errorf("failed to disconnect some ports: %s", strings.Join(disconnectErrors, "; "))
	}
	return nil
}

func disconnectPortConnections(port string, connections []string, appSessionId string, jackClientName string) []string {
	var errors []string

	// Check if this port needs to be cleaned up
	isWebRTCPort := strings.Contains(port, "webrtc-server") && strings.Contains(port, fmt.Sprintf("in_%s", appSessionId))
	isSuperColliderPort := strings.HasPrefix(port, jackClientName) && strings.Contains(jackClientName, appSessionId)

	if isWebRTCPort || isSuperColliderPort {
		for _, connectedPort := range connections {
			cmd := exec.Command("jack_disconnect", connectedPort, port)
			if err := cmd.Run(); err != nil {
				errors = append(errors, fmt.Errorf("failed to disconnect %s from %s: %w", connectedPort, port, err).Error())
			}
		}
	}

	return errors
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
