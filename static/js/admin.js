const adminKeyInput = document.getElementById('adminKey');
const userIdInput = document.getElementById('userId');

// Theme toggle
const adminThemeToggle = document.getElementById('adminThemeToggle');
(function initAdminTheme() {
    const saved = localStorage.getItem('admin_theme');
    if (saved === 'dark') {
        document.body.classList.add('dark-theme');
        if (adminThemeToggle) adminThemeToggle.textContent = '\u2600\uFE0F';
    }
    if (adminThemeToggle) {
        adminThemeToggle.addEventListener('click', () => {
            const isDark = document.body.classList.toggle('dark-theme');
            adminThemeToggle.textContent = isDark ? '\u2600\uFE0F' : '\uD83C\uDF19';
            localStorage.setItem('admin_theme', isDark ? 'dark' : 'light');
        });
    }
})();
const cooldownUntilInput = document.getElementById('cooldownUntil');
const clearCooldownInput = document.getElementById('clearCooldown');
const applyCooldownBtn = document.getElementById('applyCooldown');

const muteUntilInput = document.getElementById('muteUntil');
const clearMuteInput = document.getElementById('clearMute');
const freezeUntilInput = document.getElementById('freezeUntil');
const clearFreezeInput = document.getElementById('clearFreeze');
const moderationReasonInput = document.getElementById('moderationReason');
const applyModerationBtn = document.getElementById('applyModeration');

const rollbackStartInput = document.getElementById('rollbackStart');
const rollbackEndInput = document.getElementById('rollbackEnd');
const rollbackUserIdInput = document.getElementById('rollbackUserId');
const runRollbackBtn = document.getElementById('runRollback');

const refreshSuspiciousBtn = document.getElementById('refreshSuspicious');
const suspiciousListEl = document.getElementById('suspiciousList');
const resultBox = document.getElementById('resultBox');

function toMillis(value) {
    if (!value) return null;
    const ms = new Date(value).getTime();
    return Number.isFinite(ms) ? ms : null;
}

function getKey() {
    return (adminKeyInput.value || '').trim();
}

function getUserId() {
    return (userIdInput.value || '').trim();
}

async function callAdmin(path, method, body) {
    const key = getKey();
    if (!key) {
        throw new Error('Admin API key is required');
    }

    const response = await fetch(path, {
        method,
        headers: {
            'Content-Type': 'application/json',
            'X-Admin-Key': key,
        },
        body: body ? JSON.stringify(body) : undefined,
    });

    const payload = await response.json().catch(() => ({}));
    if (!response.ok || payload.ok === false) {
        throw new Error(payload.error || `Request failed: ${response.status}`);
    }
    return payload;
}

function showResult(payload) {
    resultBox.textContent = JSON.stringify(payload, null, 2);
}

applyCooldownBtn.addEventListener('click', async () => {
    try {
        const userId = getUserId();
        if (!userId) throw new Error('User ID is required for cooldown changes');

        const clear = clearCooldownInput.checked;
        const until = toMillis(cooldownUntilInput.value);
        const body = { userId, clear };
        if (!clear) {
            if (until == null) throw new Error('Cooldown until time is required unless clearing');
            body.cooldownUntilMs = until;
        }

        const payload = await callAdmin('/admin/cooldown', 'POST', body);
        showResult(payload);
    } catch (err) {
        showResult({ ok: false, error: String(err.message || err) });
    }
});

applyModerationBtn.addEventListener('click', async () => {
    try {
        const userId = getUserId();
        if (!userId) throw new Error('User ID is required for moderation changes');

        const body = {
            userId,
            clearMute: clearMuteInput.checked,
            clearFreeze: clearFreezeInput.checked,
            reason: (moderationReasonInput.value || '').trim(),
        };
        const muteUntil = toMillis(muteUntilInput.value);
        const freezeUntil = toMillis(freezeUntilInput.value);
        if (!body.clearMute && muteUntil != null) body.muteUntilMs = muteUntil;
        if (!body.clearFreeze && freezeUntil != null) body.freezeUntilMs = freezeUntil;

        const payload = await callAdmin('/admin/moderation', 'POST', body);
        showResult(payload);
    } catch (err) {
        showResult({ ok: false, error: String(err.message || err) });
    }
});

runRollbackBtn.addEventListener('click', async () => {
    try {
        const startMs = toMillis(rollbackStartInput.value);
        const endMs = toMillis(rollbackEndInput.value);
        if (startMs == null || endMs == null) {
            throw new Error('Rollback start/end are required');
        }

        const body = {
            startMs,
            endMs,
            userId: (rollbackUserIdInput.value || '').trim(),
        };

        const payload = await callAdmin('/admin/rollback-window', 'POST', body);
        showResult(payload);
    } catch (err) {
        showResult({ ok: false, error: String(err.message || err) });
    }
});

function renderSuspicious(items) {
    if (!Array.isArray(items) || items.length === 0) {
        suspiciousListEl.textContent = 'No suspicious activity found.';
        return;
    }

    suspiciousListEl.innerHTML = items.map((item) => {
        const uid = item.userId || '-';
        const nick = item.nickname || '-';
        const ip = item.clientIp || '-';
        const score = item.score || 0;
        const event = item.lastEvent || '-';
        const lastAt = item.lastAt ? new Date(item.lastAt).toLocaleString() : '-';
        return `<div class="list-item"><b>${nick}</b> (${uid})<br>IP: ${ip}<br>Score: ${score}<br>Last event: ${event}<br>Last seen: ${lastAt}</div>`;
    }).join('');
}

refreshSuspiciousBtn.addEventListener('click', async () => {
    try {
        const payload = await callAdmin('/admin/suspicious', 'GET');
        renderSuspicious(payload.items || []);
        showResult(payload);
    } catch (err) {
        showResult({ ok: false, error: String(err.message || err) });
    }
});
