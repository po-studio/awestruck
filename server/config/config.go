package config

type Config struct {
	Environment     string
	OpenAIAPIKey    string
	AwestruckAPIKey string
	TurnServer      string
}

var globalConfig *Config

func Init(environment string, awestruckAPIKey string, openaiAPIKey string, turnServer string) {
	if environment == "" {
		panic("AWESTRUCK_ENV is required but was empty")
	}
	if awestruckAPIKey == "" {
		panic("AWESTRUCK_API_KEY is required but was empty")
	}
	if openaiAPIKey == "" {
		panic("OPENAI_API_KEY is required but was empty")
	}
	if turnServer == "" {
		panic("TURN_SERVER is required but was empty")
	}

	globalConfig = &Config{
		Environment:     environment,
		OpenAIAPIKey:    openaiAPIKey,
		AwestruckAPIKey: awestruckAPIKey,
		TurnServer:      turnServer,
	}
}

func Get() *Config {
	return globalConfig
}

func ValidateAwestruckAPIKey(key string) bool {
	if globalConfig == nil {
		return false
	}
	return key == globalConfig.AwestruckAPIKey
}

func GetTurnServer() string {
	if globalConfig == nil {
		return ""
	}
	return globalConfig.TurnServer
}
