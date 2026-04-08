package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/db"
	"github.com/gorilla/websocket"
	"google.golang.org/api/option"
)

type PixelMessage struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Color string `json:"color"`
}

type ClientMessage struct {
	Type         string            `json:"type"`
	UserID       string            `json:"userId,omitempty"`
	Nickname     string            `json:"nickname,omitempty"`
	Pixel        *PixelMessage     `json:"pixel,omitempty"`
	Chunks       []ChunkCoord      `json:"chunks,omitempty"`
	Viewport     *ViewportPayload  `json:"viewport,omitempty"`
	ClientVersion string           `json:"clientVersion,omitempty"`
}

type ServerMessage struct {
	Type         string         `json:"type"`
	Pixel        *PixelMessage  `json:"pixel,omitempty"`
	Pixels       []PixelMessage `json:"pixels,omitempty"`
	UserID       string         `json:"userId,omitempty"`
	Nickname     string         `json:"nickname,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	RetryAfterMs int64          `json:"retryAfterMs,omitempty"`
	NowMs        int64          `json:"nowMs,omitempty"`
	CooldownMs   int64          `json:"cooldownMs,omitempty"`
	CooldownUntilMs int64       `json:"cooldownUntilMs,omitempty"`
	CooldownBypass bool         `json:"cooldownBypass,omitempty"`
	Stats        *UserStats     `json:"stats,omitempty"`
	Leaderboard  []LeaderboardEntry `json:"leaderboard,omitempty"`
	Chunk        *ChunkPayload  `json:"chunk,omitempty"`
	Message      string         `json:"message,omitempty"`
}

type ClientSession struct {
	Conn            *websocket.Conn
	ConnID          string
	Authenticated   bool
	UserID          string
	Nickname        string
	WindowStart     time.Time
	WindowMsgCount  int
	LastPlacementAt time.Time
	LastChunkAt     time.Time
	CooldownUntilMs int64
}

type PlacementEvent struct {
	Session *ClientSession
	Pixel   PixelMessage
}

type UserStats struct {
	UserID          string           `json:"userId"`
	Nickname        string           `json:"nickname"`
	TotalPlacements int64            `json:"totalPlacements"`
	PlacementsToday int64            `json:"placementsToday"`
	LastPlacementAt int64            `json:"lastPlacementAt"`
	LastPlacementDay string          `json:"lastPlacementDay,omitempty"`
	ColorCounts     map[string]int64 `json:"colorCounts,omitempty"`
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

var nicknameRegex = regexp.MustCompile(`^[a-zA-Z0-9_ ]{3,20}$`)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
var clients = make(map[*websocket.Conn]*ClientSession)
var clientsMutex sync.Mutex
var broadcast = make(chan PlacementEvent, 512)

var canvasState = make(map[string]PixelMessage)
var stateMutex sync.RWMutex

var dbClient *db.Client
var ctx = context.Background()
var userStats = make(map[string]*UserStats)
var statsMutex sync.RWMutex
var appConfig AppConfig

//go:embed index.html static/**
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
	log.Println("Successfully connected to Firebase via Admin SDK!")

	loadInitialState()
	loadUserStats()

	// 2. Define API & WebSocket Routes
	http.HandleFunc("/ws", handleConnections)
	http.HandleFunc("/admin/cooldown", handleAdminCooldown)
	go handleMessages()
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
	for key, px := range dbPixels {
		prev, ok := canvasState[key]
		if !ok || prev.Color != px.Color || prev.X != px.X || prev.Y != px.Y {
			canvasState[key] = px
			changed = append(changed, px)
		}
	}
	stateMutex.Unlock()

	if len(changed) == 0 {
		return 0, nil
	}

	clientsMutex.Lock()
	defer clientsMutex.Unlock()
	for _, px := range changed {
		msg := ServerMessage{
			Type:     "pixel_update",
			Pixel:    &px,
			UserID:   "system_sync",
			Nickname: "Sync",
			NowMs:    time.Now().UnixMilli(),
		}
		for conn := range clients {
			if err := conn.WriteJSON(msg); err != nil {
				conn.Close()
				delete(clients, conn)
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

	stateMutex.Lock()
	for _, px := range pixels {
		coordKey := fmt.Sprintf("%d_%d", px.X, px.Y)
		canvasState[coordKey] = px
	}
	stateMutex.Unlock()
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

func generateID(prefix string, n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf)
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
	var raw map[string]interface{}
	if err := dbClient.NewRef("users/" + uid).Get(ctx, &raw); err != nil {
		return 0
	}
	val, ok := raw["cooldownUntilMs"]
	if !ok {
		return 0
	}
	switch v := val.(type) {
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

func readAdminKey(r *http.Request) string {
	key := strings.TrimSpace(r.Header.Get("X-Admin-Key"))
	if key != "" {
		return key
	}
	return strings.TrimSpace(r.URL.Query().Get("key"))
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
	if appConfig.AdminAPIKey == "" {
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"ok": false, "error": "admin_api_key_not_configured",
		})
		return
	}
	if readAdminKey(r) != appConfig.AdminAPIKey {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"ok": false, "error": "unauthorized",
		})
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

func sendInitialViewportChunks(ws *websocket.Conn) {
	_ = sendChunk(ws, 0, 0)
	_ = sendChunk(ws, 1, 0)
	_ = sendChunk(ws, 0, 1)
	_ = sendChunk(ws, 1, 1)
}

func handleAuthMessage(session *ClientSession, msg ClientMessage) {
	incomingUID := strings.TrimSpace(msg.UserID)
	if incomingUID == "" {
		incomingUID = generateID("user", 8)
	}
	if len(incomingUID) > 64 {
		incomingUID = incomingUID[:64]
	}

	nickname := sanitizeNickname(msg.Nickname)
	if nickname == "Guest" && len(incomingUID) > 6 {
		nickname = "Guest-" + incomingUID[len(incomingUID)-6:]
	}

	session.UserID = incomingUID
	session.Nickname = nickname
	session.Authenticated = true
	session.CooldownUntilMs = loadUserCooldown(session.UserID)
	stats := getOrCreateStats(session.UserID, session.Nickname)
	persistUserProfile(session.UserID, session.Nickname)
	persistUserStats(session.UserID, stats)

	_ = writeServerMessage(session.Conn, ServerMessage{
		Type:       "auth_ok",
		UserID:     session.UserID,
		Nickname:   session.Nickname,
		CooldownMs: int64(appConfig.CooldownDuration / time.Millisecond),
		CooldownUntilMs: session.CooldownUntilMs,
		CooldownBypass: isCooldownBypassed(session.UserID),
		Stats:      stats,
		Leaderboard: buildLeaderboard(5),
		NowMs:      time.Now().UnixMilli(),
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

	if msg.Viewport != nil {
		cx := int(math.Floor(float64(msg.Viewport.CenterX) / float64(appConfig.ChunkSize)))
		cy := int(math.Floor(float64(msg.Viewport.CenterY) / float64(appConfig.ChunkSize)))
		r := msg.Viewport.Radius
		if r <= 0 || r > 4 {
			r = 1
		}
		for dx := -r; dx <= r; dx++ {
			for dy := -r; dy <= r; dy++ {
				_ = sendChunk(session.Conn, cx+dx, cy+dy)
			}
		}
		return
	}

	for _, c := range msg.Chunks {
		_ = sendChunk(session.Conn, c.CX, c.CY)
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer ws.Close()

	session := &ClientSession{
		Conn:            ws,
		ConnID:          generateID("conn", 6),
		Authenticated:   false,
		UserID:          generateID("guest", 6),
		Nickname:        "Guest",
		WindowStart:     time.Now(),
		WindowMsgCount:  0,
		LastPlacementAt: time.Time{},
	}

	clientsMutex.Lock()
	clients[ws] = session
	clientsMutex.Unlock()

	stats := getOrCreateStats(session.UserID, session.Nickname)
	_ = writeServerMessage(ws, ServerMessage{
		Type:       "welcome",
		UserID:     session.UserID,
		Nickname:   session.Nickname,
		CooldownMs: int64(appConfig.CooldownDuration / time.Millisecond),
		CooldownUntilMs: session.CooldownUntilMs,
		CooldownBypass: isCooldownBypassed(session.UserID),
		Stats:      stats,
		Leaderboard: buildLeaderboard(5),
		NowMs:      time.Now().UnixMilli(),
		Message:    "Connected to Pixel World server.",
	})
	_ = writeServerMessage(ws, ServerMessage{
		Type:   "canvas_snapshot",
		Pixels: allPixelsSnapshot(),
		NowMs:  time.Now().UnixMilli(),
	})
	sendInitialViewportChunks(ws)

	for {
		_, payload, err := ws.ReadMessage()
		if err != nil {
			clientsMutex.Lock()
			delete(clients, ws)
			clientsMutex.Unlock()
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
			if msg.Pixel.X < 0 || msg.Pixel.X >= appConfig.GridSize || msg.Pixel.Y < 0 || msg.Pixel.Y >= appConfig.GridSize {
				_ = writeServerMessage(ws, ServerMessage{
					Type:   "pixel_rejected",
					Reason: "out_of_bounds",
					NowMs:  now.UnixMilli(),
				})
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
			broadcast <- PlacementEvent{
				Session: session,
				Pixel:   *msg.Pixel,
			}
		default:
		}
	}
}

func handleMessages() {
	for {
		event := <-broadcast
		msg := event.Pixel

		coordKey := fmt.Sprintf("%d_%d", msg.X, msg.Y)
		stateMutex.Lock()
		canvasState[coordKey] = msg
		stateMutex.Unlock()

		stats := getOrCreateStats(event.Session.UserID, event.Session.Nickname)
		now := time.Now()
		updateUserStatsOnPlacement(stats, msg.Color, now)

		clientsMutex.Lock()
		for client := range clients {
			serverMsg := ServerMessage{
				Type:   "pixel_update",
				Pixel:  &msg,
				UserID: event.Session.UserID,
				Nickname: event.Session.Nickname,
				NowMs:  now.UnixMilli(),
			}
			if err := client.WriteJSON(serverMsg); err != nil {
				client.Close()
				delete(clients, client)
			}
		}
		clientsMutex.Unlock()

		if !isCooldownBypassed(event.Session.UserID) {
			event.Session.CooldownUntilMs = now.Add(appConfig.CooldownDuration).UnixMilli()
		} else {
			event.Session.CooldownUntilMs = 0
		}

		_ = writeServerMessage(event.Session.Conn, ServerMessage{
			Type:       "pixel_accepted",
			Pixel:      &msg,
			CooldownMs: int64(appConfig.CooldownDuration / time.Millisecond),
			CooldownBypass: isCooldownBypassed(event.Session.UserID),
			CooldownUntilMs: event.Session.CooldownUntilMs,
			NowMs:      now.UnixMilli(),
			Stats:      stats,
			Leaderboard: buildLeaderboard(5),
		})

		backupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := dbClient.NewRef("pixels/" + coordKey).Set(backupCtx, msg)
		if err == nil {
			err = dbClient.NewRef("stats/" + event.Session.UserID).Set(backupCtx, stats)
		}
		if err == nil {
			persistUserCooldown(event.Session.UserID, event.Session.CooldownUntilMs)
		}
		cancel()
		if err != nil {
			log.Println("Failed to persist pixel/stats to Firebase:", coordKey, err)
		}
	}
}