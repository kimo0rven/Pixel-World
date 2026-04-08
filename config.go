package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type AppConfig struct {
	GridSize              int
	ChunkSize             int
	CooldownDuration      time.Duration
	AntiSpamWindow        time.Duration
	MaxMessagesPerWindow  int
	MaxChunkRequestsPerSec int
	CooldownBypassUIDs    map[string]struct{}
	AdminAPIKey           string
}

func loadAppConfig() AppConfig {
	cfg := AppConfig{
		GridSize:              readIntEnv("GRID_SIZE", 250),
		ChunkSize:             readIntEnv("CHUNK_SIZE", 25),
		CooldownDuration:      readDurationEnv("COOLDOWN_DURATION", 30*time.Minute),
		AntiSpamWindow:        readDurationEnv("ANTI_SPAM_WINDOW", 2*time.Second),
		MaxMessagesPerWindow:  readIntEnv("MAX_MESSAGES_PER_WINDOW", 50),
		MaxChunkRequestsPerSec: readIntEnv("MAX_CHUNK_REQUESTS_PER_SEC", 4),
		AdminAPIKey:           strings.TrimSpace(os.Getenv("ADMIN_API_KEY")),
		CooldownBypassUIDs: map[string]struct{}{
			"HIA7LC9I3oaTlcuXecOgXESyOx92": {},
		},
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

