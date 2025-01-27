package config

type Config struct {
	Environment     string
	OpenAIAPIKey    string
	AwestruckAPIKey string
	TurnServerHost  string
	TurnUsername    string
	TurnPassword    string
}

var globalConfig *Config

func Init(
	environment string,
	awestruckAPIKey string,
	openaiAPIKey string,
	turnServerHost string,
	turnUsername string,
	turnPassword string,
) {
	if environment == "" {
		panic("AWESTRUCK_ENV is required but was empty")
	}
	if awestruckAPIKey == "" {
		panic("AWESTRUCK_API_KEY is required but was empty")
	}
	if openaiAPIKey == "" {
		panic("OPENAI_API_KEY is required but was empty")
	}
	if turnServerHost == "" {
		panic("TURN_SERVER_HOST is required but was empty")
	}

	globalConfig = &Config{
		Environment:     environment,
		OpenAIAPIKey:    openaiAPIKey,
		AwestruckAPIKey: awestruckAPIKey,
		TurnServerHost:  turnServerHost,
		TurnUsername:    turnUsername,
		TurnPassword:    turnPassword,
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

func GetTurnServerHost() string {
	if globalConfig.TurnServerHost == "" {
		panic("TURN_SERVER_HOST is required but was empty")
	}
	return globalConfig.TurnServerHost
}

func GetTurnUsername() string {
	if globalConfig.TurnUsername == "" {
		panic("TURN_USERNAME is required but was empty")
	}
	return globalConfig.TurnUsername
}

func GetTurnPassword() string {
	if globalConfig.TurnPassword == "" {
		panic("TURN_PASSWORD is required but was empty")
	}
	return globalConfig.TurnPassword
}
