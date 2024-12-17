package config

type Config struct {
	Environment  string
	OpenAIAPIKey string
	// Add other config fields as needed
}

var globalConfig *Config

func Init(environment string, openaiAPIKey string) {
	if openaiAPIKey == "" {
		panic("OpenAI API key is required but was empty")
	}

	globalConfig = &Config{
		Environment:  environment,
		OpenAIAPIKey: openaiAPIKey,
	}
}

func Get() *Config {
	return globalConfig
}
