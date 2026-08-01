// live.js — SSE Live-Push + Test-Steuerung

let evtSource = null;
let liveDownloadData = [];
let liveUploadData = [];
let liveChart = null;

function connectSSE() {
    if (evtSource) evtSource.close();
    evtSource = new EventSource('/events');

    evtSource.onopen = () => console.log('SSE connected');
    evtSource.onerror = () => {
        console.log('SSE error, reconnecting in 3s...');
        evtSource.close();
        setTimeout(connectSSE, 3000);
    };

    evtSource.onmessage = (event) => {
        try {
            const msg = JSON.parse(event.data);
            handleSSEMessage(msg);
        } catch (e) {}
    };
}

function handleSSEMessage(msg) {
    switch (msg.type) {
        case 'test_start':
            testInProgress = true;
            showProgress(true);
            setPhase(phaseLabel(msg.phase || 'server'));
            setProgress(msg.progress_pct || 2);
            liveDownloadData = [];
            liveUploadData = [];
            resetLiveChart();
            break;
        case 'progress':
            setPhase(phaseLabel(msg.phase));
            const phasePct = {server:5,ping:12,download:28,upload:58,bufferbloat_idle:82,bufferbloat_loaded:90,traceroute:94};
            setProgress(msg.progress_pct || phasePct[msg.phase] || 5);
            if (msg.phase === 'download') {
                if (msg.current_mbps > 0) {
                    liveDownloadData.push({ t: Date.now(), mbps: msg.current_mbps });
                    updateLiveChart('download', msg.current_mbps);
                }
            } else if (msg.phase === 'upload') {
                if (msg.current_mbps > 0) {
                    liveUploadData.push({ t: Date.now(), mbps: msg.current_mbps });
                    updateLiveChart('upload', msg.current_mbps);
                }
            }
            break;
        case 'ping_update':
            if (msg.ping_ms) {
                document.getElementById('result-ping').textContent = msg.ping_ms.toFixed(1);
            }
            if (msg.jitter_ms) {
                document.getElementById('result-jitter').textContent = msg.jitter_ms.toFixed(1);
            }
            break;
        case 'test_complete':
            showProgress(false);
            setProgress(100);
            setTimeout(() => setProgress(0), 1000);
            displayResult(msg.result);
            testInProgress = false;
            // Refresh dashboard data (sparkline + recent list)
            setTimeout(loadDashboard, 500);
            break;
        case 'test_error':
            showProgress(false);
            testInProgress = false;
            showToast('Test fehlgeschlagen: ' + (msg.error || 'Unbekannter Fehler'), 'error');
            break;
    }
}

function phaseLabel(phase) {
    const labels = {
        'server': 'Passenden Testserver auswählen…',
        'download': 'Download wird gemessen…',
        'upload': 'Upload wird gemessen…',
        'ping': 'Ping wird gemessen…',
        'bufferbloat_idle': 'Bufferbloat: Basis-Latenz…',
        'bufferbloat_loaded': 'Bufferbloat: Latenz unter Last…',
        'traceroute': 'Route wird verfolgt…',
    };
    return labels[phase] || phase;
}

function setPhase(text) {
    document.getElementById('phase-label').textContent = text;
}

function setProgress(pct) {
    document.getElementById('progress-bar').style.width = pct + '%';
}

function showProgress(show) {
    document.getElementById('test-progress').classList.toggle('hidden', !show);
    document.getElementById('live-chart-container').classList.toggle('hidden', !show);
    document.getElementById('btn-start-test').disabled = show;
    // Hide dashboard section during test
    const dash = document.getElementById('dashboard-section');
    if (dash) dash.style.display = show ? 'none' : '';
    if (show) {
        document.getElementById('test-results').classList.add('hidden');
    }
}

function displayResult(result) {
    document.getElementById('test-results').classList.remove('hidden');
    if (result.download_mbps > 0)
        document.getElementById('result-download').textContent = result.download_mbps.toFixed(1);
    if (result.upload_mbps > 0)
        document.getElementById('result-upload').textContent = result.upload_mbps.toFixed(1);
    if (result.ping_ms > 0)
        document.getElementById('result-ping').textContent = result.ping_ms.toFixed(1);
    if (result.jitter_ms > 0)
        document.getElementById('result-jitter').textContent = result.jitter_ms.toFixed(1);

    // Bufferbloat
    const extras = document.getElementById('result-extras');
    if (result.bufferbloat_score && result.bufferbloat_score !== 'error') {
        extras.classList.remove('hidden');
        const scoreEl = document.getElementById('result-bb-score');
        scoreEl.textContent = result.bufferbloat_score;
        scoreEl.className = 'result-value bb-' + result.bufferbloat_score;
        document.getElementById('result-bb-idle').textContent = result.bufferbloat_idle_ms ? result.bufferbloat_idle_ms.toFixed(1) : '—';
        document.getElementById('result-bb-loaded').textContent = result.bufferbloat_loaded_ms ? result.bufferbloat_loaded_ms.toFixed(1) : '—';
    } else {
        extras.classList.add('hidden');
    }

    if (result.server_name) {
        const info = document.getElementById('test-server-info');
        info.classList.remove('hidden');
        info.textContent = `Server: ${result.server_name} · ${formatRelative(result.measured_at || new Date().toISOString())}`;
    }
    if (typeof loadTariffComparison === 'function') {
        loadTariffComparison(result).catch(error => console.warn('Tariff comparison failed:', error));
    }
}

function showToast(msg, type) {
    const container = document.getElementById('toast-container');
    if (!container) { console.warn('Toast container missing:', msg); return; }
    const icons = { error: '❌', warning: '⚠️', success: '✅', info: 'ℹ️' };
    const toast = document.createElement('div');
    toast.className = `toast toast-${type || 'info'}`;
    toast.innerHTML = `<span class="toast-icon">${icons[type] || icons.info}</span><span>${escapeHtml(msg)}</span>`;
    container.appendChild(toast);
    const removeDelay = type === 'error' ? 6000 : 4000;
    setTimeout(() => {
        toast.classList.add('removing');
        setTimeout(() => toast.remove(), 300);
    }, removeDelay);
}

// === Live Chart (during test) ===
function initLiveChart() {
    const canvas = document.getElementById('live-chart');
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    liveChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: [],
            datasets: [
                {
                    label: 'Download (Mbps)',
                    data: [],
                    borderColor: '#3b82f6',
                    backgroundColor: 'rgba(59,130,246,0.1)',
                    tension: 0.3,
                    pointRadius: 0,
                },
                {
                    label: 'Upload (Mbps)',
                    data: [],
                    borderColor: '#22c55e',
                    backgroundColor: 'rgba(34,197,94,0.1)',
                    tension: 0.3,
                    pointRadius: 0,
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            scales: {
                x: { display: false },
                y: { ticks: { color: '#71717a' }, beginAtZero: true }
            },
            plugins: {
                legend: { labels: { color: '#e4e4e7' } }
            }
        }
    });
}

function resetLiveChart() {
    if (!liveChart) return;
    liveChart.data.labels = [];
    liveChart.data.datasets[0].data = [];
    liveChart.data.datasets[1].data = [];
    liveChart.update('none');
}

function updateLiveChart(phase, mbps) {
    if (!liveChart) return;
    const label = liveDownloadData.length + liveUploadData.length;
    if (phase === 'download') {
        liveChart.data.labels.push(label);
        liveChart.data.datasets[0].data.push(mbps);
        liveChart.data.datasets[1].data.push(null);
    } else if (phase === 'upload') {
        liveChart.data.labels.push(label);
        liveChart.data.datasets[0].data.push(null);
        liveChart.data.datasets[1].data.push(mbps);
    }
    if (liveChart.data.labels.length > 60) {
        liveChart.data.labels.shift();
        liveChart.data.datasets[0].data.shift();
        liveChart.data.datasets[1].data.shift();
    }
    liveChart.update('none');
}

// === Start Button ===
document.addEventListener('DOMContentLoaded', () => {
    initLiveChart();
    connectSSE();

    document.getElementById('btn-start-test').addEventListener('click', async () => {
        const profileId = document.getElementById('profile-select').value;
        try {
            const res = await API.post('/api/test/run', { profile_id: parseInt(profileId) || null });
            if (res.status === 'started') {
                showProgress(true);
                setPhase('Starte Test…');
            }
        } catch (e) {
            if (e.message.includes('409')) {
                showToast('Es läuft bereits ein Test.', 'warning');
            } else {
                showToast('Test konnte nicht gestartet werden: ' + e.message, 'error');
            }
        }
    });
});
