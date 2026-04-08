const GRID_SIZE = 200;
const CHUNK_SIZE = 25;
const DEFAULT_COOLDOWN_MS = 30 * 60 * 1000;
let basePixelSize = 6;

const STORAGE_USER_ID = 'pixel_world_user_id';
const STORAGE_NICKNAME = 'pixel_world_nickname';
const STORAGE_COOLDOWN = 'pixel_world_cooldown_until';

const canvas = document.getElementById('placeCanvas');
const ctx = canvas.getContext('2d', { alpha: false });
const colorPicker = document.getElementById('colorPicker');
const timerDisplay = document.getElementById('timerDisplay');
const connStatus = document.getElementById('connStatus');
const statsDisplay = document.getElementById('statsDisplay');
const statsToggleBtn = document.getElementById('statsToggleBtn');
const statsDrawer = document.getElementById('statsDrawer');
const statsCloseBtn = document.getElementById('statsCloseBtn');
const drawerBackdrop = document.getElementById('drawerBackdrop');
const statsTotalEl = document.getElementById('statsTotal');
const statsTodayEl = document.getElementById('statsToday');
const statsLastPlacementEl = document.getElementById('statsLastPlacement');
const statsTopColorsEl = document.getElementById('statsTopColors');
const leaderboardTopEl = document.getElementById('leaderboardTop');
const resetView = document.getElementById('resetView');
const hoverCoordsEl = document.getElementById('hoverCoords');
const cursorCoordsEl = document.getElementById('cursorCoords');
const paletteEl = document.getElementById('palette');

const pixelData = {};
const requestedChunkSet = new Set();
let hoverCell = null;
let dpr = Math.max(1, window.devicePixelRatio || 1);
let cameraZoom = 1;
let cameraX = 0;
let cameraY = 0;
let cooldownTime = 0;
let cooldownMs = DEFAULT_COOLDOWN_MS;
let lastRequestChunksAt = 0;

const state = {
    connected: false,
    authenticated: false,
    drawQueued: false,
    pendingReconnectTimer: null,
    userId: localStorage.getItem(STORAGE_USER_ID) || '',
    nickname: localStorage.getItem(STORAGE_NICKNAME) || '',
    stats: null,
    leaderboard: [],
    cooldownBypass: false,
    wsEndpointIndex: 0,
};

function normalizeNickname(name) {
    const trimmed = (name || '').trim();
    if (!trimmed) return '';
    return trimmed.slice(0, 20);
}

function ensureIdentity() {
    if (!state.nickname) {
        const entered = prompt('Choose your nickname (3-20 chars):', 'PixelArtist') || '';
        state.nickname = normalizeNickname(entered) || 'PixelArtist';
        localStorage.setItem(STORAGE_NICKNAME, state.nickname);
    }
}

function loadCooldownFromStorage() {
    const raw = localStorage.getItem(STORAGE_COOLDOWN);
    if (!raw) return;
    const parsed = Number(raw);
    if (Number.isFinite(parsed) && parsed > Date.now()) {
        cooldownTime = parsed;
    }
}

function saveCooldown() {
    if (cooldownTime > Date.now()) {
        localStorage.setItem(STORAGE_COOLDOWN, String(cooldownTime));
    } else {
        localStorage.removeItem(STORAGE_COOLDOWN);
    }
}

function fmtMs(ms) {
    const totalSeconds = Math.max(0, Math.ceil(ms / 1000));
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

function updateTimerUI() {
    const now = Date.now();
    if (state.cooldownBypass) {
        timerDisplay.textContent = 'Cooldown: bypass';
        timerDisplay.style.color = '#9ee7ff';
        return requestAnimationFrame(updateTimerUI);
    }
    if (now < cooldownTime) {
        timerDisplay.textContent = `Cooldown: ${fmtMs(cooldownTime - now)}`;
        timerDisplay.style.color = '#ff6b6b';
    } else {
        if (cooldownTime !== 0) {
            cooldownTime = 0;
            saveCooldown();
        }
        timerDisplay.textContent = 'Ready to paint!';
        timerDisplay.style.color = '#44ff88';
    }
    requestAnimationFrame(updateTimerUI);
}

function setConnState(kind, text) {
    connStatus.textContent = text;
    if (kind === 'ok') {
        connStatus.style.borderColor = 'rgba(68, 255, 136, 0.65)';
        connStatus.style.background = 'rgba(68, 255, 136, 0.12)';
        connStatus.style.color = '#baffd6';
    } else if (kind === 'warn') {
        connStatus.style.borderColor = 'rgba(255, 190, 78, 0.65)';
        connStatus.style.background = 'rgba(255, 190, 78, 0.12)';
        connStatus.style.color = '#ffe6bb';
    } else {
        connStatus.style.borderColor = 'rgba(255, 107, 107, 0.65)';
        connStatus.style.background = 'rgba(255, 107, 107, 0.12)';
        connStatus.style.color = '#ffd0d0';
    }
}

function renderStats() {
    if (!state.stats) {
        statsDisplay.textContent = 'Placements: 0';
        statsTotalEl.textContent = '0';
        statsTodayEl.textContent = '0';
        statsLastPlacementEl.textContent = '-';
        statsTopColorsEl.innerHTML = '-';
        leaderboardTopEl.innerHTML = '-';
        return;
    }
    const totalPlacements = state.stats.totalPlacements || 0;
    statsDisplay.textContent = `Placements: ${totalPlacements}`;
    statsTotalEl.textContent = String(totalPlacements);
    statsTodayEl.textContent = String(state.stats.placementsToday || 0);

    if (state.stats.lastPlacementAt) {
        const d = new Date(state.stats.lastPlacementAt);
        statsLastPlacementEl.textContent = d.toLocaleString(undefined, {
            month: 'short',
            day: 'numeric',
            year: 'numeric',
            hour: 'numeric',
            minute: '2-digit',
            hour12: true,
        });
    } else {
        statsLastPlacementEl.textContent = '-';
    }

    const colorCounts = state.stats.colorCounts || {};
    const topColors = Object.entries(colorCounts)
        .sort((a, b) => Number(b[1]) - Number(a[1]))
        .slice(0, 3)
        .map(([color, count]) => {
            const normalized = String(color || '').toLowerCase();
            const label = /^[0-9a-f]{6}$/.test(normalized) ? `#${normalized}` : normalized;
            return { normalized, label, count };
        });
    if (topColors.length === 0) {
        statsTopColorsEl.innerHTML = '-';
    } else {
        statsTopColorsEl.innerHTML = topColors
            .map((item) => {
                const swatchStyle = /^[0-9a-f]{6}$/.test(item.normalized)
                    ? ` style="background:${item.label};"`
                    : '';
                return `<div class="top-color-item"><span class="top-color-swatch"${swatchStyle}></span><span>${item.label} (${item.count})</span></div>`;
            })
            .join('');
    }

    const leaderboardEntries = (state.leaderboard || [])
        .slice(0, 5)
        .map((entry, idx) => ({
            rank: idx + 1,
            nickname: entry.nickname || 'Guest',
            totalPlacements: entry.totalPlacements || 0,
        }));
    if (leaderboardEntries.length === 0) {
        leaderboardTopEl.innerHTML = '-';
    } else {
        leaderboardTopEl.innerHTML = leaderboardEntries
            .map((entry) => `<div class="leaderboard-item"><span class="leaderboard-rank">#${entry.rank}</span><span>${entry.nickname} (${entry.totalPlacements})</span></div>`)
            .join('');
    }
}

function setDrawerOpen(isOpen) {
    if (isOpen) {
        statsDrawer.classList.add('open');
        drawerBackdrop.classList.add('show');
        statsDrawer.setAttribute('aria-hidden', 'false');
        drawerBackdrop.setAttribute('aria-hidden', 'false');
        return;
    }
    statsDrawer.classList.remove('open');
    drawerBackdrop.classList.remove('show');
    statsDrawer.setAttribute('aria-hidden', 'true');
    drawerBackdrop.setAttribute('aria-hidden', 'true');
}

function getWebSocketCandidates() {
    const candidates = [];
    if (location.origin && location.origin.startsWith('http')) {
        candidates.push(`${location.origin.replace(/^http/, 'ws')}/ws`);
    }
    if (location.hostname) {
        const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        candidates.push(`${proto}//${location.hostname}:8080/ws`);
    } else {
        candidates.push('ws://localhost:8080/ws');
    }

    // De-duplicate while preserving order.
    return [...new Set(candidates)];
}

function queueDraw() {
    if (state.drawQueued) return;
    state.drawQueued = true;
    requestAnimationFrame(() => {
        state.drawQueued = false;
        draw();
    });
}

function clamp(v, a, b) {
    return Math.max(a, Math.min(b, v));
}

function resizeCanvas() {
    dpr = Math.max(1, window.devicePixelRatio || 1);
    const controls = document.querySelector('.controls');
    const padding = 12;
    const isMobile = window.matchMedia('(max-width: 900px)').matches;
    const scale = isMobile ? 2.2 : 4;
    const availW = window.innerWidth - padding * 2;
    const availH = window.innerHeight - controls.offsetHeight - padding * 2 - 20;
    const sideBase = Math.max(520, Math.min(availW, availH));
    const side = sideBase * scale;

    canvas.style.width = side + 'px';
    canvas.style.height = side + 'px';
    canvas.width = Math.floor(side * dpr);
    canvas.height = Math.floor(side * dpr);

    basePixelSize = Math.max(
        1,
        Math.floor(Math.min(canvas.width / GRID_SIZE, canvas.height / GRID_SIZE))
    );

    const worldSize = GRID_SIZE * basePixelSize;
    cameraZoom = clamp(cameraZoom, 0.5, 5);
    cameraX = (canvas.width - worldSize) / 2;
    cameraY = (canvas.height - worldSize) / 2;
    queueDraw();
    requestVisibleChunks();
}

function worldToChunk(x, y) {
    return {
        cx: Math.floor(x / CHUNK_SIZE),
        cy: Math.floor(y / CHUNK_SIZE),
    };
}

function sendWS(obj) {
    if (!socketRef || socketRef.readyState !== WebSocket.OPEN) return;
    socketRef.send(JSON.stringify(obj));
}

function requestVisibleChunks() {
    const now = Date.now();
    if (now - lastRequestChunksAt < 300) return;
    lastRequestChunksAt = now;

    const center = getWorldCenter();
    const centerChunk = worldToChunk(center.x, center.y);
    const radius = cameraZoom < 0.9 ? 2 : 1;
    const chunks = [];
    for (let dx = -radius; dx <= radius; dx++) {
        for (let dy = -radius; dy <= radius; dy++) {
            const cx = centerChunk.cx + dx;
            const cy = centerChunk.cy + dy;
            const key = `${cx}_${cy}`;
            if (requestedChunkSet.has(key)) continue;
            requestedChunkSet.add(key);
            chunks.push({ cx, cy });
        }
    }
    if (chunks.length === 0) return;
    sendWS({
        type: 'request_chunks',
        chunks,
        viewport: { centerX: center.x, centerY: center.y, zoom: cameraZoom, radius },
    });
}

function getWorldCenter() {
    const sx = canvas.width / 2;
    const sy = canvas.height / 2;
    const worldX = Math.floor((sx - cameraX) / cameraZoom / basePixelSize);
    const worldY = Math.floor((sy - cameraY) / cameraZoom / basePixelSize);
    return {
        x: clamp(worldX, 0, GRID_SIZE - 1),
        y: clamp(worldY, 0, GRID_SIZE - 1),
    };
}

function getCellFromClient(clientX, clientY) {
    const rect = canvas.getBoundingClientRect();
    const sx = (clientX - rect.left) * dpr;
    const sy = (clientY - rect.top) * dpr;
    const worldX = (sx - cameraX) / cameraZoom;
    const worldY = (sy - cameraY) / cameraZoom;
    const gridX = Math.floor(worldX / basePixelSize);
    const gridY = Math.floor(worldY / basePixelSize);
    if (gridX < 0 || gridX >= GRID_SIZE || gridY < 0 || gridY >= GRID_SIZE) return null;
    return { x: gridX, y: gridY };
}

function draw() {
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.fillStyle = '#070a10';
    ctx.fillRect(0, 0, canvas.width, canvas.height);

    ctx.setTransform(cameraZoom, 0, 0, cameraZoom, cameraX, cameraY);
    ctx.imageSmoothingEnabled = false;
    const worldSize = GRID_SIZE * basePixelSize;

    ctx.lineWidth = 1 / cameraZoom;
    ctx.strokeStyle = 'rgba(255,255,255,0.08)';
    for (let i = 0; i <= GRID_SIZE; i++) {
        const p = i * basePixelSize;
        ctx.beginPath();
        ctx.moveTo(p, 0);
        ctx.lineTo(p, worldSize);
        ctx.stroke();
        ctx.beginPath();
        ctx.moveTo(0, p);
        ctx.lineTo(worldSize, p);
        ctx.stroke();
    }

    for (const key in pixelData) {
        const [x, y] = key.split('_');
        ctx.fillStyle = pixelData[key];
        ctx.fillRect(Number(x) * basePixelSize, Number(y) * basePixelSize, basePixelSize, basePixelSize);
    }

    ctx.strokeStyle = 'rgba(78, 115, 255, 0.45)';
    ctx.strokeRect(0, 0, worldSize, worldSize);

    if (hoverCell) {
        ctx.lineWidth = 2 / cameraZoom;
        ctx.strokeStyle = 'rgba(0, 255, 255, 0.9)';
        ctx.strokeRect(hoverCell.x * basePixelSize, hoverCell.y * basePixelSize, basePixelSize, basePixelSize);
    }
}

function handleServerMessage(data) {
    switch (data.type) {
        case 'welcome':
        case 'auth_ok':
            if (data.userId) {
                state.userId = data.userId;
                localStorage.setItem(STORAGE_USER_ID, state.userId);
            }
            if (data.nickname) {
                state.nickname = data.nickname;
                localStorage.setItem(STORAGE_NICKNAME, state.nickname);
            }
            if (data.cooldownMs) cooldownMs = data.cooldownMs;
            state.cooldownBypass = Boolean(data.cooldownBypass);
            state.authenticated = (data.type === 'auth_ok') ? true : state.authenticated;
            if (!state.cooldownBypass && Number.isFinite(Number(data.cooldownUntilMs))) {
                const until = Number(data.cooldownUntilMs);
                cooldownTime = until > Date.now() ? until : 0;
                saveCooldown();
            }
            if (data.stats) state.stats = data.stats;
            if (Array.isArray(data.leaderboard)) state.leaderboard = data.leaderboard;
            renderStats();
            requestVisibleChunks();
            break;
        case 'chunk_data':
            if (data.chunk && Array.isArray(data.chunk.pixels)) {
                for (const px of data.chunk.pixels) {
                    pixelData[`${px.x}_${px.y}`] = px.color;
                }
                queueDraw();
            }
            break;
        case 'canvas_snapshot':
            if (Array.isArray(data.pixels)) {
                for (const px of data.pixels) {
                    pixelData[`${px.x}_${px.y}`] = px.color;
                }
                queueDraw();
            }
            break;
        case 'pixel_update':
            if (data.pixel) {
                pixelData[`${data.pixel.x}_${data.pixel.y}`] = data.pixel.color;
                queueDraw();
            }
            break;
        case 'pixel_accepted':
            if (data.pixel) {
                pixelData[`${data.pixel.x}_${data.pixel.y}`] = data.pixel.color;
                queueDraw();
            }
            state.cooldownBypass = Boolean(data.cooldownBypass);
            if (!state.cooldownBypass) {
                const until = Number(data.cooldownUntilMs || 0);
                if (until > Date.now()) {
                    cooldownTime = until;
                } else {
                    cooldownTime = Date.now() + cooldownMs;
                }
                saveCooldown();
            }
            if (data.stats) state.stats = data.stats;
            if (Array.isArray(data.leaderboard)) state.leaderboard = data.leaderboard;
            renderStats();
            break;
        case 'pixel_rejected':
            if (data.reason === 'cooldown') {
                const until = Number(data.cooldownUntilMs || 0);
                if (until > Date.now()) {
                    cooldownTime = until;
                } else {
                    const retryAfter = Number(data.retryAfterMs || 0);
                    cooldownTime = Date.now() + retryAfter;
                }
                saveCooldown();
            } else if (data.reason === 'out_of_bounds') {
                setConnState('warn', 'Out of bounds');
                setTimeout(() => setConnState('ok', 'Connected'), 1200);
            } else if (data.reason === 'auth_required') {
                setConnState('warn', 'Please sign in first');
            }
            break;
        case 'error':
            if (data.reason === 'anti_spam') {
                setConnState('warn', 'Slow down (anti-spam)');
                setTimeout(() => setConnState('ok', 'Connected'), 1500);
            }
            break;
        default:
            break;
    }
}

function connectWebSocket() {
    const wsCandidates = getWebSocketCandidates();
    if (state.wsEndpointIndex >= wsCandidates.length) {
        state.wsEndpointIndex = 0;
    }
    const wsUrl = wsCandidates[state.wsEndpointIndex];
    setConnState('warn', `Connecting... (${state.wsEndpointIndex + 1}/${wsCandidates.length})`);
    state.connected = false;

    const socket = new WebSocket(wsUrl);
    const connectTimeout = setTimeout(() => {
        if (socket.readyState === WebSocket.CONNECTING) {
            setConnState('err', 'Connect timeout (retrying...)');
            socket.close();
        }
    }, 8000);
    socket.onopen = () => {
        clearTimeout(connectTimeout);
        state.connected = true;
        connectWebSocket.attempts = 0;
        setConnState('ok', 'Connected');
        sendWS({
            type: 'auth',
            userId: state.userId,
            nickname: state.nickname,
            clientVersion: '1',
        });
        requestVisibleChunks();
        queueDraw();
    };

    socket.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            handleServerMessage(data);
        } catch {
            // Ignore bad server message
        }
    };

    socket.onerror = () => {
        setConnState('err', 'WebSocket error (retrying...)');
        // Some browsers don't immediately trigger close after error.
        try {
            socket.close();
        } catch {
            // no-op
        }
    };

    socket.onclose = () => {
        clearTimeout(connectTimeout);
        state.connected = false;
        setConnState('err', 'Disconnected (reconnecting...)');
        if (wsCandidates.length > 1) {
            state.wsEndpointIndex = (state.wsEndpointIndex + 1) % wsCandidates.length;
        }
        if (state.pendingReconnectTimer) clearTimeout(state.pendingReconnectTimer);
        const attempts = (connectWebSocket.attempts = (connectWebSocket.attempts || 0) + 1);
        const delay = Math.min(7000, 500 + attempts * 300);
        state.pendingReconnectTimer = setTimeout(() => {
            socketRef = connectWebSocket();
        }, delay);
    };

    return socket;
}

function placePixel(cell) {
    if (!cell) return;
    if (!state.authenticated) return;
    if (!state.cooldownBypass && Date.now() < cooldownTime) return;
    sendWS({
        type: 'place_pixel',
        pixel: { x: cell.x, y: cell.y, color: colorPicker.value },
    });
}

const palette = ['#ff0000', '#ff7a00', '#ffd400', '#00d084', '#00c2ff', '#0066ff', '#7a00ff', '#ffffff', '#c0c0c0', '#444444', '#000000', '#ff3b7a', '#00ff88'];
for (const c of palette) {
    const sw = document.createElement('div');
    sw.className = 'swatch';
    sw.style.background = c;
    sw.title = c;
    sw.addEventListener('click', () => (colorPicker.value = c));
    paletteEl.appendChild(sw);
}

resetView.addEventListener('click', () => {
    cameraZoom = 1;
    resizeCanvas();
});

window.addEventListener('keydown', (e) => {
    if (e.key === 'r' || e.key === 'R') {
        cameraZoom = 1;
        resizeCanvas();
    }
    if (e.key === 'Escape') {
        setDrawerOpen(false);
    }
});

statsToggleBtn.addEventListener('click', () => setDrawerOpen(true));
statsCloseBtn.addEventListener('click', () => setDrawerOpen(false));
drawerBackdrop.addEventListener('click', () => setDrawerOpen(false));

let pointerDown = false;
let dragModePan = false;
let moved = false;
let pointerId = null;
let startClientX = 0;
let startClientY = 0;
let startCameraX = 0;
let startCameraY = 0;

function updateHover(e) {
    const cell = getCellFromClient(e.clientX, e.clientY);
    hoverCell = cell;
    hoverCoordsEl.textContent = cell ? `${cell.x}, ${cell.y}` : '-';
    cursorCoordsEl.textContent = `${Math.round((e.clientX - canvas.getBoundingClientRect().left) * dpr)}, ${Math.round((e.clientY - canvas.getBoundingClientRect().top) * dpr)}`;
    queueDraw();
}

canvas.addEventListener('contextmenu', (e) => e.preventDefault());
canvas.addEventListener('pointerdown', (e) => {
    dragModePan = e.pointerType === 'mouse' && e.button === 2 ? true : e.shiftKey;
    pointerDown = true;
    moved = false;
    pointerId = e.pointerId;
    startClientX = e.clientX;
    startClientY = e.clientY;
    startCameraX = cameraX;
    startCameraY = cameraY;
    canvas.setPointerCapture(pointerId);
});

canvas.addEventListener('pointermove', (e) => {
    updateHover(e);
    if (!pointerDown) return;
    const dx = e.clientX - startClientX;
    const dy = e.clientY - startClientY;
    const dist = Math.hypot(dx, dy);
    if (!dragModePan && dist > 6) dragModePan = true;
    if (dragModePan) {
        cameraX = startCameraX + dx * dpr;
        cameraY = startCameraY + dy * dpr;
        queueDraw();
        requestVisibleChunks();
    }
    if (dist > 6) moved = true;
});

canvas.addEventListener('pointerup', (e) => {
    if (!pointerDown || e.pointerId !== pointerId) return;
    pointerDown = false;
    const cell = getCellFromClient(e.clientX, e.clientY);
    if (!moved && !dragModePan) {
        placePixel(cell);
    }
    dragModePan = false;
    moved = false;
    pointerId = null;
});

canvas.addEventListener('pointercancel', () => {
    pointerDown = false;
    dragModePan = false;
    moved = false;
    pointerId = null;
});

canvas.addEventListener('wheel', (e) => {
    e.preventDefault();
    const direction = e.deltaY > 0 ? -1 : 1;
    const nextZoom = cameraZoom * (direction > 0 ? 1.08 : 0.92);
    const clampedZoom = clamp(nextZoom, 0.5, 5);
    if (Math.abs(clampedZoom - cameraZoom) < 0.0001) return;

    const rect = canvas.getBoundingClientRect();
    const sx = (e.clientX - rect.left) * dpr;
    const sy = (e.clientY - rect.top) * dpr;
    const worldX = (sx - cameraX) / cameraZoom;
    const worldY = (sy - cameraY) / cameraZoom;
    cameraZoom = clampedZoom;
    cameraX = sx - worldX * cameraZoom;
    cameraY = sy - worldY * cameraZoom;
    queueDraw();
    requestVisibleChunks();
}, { passive: false });

window.addEventListener('resize', resizeCanvas);

let socketRef = null;
ensureIdentity();
loadCooldownFromStorage();
updateTimerUI();
resizeCanvas();
socketRef = connectWebSocket();

