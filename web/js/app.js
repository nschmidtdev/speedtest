// app.js — Hauptlogik, Tab-Routing, API-Helper, Shared Utils

const API = {
    async get(path) {
        const res = await fetch(path);
        if (!res.ok) throw new Error(`GET ${path} failed: ${res.status}`);
        return res.json();
    },
    async post(path, body) {
        const res = await fetch(path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: body ? JSON.stringify(body) : undefined,
        });
        if (!res.ok) throw new Error(`POST ${path} failed: ${res.status}`);
        return res.json();
    },
    async put(path, body) {
        const res = await fetch(path, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: body ? JSON.stringify(body) : undefined,
        });
        if (!res.ok) throw new Error(`PUT ${path} failed: ${res.status}`);
        return res.json();
    },
    async del(path) {
        const res = await fetch(path, { method: 'DELETE' });
        if (!res.ok) throw new Error(`DELETE ${path} failed: ${res.status}`);
        return res.json();
    }
};

// === Shared Helpers ===

function formatTime(ts) {
    if (!ts) return '—';
    const d = new Date(ts);
    return d.toLocaleString('de-DE', {
        day: '2-digit', month: '2-digit',
        hour: '2-digit', minute: '2-digit'
    });
}

function formatRelative(ts) {
    if (!ts) return '';
    const diff = Date.now() - new Date(ts).getTime();
    const min = Math.floor(diff / 60000);
    if (min < 1) return 'gerade eben';
    if (min < 60) return `vor ${min} Min`;
    const hr = Math.floor(min / 60);
    if (hr < 24) return `vor ${hr} Std`;
    return `vor ${Math.floor(hr / 24)} Tag(en)`;
}

function fmt(v, decimals = 1) {
    return v != null && v > 0 ? v.toFixed(decimals) : '—';
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text || '';
    return div.innerHTML;
}

// Chart.js dark-theme defaults
function chartGradient(ctx, color) {
    const gradient = ctx.createLinearGradient(0, 0, 0, 300);
    gradient.addColorStop(0, color.replace('0.1', '0.25'));
    gradient.addColorStop(1, color.replace('0.1', '0.0'));
    return gradient;
}

// === Tab Routing ===
const tabs = document.querySelectorAll('.nav-tab');
const tabContents = document.querySelectorAll('.tab-content');

tabs.forEach(tab => {
    tab.addEventListener('click', () => {
        const target = tab.dataset.tab;
        tabs.forEach(t => t.classList.remove('active'));
        tab.classList.add('active');
        tabContents.forEach(c => c.classList.remove('active'));
        document.getElementById(`tab-${target}`).classList.add('active');

        if (target === 'history') loadHistoryData();
        if (target === 'profiles') loadProfiles();
        if (target === 'settings') {
            loadSchedulerStatus();
            if (typeof loadTariffs === 'function') loadTariffs();
        }
    });
});

// === Profile Dropdown ===
async function loadProfileDropdown() {
    try {
        const profiles = await API.get('/api/profiles');
        const select = document.getElementById('profile-select');
        select.innerHTML = '<option value="">Profil wählen...</option>';
        profiles.forEach(p => {
            const opt = document.createElement('option');
            opt.value = p.id;
            opt.textContent = p.name;
            select.appendChild(opt);
        });
    } catch (e) {
        console.warn('Profile not loaded yet:', e);
    }
}

// === Scheduler Status ===
async function loadSchedulerStatus() {
    try {
        const jobs = await API.get('/api/scheduler/status');
        const container = document.getElementById('scheduler-status');
        if (!container) return;
        if (!jobs || jobs.length === 0) {
            container.innerHTML = '<p class="coming-soon">Keine aktiven Scheduler-Jobs</p>';
            return;
        }
        container.innerHTML = jobs.map(j => {
            const next = j.next_run ? formatRelative(j.next_run) : '—';
            const last = j.last_run ? formatRelative(j.last_run) : 'nie';
            return `
                <div class="scheduler-job">
                    <span class="scheduler-job-name">${escapeHtml(j.profile_name)}</span>
                    <span class="scheduler-job-cron">${escapeHtml(j.cron_expr || '—')}<small>${typeof describeCron === 'function' ? escapeHtml(describeCron(j.cron_expr).text.replace('✓ ', '')) : ''}</small></span>
                    <span class="scheduler-badge ${j.enabled ? 'active' : 'inactive'}">${j.enabled ? 'aktiv' : 'inaktiv'}</span>
                    <span class="scheduler-job-next">Nächste: ${j.next_run ? formatTime(j.next_run) : '—'} (${next}) · Letzte: ${last}</span>
                </div>`;
        }).join('');
    } catch (e) {
        console.warn('Scheduler status failed:', e);
    }
}

// === Dashboard: averages, extrema and daily trends ===
let statsChart = null;
const ringCharts = {};
let dashboardDays = 7;

async function loadDashboard() {
    try {
        const [results, stats] = await Promise.all([
            API.get('/api/results?limit=1000'),
            API.get(`/api/stats?days=${dashboardDays}`),
        ]);
        renderStats(stats);
        renderRecentTests(results);
        renderTodayChart(results);
        if (results.length > 0 && !testInProgress) {
            displayResult(results[0]);
            document.getElementById('dashboard-updated').textContent =
                'Aktualisiert ' + formatRelative(results[0].measured_at);
        }
    } catch (e) {
        console.warn('Dashboard load failed:', e);
    }
}

function renderStats(stats) {
    const hasData = stats && stats.result_count > 0;
    document.getElementById('stats-empty')?.classList.toggle('hidden', hasData);
    document.getElementById('metric-rings')?.classList.toggle('hidden', !hasData);
    if (!hasData) return;

    const cfg = {
        download: { color: '#3b82f6', unit: 'Mbps' },
        upload: { color: '#22c55e', unit: 'Mbps' },
        ping: { color: '#f59e0b', unit: 'ms' },
        jitter: { color: '#a855f7', unit: 'ms' },
    };
    Object.entries(cfg).forEach(([name, meta]) => renderMetricRing(name, stats.metrics[name], meta));
}

function renderMetricRing(name, metric, meta) {
    const avgEl = document.getElementById(`avg-${name}`);
    const extrema = document.getElementById(`extrema-${name}`);
    if (!metric || !metric.count) {
        avgEl.textContent = '—';
        extrema.innerHTML = '<span>Keine Werte</span>';
        if (ringCharts[name]) { ringCharts[name].destroy(); delete ringCharts[name]; }
        return;
    }
    avgEl.textContent = metric.average.toFixed(1);
    extrema.innerHTML = `
        <span class="best">Top <strong>${metric.best.toFixed(1)} ${meta.unit}</strong><small>${formatTime(metric.best_at)}</small></span>
        <span class="worst">Schlechteste <strong>${metric.worst.toFixed(1)} ${meta.unit}</strong><small>${formatTime(metric.worst_at)}</small></span>`;
    if (ringCharts[name]) ringCharts[name].destroy();
    const ceiling = Math.max(metric.high, metric.average, 1);
    ringCharts[name] = new Chart(document.getElementById(`ring-${name}`), {
        type: 'doughnut',
        data: { labels: ['Durchschnitt', 'Abstand zum Zeitraum-Hoch'], datasets: [{
            data: [metric.average, Math.max(ceiling - metric.average, ceiling * 0.08)],
            backgroundColor: [meta.color, 'rgba(113,113,122,0.16)'], borderWidth: 0,
            hoverOffset: 7,
        }]},
        options: {
            responsive: true, maintainAspectRatio: false, animation: false, cutout: '76%', rotation: -90,
            plugins: {
                legend: { display: false },
                tooltip: { callbacks: {
                    label: (ctx) => ctx.dataIndex === 0
                        ? ` Durchschnitt: ${metric.average.toFixed(1)} ${meta.unit}`
                        : ` ${metric.count} Messungen`,
                    afterBody: () => [
                        `Top: ${metric.best.toFixed(1)} ${meta.unit} · ${formatTime(metric.best_at)}`,
                        `Schlechteste: ${metric.worst.toFixed(1)} ${meta.unit} · ${formatTime(metric.worst_at)}`,
                        `Tief/Hoch: ${metric.low.toFixed(1)} / ${metric.high.toFixed(1)} ${meta.unit}`,
                    ],
                }}
            }
        }
    });
}

function renderTodayChart(results) {
    const canvas = document.getElementById('stats-chart');
    if (!canvas) return;
    if (statsChart) statsChart.destroy();

    const now = new Date();
    const today = (results || [])
        .filter(r => {
            const d = new Date(r.measured_at);
            return d.getFullYear() === now.getFullYear()
                && d.getMonth() === now.getMonth()
                && d.getDate() === now.getDate();
        })
        .sort((a, b) => new Date(a.measured_at) - new Date(b.measured_at));

    document.getElementById('today-date').textContent = now.toLocaleDateString('de-DE', {
        weekday: 'long', day: '2-digit', month: 'long', year: 'numeric'
    });
    document.getElementById('today-count').textContent = `${today.length} Messungen heute`;

    const labels = today.map(r => new Date(r.measured_at).toLocaleTimeString('de-DE', {
        hour: '2-digit', minute: '2-digit'
    }));
    const points = key => today.flatMap((r, i) => {
        const value = Number(r[key]);
        return value > 0 ? [{ x: i, y: value }] : [];
    });
    const line = (label, key, color, axis) => ({
        label,
        data: points(key),
        borderColor: color,
        backgroundColor: color,
        borderWidth: 2,
        pointRadius: 3,
        pointHoverRadius: 6,
        tension: .28,
        yAxisID: axis,
        spanGaps: true,
    });
    statsChart = new Chart(canvas, {
        type: 'line',
        data: { labels, datasets: [
            line('Download', 'download_mbps', '#3b82f6', 'speed'),
            line('Upload', 'upload_mbps', '#22c55e', 'speed'),
            line('Ping', 'ping_ms', '#f59e0b', 'latency'),
            line('Jitter', 'jitter_ms', '#a855f7', 'latency'),
        ]},
        options: { responsive:true, maintainAspectRatio:false, animation:false, parsing:false, interaction:{mode:'nearest',intersect:false},
            scales:{ x:{type:'linear',min:0,max:Math.max(labels.length-1,1),ticks:{color:'#71717a',maxTicksLimit:10,callback:v=>labels[Math.round(v)]||''},title:{display:true,text:'Uhrzeit'}}, speed:{position:'left',beginAtZero:true,title:{display:true,text:'Mbps'},ticks:{color:'#71717a'}}, latency:{position:'right',beginAtZero:true,title:{display:true,text:'ms'},ticks:{color:'#71717a'},grid:{drawOnChartArea:false}} },
            plugins:{ legend:{labels:{color:'#e4e4e7',boxWidth:12}}, tooltip:{backgroundColor:'#0f1117',callbacks:{title:items=>items.length ? labels[Math.round(items[0].raw.x)] : ''}} }
        }
    });
}

function renderRecentTests(results) {
    const container = document.getElementById('recent-tests');
    if (!container) return;
    if (!results || results.length === 0) {
        container.innerHTML = '<p class="coming-soon">Noch keine Messungen. Starte deinen ersten Test!</p>';
        return;
    }
    container.innerHTML = results.slice(0, 8).map(r => `
        <div class="recent-item">
            <div class="recent-time">${formatRelative(r.measured_at)}</div>
            <div class="recent-stats">
                ${r.download_mbps ? `<span class="stat-dl">↓ ${r.download_mbps.toFixed(0)}</span>` : ''}
                ${r.upload_mbps ? `<span class="stat-ul">↑ ${r.upload_mbps.toFixed(0)}</span>` : ''}
                ${r.ping_ms ? `<span class="stat-ping">⚡ ${r.ping_ms.toFixed(0)}ms</span>` : ''}
            </div>
            <div class="recent-status"><span class="status-badge status-${r.status}">${r.status}</span></div>
        </div>
    `).join('');
}

// Init on load
let testInProgress = false;

document.addEventListener('DOMContentLoaded', () => {
    loadProfileDropdown();
    loadDashboard();
    document.querySelectorAll('#dashboard-range button').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('#dashboard-range button').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            dashboardDays = Number(btn.dataset.days);
            loadDashboard();
        });
    });
    document.getElementById('setting-port').textContent =
        window.location.port || '8080';
});
