// history.js — Historie-Ansicht, Chart + Tabelle

let historyChart = null;
let profileNames = {};

async function loadHistoryData() {
    const range = document.getElementById('history-range')?.value || '24h';
    const profileId = document.getElementById('history-profile')?.value || '';

    // Profilnamen für die Tabelle laden (einmalig)
    if (Object.keys(profileNames).length === 0) {
        try {
            const profiles = await API.get('/api/profiles');
            profiles.forEach(p => profileNames[p.id] = p.name);
        } catch (e) { /* ignore */ }
    }

    const params = new URLSearchParams({ limit: '500', range });
    if (profileId) params.set('profile_id', profileId);

    try {
        const results = await API.get(`/api/results?${params}`);
        renderHistoryChart(results);
        renderHistoryTable(results);
    } catch (e) {
        console.error('Failed to load history:', e);
    }
}

function renderHistoryChart(results) {
    const canvas = document.getElementById('history-chart');
    if (!canvas) return;
    const ctx = canvas.getContext('2d');

    // Chronological order (oldest → newest)
    const sorted = [...results].reverse();
    const labels = sorted.map(r => formatTime(r.measured_at));

    if (historyChart) historyChart.destroy();

    historyChart = new Chart(ctx, {
        type: 'line',
        data: {
            labels,
            datasets: [
                {
                    label: 'Download (Mbps)',
                    data: sorted.map(r => r.download_mbps || null),
                    borderColor: '#3b82f6',
                    backgroundColor: 'rgba(59,130,246,0.15)',
                    fill: true,
                    tension: 0.35,
                    pointRadius: 3,
                    pointHoverRadius: 6,
                    pointBackgroundColor: '#3b82f6',
                    pointBorderColor: '#0f1117',
                    pointBorderWidth: 2,
                    spanGaps: true,
                },
                {
                    label: 'Upload (Mbps)',
                    data: sorted.map(r => r.upload_mbps || null),
                    borderColor: '#22c55e',
                    backgroundColor: 'rgba(34,197,94,0.15)',
                    fill: true,
                    tension: 0.35,
                    pointRadius: 3,
                    pointHoverRadius: 6,
                    pointBackgroundColor: '#22c55e',
                    pointBorderColor: '#0f1117',
                    pointBorderWidth: 2,
                    spanGaps: true,
                },
                {
                    label: 'Ping (ms)',
                    data: sorted.map(r => r.ping_ms || null),
                    borderColor: '#f59e0b',
                    backgroundColor: 'rgba(245,158,11,0.08)',
                    fill: false,
                    tension: 0.35,
                    pointRadius: 2,
                    pointHoverRadius: 5,
                    yAxisID: 'y1',
                    spanGaps: true,
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
            scales: {
                x: {
                    ticks: { color: '#71717a', maxTicksLimit: 12 },
                    grid: { color: '#1a1d27' }
                },
                y: {
                    position: 'left',
                    title: { display: true, text: 'Mbps', color: '#71717a' },
                    ticks: { color: '#71717a' },
                    beginAtZero: true,
                    grid: { color: '#1a1d27' }
                },
                y1: {
                    position: 'right',
                    title: { display: true, text: 'Ping (ms)', color: '#71717a' },
                    ticks: { color: '#f59e0b' },
                    beginAtZero: true,
                    grid: { display: false }
                }
            },
            plugins: {
                legend: {
                    labels: { color: '#e4e4e7', boxWidth: 12, padding: 16 }
                },
                tooltip: {
                    backgroundColor: '#0f1117',
                    borderColor: '#2a2d3a',
                    borderWidth: 1,
                    titleColor: '#e4e4e7',
                    bodyColor: '#a1a1aa',
                    padding: 12,
                    callbacks: {
                        label: function(ctx) {
                            const v = ctx.parsed.y;
                            if (v == null) return null;
                            const unit = ctx.dataset.yAxisID === 'y1' ? 'ms' : 'Mbps';
                            return `${ctx.dataset.label.split(' ')[0]}: ${v.toFixed(1)} ${unit}`;
                        }
                    }
                }
            }
        }
    });
}

function renderHistoryTable(results) {
    const tbody = document.getElementById('history-tbody');
    if (!tbody) return;
    tbody.innerHTML = '';
    results.slice(0, 50).forEach(r => {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${formatTime(r.measured_at)}</td>
            <td>${profileNames[r.profile_id] || r.profile_id || '—'}</td>
            <td>${fmt(r.download_mbps)}${historyTariffDeviation(r.tariff_down_percent, r.tariff_down_deviation_mbps, r.tariff_down_status)}</td>
            <td>${fmt(r.upload_mbps)}${historyTariffDeviation(r.tariff_up_percent, r.tariff_up_deviation_mbps, r.tariff_up_status)}</td>
            <td>${fmt(r.ping_ms)}</td>
            <td><span class="status-badge status-${r.status}">${r.status}</span></td>
        `;
        tbody.appendChild(row);
    });
}

function historyTariffDeviation(percent, deviation, status) {
    if (!status || status === 'insufficient_data') return '';
    const signed = `${deviation > 0 ? '+' : ''}${Number(deviation).toLocaleString('de-DE', {maximumFractionDigits: 1})}`.replace('-', '−');
    return `<small class="history-tariff tariff-${status}">${Number(percent).toLocaleString('de-DE', {maximumFractionDigits: 1})} % · ${signed}</small>`;
}

document.addEventListener('DOMContentLoaded', () => {
    const range = document.getElementById('history-range');
    const profile = document.getElementById('history-profile');
    if (range) range.addEventListener('change', loadHistoryData);
    if (profile) profile.addEventListener('change', loadHistoryData);
});
