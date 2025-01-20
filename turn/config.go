package turn

import (
	"flag"
	"fmt"
	"log"
	"os"
)

// why we need centralized config:
// - validates environment at startup
// - ensures all required vars are present
// - provides type safety for config values
type Config struct {
	// Server configuration
	Port       int
	HealthPort int
	Realm      string

	// Environment
	Environment string // "production" or "development"
	ExternalIP  string // Required in production, optional in development

	// Credentials
	Credentials struct {
		Username string
		Password string
	}
}

// why we need config validation:
// - fails fast if environment is invalid
// - provides clear error messages
// - ensures consistent configuration
func (cfg *Config) Validate() error {
	// Validate environment
	if cfg.Environment != "production" && cfg.Environment != "development" {
		return fmt.Errorf("AWESTRUCK_ENV must be either 'production' or 'development', got: %q", cfg.Environment)
	}

	// Validate realm
	if cfg.Realm == "" {
		if cfg.Environment == "production" {
			cfg.Realm = "awestruck.io"
		} else {
			cfg.Realm = "localhost"
		}
	}

	// Validate external IP in production
	if cfg.Environment == "production" {
		if cfg.ExternalIP == "" {
			return fmt.Errorf("EXTERNAL_IP must be set in production environment")
		}
	}

	// Validate credentials
	if cfg.Credentials.Username == "" {
		return fmt.Errorf("TURN_USERNAME must be set")
	}
	if cfg.Credentials.Password == "" {
		return fmt.Errorf("TURN_PASSWORD must be set")
	}

	return nil
}

// why we need a config loader:
// - centralizes environment variable handling
// - provides consistent defaults
// - enables command-line overrides
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Credentials: Credentials{}, // Initialize as struct value, not pointer
	}

	// Parse command line flags
	flag.IntVar(&cfg.Port, "port", 3478, "UDP port for TURN/STUN")
	flag.IntVar(&cfg.HealthPort, "health-port", 3479, "TCP port for health checks")
	flag.Parse()

	// Load environment variables
	cfg.Environment = os.Getenv("AWESTRUCK_ENV")
	if cfg.Environment == "" {
		cfg.Environment = "development"
	}

	cfg.Realm = os.Getenv("TURN_REALM")
	cfg.ExternalIP = os.Getenv("EXTERNAL_IP")

	// Load credentials with defaults
	cfg.Credentials.Username = os.Getenv("TURN_USERNAME")
	if cfg.Credentials.Username == "" {
		cfg.Credentials.Username = "user"
		log.Printf("[CONFIG] Using default TURN username: %s", cfg.Credentials.Username)
	}
	cfg.Credentials.Password = os.Getenv("TURN_PASSWORD")
	if cfg.Credentials.Password == "" {
		cfg.Credentials.Password = "pass"
		log.Printf("[CONFIG] Using default TURN password: %s", cfg.Credentials.Password)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}

	// Log configuration (excluding sensitive data)
	log.Printf("[CONFIG] Environment: %s", cfg.Environment)
	log.Printf("[CONFIG] Realm: %s", cfg.Realm)
	log.Printf("[CONFIG] TURN/STUN Port: %d", cfg.Port)
	log.Printf("[CONFIG] Health Port: %d", cfg.HealthPort)
	log.Printf("[CONFIG] External IP: %s", cfg.ExternalIP)
	log.Printf("[CONFIG] TURN Username: %s", cfg.Credentials.Username)

	return cfg, nil
}
