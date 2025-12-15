package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath("$HOME/.go-telegram-forwarder-bot")

	// Set default values
	setDefaults()

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Validate config
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

func setDefaults() {
	viper.SetDefault("manager_bot.token", "")
	viper.SetDefault("manager_bot.superusers", []int64{})

	viper.SetDefault("database.type", "sqlite")
	viper.SetDefault("database.dsn", "bot.db")

	viper.SetDefault("redis.enabled", false)
	viper.SetDefault("redis.address", "localhost:6379")
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)

	viper.SetDefault("rate_limit.telegram_api", 25)
	viper.SetDefault("rate_limit.guest_message", 1)

	viper.SetDefault("retry.max_attempts", 10)
	viper.SetDefault("retry.interval_seconds", 30)

	viper.SetDefault("log.level", "debug")
	viper.SetDefault("log.output", "stdout")
	viper.SetDefault("log.file_path", "bot.log")

	viper.SetDefault("environment", "development")
	viper.SetDefault("encryption_key", "") // Must be set in production

	viper.SetDefault("proxy.enabled", false)
	viper.SetDefault("proxy.url", "")
	viper.SetDefault("proxy.username", "")
	viper.SetDefault("proxy.password", "")

	viper.SetDefault("ad_filter.enabled", false)
}

func validate(cfg *Config) error {
	if cfg.ManagerBot.Token == "" {
		return fmt.Errorf("manager_bot.token is required")
	}

	if len(cfg.ManagerBot.Superusers) == 0 {
		return fmt.Errorf("manager_bot.superusers must have at least one superuser")
	}

	if cfg.Database.Type == "" {
		return fmt.Errorf("database.type is required")
	}

	if cfg.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}

	if cfg.Redis.Enabled && cfg.Redis.Address == "" {
		return fmt.Errorf("redis.address is required when redis is enabled")
	}

	if cfg.RateLimit.TelegramAPI <= 0 {
		return fmt.Errorf("rate_limit.telegram_api must be greater than 0")
	}

	if cfg.RateLimit.GuestMessage <= 0 {
		return fmt.Errorf("rate_limit.guest_message must be greater than 0")
	}

	if cfg.Retry.MaxAttempts <= 0 {
		return fmt.Errorf("retry.max_attempts must be greater than 0")
	}

	if cfg.Retry.IntervalSeconds <= 0 {
		return fmt.Errorf("retry.interval_seconds must be greater than 0")
	}

	if cfg.Proxy.Enabled && cfg.Proxy.URL == "" {
		return fmt.Errorf("proxy.url is required when proxy is enabled")
	}

	// Validate log output
	validOutputs := map[string]bool{
		"stdout": true,
		"file":   true,
		"both":   true,
	}
	if !validOutputs[cfg.Log.Output] {
		return fmt.Errorf("log.output must be one of: stdout, file, both")
	}

	// If output is file or both, file_path is required
	if (cfg.Log.Output == "file" || cfg.Log.Output == "both") && cfg.Log.FilePath == "" {
		return fmt.Errorf("log.file_path is required when log.output is file or both")
	}

	return nil
}

func LoadFromFile(filePath string) (*Config, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("error getting absolute path: %w", err)
	}

	viper.SetConfigFile(absPath)

	// Set defaults
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

func SaveExampleConfig(filePath string) error {
	exampleConfig := `manager_bot:
  token: "YOUR_MANAGER_BOT_TOKEN"
  superusers: [123456789, 987654321]

database:
  type: "sqlite"
  dsn: "bot.db"

redis:
  enabled: false
  address: "localhost:6379"
  password: ""
  db: 0

rate_limit:
  telegram_api: 25
  guest_message: 1

retry:
  max_attempts: 10
  interval_seconds: 30

log:
  level: "debug"
  output: "stdout"
  file_path: "bot.log"

environment: "development"
`

	return os.WriteFile(filePath, []byte(exampleConfig), 0644)
}
