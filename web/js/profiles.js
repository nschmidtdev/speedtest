// profiles.js — Profile CRUD UI + Editor Dialog + Server Selection

let availableServers = [];
let editingProfileId = null;

async function loadProfiles() {
    try {
        const profiles = await API.get('/api/profiles');
        renderProfileList(profiles);
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
        console.error('Failed to load profiles:', e);
    }
}

function renderProfileList(profiles) {
    const container = document.getElementById('profile-list');
    container.innerHTML = '';

    if (!profiles || profiles.length === 0) {
        container.innerHTML = '<p class="coming-soon">Keine Profile vorhanden.</p>';
        return;
    }

    profiles.forEach(p => {
        const card = document.createElement('div');
        card.className = 'profile-card';

        const modeLabel = { auto: 'Auto', random: 'Random', fixed: 'Fixed' }[p.server_mode] || 'Auto';
        const serverInfo = p.server_mode === 'auto' ? 'Ookla wählt'
            : `${p.server_ids?.length || 0} Server ausgewählt`;

        card.innerHTML = `
            <div style="display:flex;justify-content:space-between;align-items:start;">
                <div class="profile-card-left" style="cursor:pointer;" data-edit-id="${p.id}">
                    <h3>${escapeHtml(p.name)}</h3>
                    <div class="profile-meta">
                        <span>📁 ${p.cron_expr || 'Manuell'}</span>
                        <span>🧮 ${p.metrics?.length || 0} Metriken</span>
                        <span>🖥 ${modeLabel}: ${serverInfo}</span>
                    </div>
                    <div class="profile-metrics">
                        ${(p.metrics || []).map(m => `<span class="metric-tag">${m}</span>`).join('')}
                    </div>
                </div>
                <div style="display:flex;flex-direction:column;align-items:end;gap:8px;">
                    <label class="toggle">
                        <input type="checkbox" ${p.enabled ? 'checked' : ''} data-profile-id="${p.id}" class="toggle-enable" />
                        <span class="toggle-slider"></span>
                    </label>
                    <button class="btn btn-sm btn-secondary" data-edit-id="${p.id}">Bearbeiten</button>
                </div>
            </div>
        `;
        container.appendChild(card);
    });

    // Toggle-Handler
    document.querySelectorAll('.toggle-enable').forEach(toggle => {
        toggle.addEventListener('change', async (e) => {
            const id = e.target.dataset.profileId;
            const enabled = e.target.checked;
            await API.post(`/api/profiles/${id}/${enabled ? 'enable' : 'disable'}`);
        });
    });

    // Edit-Handler
    document.querySelectorAll('[data-edit-id]').forEach(el => {
        el.addEventListener('click', (e) => {
            const id = parseInt(e.currentTarget.dataset.editId);
            openProfileEditor(id);
        });
    });
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
            loadProfiles();
        } catch (e) { alert('Löschen fehlgeschlagen: ' + e.message); }
    });
}

function renderServerCheckboxes(servers, selectedIDs) {
    if (!servers || servers.length === 0) {
        return '<p class="coming-soon">Keine Server verfügbar. Cache wird beim ersten Test geladen.</p>';
    }
    // Parse server IDs from the API (they may be string IDs in Ookla)
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
        loadProfiles();
    } catch (e) {
        alert('Speichern fehlgeschlagen: ' + e.message);
    }
}

function closeProfileDialog() {
    const overlay = document.getElementById('profile-dialog-overlay');
    if (overlay) overlay.remove();
    editingProfileId = null;
}

// Local escapeHtml fallback if app.js hasn't loaded yet
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text || '';
    return div.innerHTML;
}

document.addEventListener('DOMContentLoaded', () => {
    const btn = document.getElementById('btn-new-profile');
    if (btn) {
        btn.addEventListener('click', () => openProfileEditor(null));
    }
});
