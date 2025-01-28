package config

import (
	"fmt"
	"os"
)

// supported environments
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

type Config struct {
	Environment     string
	OpenAIAPIKey    string
	AwestruckAPIKey string
	TurnServerHost  string
	TurnUsername    string
	TurnPassword    string
	TurnMinPort     string
	TurnMaxPort     string
}

var globalConfig *Config

// loads environment variables and applies defaults
func LoadFromEnv() Config {
	env := os.Getenv("AWESTRUCK_ENV")
	if env == "" {
		env = EnvDevelopment
	}

	return Config{
		Environment:     env,
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		AwestruckAPIKey: os.Getenv("AWESTRUCK_API_KEY"),
		TurnServerHost:  os.Getenv("TURN_SERVER_HOST"),
		TurnUsername:    os.Getenv("TURN_USERNAME"),
		TurnPassword:    os.Getenv("TURN_PASSWORD"),
		TurnMinPort:     os.Getenv("TURN_MIN_PORT"),
		TurnMaxPort:     os.Getenv("TURN_MAX_PORT"),
	}
}

// validates the configuration
func (c *Config) validate() error {
	// check for empty required fields
	fields := map[string]string{
		"OpenAIAPIKey":    c.OpenAIAPIKey,
		"AwestruckAPIKey": c.AwestruckAPIKey,
		"TurnServerHost":  c.TurnServerHost,
		"TurnUsername":    c.TurnUsername,
		"TurnPassword":    c.TurnPassword,
		"TurnMinPort":     c.TurnMinPort,
		"TurnMaxPort":     c.TurnMaxPort,
	}

	for field, value := range fields {
		if value == "" {
			return fmt.Errorf("%s is required but was empty", field)
		}
	}

	// validate environment
	switch c.Environment {
	case EnvDevelopment, EnvProduction:
		// valid
	default:
		return fmt.Errorf("environment must be either %s or %s, got: %s",
			EnvDevelopment, EnvProduction, c.Environment)
	}

	return nil
}

// initializes the global configuration
func Init(cfg Config) error {
	if err := cfg.validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	globalConfig = &cfg
	return nil
}

// gets the current configuration
func Get() *Config {
	if globalConfig == nil {
		panic("config is not initialized")
	}
	return globalConfig
}

// validates an API key against the configured one
func ValidateAwestruckAPIKey(key string) bool {
	return globalConfig != nil && key == globalConfig.AwestruckAPIKey
}
