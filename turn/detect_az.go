package turn

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// why we need az detection:
// - each az has its own elastic ip
// - tasks can run in either az
// - ensures correct external ip is used for nat traversal
func detectAvailabilityZone() (string, error) {
	// AWS metadata service v2 requires a token first
	tokenReq, err := http.NewRequest("PUT", "http://169.254.169.254/latest/api/token", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %v", err)
	}
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "60")

	client := &http.Client{Timeout: 5 * time.Second}
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("failed to get metadata token: %v", err)
	}
	defer tokenResp.Body.Close()

	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata token request failed with status %d", tokenResp.StatusCode)
	}

	token, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read metadata token: %v", err)
	}

	// Use token to get availability zone
	azReq, err := http.NewRequest("GET", "http://169.254.169.254/latest/meta-data/placement/availability-zone", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create az request: %v", err)
	}
	azReq.Header.Set("X-aws-ec2-metadata-token", string(token))

	azResp, err := client.Do(azReq)
	if err != nil {
		return "", fmt.Errorf("failed to get availability zone: %v", err)
	}
	defer azResp.Body.Close()

	if azResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("az request failed with status %d", azResp.StatusCode)
	}

	az, err := io.ReadAll(azResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read availability zone: %v", err)
	}

	return string(az), nil
}

// why we need external ip detection:
// - maps az to correct elastic ip
// - handles both production and development
// - provides fallback for local testing
func GetExternalIP() (string, error) {
	if os.Getenv("AWESTRUCK_ENV") != "production" {
		log.Printf("[TURN] Development environment, using default external IP")
		return os.Getenv("EXTERNAL_IP"), nil
	}

	az, err := detectAvailabilityZone()
	if err != nil {
		return "", fmt.Errorf("failed to detect availability zone: %v", err)
	}

	log.Printf("[TURN] Detected availability zone: %s", az)

	// Get the appropriate IP based on AZ
	if strings.HasSuffix(az, "a") {
		externalIP := os.Getenv("EXTERNAL_IP_AZ1")
		log.Printf("[TURN] Using AZ1 external IP: %s", externalIP)
		return externalIP, nil
	} else if strings.HasSuffix(az, "b") {
		externalIP := os.Getenv("EXTERNAL_IP_AZ2")
		log.Printf("[TURN] Using AZ2 external IP: %s", externalIP)
		return externalIP, nil
	}

	return "", fmt.Errorf("unexpected availability zone format: %s", az)
}
