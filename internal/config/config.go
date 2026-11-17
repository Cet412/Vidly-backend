package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port         string
	AppEnv       string
	RateLimit    int
	RateWindow   time.Duration
	TempDir      string
	YTDLPPath    string
	MaxURLLength int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:         getEnv("PORT", "8080"),
		AppEnv:       getEnv("APP_ENV", "development"),
		RateLimit:    getEnvInt("RATE_LIMIT", 5),
		TempDir:      getEnv("TEMP_DIR", "./tmp"),
		YTDLPPath:    getEnv("YT_DLP_PATH", "yt-dlp"),
		MaxURLLength: getEnvInt("MAX_URL_LENGTH", 2048),
	}

	var err error
	if cfg.RateWindow, err = getEnvDuration("RATE_WINDOW", time.Hour); err != nil {
		return nil, err
	}
	if cfg.ReadTimeout, err = getEnvDuration("READ_TIMEOUT", 30*time.Second); err != nil {
		return nil, err
	}
	if cfg.WriteTimeout, err = getEnvDuration("WRITE_TIMEOUT", 10*time.Minute); err != nil {
		return nil, err
	}
	if cfg.IdleTimeout, err = getEnvDuration("IDLE_TIMEOUT", 60*time.Second); err != nil {
		return nil, err
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	return time.ParseDuration(v)
}
