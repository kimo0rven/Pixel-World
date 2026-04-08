package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type AppConfig struct {
	GridSize               int
	ChunkSize              int
	CooldownDuration       time.Duration
	AntiSpamWindow         time.Duration
	MaxMessagesPerWindow   int
	MaxChunkRequestsPerSec int
	SyncEnabled            bool
	SyncInterval           time.Duration
	SyncMinInterval        time.Duration
	SyncMaxInterval        time.Duration
	PixelHistoryLimit      int
	CooldownBypassUIDs     map[string]struct{}
	AllowedOrigins         map[string]struct{}
	AdminAPIKey            string
	IdentitySecret         string
	FirebaseWebAPIKey      string
}

func loadAppConfig() AppConfig {
	cfg := AppConfig{
		GridSize:               readIntEnv("GRID_SIZE", 250),
		ChunkSize:              readIntEnv("CHUNK_SIZE", 25),
		CooldownDuration:       readDurationEnv("COOLDOWN_DURATION", 5*time.Minute),
		AntiSpamWindow:         readDurationEnv("ANTI_SPAM_WINDOW", 2*time.Second),
		MaxMessagesPerWindow:   readIntEnv("MAX_MESSAGES_PER_WINDOW", 50),
		MaxChunkRequestsPerSec: readIntEnv("MAX_CHUNK_REQUESTS_PER_SEC", 4),
		SyncEnabled:            readBoolEnv("SYNC_ENABLED", true),
		SyncInterval:           readDurationEnv("SYNC_INTERVAL", 2*time.Second),
		SyncMinInterval:        readDurationEnv("SYNC_MIN_INTERVAL", 500*time.Millisecond),
		SyncMaxInterval:        readDurationEnv("SYNC_MAX_INTERVAL", 10*time.Second),
		PixelHistoryLimit:      readIntEnv("PIXEL_HISTORY_LIMIT", 8),
		AdminAPIKey:            strings.TrimSpace(os.Getenv("ADMIN_API_KEY")),
		IdentitySecret:         strings.TrimSpace(os.Getenv("IDENTITY_SECRET")),
		FirebaseWebAPIKey:      strings.TrimSpace(os.Getenv("FIREBASE_WEB_API_KEY")),
		CooldownBypassUIDs:     make(map[string]struct{}),
		AllowedOrigins:         make(map[string]struct{}),
	}

	// Optional env override/addition: comma-separated list of UIDs that bypass cooldown.
	// Example: BYPASS_COOLDOWN_UIDS="uid1,uid2"
	for _, uid := range strings.Split(strings.TrimSpace(os.Getenv("BYPASS_COOLDOWN_UIDS")), ",") {
		trimmed := strings.TrimSpace(uid)
		if trimmed == "" {
			continue
		}
		cfg.CooldownBypassUIDs[trimmed] = struct{}{}
	}

	// Optional env list of allowed websocket origins.
	// Example: ALLOWED_ORIGINS="https://example.com,http://localhost:8080"
	for _, origin := range strings.Split(strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS")), ",") {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		cfg.AllowedOrigins[trimmed] = struct{}{}
	}

	if cfg.GridSize <= 0 {
		cfg.GridSize = 200
	}
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 25
	}
	if cfg.MaxMessagesPerWindow <= 0 {
		cfg.MaxMessagesPerWindow = 50
	}
	if cfg.MaxChunkRequestsPerSec <= 0 {
		cfg.MaxChunkRequestsPerSec = 4
	}
	if cfg.CooldownDuration < 0 {
		cfg.CooldownDuration = 30 * time.Minute
	}
	if cfg.AntiSpamWindow <= 0 {
		cfg.AntiSpamWindow = 2 * time.Second
	}
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 2 * time.Second
	}
	if cfg.SyncMinInterval <= 0 {
		cfg.SyncMinInterval = 500 * time.Millisecond
	}
	if cfg.SyncMaxInterval <= 0 {
		cfg.SyncMaxInterval = 10 * time.Second
	}
	if cfg.SyncMinInterval > cfg.SyncMaxInterval {
		cfg.SyncMinInterval, cfg.SyncMaxInterval = cfg.SyncMaxInterval, cfg.SyncMinInterval
	}
	if cfg.SyncInterval < cfg.SyncMinInterval {
		cfg.SyncInterval = cfg.SyncMinInterval
	}
	if cfg.SyncInterval > cfg.SyncMaxInterval {
		cfg.SyncInterval = cfg.SyncMaxInterval
	}
	if cfg.PixelHistoryLimit <= 0 {
		cfg.PixelHistoryLimit = 8
	}

	return cfg
}

func readIntEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func readDurationEnv(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return v
}

func readBoolEnv(name string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

