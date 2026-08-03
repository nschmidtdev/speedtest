// profiles.js — Profile CRUD UI + Editor Dialog + Server Selection
// Die Profil-Liste wird per HTMX geladen (#profile-list). Dieser JS-Code
// übernimmt Event-Delegation für die Buttons/Toggles in der HTMX-Liste und
// steuert den Editor-Dialog (komplex: Server-Auswahl, Cron-Presets).

let availableServers = [];
let editingProfileId = null;

// Event-Delegation: funktioniert auch nach HTMX-Reloads der Liste.
document.addEventListener('click', (e) => {
    // Edit-Button oder Klick auf die Karte
    const editEl = e.target.closest('[data-edit-id]');
    if (editEl && !e.target.classList.contains('toggle-enable')) {
        const id = parseInt(editEl.dataset.editId);
        openProfileEditor(id);
        return;
    }
});

document.addEventListener('change', (e) => {
    if (e.target.classList.contains('toggle-enable')) {
        const id = e.target.dataset.profileId;
        const enabled = e.target.checked;
        API.post(`/api/profiles/${id}/${enabled ? 'enable' : 'disable'}`);
    }
});

// Neue-Profile-Button (HTMX-basiert im index.html)

// Profile für die History-Dropdowns nachladen (nur Datensync, kein Rendering)
async function syncProfileDropdowns() {
    try {
        const profiles = await API.get('/api/profiles');
        const histSelect = document.getElementById('history-profile');
        if (histSelect) {
            histSelect.innerHTML = '<option value="">Alle Profile</option>';
            profiles.forEach(p => {
                const opt = document.createElement('option');
                opt.value = p.id;
                opt.textContent = p.name;
                histSelect.appendChild(opt);
            });
        }
    } catch (e) {
        console.error('Failed to sync profile dropdowns:', e);
    }
}

// === Profile Editor Dialog ===

async function openProfileEditor(profileId) {
    editingProfileId = profileId;

    // Load servers if not already loaded
    if (availableServers.length === 0) {
        try {
            availableServers = await API.get('/api/servers?limit=50');
        } catch (e) {
            console.warn('Failed to load servers:', e);
            availableServers = [];
        }
    }

    let profile = {
        name: '', description: '', metrics: ['ping'],
        cron_expr: '', enabled: true, server_mode: 'auto', server_ids: []
    };

    if (profileId) {
        try {
            profile = await API.get(`/api/profiles/${profileId}`);
        } catch (e) {
            console.error('Failed to load profile:', e);
            return;
        }
    }

    showProfileDialog(profile);
}

function showProfileDialog(p) {
    // Remove existing dialog
    const existing = document.getElementById('profile-dialog-overlay');
    if (existing) existing.remove();

    const metricsList = ['download', 'upload', 'ping', 'jitter', 'bufferbloat', 'traceroute'];

    const overlay = document.createElement('div');
    overlay.id = 'profile-dialog-overlay';
    overlay.className = 'dialog-overlay';

    overlay.innerHTML = `
        <div class="dialog">
            <div class="dialog-header">
                <h2>${p.id ? 'Profil bearbeiten' : 'Neues Profil'}</h2>
                <button class="dialog-close" id="dialog-close">✕</button>
            </div>
            <div class="dialog-body">
                <div class="form-group">
                    <label>Name</label>
                    <input type="text" id="form-name" value="${escapeHtml(p.name)}" placeholder="z.B. Mein Speedtest" />
                </div>
                <div class="form-group">
                    <label>Beschreibung</label>
                    <input type="text" id="form-desc" value="${escapeHtml(p.description || '')}" placeholder="Optional" />
                </div>
                <div class="form-group">
                    <label>Cron-Ausdruck <span class="form-hint">(Sek Min Std Tag Mon Wochentag — leer = manuell)</span></label>
                    <input type="text" id="form-cron" value="${escapeHtml(p.cron_expr || '')}" placeholder="0 */5 * * * * (alle 5 Min)" />
                    <div id="cron-meaning" class="cron-meaning"></div>
                    <div class="cron-presets">
                        <button class="cron-preset" data-cron="0 */5 * * * *">5 Min</button>
                        <button class="cron-preset" data-cron="0 */15 * * * *">15 Min</button>
                        <button class="cron-preset" data-cron="0 */30 * * * *">30 Min</button>
                        <button class="cron-preset" data-cron="0 0 */1 * * *">Stündlich</button>
                        <button class="cron-preset" data-cron="0 0 */6 * * *">6 Std</button>
                        <button class="cron-preset" data-cron="">Manuell</button>
                    </div>
                </div>
                <div class="form-group">
                    <label>Metriken</label>
                    <div class="checkbox-group">
                        ${metricsList.map(m => `
                            <label class="checkbox-label">
                                <input type="checkbox" class="form-metric" value="${m}" ${(p.metrics || []).includes(m) ? 'checked' : ''} />
                                <span>${m}</span>
                            </label>
                        `).join('')}
                    </div>
                </div>
                <div class="form-group">
                    <label>Server-Auswahl</label>
                    <div class="server-mode-group">
                        <label class="radio-label">
                            <input type="radio" name="server-mode" value="auto" ${p.server_mode === 'auto' ? 'checked' : ''} />
                            <span><strong>Auto</strong> — Ookla wählt den besten Server</span>
                        </label>
                        <label class="radio-label">
                            <input type="radio" name="server-mode" value="random" ${p.server_mode === 'random' ? 'checked' : ''} />
                            <span><strong>Random</strong> — Zufälliger Server aus Auswahl</span>
                        </label>
                        <label class="radio-label">
                            <input type="radio" name="server-mode" value="fixed" ${p.server_mode === 'fixed' ? 'checked' : ''} />
                            <span><strong>Fixed</strong> — Immer der erste ausgewählte Server</span>
                        </label>
                    </div>
                </div>
                <div class="form-group" id="server-select-group" style="display:none;">
                    <label>Server auswählen <span class="form-hint">( Mehrfachauswahl mit Klick )</span></label>
                    <div class="server-search">
                        <input type="text" id="server-search" placeholder="Server suchen..." />
                    </div>
                    <div class="server-multiselect" id="server-multiselect">
                        ${renderServerCheckboxes(availableServers, p.server_ids || [])}
                    </div>
                </div>
                <div class="form-group">
                    <label class="checkbox-label">
                        <input type="checkbox" id="form-enabled" ${p.enabled ? 'checked' : ''} />
                        <span>Profil aktiviert</span>
                    </label>
                </div>
            </div>
            <div class="dialog-footer">
                ${p.id ? `<button class="btn btn-danger" id="dialog-delete">Löschen</button>` : ''}
                <button class="btn btn-secondary" id="dialog-cancel">Abbrechen</button>
                <button class="btn btn-primary" id="dialog-save">Speichern</button>
            </div>
        </div>
    `;

    document.body.appendChild(overlay);

    // === Dialog Event Handlers ===
    document.getElementById('dialog-close').addEventListener('click', closeProfileDialog);
    document.getElementById('dialog-cancel').addEventListener('click', closeProfileDialog);
    overlay.addEventListener('click', (e) => { if (e.target === overlay) closeProfileDialog(); });

    // Server mode toggle
    function updateServerGroup() {
        const mode = document.querySelector('input[name="server-mode"]:checked').value;
        document.getElementById('server-select-group').style.display = (mode === 'random' || mode === 'fixed') ? '' : 'none';
    }
    document.querySelectorAll('input[name="server-mode"]').forEach(r => r.addEventListener('change', updateServerGroup));
    updateServerGroup();

    // Cron live interpretation
    const cronInput = document.getElementById('form-cron');
    const updateCronHelp = () => {
        const info = describeCron(cronInput.value.trim());
        const el = document.getElementById('cron-meaning');
        el.textContent = info.text;
        el.className = `cron-meaning ${info.valid ? 'valid' : 'invalid'}`;
        document.getElementById('dialog-save').disabled = !info.valid;
    };
    cronInput.addEventListener('input', updateCronHelp);

    // Cron presets
    document.querySelectorAll('.cron-preset').forEach(btn => {
        btn.addEventListener('click', () => {
            cronInput.value = btn.dataset.cron;
            updateCronHelp();
        });
    });
    updateCronHelp();

    // Server search
    document.getElementById('server-search').addEventListener('input', (e) => {
        const term = e.target.value.toLowerCase();
        const filtered = availableServers.filter(s =>
            s.name?.toLowerCase().includes(term) || s.country?.toLowerCase().includes(term) || s.sponsor?.toLowerCase().includes(term)
        );
        const selected = getSelectedServerIDs();
        document.getElementById('server-multiselect').innerHTML = renderServerCheckboxes(filtered, selected);
    });

    // Save
    document.getElementById('dialog-save').addEventListener('click', saveProfile);

    // Delete
    const delBtn = document.getElementById('dialog-delete');
    if (delBtn) delBtn.addEventListener('click', async () => {
        if (!confirm('Profil wirklich löschen?')) return;
        try {
            await API.del(`/api/profiles/${editingProfileId}`);
            closeProfileDialog();
            reloadProfileList();
        } catch (e) { alert('Löschen fehlgeschlagen: ' + e.message); }
    });
}

function renderServerCheckboxes(servers, selectedIDs) {
    if (!servers || servers.length === 0) {
        return '<p class="coming-soon">Keine Server verfügbar. Cache wird beim ersten Test geladen.</p>';
    }
    const selectedSet = new Set((selectedIDs || []).map(String));
    return servers.map(s => {
        const id = s.id;
        const checked = selectedSet.has(String(id)) ? 'checked' : '';
        const dist = s.distance ? `(${s.distance.toFixed(0)} km)` : '';
        return `
            <label class="server-checkbox-item">
                <input type="checkbox" class="server-cb" value="${id}" ${checked} />
                <span class="server-info-text">
                    <strong>${escapeHtml(s.name || s.sponsor || id)}</strong>
                    <small>${escapeHtml(s.country || '')} ${dist}</small>
                </span>
            </label>
        `;
    }).join('');
}

function getSelectedServerIDs() {
    return Array.from(document.querySelectorAll('.server-cb:checked')).map(cb => {
        const v = cb.value;
        return isNaN(v) ? v : parseInt(v);
    });
}

function describeCron(expr) {
    if (!expr) return { valid: true, text: 'Manuell – keine automatische Ausführung' };
    const p = expr.trim().split(/\s+/);
    if (p.length !== 6) return { valid: false, text: 'Ungültig: Es werden 6 Felder erwartet (Sek Min Std Tag Mon Wochentag)' };
    if (p.some(v => !/^[\d*/?,\-]+$/.test(v))) return { valid: false, text: 'Ungültiges Zeichen im Cron-Ausdruck' };
    const [sec, min, hour, dom, mon, dow] = p;
    let text;
    if (sec === '0' && /^\*\/\d+$/.test(min) && hour === '*' && dom === '*' && mon === '*' && dow === '*')
        text = `Alle ${min.slice(2)} Minuten`;
    else if (sec === '0' && min === '0' && /^\*\/\d+$/.test(hour) && dom === '*' && mon === '*' && dow === '*')
        text = hour.slice(2) === '1' ? 'Stündlich' : `Alle ${hour.slice(2)} Stunden`;
    else if (sec === '0' && /^\d+$/.test(min) && /^\d+$/.test(hour) && dom === '*' && mon === '*' && dow === '*')
        text = `Täglich um ${hour.padStart(2,'0')}:${min.padStart(2,'0')} Uhr`;
    else if (sec === '0' && min === '0' && hour === '0' && dom === '*' && mon === '*' && dow === '*')
        text = 'Täglich um 00:00 Uhr';
    else if (/^\*\/\d+$/.test(sec) && min === '*' && hour === '*')
        text = `Alle ${sec.slice(2)} Sekunden`;
    else
        text = `Zeitplan: Sek. ${sec}, Min. ${min}, Std. ${hour}, Tag ${dom}, Monat ${mon}, Wochentag ${dow}`;
    return { valid: true, text: `✓ ${text}` };
}

async function saveProfile() {
    const name = document.getElementById('form-name').value.trim();
    if (!name) { alert('Name darf nicht leer sein'); return; }

    const profile = {
        name,
        description: document.getElementById('form-desc').value.trim(),
        cron_expr: document.getElementById('form-cron').value.trim(),
        metrics: Array.from(document.querySelectorAll('.form-metric:checked')).map(cb => cb.value),
        enabled: document.getElementById('form-enabled').checked,
        server_mode: document.querySelector('input[name="server-mode"]:checked').value,
        server_ids: getSelectedServerIDs(),
    };

    if (profile.metrics.length === 0) {
        alert('Mindestens eine Metrik auswählen');
        return;
    }

    try {
        if (editingProfileId) {
            await API.put(`/api/profiles/${editingProfileId}`, profile);
        } else {
            await API.post('/api/profiles', profile);
        }
        closeProfileDialog();
        reloadProfileList();
        syncProfileDropdowns();
    } catch (e) {
        alert('Speichern fehlgeschlagen: ' + e.message);
    }
}

// reloadProfileList löst einen HTMX-Reload der Profilliste aus.
function reloadProfileList() {
    const list = document.getElementById('profile-list');
    if (list) {
        htmx.trigger(list, 'reload');
    }
}

function closeProfileDialog() {
    const overlay = document.getElementById('profile-dialog-overlay');
    if (overlay) overlay.remove();
    editingProfileId = null;
}

document.addEventListener('DOMContentLoaded', () => {
    syncProfileDropdowns();
});