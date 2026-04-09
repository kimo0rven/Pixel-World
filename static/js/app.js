const DEFAULT_GRID_SIZE = 200;
const DEFAULT_CHUNK_SIZE = 25;
const DEFAULT_COOLDOWN_MS = 30 * 60 * 1000;
const FAST_PLACEMENT_THRESHOLD = 3; // Show captcha after N placements in 5 seconds
const FAST_PLACEMENT_WINDOW_MS = 5000;
let gridSize = DEFAULT_GRID_SIZE;
let chunkSize = DEFAULT_CHUNK_SIZE;
let basePixelSize = 6;

const STORAGE_USER_ID = 'pixel_world_user_id';
const STORAGE_COOLDOWN = 'pixel_world_cooldown_until';
const STORAGE_THEME = 'pixel_world_theme';
const STORAGE_IDENTITY_TOKEN = 'pixel_world_identity_token';
const STORAGE_FIREBASE_ID_TOKEN = 'pixel_world_firebase_id_token';
const STORAGE_EMAIL = 'pixel_world_email';

const canvas = document.getElementById('placeCanvas');
const ctx = canvas.getContext('2d', { alpha: false });
const colorActionBtn = document.getElementById('colorActionBtn');
const colorActionText = document.getElementById('colorActionText');
const colorActionSwatch = document.getElementById('colorActionSwatch');
const colorPanel = document.getElementById('colorPanel');
const profileMenuWrap = document.getElementById('profileMenuWrap');
const profileMenuBtn = document.getElementById('profileMenuBtn');
const profileDropdown = document.getElementById('profileDropdown');
const nicknameDisplay = document.getElementById('nicknameDisplay');
const profileMenuPixels = document.getElementById('profileMenuPixels');
const profileMenuLevel = document.getElementById('profileMenuLevel');
const profileAvatarRing = document.getElementById('profileAvatarRing');
const profileAvatarEmoji = document.getElementById('profileAvatarEmoji');
const profileMenuLevelLabel = document.getElementById('profileMenuLevelLabel');
const canvasTopStatus = document.getElementById('canvasTopStatus');
const zoomDisplay = document.getElementById('zoomDisplay');
const timerDisplay = document.getElementById('timerDisplay');
const connStatus = document.getElementById('connStatus');
const statsDisplay = document.getElementById('statsDisplay');
const statsToggleBtn = document.getElementById('statsToggleBtn');
const statsDrawer = document.getElementById('statsDrawer');
const statsCloseBtn = document.getElementById('statsCloseBtn');
const drawerBackdrop = document.getElementById('drawerBackdrop');
const authModal = document.getElementById('authModal');
const authBkg = document.getElementById('authBkg');
const authCloseBtn = document.getElementById('authCloseBtn');
const authGoogleBtn = document.getElementById('authGoogleBtn');
const authStatusMsg = document.getElementById('authStatusMsg');
const nicknameModal = document.getElementById('nicknameModal');
const nicknameBkg = document.getElementById('nicknameBkg');
const nicknameInput = document.getElementById('nicknameInput');
const nicknameSaveBtn = document.getElementById('nicknameSaveBtn');
const nicknameCancelBtn = document.getElementById('nicknameCancelBtn');
const captchaModal = document.getElementById('captchaModal');
const captchaBkg = document.getElementById('captchaBkg');
const captchaOkBtn = document.getElementById('captchaOkBtn');
const statsTotalEl = document.getElementById('statsTotal');
const statsTodayEl = document.getElementById('statsToday');
const statsLastPlacementEl = document.getElementById('statsLastPlacement');
const statsRetained1hEl = document.getElementById('statsRetained1h');
const statsRetained24hEl = document.getElementById('statsRetained24h');
const statsAvgLifetimeEl = document.getElementById('statsAvgLifetime');
const statsLongestLifetimeEl = document.getElementById('statsLongestLifetime');
const statsTopColorsEl = document.getElementById('statsTopColors');
const colorLeaderboardEl = document.getElementById('colorLeaderboard');
const leaderboardTopEl = document.getElementById('leaderboardTop');
const hoverCoordsEl = document.getElementById('hoverCoords');
const cursorCoordsEl = document.getElementById('cursorCoords');
const hoverUsernameRow = document.getElementById('hoverUsernameRow');
const hoverUsernameEl = document.getElementById('hoverUsername');
const hoverHistoryRow = document.getElementById('hoverHistoryRow');
const hoverHistoryEl = document.getElementById('hoverHistory');
const paletteEl = document.getElementById('palette');
const gridToggle = document.getElementById('gridToggle');
const themeToggle = document.getElementById('themeToggle');
const coordX = document.getElementById('coordX');
const coordY = document.getElementById('coordY');
const placeCoordBtn = document.getElementById('placeCoordBtn');
const placementSound = document.getElementById('placementSound');
const placePixelBtn = document.getElementById('placePixelBtn');
const undoPixelBtn = document.getElementById('undoPixelBtn');
const redoPixelBtn = document.getElementById('redoPixelBtn');
const eyedropperBtn = document.getElementById('eyedropperBtn');
const shortcutsModal = document.getElementById('shortcutsModal');
const shortcutsBkg = document.getElementById('shortcutsBkg');
const shortcutsCloseBtn = document.getElementById('shortcutsCloseBtn');
const profileModal = document.getElementById('profileModal');
const profileBkg = document.getElementById('profileBkg');
const profileCloseBtn = document.getElementById('profileCloseBtn');
const profileUsernameEl = document.getElementById('profileUsername');
const profileTotalEl = document.getElementById('profileTotal');
const profileTodayEl = document.getElementById('profileToday');
const profileLastEl = document.getElementById('profileLast');
const profileColorsEl = document.getElementById('profileColors');
const hud = document.getElementById('hud');

const pixelData = {}; // Now stores {color, username, placedAt}
const cellHistoryCache = {};
const requestedChunkSet = new Set();
let hoverCell = null;
let hoverUsername = null;
let lastHistoryRequestKey = '';
let dpr = Math.max(1, window.devicePixelRatio || 1);
let cameraZoom = 1;
let cameraX = 0;
let cameraY = 0;
let cooldownTime = 0;
let cooldownMs = DEFAULT_COOLDOWN_MS;
let lastRequestChunksAt = 0;
let lastPlacementTimes = []; // Track placement times for captcha detection
let ownPixels = new Set(); // Track pixels placed by user (format: "x_y")
let lastPlacedPixel = null; // For undo functionality (format: {x, y, time})
let lastPlacedPixelTime = null;
let pendingUndoSnapshot = null; // { x, y, color }
let lastUndonePixel = null; // { x, y, color }
let lastOptimisticPixel = null; // { key, prev } — used to revert on rejection
let showGrid = false; // Toggle for grid display
let eyedropperMode = false;
let firebaseClientAuth = null;

const state = {
    connected: false,
    authenticated: false,
    drawQueued: false,
    pendingReconnectTimer: null,
    userId: localStorage.getItem(STORAGE_USER_ID) || '',
    nickname: '', // always set from server auth_ok, never from localStorage
    firebaseIdToken: localStorage.getItem(STORAGE_FIREBASE_ID_TOKEN) || '',
    email: localStorage.getItem(STORAGE_EMAIL) || '',
    firebaseWebApiKey: '',
    firebaseProjectId: '',
    firebaseAuthDomain: '',
    stats: null,
    leaderboard: [],
    colorStats: {},
    cooldownBypass: false,
    wsEndpointIndex: 0,
    theme: localStorage.getItem(STORAGE_THEME) || 'dark',
    connStateKind: 'ok',
    selectedColor: '#ff4500',
    intentionalLogout: false,
};

const DEBUG = localStorage.getItem('pixel_world_debug') === '1';
function debugLog(...args) {
    if (DEBUG) console.log(...args);
}
function debugError(...args) {
    if (DEBUG) console.error(...args);
}

function normalizeNickname(name) {
    const trimmed = (name || '').trim();
    if (!trimmed) return '';
    return trimmed.slice(0, 20);
}

function generateRandomNickname() {
    const adjectives = ['Swift', 'Bright', 'Cosmic', 'Pixel', 'Neon', 'Crimson', 'Aqua', 'Solar', 'Nova', 'Lucky'];
    const nouns = ['Painter', 'Brush', 'Canvas', 'Comet', 'Falcon', 'Otter', 'Wolf', 'Panda', 'Maker', 'Rider'];
    const adj = adjectives[Math.floor(Math.random() * adjectives.length)];
    const noun = nouns[Math.floor(Math.random() * nouns.length)];
    const digits = String(Math.floor(100 + Math.random() * 900));
    return normalizeNickname(`${adj}${noun}${digits}`);
}

function applyTheme() {
    const isLight = state.theme === 'light';
    document.body.classList.toggle('light-theme', isLight);
    const icon = document.getElementById('themeToggleIcon');
    if (icon) {
        if (isLight) {
            // show moon icon (click to go dark)
            icon.innerHTML = '<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path>';
        } else {
            // show sun icon (click to go light)
            icon.innerHTML = '<circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line>';
        }
    }
    // Sync grid button active state
    const gridLabel = document.getElementById('gridToggleLabel');
    if (gridLabel) gridLabel.classList.toggle('active', showGrid);
}

function toggleTheme() {
    state.theme = state.theme === 'dark' ? 'light' : 'dark';
    localStorage.setItem(STORAGE_THEME, state.theme);
    applyTheme();
    if (connStatus) {
        setConnState(state.connStateKind, connStatus.textContent);
    }
    queueDraw();
}

function toggleGrid() {
    showGrid = !showGrid;
    gridToggle.checked = showGrid;
    const gridLabel = document.getElementById('gridToggleLabel');
    if (gridLabel) gridLabel.classList.toggle('active', showGrid);
    queueDraw();
}

function playPlacementSound() {
    try {
        if (placementSound && placementSound.play) {
            placementSound.currentTime = 0;
            placementSound.play().catch(() => {
                // Silently fail if audio can't play
            });
        }
    } catch (e) {
        // Silently fail
    }
}

function updateZoomDisplay() {
    if (zoomDisplay) {
        zoomDisplay.textContent = `Zoom: ${cameraZoom.toFixed(1)}x`;
    }
    updateCanvasTopStatus();
}

function formatZoomLabel() {
    const fixed = cameraZoom.toFixed(1);
    return fixed.endsWith('.0') ? `${fixed.slice(0, -2)}x` : `${fixed}x`;
}

function updateCanvasTopStatus() {
    if (!canvasTopStatus) return;
    const coords = hoverCell ? `(${hoverCell.x}, ${hoverCell.y})` : '(-, -)';
    canvasTopStatus.textContent = `${coords} ${formatZoomLabel()}`;
}

function updateColorActionLabel() {
    if (!colorActionText) return;
    if (!state.authenticated || state.cooldownBypass) {
        colorActionText.textContent = 'Paint';
        return;
    }
    const remainingMs = cooldownTime - Date.now();
    if (remainingMs > 0) {
        const seconds = Math.max(1, Math.ceil(remainingMs / 1000));
        colorActionText.textContent = `Paint (${seconds}s)`;
        return;
    }
    colorActionText.textContent = 'Paint';
}

function setColorPanelOpen(isOpen) {
    if (!colorPanel) return;
    const nextOpen = Boolean(isOpen) && state.authenticated;
    colorPanel.classList.toggle('open', nextOpen);
    colorPanel.setAttribute('aria-hidden', nextOpen ? 'false' : 'true');
}

function setProfileMenuOpen(isOpen) {
    if (!profileDropdown || !profileMenuBtn) return;
    const nextOpen = Boolean(isOpen) && state.authenticated;
    profileDropdown.classList.toggle('open', nextOpen);
    profileDropdown.setAttribute('aria-hidden', nextOpen ? 'false' : 'true');
    profileMenuBtn.setAttribute('aria-expanded', nextOpen ? 'true' : 'false');
}

function updateProfileMenuSummary() {
    const total = Number(state.stats?.totalPlacements || 0);
    const level = Number(state.stats?.level || 0) || Math.max(1, Math.floor(total / 25) + 1);
    const levelExpCurrent = Number(state.stats?.levelExpCurrent);
    const levelExpRequired = Number(state.stats?.levelExpRequired);
    const progressPct = Number.isFinite(levelExpCurrent) && Number.isFinite(levelExpRequired) && levelExpRequired > 0
        ? Math.max(0, Math.min(100, (levelExpCurrent / levelExpRequired) * 100))
        : Math.max(0, Math.min(100, ((total % 25) / 25) * 100));

    if (profileMenuPixels) profileMenuPixels.textContent = String(total);
    if (profileMenuLevel) profileMenuLevel.textContent = String(level);
    const levelPctEl = document.getElementById('profileMenuLevelPct');
    if (levelPctEl) {
        levelPctEl.textContent = `(${Math.round(progressPct)}%)`;
    }
    if (profileMenuLevelLabel) {
        profileMenuLevelLabel.textContent = String(level);
    }
    if (profileAvatarRing) {
        profileAvatarRing.style.setProperty('--xp-progress', `${progressPct}%`);
    }
    if (profileAvatarEmoji) {
        profileAvatarEmoji.textContent = '🤖';
    }
    if (profileMenuBtn) {
        const label = state.authenticated ? `Profile menu for ${state.nickname || 'User'}` : 'Login';
        profileMenuBtn.setAttribute('aria-label', label);
    }
}

function checkFastPlacement() {
    const now = Date.now();
    lastPlacementTimes = lastPlacementTimes.filter(t => now - t < FAST_PLACEMENT_WINDOW_MS);
    lastPlacementTimes.push(now);
    
    if (lastPlacementTimes.length >= FAST_PLACEMENT_THRESHOLD) {
        showCaptchaModal();
        return true;
    }
    return false;
}

function showCaptchaModal() {
    captchaModal.classList.add('show');
    captchaBkg.classList.add('show');
    captchaModal.setAttribute('aria-hidden', 'false');
    captchaBkg.setAttribute('aria-hidden', 'false');
}

function hideCaptchaModal() {
    captchaModal.classList.remove('show');
    captchaBkg.classList.remove('show');
    captchaModal.setAttribute('aria-hidden', 'true');
    captchaBkg.setAttribute('aria-hidden', 'true');
}

function placePixelAtCoordinates(x, y) {
    if (x < 0 || x >= gridSize || y < 0 || y >= gridSize) {
        alert(`Coordinates out of bounds. Must be 0-${gridSize - 1}.`);
        return;
    }

    const pixelColor = state.selectedColor;
    sendWS({
        type: 'place_pixel',
        userId: state.userId,
        nickname: state.nickname,
        pixel: {
            x: Math.floor(x),
            y: Math.floor(y),
            color: pixelColor,
        },
    });
}

function renderColorLeaderboard() {
    const colorStats = state.colorStats || {};
    const entries = Object.entries(colorStats)
        .sort((a, b) => Number(b[1]) - Number(a[1]))
        .slice(0, 5)
        .map(([color, count]) => {
            const normalized = String(color || '').toLowerCase();
            const label = /^[0-9a-f]{6}$/.test(normalized) ? `#${normalized}` : normalized;
            return { normalized, label, count };
        });
    
    if (entries.length === 0) {
        colorLeaderboardEl.innerHTML = '-';
    } else {
        colorLeaderboardEl.innerHTML = entries
            .map((item) => {
                const swatchStyle = /^[0-9a-f]{6}$/.test(item.normalized)
                    ? ` style="background:${item.label};"`
                    : '';
                return `<div class="color-leaderboard-item"><span class="color-leaderboard-swatch"${swatchStyle}></span><span class="color-leaderboard-name">${item.label}</span><span class="color-leaderboard-count">${item.count}</span></div>`;
            })
            .join('');
    }
}

function showModal(modal, backdrop) {
    debugLog('showModal called for:', modal?.id, backdrop?.id);
    if (!modal || !backdrop) {
        debugError('showModal: modal or backdrop is null/undefined', { modal, backdrop });
        return;
    }
    modal.classList.add('show');
    backdrop.classList.add('show');
    modal.setAttribute('aria-hidden', 'false');
    backdrop.setAttribute('aria-hidden', 'false');
    debugLog('showModal: classes added, modal is now visible');
}

function hideModal(modal, backdrop) {
    debugLog('hideModal called for:', modal?.id, backdrop?.id);
    if (!modal || !backdrop) {
        debugError('hideModal: modal or backdrop is null/undefined', { modal, backdrop });
        return;
    }
    modal.classList.remove('show');
    backdrop.classList.remove('show');
    modal.setAttribute('aria-hidden', 'true');
    backdrop.setAttribute('aria-hidden', 'true');
    debugLog('hideModal: classes removed, modal is now hidden');
}

function showProfileModal(userID) {
    debugLog('showProfileModal called with userID:', userID);
    debugLog('state.leaderboard:', state.leaderboard);
    // Find the user in the leaderboard or stats
    const user = state.leaderboard.find(u => u.userId === userID);
    debugLog('Found user:', user);
    if (!user) {
        debugError('User not found in leaderboard');
        return;
    }

    profileUsernameEl.textContent = user.nickname || 'Unknown User';
    profileTotalEl.textContent = user.totalPlacements || 0;
    profileTodayEl.textContent = user.placementsToday || 0;
    profileLastEl.textContent = '-';

    // For now, show a note that detailed profile data isn't available
    // In a real app, you'd fetch this from the server
    profileColorsEl.innerHTML = '<em style="opacity:0.6;">Profile data not yet available</em>';

    debugLog('About to show profile modal');
    showModal(profileModal, profileBkg);
}

function updatePlacePixelButtonState() {
    if (!placePixelBtn) return;
    const now = Date.now();
    const timeUntilReady = cooldownTime - now;

    if (state.cooldownBypass) {
        placePixelBtn.disabled = false;
        placePixelBtn.textContent = 'Place Pixel';
        placePixelBtn.classList.remove('btn-cooling');
        placePixelBtn.style.setProperty('--cooldown-progress', '0%');
        return;
    }

    if (timeUntilReady > 0) {
        placePixelBtn.disabled = true;
        placePixelBtn.textContent = `Wait ${fmtMs(timeUntilReady)}`;
        placePixelBtn.classList.add('btn-cooling');
        // Calculate progress: 0% at start of cooldown, 100% when ready
        const totalCooldown = cooldownMs;
        const elapsed = totalCooldown - timeUntilReady;
        const progress = Math.max(0, Math.min(100, (elapsed / totalCooldown) * 100));
        placePixelBtn.style.setProperty('--cooldown-progress', progress + '%');
    } else {
        placePixelBtn.disabled = false;
        placePixelBtn.textContent = 'Place Pixel';
        placePixelBtn.classList.remove('btn-cooling');
        placePixelBtn.style.setProperty('--cooldown-progress', '100%');
    }
}

function placePixelAtHover() {
    if (!hoverCell) {
        alert('Move your cursor over the canvas first');
        return;
    }
    placePixel(hoverCell);
}

function requestUndoPixel() {
    if (!lastPlacedPixel || !lastPlacedPixelTime) return;

    const elapsed = Date.now() - lastPlacedPixelTime;
    if (elapsed > 30000) {
        alert('Undo window expired (30 seconds)');
        return;
    }

    const coordKey = `${lastPlacedPixel.x}_${lastPlacedPixel.y}`;
    const pixelInfo = pixelData[coordKey];
    const rawColor = pixelInfo ? (typeof pixelInfo === 'string' ? pixelInfo : pixelInfo.color) : '';
    const normalizedColor = rawColor
        ? (rawColor.startsWith('#') ? rawColor : `#${rawColor}`)
        : state.selectedColor;
    pendingUndoSnapshot = {
        x: lastPlacedPixel.x,
        y: lastPlacedPixel.y,
        color: normalizedColor,
    };

    sendWS({
        type: 'undo_pixel',
        pixel: lastPlacedPixel,
    });
}

function requestRedoPixel() {
    if (!lastUndonePixel) return;
    if (!state.authenticated) return;
    if (!state.cooldownBypass && Date.now() < cooldownTime) return;

    sendWS({
        type: 'place_pixel',
        pixel: {
            x: Math.floor(lastUndonePixel.x),
            y: Math.floor(lastUndonePixel.y),
            color: lastUndonePixel.color,
        },
    });
    lastUndonePixel = null;
    if (redoPixelBtn) {
        redoPixelBtn.disabled = true;
    }
}

function setEyedropperMode(enabled) {
    eyedropperMode = Boolean(enabled);
    if (eyedropperBtn) {
        eyedropperBtn.classList.toggle('active', eyedropperMode);
        eyedropperBtn.setAttribute('aria-pressed', eyedropperMode ? 'true' : 'false');
    }
    canvas.style.cursor = eyedropperMode ? 'copy' : 'crosshair';
}

function sampleColorFromCell(cell) {
    if (!cell) {
        setConnState('warn', 'Move cursor over a painted pixel first');
        setTimeout(() => setConnState('ok', state.authenticated ? 'Connected' : 'Browsing'), 1200);
        return;
    }
    const key = `${cell.x}_${cell.y}`;
    const pixelInfo = pixelData[key];
    const raw = pixelInfo ? (typeof pixelInfo === 'string' ? pixelInfo : pixelInfo.color) : '';
    if (!raw) {
        setConnState('warn', 'No painted pixel at that position');
        setTimeout(() => setConnState('ok', state.authenticated ? 'Connected' : 'Browsing'), 1200);
        return;
    }

    const sampled = raw.startsWith('#') ? raw.toLowerCase() : `#${String(raw).toLowerCase()}`;
    selectColor(sampled);
    setEyedropperMode(false);
}

function changeNickname() {
    // Check if player has enough placements
    if (!state.stats || state.stats.totalPlacements < 10) {
        setConnState('warn', `Need 10 placements, you have ${state.stats?.totalPlacements || 0}`);
        setTimeout(() => setConnState('ok', 'Connected'), 2500);
        return;
    }

    return new Promise((resolve) => {
        const handleSave = () => {
            const entered = nicknameInput.value || '';
            const nickname = normalizeNickname(entered);
            
            if (!nickname) {
                setConnState('warn', 'Nickname too short');
                setTimeout(() => setConnState('ok', 'Connected'), 2000);
                resolve(false);
                return;
            }
            
            // Hide modal
            nicknameModal.classList.remove('show');
            nicknameBkg.classList.remove('show');
            nicknameModal.setAttribute('aria-hidden', 'true');
            nicknameBkg.setAttribute('aria-hidden', 'true');
            
            // Clean up listeners
            nicknameSaveBtn.removeEventListener('click', handleSave);
            nicknameCancelBtn.removeEventListener('click', handleCancel);
            nicknameInput.removeEventListener('keypress', handleEnter);
            
            // Send change_nickname message
            debugLog('Sending change_nickname message:', { type: 'change_nickname', nickname });
            sendWS({
                type: 'change_nickname',
                nickname: nickname,
            });
            
            resolve(true);
        };
        
        const handleCancel = () => {
            // Hide modal
            nicknameModal.classList.remove('show');
            nicknameBkg.classList.remove('show');
            nicknameModal.setAttribute('aria-hidden', 'true');
            nicknameBkg.setAttribute('aria-hidden', 'true');
            
            // Clean up listeners
            nicknameSaveBtn.removeEventListener('click', handleSave);
            nicknameCancelBtn.removeEventListener('click', handleCancel);
            nicknameInput.removeEventListener('keypress', handleEnter);
            
            resolve(false);
        };
        
        const handleEnter = (e) => {
            if (e.key === 'Enter') handleSave();
        };
        
        // Update modal for changing nickname
        const h2 = nicknameModal.querySelector('h2');
        const p = nicknameModal.querySelector('p');
        h2.textContent = 'Change Your Nickname';
        p.textContent = 'Choose a nickname (3-20 characters, alphanumeric, spaces, and underscores)';
        
        // Show modal
        nicknameModal.classList.add('show');
        nicknameBkg.classList.add('show');
        nicknameModal.setAttribute('aria-hidden', 'false');
        nicknameBkg.setAttribute('aria-hidden', 'false');
        nicknameInput.value = '';
        nicknameInput.focus();
        
        // Add listeners
        nicknameSaveBtn.addEventListener('click', handleSave);
        nicknameCancelBtn.addEventListener('click', handleCancel);
        nicknameInput.addEventListener('keypress', handleEnter);
    });
}

async function fetchAppConfig() {
    const response = await fetch('/app-config', { cache: 'no-store' });
    if (!response.ok) {
        const text = await response.text();
        console.error(`/app-config failed: ${response.status} ${response.statusText}`, text);
        throw new Error(`Failed to load app config: ${response.status} ${response.statusText}`);
    }
    const data = await response.json();
    state.firebaseWebApiKey = (data.firebaseWebApiKey || '').trim();
    state.firebaseProjectId = (data.firebaseProjectId || '').trim();
    state.firebaseAuthDomain = (data.firebaseAuthDomain || '').trim();
    if (!state.firebaseWebApiKey) {
        throw new Error('FIREBASE_WEB_API_KEY is not configured on server');
    }
}

async function ensureFirebaseAuthClient() {
    if (firebaseClientAuth) return firebaseClientAuth;
    if (!state.firebaseWebApiKey) {
        await fetchAppConfig();
    }
    if (!window.firebase || !window.firebase.initializeApp || !window.firebase.auth) {
        throw new Error('Firebase Auth SDK failed to load. Check network/CDN access.');
    }

    const authDomain = state.firebaseAuthDomain || (state.firebaseProjectId ? `${state.firebaseProjectId}.firebaseapp.com` : '');
    if (!authDomain) {
        throw new Error('Firebase auth domain is not configured. Set FIREBASE_AUTH_DOMAIN or FIREBASE_PROJECT_ID.');
    }

    const appConfig = {
        apiKey: state.firebaseWebApiKey,
        authDomain,
    };

    if (window.firebase.apps && window.firebase.apps.length > 0) {
        firebaseClientAuth = window.firebase.auth();
    } else {
        window.firebase.initializeApp(appConfig);
        firebaseClientAuth = window.firebase.auth();
    }
    return firebaseClientAuth;
}

async function firebaseGoogleAuth() {
    const auth = await ensureFirebaseAuthClient();
    const provider = new window.firebase.auth.GoogleAuthProvider();
    provider.setCustomParameters({ prompt: 'select_account' });
    const result = await auth.signInWithPopup(provider);
    const user = result?.user || auth.currentUser;
    if (!user) {
        throw new Error('Google sign-in did not return a user.');
    }
    const idToken = await user.getIdToken(true);
    return {
        idToken,
        email: user.email || '',
        displayName: user.displayName || '',
    };
}

function promptForAuth() {
    return new Promise((resolve) => {
        const setAuthStatus = (kind, text) => {
            if (!authStatusMsg) return;
            authStatusMsg.textContent = text || '';
            if (!text) {
                authStatusMsg.style.color = '';
                return;
            }
            if (kind === 'ok') {
                authStatusMsg.style.color = 'var(--success, #27ae60)';
            } else {
                authStatusMsg.style.color = 'var(--warn, #e67e22)';
            }
        };

        const doGoogleAuth = async () => {
            try {
                const result = await firebaseGoogleAuth();
                state.firebaseIdToken = result.idToken || '';
                state.email = result.email || '';
                localStorage.setItem(STORAGE_FIREBASE_ID_TOKEN, state.firebaseIdToken);
                if (state.email) {
                    localStorage.setItem(STORAGE_EMAIL, state.email);
                }
                if (!state.nickname) {
                    state.nickname = generateRandomNickname();
                }
                setAuthStatus('', '');
                hideModal(authModal, authBkg);
                resolve();
            } catch (err) {
                setAuthStatus('warn', `Google sign-in failed: ${err.message || err}`);
            }
        };

        authGoogleBtn.onclick = doGoogleAuth;
        setAuthStatus('', '');
        showModal(authModal, authBkg);
        if (authGoogleBtn && authGoogleBtn.focus) authGoogleBtn.focus();
    });
}

async function ensureIdentity() {
    if (!state.firebaseWebApiKey) {
        await fetchAppConfig();
    }
    // Login is optional. We only authenticate if a token already exists or user clicks Login.
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

function fmtAgo(ts) {
    if (!Number.isFinite(Number(ts)) || Number(ts) <= 0) return '-';
    const elapsed = Date.now() - Number(ts);
    if (elapsed < 60000) return `${Math.max(1, Math.round(elapsed / 1000))}s ago`;
    if (elapsed < 3600000) return `${Math.round(elapsed / 60000)}m ago`;
    if (elapsed < 86400000) return `${Math.round(elapsed / 3600000)}h ago`;
    return `${Math.round(elapsed / 86400000)}d ago`;
}

function updateHudVisibility() {
    if (!hud) return;
    const hasVisibleChild = Array.from(hud.children).some((child) => {
        if (!(child instanceof HTMLElement)) return false;
        return window.getComputedStyle(child).display !== 'none';
    });
    hud.style.display = hasVisibleChild ? '' : 'none';
}

function updateHoverHistoryText(cell) {
    if (!hoverHistoryRow || !hoverHistoryEl) return;
    if (!cell) {
        hoverHistoryRow.style.display = 'none';
        updateHudVisibility();
        return;
    }
    const key = `${cell.x}_${cell.y}`;
    const history = Array.isArray(cellHistoryCache[key]) ? cellHistoryCache[key] : [];
    if (history.length === 0) {
        const pixelInfo = pixelData[key];
        if (pixelInfo && pixelInfo.placedAt) {
            hoverHistoryEl.textContent = `Last edit ${fmtAgo(pixelInfo.placedAt)}`;
            hoverHistoryRow.style.display = 'block';
            updateHudVisibility();
            return;
        }
        hoverHistoryRow.style.display = 'none';
        updateHudVisibility();
        return;
    }
    const lines = history.slice(-3).reverse().map((item) => {
        const action = item.action || 'edit';
        const who = item.username || item.userId || 'unknown';
        const color = item.color ? ` ${item.color}` : '';
        return `${action} by ${who}${color} (${fmtAgo(item.updatedAt)})`;
    });
    hoverHistoryEl.textContent = lines.join(' | ');
    hoverHistoryRow.style.display = 'block';
    updateHudVisibility();
}

function isLightMode() {
    return document.body.classList.contains('light-theme');
}

function updateTimerUI() {
    const now = Date.now();
    const lightMode = isLightMode();
    if (state.cooldownBypass) {
        if (timerDisplay) {
            timerDisplay.textContent = 'Cooldown: bypass';
            timerDisplay.style.color = lightMode ? '#0b0f17' : '#9ee7ff';
        }
        updatePlacePixelButtonState();
        updateColorActionLabel();
        return requestAnimationFrame(updateTimerUI);
    }
    if (now < cooldownTime) {
        if (timerDisplay) {
            timerDisplay.textContent = `Cooldown: ${fmtMs(cooldownTime - now)}`;
            timerDisplay.style.color = lightMode ? '#8b0000' : '#ff6b6b';
        }
    } else {
        if (cooldownTime !== 0) {
            cooldownTime = 0;
            saveCooldown();
        }
        if (timerDisplay) {
            timerDisplay.textContent = 'Ready to paint!';
            timerDisplay.style.color = lightMode ? '#1a7026' : '#44ff88';
        }
    }
    updatePlacePixelButtonState();
    updateColorActionLabel();
    requestAnimationFrame(updateTimerUI);
}

function setConnState(kind, text) {
    const lightMode = isLightMode();
    state.connStateKind = kind;
    connStatus.textContent = text;
    if (kind === 'ok') {
        connStatus.style.borderColor = 'rgba(68, 255, 136, 0.65)';
        connStatus.style.background = lightMode ? 'rgba(68, 255, 136, 0.16)' : 'rgba(68, 255, 136, 0.12)';
        connStatus.style.color = lightMode ? '#0b0f17' : '#baffd6';
    } else if (kind === 'warn') {
        connStatus.style.borderColor = 'rgba(255, 190, 78, 0.65)';
        connStatus.style.background = lightMode ? 'rgba(255, 190, 78, 0.18)' : 'rgba(255, 190, 78, 0.12)';
        connStatus.style.color = lightMode ? '#5a3a00' : '#ffe6bb';
    } else {
        connStatus.style.borderColor = 'rgba(255, 107, 107, 0.65)';
        connStatus.style.background = lightMode ? 'rgba(255, 107, 107, 0.18)' : 'rgba(255, 107, 107, 0.12)';
        connStatus.style.color = lightMode ? '#7a1212' : '#ffd0d0';
    }
}

function updateNicknameDisplay() {
    nicknameDisplay.textContent = state.nickname || '-';
    updateProfileMenuSummary();
}

function updateAuthUI() {
    document.body.classList.toggle('is-authenticated', Boolean(state.authenticated));
    updateNicknameDisplay();
    updateColorActionLabel();
    if (!state.authenticated) {
        setColorPanelOpen(false);
        setProfileMenuOpen(false);
    }
    if (state.connected) {
        setConnState('ok', state.authenticated ? 'Connected' : 'Browsing');
    }
}

function formatLifetimeMs(ms) {
    const safeMs = Number(ms) || 0;
    if (safeMs <= 0) return '-';
    const totalSeconds = Math.floor(safeMs / 1000);
    const days = Math.floor(totalSeconds / 86400);
    const hours = Math.floor((totalSeconds % 86400) / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);

    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${minutes}m`;
    if (minutes > 0) return `${minutes}m`;
    return '<1m';
}

function renderStats() {
    if (!state.stats) {
        if (statsDisplay) statsDisplay.textContent = 'Placements: 0';
        statsTotalEl.textContent = '0';
        statsTodayEl.textContent = '0';
        statsLastPlacementEl.textContent = '-';
        if (statsRetained1hEl) statsRetained1hEl.textContent = '0';
        if (statsRetained24hEl) statsRetained24hEl.textContent = '0';
        if (statsAvgLifetimeEl) statsAvgLifetimeEl.textContent = '-';
        if (statsLongestLifetimeEl) statsLongestLifetimeEl.textContent = '-';
        statsTopColorsEl.innerHTML = '-';
        leaderboardTopEl.innerHTML = '-';
        return;
    }
    const totalPlacements = state.stats.totalPlacements || 0;
    if (statsDisplay) statsDisplay.textContent = `Placements: ${totalPlacements}`;
    statsTotalEl.textContent = String(totalPlacements);
    statsTodayEl.textContent = String(state.stats.placementsToday || 0);
    if (statsRetained1hEl) statsRetained1hEl.textContent = String(state.stats.pixelsUnchanged1h || 0);
    if (statsRetained24hEl) statsRetained24hEl.textContent = String(state.stats.pixelsUnchanged24h || 0);
    if (statsAvgLifetimeEl) statsAvgLifetimeEl.textContent = formatLifetimeMs(state.stats.averageLifetimeMs || 0);
    if (statsLongestLifetimeEl) statsLongestLifetimeEl.textContent = formatLifetimeMs(state.stats.longestSurvivingMs || 0);

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
    updateProfileMenuSummary();

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
                return `<div class="top-color-item"><span class="top-color-swatch"${swatchStyle}></span><span class="top-color-name">${item.label}</span><span class="top-color-count">${item.count}</span></div>`;
            })
            .join('');
    }

    const leaderboardEntries = (state.leaderboard || [])
        .slice(0, 5)
        .map((entry, idx) => ({
            rank: idx + 1,
            userId: entry.userId,
            nickname: entry.nickname || 'Guest',
            totalPlacements: entry.totalPlacements || 0,
        }));
    if (leaderboardEntries.length === 0) {
        leaderboardTopEl.innerHTML = '-';
    } else {
        leaderboardTopEl.innerHTML = leaderboardEntries
            .map((entry) => `<div class="leaderboard-item" onclick="showProfileModal('${entry.userId}')" style="cursor: pointer;"><span class="leaderboard-rank">#${entry.rank}</span><span class="leaderboard-name">${entry.nickname}</span><span class="leaderboard-count">${entry.totalPlacements}</span></div>`)
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
    const padding = 12;
    const isMobile = window.matchMedia('(max-width: 900px)').matches;
    const scale = isMobile ? 2.2 : 4;
    const availW = window.innerWidth - padding * 2;
    const availH = window.innerHeight - padding * 2;
    const sideBase = Math.max(520, Math.min(availW, availH));
    const side = sideBase * scale;

    canvas.style.width = side + 'px';
    canvas.style.height = side + 'px';
    canvas.width = Math.floor(side * dpr);
    canvas.height = Math.floor(side * dpr);

    basePixelSize = Math.max(
        1,
        Math.floor(Math.min(canvas.width / gridSize, canvas.height / gridSize))
    );

    const worldSize = gridSize * basePixelSize;
    cameraZoom = clamp(cameraZoom, 0.5, 5);
    // Center the world in the canvas viewport
    cameraX = (canvas.width / cameraZoom - worldSize) / 2 * cameraZoom;
    cameraY = (canvas.height / cameraZoom - worldSize) / 2 * cameraZoom;
    queueDraw();
    requestVisibleChunks();
}

function worldToChunk(x, y) {
    return {
        cx: Math.floor(x / chunkSize),
        cy: Math.floor(y / chunkSize),
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
        x: clamp(worldX, 0, gridSize - 1),
        y: clamp(worldY, 0, gridSize - 1),
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
    if (gridX < 0 || gridX >= gridSize || gridY < 0 || gridY >= gridSize) return null;
    return { x: gridX, y: gridY };
}

function draw() {
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.fillStyle = state.theme === 'light' ? '#fafbfe' : '#070a10';
    ctx.fillRect(0, 0, canvas.width, canvas.height);

    ctx.setTransform(cameraZoom, 0, 0, cameraZoom, cameraX, cameraY);
    ctx.imageSmoothingEnabled = false;
    const worldSize = gridSize * basePixelSize;

    ctx.lineWidth = 1 / cameraZoom;
    ctx.strokeStyle = state.theme === 'light' ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)';

    if (showGrid) {
        for (let i = 0; i <= gridSize; i++) {
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
    }

    for (const key in pixelData) {
        const [x, y] = key.split('_');
        const pixelInfo = pixelData[key];
        const rawColor = typeof pixelInfo === 'string' ? pixelInfo : pixelInfo.color;
        const color = rawColor && !rawColor.startsWith('#') ? '#' + rawColor : rawColor;
        
        ctx.fillStyle = color;
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
        case 'auth_ok': {
            if (data.userId) {
                state.userId = data.userId;
                localStorage.setItem(STORAGE_USER_ID, state.userId);
            }
            if (data.identityToken) {
                localStorage.setItem(STORAGE_IDENTITY_TOKEN, data.identityToken);
            }
            if (data.nickname) {
                state.nickname = data.nickname;
                updateAuthUI();
            }
            if (data.type === 'auth_ok') {
                state.authenticated = true;
            }
            if (Number.isFinite(Number(data.gridSize)) && Number(data.gridSize) > 0) {
                const nextGridSize = Number(data.gridSize);
                if (nextGridSize !== gridSize) {
                    gridSize = nextGridSize;
                    viewportConfigChanged = true;
                }
            }
            if (Number.isFinite(Number(data.chunkSize)) && Number(data.chunkSize) > 0) {
                const nextChunkSize = Number(data.chunkSize);
                if (nextChunkSize !== chunkSize) {
                    chunkSize = nextChunkSize;
                    viewportConfigChanged = true;
                }
            }
            if (Number.isFinite(Number(data.cooldownMs)) && Number(data.cooldownMs) > 0) {
                cooldownMs = Number(data.cooldownMs);
            }

            state.cooldownBypass = Boolean(data.cooldownBypass);
            if (data.type === 'auth_ok') updateAuthUI(); // sync body class after all state is set
            if (!state.cooldownBypass && Number.isFinite(Number(data.cooldownUntilMs))) {
                const until = Number(data.cooldownUntilMs);
                if (until > Date.now()) {
                    cooldownTime = until;
                    saveCooldown();
                } else if (cooldownTime <= Date.now()) {
                    cooldownTime = 0;
                    saveCooldown();
                }
            }
            if (data.stats) state.stats = data.stats;
            if (Array.isArray(data.leaderboard)) state.leaderboard = data.leaderboard;
            if (data.colorStats) {
                state.colorStats = data.colorStats;
            }
            if (viewportConfigChanged) {
                requestedChunkSet.clear();
                resizeCanvas();
            }
            renderStats();
            renderColorLeaderboard();
            requestVisibleChunks();
            break;
        }
        case 'chunk_data':
            if (data.chunk && Array.isArray(data.chunk.pixels)) {
                for (const px of data.chunk.pixels) {
                    pixelData[`${px.x}_${px.y}`] = {
                        color: px.color,
                        username: px.username || '-',
                        placedAt: px.updatedAt || Date.now()
                    };
                }
                queueDraw();
            }
            break;
        case 'canvas_snapshot':
            if (Array.isArray(data.pixels)) {
                for (const px of data.pixels) {
                    pixelData[`${px.x}_${px.y}`] = {
                        color: px.color,
                        username: px.username || '-',
                        placedAt: px.updatedAt || Date.now()
                    };
                }
                queueDraw();
            }
            break;
        case 'pixel_update':
            if (data.pixel) {
                delete cellHistoryCache[`${data.pixel.x}_${data.pixel.y}`];
                pixelData[`${data.pixel.x}_${data.pixel.y}`] = {
                    color: data.pixel.color,
                    username: data.pixel.username || data.nickname || '-',
                    placedAt: Date.now()
                };
                queueDraw();
            }
            break;
        case 'pixel_removed':
            if (data.pixel) {
                const key = `${data.pixel.x}_${data.pixel.y}`;
                delete pixelData[key];
                delete cellHistoryCache[key];
                ownPixels.delete(key);
                queueDraw();
            }
            break;
        case 'cell_history':
            if (data.pixel) {
                const key = `${data.pixel.x}_${data.pixel.y}`;
                cellHistoryCache[key] = Array.isArray(data.history) ? data.history : [];
                if (hoverCell && hoverCell.x === data.pixel.x && hoverCell.y === data.pixel.y) {
                    updateHoverHistoryText(hoverCell);
                }
            }
            break;
        case 'pixel_accepted':
            if (data.pixel) {
                const key = `${data.pixel.x}_${data.pixel.y}`;
                const now = Date.now();
                delete cellHistoryCache[key];
                lastOptimisticPixel = null; // confirmed — no need to revert
                pixelData[key] = {
                    color: data.pixel.color,
                    username: state.nickname,
                    placedAt: now
                };
                // Track for undo
                lastPlacedPixel = { x: data.pixel.x, y: data.pixel.y };
                lastPlacedPixelTime = now;
                pendingUndoSnapshot = null;
                lastUndonePixel = null;
                
                // Show undo button in dock
                if (undoPixelBtn) {
                    undoPixelBtn.disabled = false;
                    debugLog('Undo button shown in dock');
                } else {
                    debugError('undoPixelBtn not found!');
                }
                if (redoPixelBtn) {
                    redoPixelBtn.disabled = true;
                }
                
                // Auto-hide undo button after 30 seconds
                setTimeout(() => {
                    if (Date.now() - lastPlacedPixelTime > 30000) {
                        if (undoPixelBtn) {
                            undoPixelBtn.disabled = true;
                        }
                    }
                }, 30000);
                
                ownPixels.add(key);
                playPlacementSound();
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
            if (data.colorStats) {
                state.colorStats = data.colorStats;
                renderColorLeaderboard();
            }
            renderStats();
            break;
        case 'pixel_rejected':
            // Revert optimistic pixel on any rejection
            if (lastOptimisticPixel) {
                const { key: oKey, prev: oPrev } = lastOptimisticPixel;
                if (oPrev) {
                    pixelData[oKey] = oPrev;
                } else {
                    delete pixelData[oKey];
                }
                lastOptimisticPixel = null;
                queueDraw();
            }
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
            } else if (data.reason === 'not_owner') {
                setConnState('warn', 'Can only undo your own pixels');
                setTimeout(() => setConnState('ok', 'Connected'), 2000);
            } else if (data.reason === 'undo_expired') {
                setConnState('warn', 'Undo expired (30s window)');
                setTimeout(() => setConnState('ok', 'Connected'), 2000);
            } else if (data.reason === 'frozen') {
                setConnState('warn', 'Account is frozen by moderation');
                setTimeout(() => setConnState('ok', 'Connected'), 2500);
            }
            break;
        case 'undo_success':
            // Clear undo tracking after successful undo
            lastUndonePixel = pendingUndoSnapshot;
            pendingUndoSnapshot = null;
            lastPlacedPixel = null;
            lastPlacedPixelTime = null;
            if (undoPixelBtn) undoPixelBtn.disabled = true;
            if (redoPixelBtn && lastUndonePixel) {
                redoPixelBtn.disabled = false;
            }
            setConnState('ok', 'Pixel undone!');
            setTimeout(() => setConnState('ok', 'Connected'), 1500);
            break;
        case 'error':
            if (data.reason === 'anti_spam') {
                setConnState('warn', 'Slow down (anti-spam)');
                setTimeout(() => setConnState('ok', 'Connected'), 1500);
            }
            break;
        case 'nickname_changed':
            state.nickname = data.nickname;
            updateAuthUI();
            if (data.stats) state.stats = data.stats;
            renderStats();
            setConnState('ok', `Nickname changed to '${data.nickname}'`);
            setTimeout(() => setConnState('ok', 'Connected'), 2200);
            break;
        case 'nickname_change_rejected':
            if (data.reason === 'insufficient_placements') {
                setConnState('warn', `Need 10 placements, you have ${state.stats?.totalPlacements || 0}`);
            } else if (data.reason === 'cooldown') {
                setConnState('warn', 'Can only change once per day');
            } else if (data.reason === 'invalid_format') {
                setConnState('warn', 'Invalid nickname format');
            } else if (data.reason === 'auth_required') {
                setConnState('warn', 'Please sign in first');
            } else if (data.reason === 'muted') {
                setConnState('warn', 'Account is muted by moderation');
            } else {
                setConnState('warn', data.message || 'Nickname change failed');
            }
            setTimeout(() => setConnState('ok', 'Connected'), 2500);
            break;
        case 'auth_failed':
            state.authenticated = false;
            state.firebaseIdToken = '';
            localStorage.removeItem(STORAGE_FIREBASE_ID_TOKEN);
            updateAuthUI();
            setConnState('warn', data.message || 'Sign in required (click Login)');
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
        setConnState('ok', state.authenticated ? 'Connected' : 'Browsing');
        const token = state.firebaseIdToken || localStorage.getItem(STORAGE_FIREBASE_ID_TOKEN) || '';
        if (token) {
            sendWS({
                type: 'auth',
                nickname: state.nickname,
                clientVersion: '1',
                firebaseIdToken: token,
            });
        }
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
        if (state.intentionalLogout) {
            // Don't auto-reconnect; promptForAuth will handle it
            return;
        }
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
    if (eyedropperMode) {
        sampleColorFromCell(cell);
        return;
    }
    if (!state.authenticated) return;
    if (!state.cooldownBypass && Date.now() < cooldownTime) return;
    
    if (!state.cooldownBypass && checkFastPlacement()) {
        return;
    }

    // Optimistic rendering: draw immediately without waiting for server round-trip
    const key = `${cell.x}_${cell.y}`;
    const prevPixel = pixelData[key] ? { ...pixelData[key] } : null;
    lastOptimisticPixel = { key, prev: prevPixel };
    pixelData[key] = { color: state.selectedColor, username: state.nickname, placedAt: Date.now(), optimistic: true };
    queueDraw();

    sendWS({
        type: 'place_pixel',
        pixel: { x: cell.x, y: cell.y, color: state.selectedColor },
        _optimisticKey: key,
        _optimisticPrev: prevPixel,
    });
}

const palette = [
    { hex: '#6d001a', name: 'Dark Maroon' },
    { hex: '#be0039', name: 'Crimson' },
    { hex: '#ff4500', name: 'Vermilion' },
    { hex: '#ffa800', name: 'Amber' },
    { hex: '#ffd635', name: 'Sunflower' },
    { hex: '#fff8b8', name: 'Cream' },
    { hex: '#00a368', name: 'Forest' },
    { hex: '#00cc78', name: 'Emerald' },
    { hex: '#7eed56', name: 'Lime' },
    { hex: '#00756f', name: 'Deep Teal' },
    { hex: '#009eaa', name: 'Teal' },
    { hex: '#00ccc0', name: 'Aqua' },
    { hex: '#2450a4', name: 'Navy' },
    { hex: '#3690ea', name: 'Azure' },
    { hex: '#51e9f4', name: 'Sky' },
    { hex: '#493ac1', name: 'Indigo' },
    { hex: '#6a5cff', name: 'Periwinkle' },
    { hex: '#94b3ff', name: 'Lavender' },
    { hex: '#811e9f', name: 'Royal Purple' },
    { hex: '#b44ac0', name: 'Orchid' },
    { hex: '#e4abff', name: 'Lilac' },
    { hex: '#de107f', name: 'Magenta' },
    { hex: '#ff3881', name: 'Hot Pink' },
    { hex: '#ff99aa', name: 'Blush' },
    { hex: '#6d482f', name: 'Walnut' },
    { hex: '#9c6926', name: 'Bronze' },
    { hex: '#ffb470', name: 'Sand' },
    { hex: '#000000', name: 'Black' },
    { hex: '#515252', name: 'Charcoal' },
    { hex: '#898d90', name: 'Steel' },
    { hex: '#d4d7d9', name: 'Silver' },
    { hex: '#ffffff', name: 'White' },
    { hex: '#7f001f', name: 'Ruby' },
    { hex: '#ff6b6b', name: 'Salmon' },
    { hex: '#ff8c42', name: 'Tangerine' },
    { hex: '#f6c445', name: 'Gold' },
    { hex: '#ffef7a', name: 'Butter' },
    { hex: '#b5e48c', name: 'Pistachio' },
    { hex: '#3a5a40', name: 'Dark Olive' },
    { hex: '#2dc653', name: 'Jade' },
    { hex: '#87c38f', name: 'Sage' },
    { hex: '#3ddad7', name: 'Arctic Teal' },
    { hex: '#48cae4', name: 'Cyan' },
    { hex: '#a9def9', name: 'Glacier' },
    { hex: '#1b263b', name: 'Midnight' },
    { hex: '#3a86ff', name: 'Cobalt' },
    { hex: '#5c7cfa', name: 'Denim' },
    { hex: '#9ad1ff', name: 'Powder Blue' },
    { hex: '#5a189a', name: 'Eggplant' },
    { hex: '#9d4edd', name: 'Violet' },
    { hex: '#c77dff', name: 'Mauve' },
    { hex: '#ff4dc4', name: 'Fuchsia' },
    { hex: '#ff7096', name: 'Rose' },
    { hex: '#ffc09f', name: 'Peach' },
    { hex: '#5a3d2b', name: 'Coffee' },
    { hex: '#c68b3c', name: 'Ochre' },
    { hex: '#e6b566', name: 'Caramel' },
    { hex: '#1f1f1f', name: 'Obsidian' },
    { hex: '#6b7280', name: 'Graphite' },
    { hex: '#aeb8c4', name: 'Mist' },
    { hex: '#e5e7eb', name: 'Pearl' },
    { hex: '#faf3dd', name: 'Ivory' },
    { hex: '#6b8e23', name: 'Olive' },
    { hex: '#0077b6', name: 'Cerulean' },
];
const paletteNameByHex = Object.fromEntries(palette.map((entry) => [entry.hex.toLowerCase(), entry.name]));

function selectColor(hex) {
    state.selectedColor = hex;
    if (colorActionSwatch) colorActionSwatch.style.background = hex;
    if (colorActionSwatch) colorActionSwatch.title = paletteNameByHex[hex.toLowerCase()] || hex;
    document.querySelectorAll('#palette .swatch').forEach(s => {
        s.classList.toggle('selected', s.dataset.color === hex);
    });
}

// Pick a random initial color.
selectColor(palette[Math.floor(Math.random() * palette.length)].hex);

for (const entry of palette) {
    const sw = document.createElement('div');
    sw.className = 'swatch';
    sw.dataset.color = entry.hex;
    sw.style.background = entry.hex;
    sw.title = entry.name;
    sw.setAttribute('aria-label', entry.name);
    sw.addEventListener('click', () => selectColor(entry.hex));
    paletteEl.appendChild(sw);
}

document.getElementById('exportBtn').addEventListener('click', () => {
    const EXPORT_SCALE = 8;
    const offscreen = document.createElement('canvas');
    offscreen.width = gridSize * EXPORT_SCALE;
    offscreen.height = gridSize * EXPORT_SCALE;
    const octx = offscreen.getContext('2d');
    octx.imageSmoothingEnabled = false;
    octx.fillStyle = '#ffffff';
    octx.fillRect(0, 0, offscreen.width, offscreen.height);
    for (const key in pixelData) {
        const [x, y] = key.split('_');
        const info = pixelData[key];
        const raw = typeof info === 'string' ? info : info.color;
        const color = raw && !raw.startsWith('#') ? '#' + raw : raw;
        if (!color) continue;
        octx.fillStyle = color;
        octx.fillRect(Number(x) * EXPORT_SCALE, Number(y) * EXPORT_SCALE, EXPORT_SCALE, EXPORT_SCALE);
    }
    const a = document.createElement('a');
    a.href = offscreen.toDataURL('image/png');
    a.download = `pixelworld-${new Date().toISOString().slice(0,10)}.png`;
    a.click();
});

window.addEventListener('keydown', (e) => {
    debugLog('keydown event - key:', e.key, 'code:', e.code, 'shiftKey:', e.shiftKey);
    if (e.key === 'r' || e.key === 'R') {
        cameraZoom = 1;
        resizeCanvas();
    }
    if (e.key === 'g' || e.key === 'G') {
        toggleGrid();
    }
    if (e.key === 'i' || e.key === 'I') {
        setEyedropperMode(!eyedropperMode);
    }
    if (e.key === '?' || (e.shiftKey && e.key === '/')) {
        debugLog('? key pressed, showing shortcuts modal');
        showModal(shortcutsModal, shortcutsBkg);
    }
    if (e.key === 'Escape') {
        setDrawerOpen(false);
        setColorPanelOpen(false);
        setProfileMenuOpen(false);
        hideCaptchaModal();
        hideModal(authModal, authBkg);
        hideModal(shortcutsModal, shortcutsBkg);
        hideModal(profileModal, profileBkg);
    }
});

statsToggleBtn.addEventListener('click', () => setDrawerOpen(true));
statsCloseBtn.addEventListener('click', () => setDrawerOpen(false));
drawerBackdrop.addEventListener('click', () => setDrawerOpen(false));

const logoutBtn = document.getElementById('logoutBtn');

function beginAuthAndConnect() {
    promptForAuth().then(() => {
        if (socketRef && socketRef.readyState === WebSocket.OPEN) {
            sendWS({
                type: 'auth',
                nickname: state.nickname,
                clientVersion: '1',
                firebaseIdToken: state.firebaseIdToken,
            });
        } else {
            socketRef = connectWebSocket();
        }
    });
}

if (profileMenuBtn) {
    profileMenuBtn.addEventListener('click', () => {
        if (!state.authenticated) {
            beginAuthAndConnect();
            return;
        }
        setProfileMenuOpen(!profileDropdown.classList.contains('open'));
    });
}

document.addEventListener('click', (e) => {
    if (!profileMenuWrap || !profileDropdown || !profileDropdown.classList.contains('open')) return;
    if (!profileMenuWrap.contains(e.target)) {
        setProfileMenuOpen(false);
    }
});

if (colorActionBtn) {
    colorActionBtn.addEventListener('click', () => {
        if (!state.authenticated) {
            beginAuthAndConnect();
            return;
        }

        setColorPanelOpen(!colorPanel.classList.contains('open'));
    });
}

if (authCloseBtn) {
    authCloseBtn.addEventListener('click', () => hideModal(authModal, authBkg));
}
if (authBkg) {
    authBkg.addEventListener('click', () => hideModal(authModal, authBkg));
}

logoutBtn.addEventListener('click', () => {
    if (!confirm('Sign out?')) return;
    // Clear all auth state and continue as guest
    state.authenticated = false;
    state.firebaseIdToken = '';
    state.userId = '';
    state.nickname = '';
    state.email = '';
    state.cooldownBypass = false;
    cooldownTime = 0;
    localStorage.removeItem(STORAGE_FIREBASE_ID_TOKEN);
    localStorage.removeItem(STORAGE_USER_ID);
    localStorage.removeItem(STORAGE_EMAIL);
    localStorage.removeItem(STORAGE_IDENTITY_TOKEN);
    localStorage.removeItem(STORAGE_COOLDOWN);
    setColorPanelOpen(false);
    setProfileMenuOpen(false);
    updateAuthUI();

    if (socketRef) {
        state.intentionalLogout = true;
        socketRef.close();
        setTimeout(() => {
            state.intentionalLogout = false;
            socketRef = connectWebSocket();
        }, 50);
    } else {
        socketRef = connectWebSocket();
    }
});
if (nicknameDisplay) {
    nicknameDisplay.addEventListener('click', () => {
        if (state.authenticated) {
            changeNickname();
        }
    });
}

if (gridToggle) gridToggle.addEventListener('change', toggleGrid);
if (themeToggle) themeToggle.addEventListener('click', toggleTheme);
if (captchaOkBtn) captchaOkBtn.addEventListener('click', hideCaptchaModal);
if (captchaBkg) captchaBkg.addEventListener('click', hideCaptchaModal);
if (shortcutsCloseBtn) shortcutsCloseBtn.addEventListener('click', () => hideModal(shortcutsModal, shortcutsBkg));
if (shortcutsBkg) shortcutsBkg.addEventListener('click', () => hideModal(shortcutsModal, shortcutsBkg));
if (profileCloseBtn) profileCloseBtn.addEventListener('click', () => hideModal(profileModal, profileBkg));
if (profileBkg) profileBkg.addEventListener('click', () => hideModal(profileModal, profileBkg));

if (placePixelBtn) placePixelBtn.addEventListener('click', placePixelAtHover);
if (undoPixelBtn) undoPixelBtn.addEventListener('click', requestUndoPixel);
if (redoPixelBtn) redoPixelBtn.addEventListener('click', requestRedoPixel);
if (eyedropperBtn) eyedropperBtn.addEventListener('click', () => setEyedropperMode(!eyedropperMode));

if (placeCoordBtn) {
    placeCoordBtn.addEventListener('click', () => {
        const x = Number(coordX.value);
        const y = Number(coordY.value);
        placePixelAtCoordinates(x, y);
        coordX.value = '';
        coordY.value = '';
    });
}

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
    const prevKey = hoverCell ? `${hoverCell.x}_${hoverCell.y}` : '';
    hoverCell = cell;
    if (cell) {
        if (hoverCoordsEl) hoverCoordsEl.textContent = `${cell.x}, ${cell.y}`;
        const pixelKey = `${cell.x}_${cell.y}`;
        const pixelInfo = pixelData[pixelKey];
        hoverUsername = pixelInfo ? (typeof pixelInfo === 'string' ? '-' : pixelInfo.username || '-') : '-';
        
        if (hoverUsernameRow && hoverUsernameEl) {
            if (hoverUsername && hoverUsername !== '-') {
                hoverUsernameEl.textContent = hoverUsername;
                hoverUsernameRow.style.display = 'block';
            } else {
                hoverUsernameRow.style.display = 'none';
            }
            updateHudVisibility();
        }

        updateHoverHistoryText(cell);
        if (pixelKey !== prevKey && pixelKey !== lastHistoryRequestKey) {
            lastHistoryRequestKey = pixelKey;
            sendWS({
                type: 'cell_history_request',
                pixel: { x: cell.x, y: cell.y },
            });
        }
    } else {
        if (hoverCoordsEl) hoverCoordsEl.textContent = '-';
        hoverUsername = null;
        if (hoverUsernameRow) hoverUsernameRow.style.display = 'none';
        if (hoverHistoryRow) hoverHistoryRow.style.display = 'none';
        updateHudVisibility();
    }
    if (cursorCoordsEl) {
        cursorCoordsEl.textContent = `${Math.round((e.clientX - canvas.getBoundingClientRect().left) * dpr)}, ${Math.round((e.clientY - canvas.getBoundingClientRect().top) * dpr)}`;
    }
    updateCanvasTopStatus();
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
    updateZoomDisplay();
    queueDraw();
    requestVisibleChunks();
}, { passive: false });

window.addEventListener('resize', resizeCanvas);

let socketRef = null;

(async () => {
    await ensureIdentity();
    updateAuthUI();
    applyTheme();
    updateHudVisibility();
    loadCooldownFromStorage();
    updateTimerUI();
    updateZoomDisplay();
    resizeCanvas();
    socketRef = connectWebSocket();
})();

