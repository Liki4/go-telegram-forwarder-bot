package config

type Config struct {
	ManagerBot    ManagerBotConfig `mapstructure:"manager_bot"`
	Database      DatabaseConfig   `mapstructure:"database"`
	Redis         RedisConfig      `mapstructure:"redis"`
	RateLimit     RateLimitConfig  `mapstructure:"rate_limit"`
	Retry         RetryConfig      `mapstructure:"retry"`
	Log           LogConfig        `mapstructure:"log"`
	Environment   string           `mapstructure:"environment"`
	EncryptionKey string           `mapstructure:"encryption_key"` // Base64 encoded 32-byte key
	Proxy         ProxyConfig      `mapstructure:"proxy"`
}

type ManagerBotConfig struct {
	Token      string  `mapstructure:"token"`
	Superusers []int64 `mapstructure:"superusers"`
}

type DatabaseConfig struct {
	Type string `mapstructure:"type"`
	DSN  string `mapstructure:"dsn"`
}

type RedisConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Address  string `mapstructure:"address"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type RateLimitConfig struct {
	TelegramAPI  int `mapstructure:"telegram_api"`
	GuestMessage int `mapstructure:"guest_message"`
}

type RetryConfig struct {
	MaxAttempts     int `mapstructure:"max_attempts"`
	IntervalSeconds int `mapstructure:"interval_seconds"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Output   string `mapstructure:"output"`
	FilePath string `mapstructure:"file_path"`
}

type ProxyConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	URL      string `mapstructure:"url"`      // Proxy URL, e.g., "http://127.0.0.1:7890" or "socks5://127.0.0.1:1080"
	Username string `mapstructure:"username"` // Optional: proxy username
	Password string `mapstructure:"password"` // Optional: proxy password
}
