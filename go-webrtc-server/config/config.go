package config

import (
	"os"
)

type Config struct {
	Environment  string
	OpenAIAPIKey string
	APIKey       string
}

var globalConfig *Config

func Init(environment string, openaiAPIKey string) {
	if openaiAPIKey == "" {
		panic("OpenAI API key is required but was empty")
	}

	apiKey := os.Getenv("AWESTRUCK_API_KEY")
	if apiKey == "" {
		panic("AWESTRUCK_API_KEY is required but was empty")
	}

	globalConfig = &Config{
		Environment:  environment,
		OpenAIAPIKey: openaiAPIKey,
		APIKey:       apiKey,
	}
}

func Get() *Config {
	return globalConfig
}

func ValidateAPIKey(key string) bool {
	if globalConfig == nil {
		return false
	}
	return key == globalConfig.APIKey
}
