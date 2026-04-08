package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	firebase "firebase.google.com/go/v4"
	firebaseauth "firebase.google.com/go/v4/auth"
	"firebase.google.com/go/v4/db"
	"github.com/gorilla/websocket"
	"google.golang.org/api/option"
)

type PixelMessage struct {
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Color       string `json:"color"`
	Username    string `json:"username,omitempty"`
	OwnerUserID string `json:"ownerUserId,omitempty"`
	UpdatedAt   int64  `json:"updatedAt,omitempty"`
}

type ClientMessage struct {
	Type          string           `json:"type"`
	UserID        string           `json:"userId,omitempty"`
	Nickname      string           `json:"nickname,omitempty"`
	Pixel         *PixelMessage    `json:"pixel,omitempty"`
	Chunks        []ChunkCoord     `json:"chunks,omitempty"`
	Viewport      *ViewportPayload `json:"viewport,omitempty"`
	ClientVersion string           `json:"clientVersion,omitempty"`
	IdentityToken string           `json:"identityToken,omitempty"`
	FirebaseIDToken string         `json:"firebaseIdToken,omitempty"`
}

type ServerMessage struct {
	Type         string         `json:"type"`
	Pixel        *PixelMessage  `json:"pixel,omitempty"`
	Pixels       []PixelMessage `json:"pixels,omitempty"`
	UserID       string         `json:"userId,omitempty"`
	Nickname      string         `json:"nickname,omitempty"`
	IdentityToken string         `json:"identityToken,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	RetryAfterMs int64          `json:"retryAfterMs,omitempty"`
	NowMs        int64          `json:"nowMs,omitempty"`
	CooldownMs      int64 `json:"cooldownMs,omitempty"`
	CooldownUntilMs int64 `json:"cooldownUntilMs,omitempty"`
	CooldownBypass  bool  `json:"cooldownBypass,omitempty"`
	GridSize        int   `json:"gridSize,omitempty"`
	ChunkSize       int   `json:"chunkSize,omitempty"`
	Stats        *UserStats     `json:"stats,omitempty"`
	Leaderboard  []LeaderboardEntry `json:"leaderboard,omitempty"`
	ColorStats   map[string]int64   `json:"colorStats,omitempty"`
	Chunk        *ChunkPayload  `json:"chunk,omitempty"`
	History      []PixelHistoryEntry `json:"history,omitempty"`
	Message      string         `json:"message,omitempty"`
}

type ClientSession struct {
	Conn              *websocket.Conn
	ConnID            string
	Authenticated     bool
	UserID            string
	Nickname          string
	WindowStart       time.Time
	WindowMsgCount    int
	LastPlacementAt   time.Time
	LastChunkAt       time.Time
	CooldownUntilMs   int64
	SubscribedChunks  map[string]struct{} // Format: "cx_cy"
	ClientIP          string              // Track client IP for rate limiting
}

type PlacementEvent struct {
	Session *ClientSession
	Pixel   PixelMessage
}

type UserStats struct {
	UserID              string           `json:"userId"`
	Nickname            string           `json:"nickname"`
	TotalPlacements     int64            `json:"totalPlacements"`
	PlacementsToday     int64            `json:"placementsToday"`
	LastPlacementAt     int64            `json:"lastPlacementAt"`
	LastPlacementDay    string           `json:"lastPlacementDay,omitempty"`
	LastNicknameChangeAt int64           `json:"lastNicknameChangeAt,omitempty"`
	ColorCounts         map[string]int64 `json:"colorCounts,omitempty"`
}

type LeaderboardEntry struct {
	UserID          string `json:"userId"`
	Nickname        string `json:"nickname"`
	TotalPlacements int64  `json:"totalPlacements"`
	PlacementsToday int64  `json:"placementsToday"`
}

type ChunkCoord struct {
	CX int `json:"cx"`
	CY int `json:"cy"`
}

type ChunkPayload struct {
	CX     int            `json:"cx"`
	CY     int            `json:"cy"`
	Pixels []PixelMessage `json:"pixels"`
}

type ViewportPayload struct {
	CenterX int     `json:"centerX"`
	CenterY int     `json:"centerY"`
	Zoom    float64 `json:"zoom"`
	Radius  int     `json:"radius"`
}

type AdminCooldownRequest struct {
	UserID          string `json:"userId"`
	CooldownUntilMs *int64 `json:"cooldownUntilMs,omitempty"`
	Clear           bool   `json:"clear,omitempty"`
}

type AdminModerationRequest struct {
	UserID       string `json:"userId"`
	MuteUntilMs  *int64 `json:"muteUntilMs,omitempty"`
	FreezeUntilMs *int64 `json:"freezeUntilMs,omitempty"`
	ClearMute    bool   `json:"clearMute,omitempty"`
	ClearFreeze  bool   `json:"clearFreeze,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type AdminRollbackWindowRequest struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	UserID  string `json:"userId,omitempty"`
}

type PixelHistoryEntry struct {
	UserID    string `json:"userId,omitempty"`
	Username  string `json:"username,omitempty"`
	Color     string `json:"color,omitempty"`
	Action    string `json:"action,omitempty"`
	UpdatedAt int64  `json:"updatedAt"`
}

type UserModerationState struct {
	MuteUntilMs   int64  `json:"muteUntilMs,omitempty"`
	FreezeUntilMs int64  `json:"freezeUntilMs,omitempty"`
	Reason        string `json:"reason,omitempty"`
	UpdatedAt     int64  `json:"updatedAt,omitempty"`
}

type SuspiciousActivity struct {
	UserID       string         `json:"userId,omitempty"`
	Nickname     string         `json:"nickname,omitempty"`
	ClientIP     string         `json:"clientIp,omitempty"`
	Score        int64          `json:"score"`
	LastEvent    string         `json:"lastEvent,omitempty"`
	LastAt       int64          `json:"lastAt"`
	EventCounts  map[string]int64 `json:"eventCounts,omitempty"`
}

var nicknameRegex = regexp.MustCompile(`^[a-zA-Z0-9_ ]{3,20}$`)

// Whitelist of allowed colors (hex format without #) — r/Place 2022 32-color palette
var allowedColors = map[string]bool{
	"6d001a": true, // Darkest Red
	"be0039": true, // Dark Red
	"ff4500": true, // Red
	"ffa800": true, // Orange
	"ffd635": true, // Yellow
	"fff8b8": true, // Pale Yellow
	"00a368": true, // Dark Green
	"00cc78": true, // Green
	"7eed56": true, // Light Green
	"00756f": true, // Dark Teal
	"009eaa": true, // Teal
	"00ccc0": true, // Light Teal
	"2450a4": true, // Dark Blue
	"3690ea": true, // Blue
	"51e9f4": true, // Light Blue
	"493ac1": true, // Indigo
	"6a5cff": true, // Periwinkle
	"94b3ff": true, // Lavender
	"811e9f": true, // Dark Purple
	"b44ac0": true, // Purple
	"e4abff": true, // Pale Purple
	"de107f": true, // Magenta
	"ff3881": true, // Pink
	"ff99aa": true, // Light Pink
	"6d482f": true, // Dark Brown
	"9c6926": true, // Brown
	"ffb470": true, // Beige
	"000000": true, // Black
	"515252": true, // Dark Gray
	"898d90": true, // Gray
	"d4d7d9": true, // Light Gray
	"ffffff": true, // White
}

// IP-based rate limiting: track requests per IP
type IPRateLimit struct {
	count     int
	windowEnd time.Time
}

var ipRateLimits = make(map[string]*IPRateLimit)
var ipRateLimitsMutex sync.Mutex

// Constants for rate limiting
const (
	IPRateLimitWindow  = 10 * time.Second
	IPRateLimitMax     = 100 // Max requests per IP per window
)

func isAllowedWebSocketOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// Non-browser clients may not send Origin.
		return true
	}
	if len(appConfig.AllowedOrigins) > 0 {
		_, ok := appConfig.AllowedOrigins[origin]
		return ok
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	requestHost := strings.ToLower(r.Host)
	if strings.EqualFold(u.Host, requestHost) {
		return true
	}
	if host == "localhost" || host == "127.0.0.1" {
		return true
	}
	return false
}

var upgrader = websocket.Upgrader{CheckOrigin: isAllowedWebSocketOrigin}
var clients = make(map[*websocket.Conn]*ClientSession)
var clientsMutex sync.Mutex
var broadcast = make(chan PlacementEvent, 512)
var pendingWrites = make(chan PlacementEvent, 256)
var pendingDeletes = make(chan struct{ X int; Y int }, 256) // Channel for pixel deletions

var canvasState = make(map[string]PixelMessage)
var stateMutex sync.RWMutex
var lastSyncAt int64 = 0 // Track last sync timestamp for change-only sync

var dbClient *db.Client
var authClient *firebaseauth.Client
var ctx = context.Background()
var userStats = make(map[string]*UserStats)
var statsMutex sync.RWMutex
var appConfig AppConfig
var cellHistory = make(map[string][]PixelHistoryEntry)
var historyMutex sync.RWMutex
var userModeration = make(map[string]UserModerationState)
var moderationMutex sync.RWMutex
var suspiciousByUser = make(map[string]*SuspiciousActivity)
var suspiciousMutex sync.Mutex

//go:embed index.html admin.html static/**
var embeddedAssets embed.FS

func loadDotEnv() {
	b, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, rawLine := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)
		if key == "" || val == "" {
			continue
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}

func firebaseCredentialOption() (option.ClientOption, error) {
	// Prefer env JSON for cloud deploys (Render/Railway/etc).
	if rawJSON := strings.TrimSpace(os.Getenv("FIREBASE_SERVICE_ACCOUNT_JSON")); rawJSON != "" {
		return option.WithCredentialsJSON([]byte(rawJSON)), nil
	}

	// Local dev fallback.
	const credentialsPath = "serviceAccountKey.json"
	if _, err := os.Stat(credentialsPath); err == nil {
		return option.WithCredentialsFile(credentialsPath), nil
	}

	return nil, fmt.Errorf(
		"missing Firebase credentials: set FIREBASE_SERVICE_ACCOUNT_JSON or provide %s",
		credentialsPath,
	)
}

func main() {
	loadDotEnv()
	appConfig = loadAppConfig()

	// Auto-generate identity secret if not provided via env.
	// WARNING: tokens are invalidated on restart when this happens.
	// Set IDENTITY_SECRET env var to a stable secret in production.
	if appConfig.IdentitySecret == "" {
		b := make([]byte, 32)
		if _, randErr := rand.Read(b); randErr == nil {
			appConfig.IdentitySecret = hex.EncodeToString(b)
		}
		log.Println("WARNING: IDENTITY_SECRET not set. User identity tokens will reset on every server restart. Set IDENTITY_SECRET to a stable value in production.")
	}

	// 1. Handle Firebase Credentials
	opt, err := firebaseCredentialOption()
	if err != nil {
		log.Fatal(err)
	}
	
	dbURL := strings.TrimSpace(os.Getenv("FIREBASE_DATABASE_URL"))
	if dbURL == "" || strings.Contains(dbURL, "YOUR_PROJECT") {
		log.Fatal("Missing/invalid FIREBASE_DATABASE_URL env var (required to persist pixels).")
	}

	config := &firebase.Config{
		DatabaseURL: dbURL,
	}

	app, err := firebase.NewApp(ctx, config, opt)
	if err != nil {
		log.Fatalf("Error initializing app: %v\n", err)
	}

	dbClient, err = app.Database(ctx)
	if err != nil {
		log.Fatalf("Error initializing database: %v\n", err)
	}
	authClient, err = app.Auth(ctx)
	if err != nil {
		log.Fatalf("Error initializing auth client: %v\n", err)
	}
	log.Println("Successfully connected to Firebase via Admin SDK!")

	loadInitialState()
	loadUserStats()
	loadModerationState()

	// 2. Define API & WebSocket Routes
	http.HandleFunc("/ws", handleConnections)
	http.HandleFunc("/app-config", handleAppConfig)
	http.HandleFunc("/admin/cooldown", handleAdminCooldown)
	http.HandleFunc("/admin/moderation", handleAdminModeration)
	http.HandleFunc("/admin/rollback-window", handleAdminRollbackWindow)
	http.HandleFunc("/admin/suspicious", handleAdminSuspicious)
	http.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin.html", http.StatusTemporaryRedirect)
	})
	go handleMessages()
	go persistenceWorker() // Start the write queue processor
	go cleanupOldIPLimits() // Start IP rate limit cleanup
	if appConfig.SyncEnabled {
		go syncCanvasStateLoop()
		log.Printf(
			"Canvas sync enabled (base: %s, min: %s, max: %s)",
			appConfig.SyncInterval,
			appConfig.SyncMinInterval,
			appConfig.SyncMaxInterval,
		)
	}

	// 3. Serve Frontend Files from embedded assets (deploy-safe)
	assetsFS, err := fs.Sub(embeddedAssets, ".")
	if err != nil {
		log.Fatalf("failed to mount embedded assets: %v", err)
	}
	http.Handle("/", http.FileServer(http.FS(assetsFS)))

	// 4. Dynamic Port for Render
	// Render assigns a port via the PORT env var; fallback to 8080 for local dev
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Pixel World Server started on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

func syncCanvasStateLoop() {
	currentInterval := appConfig.SyncInterval
	for {
		time.Sleep(currentInterval)
		changedCount, err := syncCanvasStateOnce()
		if err != nil {
			// Back off on read errors.
			currentInterval *= 2
			if currentInterval > appConfig.SyncMaxInterval {
				currentInterval = appConfig.SyncMaxInterval
			}
			continue
		}

		if changedCount > 0 {
			// Speed up briefly while there is activity.
			currentInterval = appConfig.SyncMinInterval
			continue
		}

		// No changes -> gradually back off.
		next := time.Duration(float64(currentInterval) * 1.5)
		if next > appConfig.SyncMaxInterval {
			next = appConfig.SyncMaxInterval
		}
		if next < appConfig.SyncMinInterval {
			next = appConfig.SyncMinInterval
		}
		currentInterval = next
	}
}

func syncCanvasStateOnce() (int, error) {
	var dbPixels map[string]PixelMessage
	if err := dbClient.NewRef("pixels").Get(ctx, &dbPixels); err != nil {
		log.Println("sync: failed to read pixels:", err)
		return 0, err
	}
	if dbPixels == nil {
		dbPixels = make(map[string]PixelMessage)
	}

	changed := make([]PixelMessage, 0, 32)
	stateMutex.Lock()
	defer stateMutex.Unlock()
	
	currentLastSync := lastSyncAt
	newLastSync := time.Now().UnixMilli()
	
	for key, px := range dbPixels {
		// Only process pixels updated after the last sync time
		if px.UpdatedAt > currentLastSync {
			// Ensure pixel has UpdatedAt set
			if px.UpdatedAt == 0 {
				px.UpdatedAt = newLastSync
			}
			canvasState[key] = px
			history := appendCellHistory(key, PixelHistoryEntry{
				UserID:    px.OwnerUserID,
				Username:  px.Username,
				Color:     px.Color,
				Action:    "sync_place",
				UpdatedAt: px.UpdatedAt,
			})
			persistCellHistory(key, history)
			changed = append(changed, px)
		}
	}
	
	// Update lastSyncAt for next iteration
	lastSyncAt = newLastSync

	if len(changed) == 0 {
		return 0, nil
	}

	// Broadcast changed pixels to subscribed clients.
	for _, px := range changed {
		cx := px.X / appConfig.ChunkSize
		cy := px.Y / appConfig.ChunkSize
		chunkKey := fmt.Sprintf("%d_%d", cx, cy)

		for _, conn := range subscribedClientsSnapshot(chunkKey) {
			msg := ServerMessage{
				Type:     "pixel_update",
				Pixel:    &px,
				UserID:   "system_sync",
				Nickname: "Sync",
				NowMs:    time.Now().UnixMilli(),
			}
			if err := conn.WriteJSON(msg); err != nil {
				conn.Close()
				removeClient(conn)
			}
		}
	}
	return len(changed), nil
}

func loadInitialState() {
	log.Println("Downloading canvas history from Firebase...")
	var pixels map[string]PixelMessage
	err := dbClient.NewRef("pixels").Get(ctx, &pixels)
	if err != nil {
		log.Println("No existing pixels found or error reading: ", err)
		return
	}

	now := time.Now().UnixMilli()
	stateMutex.Lock()
	for _, px := range pixels {
		// Ensure UpdatedAt is set (for backward compatibility)
		if px.UpdatedAt == 0 {
			px.UpdatedAt = now
		}
		coordKey := fmt.Sprintf("%d_%d", px.X, px.Y)
		canvasState[coordKey] = px
		appendCellHistory(coordKey, PixelHistoryEntry{
			UserID:    px.OwnerUserID,
			Username:  px.Username,
			Color:     px.Color,
			Action:    "snapshot",
			UpdatedAt: px.UpdatedAt,
		})
	}
	stateMutex.Unlock()
	
	// Set last sync time to now to start syncing only new changes
	lastSyncAt = now
	log.Printf("Loaded %d pixels into memory.\n", len(canvasState))
}

func loadUserStats() {
	var stats map[string]UserStats
	err := dbClient.NewRef("stats").Get(ctx, &stats)
	if err != nil {
		log.Println("No existing stats found or error reading: ", err)
		return
	}

	statsMutex.Lock()
	defer statsMutex.Unlock()
	for uid, s := range stats {
		copyS := s
		copyS.UserID = uid
		userStats[uid] = &copyS
	}
	log.Printf("Loaded %d user stats entries.\n", len(userStats))
}

func loadModerationState() {
	var moderation map[string]UserModerationState
	err := dbClient.NewRef("moderation").Get(ctx, &moderation)
	if err != nil {
		log.Println("No existing moderation entries found or error reading: ", err)
		return
	}

	moderationMutex.Lock()
	defer moderationMutex.Unlock()
	for uid, m := range moderation {
		userModeration[uid] = m
	}
	log.Printf("Loaded %d moderation entries.\n", len(userModeration))
}

func appendCellHistory(coordKey string, entry PixelHistoryEntry) []PixelHistoryEntry {
	historyMutex.Lock()
	defer historyMutex.Unlock()

	limit := appConfig.PixelHistoryLimit
	if limit <= 0 {
		limit = 8
	}

	history := append(cellHistory[coordKey], entry)
	if len(history) > limit {
		history = history[len(history)-limit:]
	}
	cellHistory[coordKey] = history
	out := make([]PixelHistoryEntry, len(history))
	copy(out, history)
	return out
}

func getCellHistory(coordKey string) []PixelHistoryEntry {
	historyMutex.RLock()
	history, ok := cellHistory[coordKey]
	historyMutex.RUnlock()
	if ok {
		out := make([]PixelHistoryEntry, len(history))
		copy(out, history)
		return out
	}

	var persisted []PixelHistoryEntry
	if err := dbClient.NewRef("pixel_history/" + coordKey).Get(ctx, &persisted); err == nil && len(persisted) > 0 {
		historyMutex.Lock()
		cellHistory[coordKey] = persisted
		historyMutex.Unlock()
		out := make([]PixelHistoryEntry, len(persisted))
		copy(out, persisted)
		return out
	}

	return nil
}

func persistCellHistory(coordKey string, history []PixelHistoryEntry) {
	if err := dbClient.NewRef("pixel_history/" + coordKey).Set(ctx, history); err != nil {
		log.Println("Failed to persist pixel history:", coordKey, err)
	}
}

func persistUserModeration(uid string, state UserModerationState) {
	if err := dbClient.NewRef("moderation/" + uid).Set(ctx, state); err != nil {
		log.Println("Failed to persist moderation state:", uid, err)
	}
}

func getUserModeration(uid string) UserModerationState {
	moderationMutex.RLock()
	state := userModeration[uid]
	moderationMutex.RUnlock()
	return state
}

func updateUserModeration(uid string, state UserModerationState) {
	moderationMutex.Lock()
	userModeration[uid] = state
	moderationMutex.Unlock()
}

func isUserFrozen(uid string, nowMs int64) bool {
	state := getUserModeration(uid)
	return state.FreezeUntilMs > nowMs
}

func isUserMuted(uid string, nowMs int64) bool {
	state := getUserModeration(uid)
	return state.MuteUntilMs > nowMs
}

func trackSuspicious(session *ClientSession, event string, weight int64) {
	if weight <= 0 {
		weight = 1
	}
	uid := strings.TrimSpace(session.UserID)
	if uid == "" {
		uid = "anonymous_" + session.ClientIP
	}
	nowMs := time.Now().UnixMilli()

	suspiciousMutex.Lock()
	defer suspiciousMutex.Unlock()

	entry, ok := suspiciousByUser[uid]
	if !ok {
		entry = &SuspiciousActivity{
			UserID:      session.UserID,
			Nickname:    session.Nickname,
			ClientIP:    session.ClientIP,
			EventCounts: make(map[string]int64),
		}
		suspiciousByUser[uid] = entry
	}
	if entry.EventCounts == nil {
		entry.EventCounts = make(map[string]int64)
	}
	entry.Score += weight
	entry.LastEvent = event
	entry.LastAt = nowMs
	if session.UserID != "" {
		entry.UserID = session.UserID
	}
	if session.Nickname != "" {
		entry.Nickname = session.Nickname
	}
	if session.ClientIP != "" {
		entry.ClientIP = session.ClientIP
	}
	entry.EventCounts[event]++
}

func listSuspiciousActivity(limit int) []SuspiciousActivity {
	if limit <= 0 {
		limit = 50
	}
	suspiciousMutex.Lock()
	all := make([]SuspiciousActivity, 0, len(suspiciousByUser))
	for _, v := range suspiciousByUser {
		copyEntry := *v
		if v.EventCounts != nil {
			copyEntry.EventCounts = make(map[string]int64, len(v.EventCounts))
			for k, c := range v.EventCounts {
				copyEntry.EventCounts[k] = c
			}
		}
		all = append(all, copyEntry)
	}
	suspiciousMutex.Unlock()

	sort.Slice(all, func(i, j int) bool {
		if all[i].Score == all[j].Score {
			return all[i].LastAt > all[j].LastAt
		}
		return all[i].Score > all[j].Score
	})
	if len(all) > limit {
		all = all[:limit]
	}
	return all
}

func subscribedClientsSnapshot(chunkKey string) []*websocket.Conn {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	conns := make([]*websocket.Conn, 0, len(clients))
	for conn, session := range clients {
		if _, subscribed := session.SubscribedChunks[chunkKey]; subscribed {
			conns = append(conns, conn)
		}
	}
	return conns
}

func removeClient(conn *websocket.Conn) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()
	delete(clients, conn)
}

func generateID(prefix string, n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

func checkIPRateLimit(ip string) bool {
	ipRateLimitsMutex.Lock()
	defer ipRateLimitsMutex.Unlock()

	now := time.Now()
	limiter, exists := ipRateLimits[ip]

	if !exists || now.After(limiter.windowEnd) {
		// New window
		ipRateLimits[ip] = &IPRateLimit{
			count:     1,
			windowEnd: now.Add(IPRateLimitWindow),
		}
		return true
	}

	// Within existing window
	if limiter.count < IPRateLimitMax {
		limiter.count++
		return true
	}

	return false
}

// Clean up old IP rate limit entries (call periodically)
func cleanupOldIPLimits() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ipRateLimitsMutex.Lock()
		now := time.Now()
		for ip, limiter := range ipRateLimits {
			if now.After(limiter.windowEnd) {
				delete(ipRateLimits, ip)
			}
		}
		ipRateLimitsMutex.Unlock()
	}
}

func validateColor(color string) bool {
	// Normalize color (remove # if present)
	color = strings.TrimSpace(color)
	color = strings.TrimPrefix(strings.ToLower(color), "#")

	// Check if it's a valid hex color (6 characters)
	if len(color) != 6 {
		return false
	}

	// Validate hex characters
	for _, ch := range color {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return false
		}
	}

	// Check against whitelist
	return allowedColors[color]
}

func getClientIP(r *http.Request) string {
	// Try X-Forwarded-For first (for proxies)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take first IP if multiple
		if idx := strings.Index(forwarded, ","); idx != -1 {
			forwarded = forwarded[:idx]
		}
		return strings.TrimSpace(forwarded)
	}

	// Try X-Real-IP
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func sanitizeNickname(nickname string) string {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		return "Guest"
	}
	if len(nickname) > 20 {
		nickname = nickname[:20]
	}
	if !nicknameRegex.MatchString(nickname) {
		return "Guest"
	}
	return nickname
}

func writeServerMessage(ws *websocket.Conn, msg ServerMessage) error {
	return ws.WriteJSON(msg)
}

func getOrCreateStats(uid, nickname string) *UserStats {
	statsMutex.Lock()
	defer statsMutex.Unlock()
	s, ok := userStats[uid]
	if !ok {
		s = &UserStats{
			UserID:          uid,
			Nickname:        nickname,
			TotalPlacements: 0,
			PlacementsToday: 0,
			LastPlacementAt: 0,
			ColorCounts:     make(map[string]int64),
		}
		userStats[uid] = s
	}
	if s.ColorCounts == nil {
		s.ColorCounts = make(map[string]int64)
	}
	if nickname != "" {
		s.Nickname = nickname
	}
	return s
}

func updateUserStatsOnPlacement(stats *UserStats, color string, now time.Time) {
	stats.TotalPlacements++
	stats.LastPlacementAt = now.UnixMilli()

	day := now.UTC().Format("2006-01-02")
	if stats.LastPlacementDay == day {
		stats.PlacementsToday++
	} else {
		stats.LastPlacementDay = day
		stats.PlacementsToday = 1
	}

	if stats.ColorCounts == nil {
		stats.ColorCounts = make(map[string]int64)
	}
	colorKey := strings.ToLower(strings.TrimSpace(color))
	colorKey = strings.TrimPrefix(colorKey, "#")
	if colorKey == "" {
		colorKey = "unknown"
	}
	stats.ColorCounts[colorKey]++
}

func persistUserProfile(uid, nickname string) {
	data := map[string]interface{}{
		"nickname":  nickname,
		"updatedAt": time.Now().UnixMilli(),
	}
	if err := dbClient.NewRef("users/"+uid).Update(ctx, data); err != nil {
		log.Println("Failed to persist user profile:", uid, err)
	}
}

func persistUserCooldown(uid string, cooldownUntilMs int64) {
	data := map[string]interface{}{
		"cooldownUntilMs": cooldownUntilMs,
		"updatedAt":       time.Now().UnixMilli(),
	}
	if err := dbClient.NewRef("users/"+uid).Update(ctx, data); err != nil {
		log.Println("Failed to persist user cooldown:", uid, err)
	}
}

func loadUserCooldown(uid string) int64 {
	var cooldownVal interface{}
	if err := dbClient.NewRef("cooldowns/" + uid).Get(ctx, &cooldownVal); err != nil {
		return 0
	}
	if cooldownVal == nil {
		return 0
	}
	switch v := cooldownVal.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

func writeJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func handleAppConfig(w http.ResponseWriter, r *http.Request) {
	log.Printf("handleAppConfig called: method=%s, path=%s", r.Method, r.URL.Path)
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"ok": false, "error": "method_not_allowed",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                true,
		"firebaseWebApiKey": appConfig.FirebaseWebAPIKey,
	})
}

func readAdminKey(r *http.Request) string {
	key := strings.TrimSpace(r.Header.Get("X-Admin-Key"))
	if key != "" {
		return key
	}
	return strings.TrimSpace(r.URL.Query().Get("key"))
}

func requireAdminAuth(w http.ResponseWriter, r *http.Request) bool {
	if appConfig.AdminAPIKey == "" {
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"ok": false, "error": "admin_api_key_not_configured",
		})
		return false
	}
	if readAdminKey(r) != appConfig.AdminAPIKey {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"ok": false, "error": "unauthorized",
		})
		return false
	}
	return true
}

func updateLiveSessionCooldown(uid string, cooldownUntilMs int64) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()
	for _, s := range clients {
		if s.UserID == uid {
			s.CooldownUntilMs = cooldownUntilMs
		}
	}
}

func handleAdminCooldown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{
			"ok": false, "error": "method_not_allowed",
		})
		return
	}
	if !requireAdminAuth(w, r) {
		return
	}

	var req AdminCooldownRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"ok": false, "error": "bad_json",
		})
		return
	}
	uid := strings.TrimSpace(req.UserID)
	if uid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"ok": false, "error": "missing_user_id",
		})
		return
	}

	var until int64
	if req.Clear {
		until = 0
	} else if req.CooldownUntilMs != nil {
		until = *req.CooldownUntilMs
		if until < 0 {
			until = 0
		}
	} else {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"ok": false, "error": "missing_cooldown_until_ms_or_clear",
		})
		return
	}

	persistUserCooldown(uid, until)
	updateLiveSessionCooldown(uid, until)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"userId":          uid,
		"cooldownUntilMs": until,
		"clear":           until == 0,
	})
}

func handleAdminModeration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method_not_allowed"})
		return
	}
	if !requireAdminAuth(w, r) {
		return
	}

	var req AdminModerationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "bad_json"})
		return
	}
	uid := strings.TrimSpace(req.UserID)
	if uid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "missing_user_id"})
		return
	}

	nowMs := time.Now().UnixMilli()
	state := getUserModeration(uid)
	if req.ClearMute {
		state.MuteUntilMs = 0
	} else if req.MuteUntilMs != nil {
		state.MuteUntilMs = *req.MuteUntilMs
		if state.MuteUntilMs < 0 {
			state.MuteUntilMs = 0
		}
	}
	if req.ClearFreeze {
		state.FreezeUntilMs = 0
	} else if req.FreezeUntilMs != nil {
		state.FreezeUntilMs = *req.FreezeUntilMs
		if state.FreezeUntilMs < 0 {
			state.FreezeUntilMs = 0
		}
	}
	if req.Reason != "" {
		state.Reason = strings.TrimSpace(req.Reason)
	}
	state.UpdatedAt = nowMs

	updateUserModeration(uid, state)
	persistUserModeration(uid, state)

	clientsMutex.Lock()
	for conn, s := range clients {
		if s.UserID != uid {
			continue
		}
		_ = writeServerMessage(conn, ServerMessage{
			Type:    "moderation_updated",
			Reason:  "moderation_changed",
			NowMs:   nowMs,
			Message: "Your account moderation status was updated by an admin.",
		})
	}
	clientsMutex.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":     true,
		"userId": uid,
		"state":  state,
	})
}

func handleAdminRollbackWindow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method_not_allowed"})
		return
	}
	if !requireAdminAuth(w, r) {
		return
	}

	var req AdminRollbackWindowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "bad_json"})
		return
	}
	if req.StartMs <= 0 || req.EndMs <= 0 || req.EndMs < req.StartMs {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "invalid_time_window"})
		return
	}
	targetUID := strings.TrimSpace(req.UserID)

	removed := make([]PixelMessage, 0, 64)
	stateMutex.Lock()
	for key, px := range canvasState {
		if px.UpdatedAt < req.StartMs || px.UpdatedAt > req.EndMs {
			continue
		}
		if targetUID != "" && px.OwnerUserID != targetUID {
			continue
		}
		removed = append(removed, px)
		delete(canvasState, key)
	}
	stateMutex.Unlock()

	pixelsRef := dbClient.NewRef("pixels")
	for _, px := range removed {
		coordKey := fmt.Sprintf("%d_%d", px.X, px.Y)
		if err := pixelsRef.Child(coordKey).Delete(ctx); err != nil {
			log.Println("Failed rollback delete:", coordKey, err)
		}
		history := appendCellHistory(coordKey, PixelHistoryEntry{
			UserID:    "admin",
			Username:  "admin",
			Action:    "rollback_remove",
			UpdatedAt: time.Now().UnixMilli(),
		})
		persistCellHistory(coordKey, history)
	}

	broadcastNow := time.Now().UnixMilli()
	for _, px := range removed {
		cx := px.X / appConfig.ChunkSize
		cy := px.Y / appConfig.ChunkSize
		chunkKey := fmt.Sprintf("%d_%d", cx, cy)
		for _, client := range subscribedClientsSnapshot(chunkKey) {
			serverMsg := ServerMessage{
				Type:     "pixel_removed",
				Pixel:    &PixelMessage{X: px.X, Y: px.Y},
				UserID:   "admin",
				Nickname: "Admin",
				NowMs:    broadcastNow,
			}
			if err := client.WriteJSON(serverMsg); err != nil {
				client.Close()
				removeClient(client)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":           true,
		"removedCount": len(removed),
		"startMs":      req.StartMs,
		"endMs":        req.EndMs,
		"userId":       targetUID,
	})
}

func handleAdminSuspicious(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"ok": false, "error": "method_not_allowed"})
		return
	}
	if !requireAdminAuth(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"items": listSuspiciousActivity(100),
	})
}

func persistUserStats(uid string, stats *UserStats) {
	if err := dbClient.NewRef("stats/"+uid).Set(ctx, stats); err != nil {
		log.Println("Failed to persist stats:", uid, err)
	}
}

func buildLeaderboard(limit int) []LeaderboardEntry {
	if limit <= 0 {
		limit = 5
	}
	statsMutex.RLock()
	defer statsMutex.RUnlock()

	all := make([]LeaderboardEntry, 0, len(userStats))
	for uid, s := range userStats {
		all = append(all, LeaderboardEntry{
			UserID:          uid,
			Nickname:        s.Nickname,
			TotalPlacements: s.TotalPlacements,
			PlacementsToday: s.PlacementsToday,
		})
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].TotalPlacements == all[j].TotalPlacements {
			return all[i].PlacementsToday > all[j].PlacementsToday
		}
		return all[i].TotalPlacements > all[j].TotalPlacements
	})

	if len(all) > limit {
		all = all[:limit]
	}
	return all
}

func buildColorStats() map[string]int64 {
	statsMutex.RLock()
	defer statsMutex.RUnlock()

	colorCounts := make(map[string]int64)
	for _, stats := range userStats {
		for color, count := range stats.ColorCounts {
			colorCounts[color] += count
		}
	}
	return colorCounts
}

func chunkPixels(cx, cy int) []PixelMessage {
	minX := cx * appConfig.ChunkSize
	minY := cy * appConfig.ChunkSize
	maxX := minX + appConfig.ChunkSize - 1
	maxY := minY + appConfig.ChunkSize - 1
	if minX > appConfig.GridSize-1 || minY > appConfig.GridSize-1 || maxX < 0 || maxY < 0 {
		return nil
	}

	stateMutex.RLock()
	defer stateMutex.RUnlock()
	out := make([]PixelMessage, 0, appConfig.ChunkSize*appConfig.ChunkSize/4)
	for _, px := range canvasState {
		if px.X >= minX && px.X <= maxX && px.Y >= minY && px.Y <= maxY {
			out = append(out, px)
		}
	}
	return out
}

func allPixelsSnapshot() []PixelMessage {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	out := make([]PixelMessage, 0, len(canvasState))
	for _, px := range canvasState {
		out = append(out, px)
	}
	return out
}

func sendChunk(ws *websocket.Conn, cx, cy int) error {
	chunk := ChunkPayload{
		CX:     cx,
		CY:     cy,
		Pixels: chunkPixels(cx, cy),
	}
	return writeServerMessage(ws, ServerMessage{
		Type:  "chunk_data",
		Chunk: &chunk,
		NowMs: time.Now().UnixMilli(),
	})
}

func sendInitialViewportChunks(ws *websocket.Conn, session *ClientSession) {
	initialChunks := [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
	for _, chunk := range initialChunks {
		_ = sendChunk(ws, chunk[0], chunk[1])
		session.SubscribedChunks[fmt.Sprintf("%d_%d", chunk[0], chunk[1])] = struct{}{}
	}
}

func handleAuthMessage(session *ClientSession, msg ClientMessage) {
	idToken := strings.TrimSpace(msg.FirebaseIDToken)
	if idToken == "" {
		_ = writeServerMessage(session.Conn, ServerMessage{
			Type:    "auth_failed",
			Reason:  "missing_firebase_id_token",
			NowMs:   time.Now().UnixMilli(),
			Message: "Please sign in with email/password first.",
		})
		trackSuspicious(session, "missing_firebase_id_token", 2)
		return
	}
	if authClient == nil {
		_ = writeServerMessage(session.Conn, ServerMessage{
			Type:    "auth_failed",
			Reason:  "auth_unavailable",
			NowMs:   time.Now().UnixMilli(),
			Message: "Authentication service unavailable.",
		})
		return
	}

	verified, err := authClient.VerifyIDToken(ctx, idToken)
	if err != nil {
		_ = writeServerMessage(session.Conn, ServerMessage{
			Type:    "auth_failed",
			Reason:  "invalid_firebase_id_token",
			NowMs:   time.Now().UnixMilli(),
			Message: "Invalid or expired sign-in token. Please sign in again.",
		})
		trackSuspicious(session, "invalid_firebase_id_token", 5)
		return
	}

	uid := strings.TrimSpace(verified.UID)
	if uid == "" {
		_ = writeServerMessage(session.Conn, ServerMessage{
			Type:    "auth_failed",
			Reason:  "invalid_firebase_uid",
			NowMs:   time.Now().UnixMilli(),
			Message: "Account UID missing from token.",
		})
		return
	}

	// Prefer server-stored nickname (from Firebase/in-memory stats) over client-provided value.
	// This ensures a nickname changed directly in Firebase is respected and not overwritten on reconnect.
	statsMutex.Lock()
	existingStats, hasExistingStats := userStats[uid]
	storedNickname := ""
	if hasExistingStats && existingStats.Nickname != "" {
		storedNickname = existingStats.Nickname
	}
	statsMutex.Unlock()

	var nickname string
	if storedNickname != "" {
		nickname = storedNickname
	} else {
		nickname = sanitizeNickname(msg.Nickname)
		if nickname == "Guest" || nickname == "" {
			if emailRaw, ok := verified.Claims["email"]; ok {
				if email, ok := emailRaw.(string); ok && email != "" {
					if at := strings.Index(email, "@"); at > 0 {
						nickname = sanitizeNickname(email[:at])
					}
				}
			}
		}
		if nickname == "Guest" && len(uid) > 6 {
			nickname = "User-" + uid[len(uid)-6:]
		}
	}

	session.UserID = uid
	session.Nickname = nickname
	session.Authenticated = true
	session.CooldownUntilMs = loadUserCooldown(session.UserID)
	stats := getOrCreateStats(session.UserID, session.Nickname)
	persistUserProfile(session.UserID, session.Nickname)
	persistUserStats(session.UserID, stats)

	_ = writeServerMessage(session.Conn, ServerMessage{
		Type:            "auth_ok",
		UserID:          session.UserID,
		Nickname:        session.Nickname,
		CooldownMs:      int64(appConfig.CooldownDuration / time.Millisecond),
		CooldownUntilMs: session.CooldownUntilMs,
		CooldownBypass:  isCooldownBypassed(session.UserID),
		GridSize:        appConfig.GridSize,
		ChunkSize:       appConfig.ChunkSize,
		Stats:           stats,
		Leaderboard:     buildLeaderboard(5),
		ColorStats:      buildColorStats(),
		NowMs:           time.Now().UnixMilli(),
		IdentityToken:   issueIdentityToken(session.UserID),
	})
	_ = writeServerMessage(session.Conn, ServerMessage{
		Type:   "canvas_snapshot",
		Pixels: allPixelsSnapshot(),
		NowMs:  time.Now().UnixMilli(),
	})
}

func isCooldownBypassed(uid string) bool {
	_, ok := appConfig.CooldownBypassUIDs[uid]
	return ok
}

func inCooldown(session *ClientSession, now time.Time) (bool, int64) {
	if isCooldownBypassed(session.UserID) {
		return false, 0
	}
	nowMs := now.UnixMilli()
	if session.CooldownUntilMs <= 0 {
		return false, 0
	}
	if nowMs >= session.CooldownUntilMs {
		session.CooldownUntilMs = 0
		return false, 0
	}
	return true, session.CooldownUntilMs - nowMs
}


// issueIdentityToken returns an HMAC-SHA256 of userID signed with the app's IdentitySecret.
func issueIdentityToken(userID string) string {
	mac := hmac.New(sha256.New, []byte(appConfig.IdentitySecret))
	mac.Write([]byte(userID))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyIdentityToken checks that token == HMAC-SHA256(userID, IdentitySecret)
// using constant-time comparison to prevent timing attacks.
func verifyIdentityToken(userID, token string) bool {
	if token == "" {
		return false
	}
	expected := issueIdentityToken(userID)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(token)) == 1
}

func antiSpamExceeded(session *ClientSession, now time.Time) bool {
	if session.WindowStart.IsZero() || now.Sub(session.WindowStart) > appConfig.AntiSpamWindow {
		session.WindowStart = now
		session.WindowMsgCount = 0
	}
	session.WindowMsgCount++
	return session.WindowMsgCount > appConfig.MaxMessagesPerWindow
}

func handleChunkRequest(session *ClientSession, msg ClientMessage) {
	now := time.Now()
	if !session.LastChunkAt.IsZero() && now.Sub(session.LastChunkAt) < time.Second/time.Duration(appConfig.MaxChunkRequestsPerSec) {
		return
	}
	session.LastChunkAt = now

	// Track which chunks are being subscribed to
	requestedChunks := make([]string, 0)

	if msg.Viewport != nil {
		cx := int(math.Floor(float64(msg.Viewport.CenterX) / float64(appConfig.ChunkSize)))
		cy := int(math.Floor(float64(msg.Viewport.CenterY) / float64(appConfig.ChunkSize)))
		r := msg.Viewport.Radius
		if r <= 0 || r > 4 {
			r = 1
		}
		for dx := -r; dx <= r; dx++ {
			for dy := -r; dy <= r; dy++ {
				chunkX := cx + dx
				chunkY := cy + dy
				_ = sendChunk(session.Conn, chunkX, chunkY)
				requestedChunks = append(requestedChunks, fmt.Sprintf("%d_%d", chunkX, chunkY))
			}
		}
	} else {
		for _, c := range msg.Chunks {
			_ = sendChunk(session.Conn, c.CX, c.CY)
			requestedChunks = append(requestedChunks, fmt.Sprintf("%d_%d", c.CX, c.CY))
		}
	}

	// Update subscribed chunks for this session
	session.SubscribedChunks = make(map[string]struct{})
	for _, chunkKey := range requestedChunks {
		session.SubscribedChunks[chunkKey] = struct{}{}
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	// Get client IP for rate limiting
	clientIP := getClientIP(r)

	// Check IP-based rate limiting
	if !checkIPRateLimit(clientIP) {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer ws.Close()

	session := &ClientSession{
		Conn:             ws,
		ConnID:           generateID("conn", 6),
		Authenticated:    false,
		UserID:           generateID("guest", 6),
		Nickname:         "Guest",
		WindowStart:      time.Now(),
		WindowMsgCount:   0,
		LastPlacementAt:  time.Time{},
		SubscribedChunks: make(map[string]struct{}),
		ClientIP:         clientIP,
	}

	clientsMutex.Lock()
	clients[ws] = session
	clientsMutex.Unlock()

	stats := getOrCreateStats(session.UserID, session.Nickname)
	_ = writeServerMessage(ws, ServerMessage{
		Type:            "welcome",
		UserID:          session.UserID,
		Nickname:        session.Nickname,
		IdentityToken:   issueIdentityToken(session.UserID),
		CooldownMs:      int64(appConfig.CooldownDuration / time.Millisecond),
		CooldownUntilMs: session.CooldownUntilMs,
		CooldownBypass:  isCooldownBypassed(session.UserID),
		GridSize:        appConfig.GridSize,
		ChunkSize:       appConfig.ChunkSize,
		Stats:           stats,
		Leaderboard:     buildLeaderboard(5),
		NowMs:           time.Now().UnixMilli(),
		Message:         "Connected to Pixel World server.",
	})
	_ = writeServerMessage(ws, ServerMessage{
		Type:   "canvas_snapshot",
		Pixels: allPixelsSnapshot(),
		NowMs:  time.Now().UnixMilli(),
	})
	sendInitialViewportChunks(ws, session)

	for {
		_, payload, err := ws.ReadMessage()
		if err != nil {
			// Client disconnected - ensure proper cleanup
			clientsMutex.Lock()
			if _, exists := clients[ws]; exists {
				delete(clients, ws)
				log.Printf("Client disconnected: %s (IP: %s, UserID: %s)\n", session.ConnID, session.ClientIP, session.UserID)
			}
			clientsMutex.Unlock()
			// WebSocket is closed by defer ws.Close() at function start
			break
		}

		now := time.Now()
		if antiSpamExceeded(session, now) {
			_ = writeServerMessage(ws, ServerMessage{
				Type:   "error",
				Reason: "anti_spam",
				NowMs:  now.UnixMilli(),
				Message: "Too many messages in a short time. Slow down.",
			})
			trackSuspicious(session, "anti_spam", 2)
			continue
		}

		var msg ClientMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			_ = writeServerMessage(ws, ServerMessage{
				Type:   "error",
				Reason: "bad_payload",
				NowMs:  now.UnixMilli(),
			})
			continue
		}

		switch msg.Type {
		case "auth":
			handleAuthMessage(session, msg)
		case "cell_history_request":
			if msg.Pixel == nil {
				continue
			}
			if msg.Pixel.X < 0 || msg.Pixel.X >= appConfig.GridSize || msg.Pixel.Y < 0 || msg.Pixel.Y >= appConfig.GridSize {
				continue
			}
			coordKey := fmt.Sprintf("%d_%d", msg.Pixel.X, msg.Pixel.Y)
			history := getCellHistory(coordKey)
			_ = writeServerMessage(ws, ServerMessage{
				Type:    "cell_history",
				Pixel:   &PixelMessage{X: msg.Pixel.X, Y: msg.Pixel.Y},
				History: history,
				NowMs:   now.UnixMilli(),
			})
		case "request_chunks":
			handleChunkRequest(session, msg)
		case "place_pixel":
			if !session.Authenticated {
				_ = writeServerMessage(ws, ServerMessage{
					Type:   "pixel_rejected",
					Reason: "auth_required",
					NowMs:  now.UnixMilli(),
					Message: "Authenticate first before placing pixels.",
				})
				continue
			}
			if msg.Pixel == nil {
				continue
			}
			if isUserFrozen(session.UserID, now.UnixMilli()) {
				_ = writeServerMessage(ws, ServerMessage{
					Type:    "pixel_rejected",
					Reason:  "frozen",
					NowMs:   now.UnixMilli(),
					Message: "This account is frozen by moderation.",
				})
				trackSuspicious(session, "frozen_place_attempt", 3)
				continue
			}
			if msg.Pixel.X < 0 || msg.Pixel.X >= appConfig.GridSize || msg.Pixel.Y < 0 || msg.Pixel.Y >= appConfig.GridSize {
				_ = writeServerMessage(ws, ServerMessage{
					Type:   "pixel_rejected",
					Reason: "out_of_bounds",
					NowMs:  now.UnixMilli(),
				})
				trackSuspicious(session, "place_out_of_bounds", 1)
				continue
			}
			if !validateColor(msg.Pixel.Color) {
				_ = writeServerMessage(ws, ServerMessage{
					Type:   "pixel_rejected",
					Reason: "invalid_color",
					NowMs:  now.UnixMilli(),
					Message: "Color not in whitelist. Use an allowed color.",
				})
				trackSuspicious(session, "invalid_color", 1)
				continue
			}
			if cooldown, remaining := inCooldown(session, now); cooldown {
				_ = writeServerMessage(ws, ServerMessage{
					Type:         "pixel_rejected",
					Reason:       "cooldown",
					RetryAfterMs: remaining,
					CooldownUntilMs: session.CooldownUntilMs,
					NowMs:        now.UnixMilli(),
				})
				continue
			}
			session.LastPlacementAt = now
			px := *msg.Pixel
			px.Username = session.Nickname
			px.OwnerUserID = session.UserID
			broadcast <- PlacementEvent{
				Session: session,
				Pixel:   px,
			}
		case "undo_pixel":
			if !session.Authenticated {
				_ = writeServerMessage(ws, ServerMessage{
					Type:   "pixel_rejected",
					Reason: "auth_required",
					NowMs:  now.UnixMilli(),
					Message: "Authenticate first.",
				})
				continue
			}
			if msg.Pixel == nil {
				continue
			}
			if isUserFrozen(session.UserID, now.UnixMilli()) {
				_ = writeServerMessage(ws, ServerMessage{
					Type:    "pixel_rejected",
					Reason:  "frozen",
					NowMs:   now.UnixMilli(),
					Message: "This account is frozen by moderation.",
				})
				trackSuspicious(session, "frozen_undo_attempt", 2)
				continue
			}
			if msg.Pixel.X < 0 || msg.Pixel.X >= appConfig.GridSize || msg.Pixel.Y < 0 || msg.Pixel.Y >= appConfig.GridSize {
				_ = writeServerMessage(ws, ServerMessage{
					Type:   "pixel_rejected",
					Reason: "out_of_bounds",
					NowMs:  now.UnixMilli(),
				})
				continue
			}
			
			coordKey := fmt.Sprintf("%d_%d", msg.Pixel.X, msg.Pixel.Y)
			stateMutex.Lock()
			px, exists := canvasState[coordKey]
			ownerMatches := exists && ((px.OwnerUserID != "" && px.OwnerUserID == session.UserID) || (px.OwnerUserID == "" && px.Username == session.Nickname))
			if !ownerMatches {
				stateMutex.Unlock()
				_ = writeServerMessage(ws, ServerMessage{
					Type:    "pixel_rejected",
					Reason:  "not_owner",
					NowMs:   now.UnixMilli(),
					Message: "You can only undo your own pixels.",
				})
				continue
			}
			
			// Check if pixel was placed recently (within 30 seconds)
			lastPlaceTime := time.UnixMilli(px.UpdatedAt)
			if now.Sub(lastPlaceTime) > 30*time.Second {
				stateMutex.Unlock()
				_ = writeServerMessage(ws, ServerMessage{
					Type:   "pixel_rejected",
					Reason: "undo_expired",
					NowMs:  now.UnixMilli(),
					Message: "Can only undo pixels placed within 30 seconds.",
				})
				continue
			}
			
			// Remove the pixel
			delete(canvasState, coordKey)
			stateMutex.Unlock()
			history := appendCellHistory(coordKey, PixelHistoryEntry{
				UserID:    session.UserID,
				Username:  session.Nickname,
				Color:     px.Color,
				Action:    "undo_remove",
				UpdatedAt: now.UnixMilli(),
			})
			persistCellHistory(coordKey, history)
			
			// Queue the deletion for database persistence
			select {
			case pendingDeletes <- struct{ X int; Y int }{X: msg.Pixel.X, Y: msg.Pixel.Y}:
			default:
				log.Println("Warning: pendingDeletes channel full, pixel deletion may not persist")
			}
			
			// Decrement user stats
			stats := getOrCreateStats(session.UserID, session.Nickname)
			statsMutex.Lock()
			if stats.TotalPlacements > 0 {
				stats.TotalPlacements--
			}
			if stats.PlacementsToday > 0 {
				stats.PlacementsToday--
			}
			// Decrement color count if color stats exist
			if stats.ColorCounts != nil && px.Color != "" {
				if count, exists := stats.ColorCounts[px.Color]; exists && count > 0 {
					stats.ColorCounts[px.Color]--
				}
			}
			statsMutex.Unlock()
			
			// Broadcast pixel removal (send as pixel_update with empty/cleared state)
			// or send as a distinct message type
			cx := msg.Pixel.X / appConfig.ChunkSize
			cy := msg.Pixel.Y / appConfig.ChunkSize
			chunkKey := fmt.Sprintf("%d_%d", cx, cy)
			
			for _, client := range subscribedClientsSnapshot(chunkKey) {
				serverMsg := ServerMessage{
					Type:     "pixel_removed",
					Pixel:    &PixelMessage{X: msg.Pixel.X, Y: msg.Pixel.Y},
					UserID:   session.UserID,
					Nickname: session.Nickname,
					NowMs:    now.UnixMilli(),
				}
				if err := client.WriteJSON(serverMsg); err != nil {
					client.Close()
					removeClient(client)
				}
			}
			
			_ = writeServerMessage(ws, ServerMessage{
				Type:   "undo_success",
				NowMs:  now.UnixMilli(),
				Message: "Pixel undone successfully.",
			})
		case "change_nickname":
			if !session.Authenticated {
				_ = writeServerMessage(ws, ServerMessage{
					Type:    "nickname_change_rejected",
					Reason:  "auth_required",
					NowMs:   now.UnixMilli(),
					Message: "Authenticate first.",
				})
				continue
			}
			if msg.Nickname == "" {
				_ = writeServerMessage(ws, ServerMessage{
					Type:    "nickname_change_rejected",
					Reason:  "invalid_nickname",
					NowMs:   now.UnixMilli(),
					Message: "Nickname cannot be empty.",
				})
				continue
			}
			if isUserMuted(session.UserID, now.UnixMilli()) {
				_ = writeServerMessage(ws, ServerMessage{
					Type:    "nickname_change_rejected",
					Reason:  "muted",
					NowMs:   now.UnixMilli(),
					Message: "This account is muted by moderation.",
				})
				trackSuspicious(session, "muted_change_nickname_attempt", 1)
				continue
			}
			
			newNickname := sanitizeNickname(msg.Nickname)
			if newNickname == "" || !nicknameRegex.MatchString(newNickname) {
				_ = writeServerMessage(ws, ServerMessage{
					Type:    "nickname_change_rejected",
					Reason:  "invalid_format",
					NowMs:   now.UnixMilli(),
					Message: "Nickname must be 3-20 characters (alphanumeric, spaces, underscores).",
				})
				continue
			}
			
			// Check cooldown - once per day
			stats := getOrCreateStats(session.UserID, session.Nickname)
			statsMutex.RLock()
			lastChange := stats.LastNicknameChangeAt
			statsMutex.RUnlock()
			
			if lastChange > 0 {
				lastChangeTime := time.UnixMilli(lastChange)
				if now.Sub(lastChangeTime) < 24*time.Hour {
					remainingMs := (24*time.Hour - now.Sub(lastChangeTime)).Milliseconds()
					_ = writeServerMessage(ws, ServerMessage{
						Type:         "nickname_change_rejected",
						Reason:       "cooldown",
						NowMs:        now.UnixMilli(),
						RetryAfterMs: remainingMs,
						Message:      "You can only change nickname once per day.",
					})
					continue
				}
			}
			
			// Check cost - need at least 10 placements
			statsMutex.RLock()
			placements := stats.TotalPlacements
			statsMutex.RUnlock()
			
			if placements < 10 {
				_ = writeServerMessage(ws, ServerMessage{
					Type:    "nickname_change_rejected",
					Reason:  "insufficient_placements",
					NowMs:   now.UnixMilli(),
					Message: fmt.Sprintf("You need at least 10 placements to change nickname. You have %d.", placements),
				})
				continue
			}
			
			// Deduct cost and update nickname
			statsMutex.Lock()
			stats.TotalPlacements -= 10
			if stats.PlacementsToday >= 10 {
				stats.PlacementsToday -= 10
			}
			stats.LastNicknameChangeAt = now.UnixMilli()
			stats.Nickname = newNickname
			statsMutex.Unlock()
			
			session.Nickname = newNickname
			session.LastPlacementAt = now
			
			// Persist updates
			persistUserProfile(session.UserID, newNickname)
			persistUserStats(session.UserID, stats)
			
			_ = writeServerMessage(ws, ServerMessage{
				Type:     "nickname_changed",
				NowMs:    now.UnixMilli(),
				Message:  fmt.Sprintf("Nickname changed to '%s' (cost: 10 placements).", newNickname),
				Nickname: newNickname,
				Stats:    stats,
			})
		default:
		}
	}
}

func persistenceWorker() {
	// Batch write settings
	const batchFlushInterval = 150 * time.Millisecond
	const batchFlushSize = 20
	
	ticker := time.NewTicker(batchFlushInterval)
	defer ticker.Stop()
	
	batch := make([]PlacementEvent, 0, batchFlushSize)
	deletes := make([]struct{ X int; Y int }, 0, batchFlushSize)
	statsToUpdate := make(map[string]*UserStats)
	
	for {
		select {
		case event := <-pendingWrites:
			batch = append(batch, event)
			// Update stats map for batch flush
			stats := getOrCreateStats(event.Session.UserID, event.Session.Nickname)
			statsToUpdate[event.Session.UserID] = stats
			
			// Flush if batch reaches size threshold
			if len(batch) >= batchFlushSize {
				flushBatch(batch, deletes, statsToUpdate)
				batch = batch[:0]
				deletes = deletes[:0]
				statsToUpdate = make(map[string]*UserStats)
			}
		case deletePixel := <-pendingDeletes:
			deletes = append(deletes, deletePixel)
			
			// Flush if delete batch reaches size threshold
			if len(deletes) >= batchFlushSize {
				flushBatch(batch, deletes, statsToUpdate)
				batch = batch[:0]
				deletes = deletes[:0]
				statsToUpdate = make(map[string]*UserStats)
			}
		case <-ticker.C:
			// Flush if there are pending writes, even if batch is small
			if len(batch) > 0 || len(deletes) > 0 {
				flushBatch(batch, deletes, statsToUpdate)
				batch = batch[:0]
				deletes = deletes[:0]
				statsToUpdate = make(map[string]*UserStats)
			}
		}
	}
}

func flushBatch(batch []PlacementEvent, deletes []struct{ X int; Y int }, statsToUpdate map[string]*UserStats) {
	if len(batch) == 0 && len(deletes) == 0 {
		return
	}
	
	// Create batch write operations
	batchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	pixelsRef := dbClient.NewRef("pixels")
	statsRef := dbClient.NewRef("stats")
	cooldownRef := dbClient.NewRef("cooldowns")
	
	for _, event := range batch {
		coordKey := fmt.Sprintf("%d_%d", event.Pixel.X, event.Pixel.Y)
		
		// Write pixel with UpdatedAt timestamp
		pixel := event.Pixel
		pixel.UpdatedAt = time.Now().UnixMilli()
		
		if err := pixelsRef.Child(coordKey).Set(batchCtx, pixel); err != nil {
			log.Println("Failed to persist pixel in batch:", coordKey, err)
		}
		
		// Write cooldown
		if !isCooldownBypassed(event.Session.UserID) {
			if err := cooldownRef.Child(event.Session.UserID).Set(batchCtx, event.Session.CooldownUntilMs); err != nil {
				log.Println("Failed to persist cooldown in batch:", event.Session.UserID, err)
			}
		}
	}
	
	// Delete pixels from database
	for _, deletePixel := range deletes {
		coordKey := fmt.Sprintf("%d_%d", deletePixel.X, deletePixel.Y)
		if err := pixelsRef.Child(coordKey).Delete(batchCtx); err != nil {
			log.Println("Failed to delete pixel from database:", coordKey, err)
		}
	}
	
	// Write all updated stats in batch
	for uid, stats := range statsToUpdate {
		if err := statsRef.Child(uid).Set(batchCtx, stats); err != nil {
			log.Println("Failed to persist stats in batch:", uid, err)
		}
	}
}

func handleMessages() {
	for {
		event := <-broadcast
		msg := event.Pixel

		// Set UpdatedAt timestamp for this pixel
		now := time.Now()
		msg.UpdatedAt = now.UnixMilli()

		coordKey := fmt.Sprintf("%d_%d", msg.X, msg.Y)
		stateMutex.Lock()
		canvasState[coordKey] = msg
		stateMutex.Unlock()
		history := appendCellHistory(coordKey, PixelHistoryEntry{
			UserID:    event.Session.UserID,
			Username:  event.Session.Nickname,
			Color:     msg.Color,
			Action:    "place",
			UpdatedAt: msg.UpdatedAt,
		})
		persistCellHistory(coordKey, history)

		stats := getOrCreateStats(event.Session.UserID, event.Session.Nickname)
		updateUserStatsOnPlacement(stats, msg.Color, now)

		// Calculate which chunk this pixel belongs to
		cx := msg.X / appConfig.ChunkSize
		cy := msg.Y / appConfig.ChunkSize
		chunkKey := fmt.Sprintf("%d_%d", cx, cy)

		// Broadcast only to subscribed clients (not all clients).
		for _, client := range subscribedClientsSnapshot(chunkKey) {
			serverMsg := ServerMessage{
				Type:     "pixel_update",
				Pixel:    &msg,
				UserID:   event.Session.UserID,
				Nickname: event.Session.Nickname,
				NowMs:    now.UnixMilli(),
			}
			if err := client.WriteJSON(serverMsg); err != nil {
				client.Close()
				removeClient(client)
			}
		}

		// Set cooldown for the session
		if !isCooldownBypassed(event.Session.UserID) {
			event.Session.CooldownUntilMs = now.Add(appConfig.CooldownDuration).UnixMilli()
		} else {
			event.Session.CooldownUntilMs = 0
		}

		// Send immediate confirmation to the placer
		_ = writeServerMessage(event.Session.Conn, ServerMessage{
			Type:            "pixel_accepted",
			Pixel:           &msg,
			CooldownMs:      int64(appConfig.CooldownDuration / time.Millisecond),
			CooldownBypass:  isCooldownBypassed(event.Session.UserID),
			CooldownUntilMs: event.Session.CooldownUntilMs,
			GridSize:        appConfig.GridSize,
			ChunkSize:       appConfig.ChunkSize,
			NowMs:           now.UnixMilli(),
			Stats:           stats,
			Leaderboard:     buildLeaderboard(5),
			ColorStats:      buildColorStats(),
		})

		// Queue for persistence (batching) instead of immediate write
		event.Pixel = msg // Update with timestamp
		pendingWrites <- event
	}
}