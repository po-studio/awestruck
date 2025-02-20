package utils

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// SCSynthDefDirectory = "/app/sc/synthdefs"
	SCSynthDefDirectory = "/app/sc/synthdefs_ai"
)

func GetRandomSynthDefName() (string, error) {
	files, err := os.ReadDir(SCSynthDefDirectory)
	if err != nil {
		return "", err
	}

	var synthDefs []string
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".scsyndef" {
			baseName := strings.TrimSuffix(file.Name(), ".scsyndef")
			synthDefs = append(synthDefs, baseName)
		}
	}

	if len(synthDefs) == 0 {
		return "", fmt.Errorf("no .scsyndef files found in %s", SCSynthDefDirectory)
	}

	randTimeSeed := rand.NewSource(time.Now().UnixNano())
	rnd := rand.New(randTimeSeed)

	chosenFile := synthDefs[rnd.Intn(len(synthDefs))]

	return chosenFile, nil
}

// TODO reserve port ranges for specific processes like scsynth
func FindAvailablePort() (int, error) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenUDP("udp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.LocalAddr().(*net.UDPAddr).Port, nil
}
