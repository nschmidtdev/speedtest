// tariffs.js — Tarif-Vergleichsanzeige für Live-Ergebnisse + Edit-Logik.
// Die gesamte Tarif-Verwaltung (Liste, Formular, Katalog) läuft über HTMX.

async function loadTariffComparison(result) {
    const download = document.getElementById('tariff-download');
    const upload = document.getElementById('tariff-upload');
    const warningEl = document.getElementById('tariff-warning');
    if (download) download.classList.add('hidden');
    if (upload) upload.classList.add('hidden');
    if (warningEl) warningEl.classList.add('hidden');
    if (!result?.id || !result?.tariff_id) return;
    const response = await fetch(`/api/tariff-comparison?result_id=${result.id}`);
    if (!response.ok) return;
    const comparison = await response.json();
    renderTariffMetric(download, comparison.download, comparison.tariff);
    renderTariffMetric(upload, comparison.upload, comparison.tariff);
    updateTariffWarning(comparison, result);
}

// Tarif-Warnung bei Unterschreitung + Streak-Info
function updateTariffWarning(comparison, result) {
    const warningEl = document.getElementById('tariff-warning');
    if (!warningEl) return;
    const problems = [];
    ['download', 'upload'].forEach(metric => {
        const m = comparison[metric];
        if (!m) return;
        if (m.status === 'below_minimum' || m.status === 'below_normal') {
            problems.push({metric: metric.charAt(0).toUpperCase() + metric.slice(1), ...m});
        }
    });
    if (problems.length === 0) {
        warningEl.classList.add('hidden');
        return;
    }
    const worst = problems.some(p => p.status === 'below_minimum');
    const title = worst ? 'Tarif wird deutlich unterschritten' : 'Tarif wird unterschritten';
    const detail = problems.map(p =>
        `${p.metric}: ${p.percent.toLocaleString('de-DE',{maximumFractionDigits:1})}% vom Tarif`
    ).join(' · ');
    warningEl.querySelector('.warning-title').textContent = title;
    warningEl.querySelector('.warning-detail').textContent = detail;
    warningEl.className = `tariff-warning ${worst ? 'severity-critical' : 'severity-warning'}`;

    // Streak abfragen
    const streakEl = warningEl.querySelector('.warning-streak');
    if (result?.profile_id) {
        fetch(`/api/tariff-streak?profile_id=${result.profile_id}&days=14`)
            .then(r => r.ok ? r.json() : null)
            .then(data => {
                if (!data || data.current_streak < 1) {
                    streakEl.classList.add('hidden');
                    return;
                }
                let text = `${data.current_streak}. Werktag in Folge unterschritten`;
                if (data.ready_to_complain) {
                    text += ' — Bedingungen für Mängelmeldung erfüllt (§41 TKG)';
                }
                streakEl.textContent = text;
                streakEl.classList.remove('hidden');
            })
            .catch(() => streakEl.classList.add('hidden'));
    }
}

function renderTariffMetric(element, metric, tariff) {
    if (!metric || metric.status === 'insufficient_data') return;
    const labels = { meets_advertised: 'Tarif erreicht', within_normal: 'im Normalbereich', below_normal: 'unter Normalwert', below_minimum: 'unter Minimum' };
    element.className = `tariff-result tariff-${metric.status}`;
    const deviation = Number(metric.deviation_mbps ?? (metric.actual_mbps - metric.advertised_mbps));
    const signedDeviation = `${deviation > 0 ? '+' : ''}${deviation.toLocaleString('de-DE', {maximumFractionDigits: 1})}`.replace('-', '−');
    const title = `${tariff.provider} · ${tariff.name} · ${labels[metric.status] || ''}`;
    element.title = title;
    element.innerHTML = `<span>${metric.percent.toLocaleString('de-DE', {maximumFractionDigits: 1})} % vom Tarif</span><span class="tariff-deviation">${signedDeviation} Mbps</span>`;
}

// applyCatalogTariff füllt die Geschwindigkeitsfelder aus einem Katalog-Tarif.
// Lädt den Katalog asynchron und sucht per Tarif-Name.
async function applyCatalogTariff(name) {
    if (!name || name === '__custom' || name === '') return;
    try {
        const res = await fetch('/api/tariff-catalog');
        if (!res.ok) return;
        const catalog = await res.json();
        for (const provider of catalog.providers) {
            for (const plan of provider.tariffs) {
                if (plan.name === name) {
                    const form = document.querySelector('.tariff-form');
                    if (!form) return;
                    const setVal = (n, v) => { const el = form.querySelector(`[name="${n}"]`); if (el && v != null) el.value = v; };
                    setVal('access_technology', plan.access_technology || '');
                    setVal('advertised_down_mbps', plan.advertised_down_mbps || '');
                    // Upload: nur setzen, wenn nicht requires_upload_input
                    if (!plan.requires_upload_input) {
                        setVal('advertised_up_mbps', plan.advertised_up_mbps || '');
                    } else {
                        setVal('advertised_up_mbps', '');
                    }
                    // Normal/Minimum werden vom Katalog nicht geliefert → Felder leeren
                    ['normal_down_mbps','normal_up_mbps','minimum_down_mbps','minimum_up_mbps'].forEach(n => {
                        const el = form.querySelector(`[name="${n}"]`);
                        if (el) el.value = '';
                    });
                    return;
                }
            }
        }
    } catch (e) {
        console.error('applyCatalogTariff failed:', e);
    }
}

// editTariff lädt einen Tarif per JSON und füllt das HTMX-Formular vor.
// Speichern erstellt eine neue Version (Versionierung).
async function editTariff(id) {
    try {
        const res = await fetch(`/api/tariffs/${id}`);
        if (!res.ok) return;
        const t = await res.json();

        const form = document.querySelector('.tariff-form');
        if (!form) return;

        const setVal = (name, val) => {
            const el = form.querySelector(`[name="${name}"]`);
            if (el) el.value = val;
        };

        setVal('profile_id', t.profile_id);
        setVal('access_technology', t.access_technology || '');
        setVal('advertised_down_mbps', t.advertised_down_mbps || '');
        setVal('advertised_up_mbps', t.advertised_up_mbps || '');
        setVal('normal_down_mbps', t.normal_down_mbps || '');
        setVal('normal_up_mbps', t.normal_up_mbps || '');
        setVal('minimum_down_mbps', t.minimum_down_mbps || '');
        setVal('minimum_up_mbps', t.minimum_up_mbps || '');

        // Provider: im Dropdown suchen, sonst Custom-Feld
        const providerSelect = form.querySelector('[name="provider_select"]');
        const providerInput = form.querySelector('[name="provider"]');
        const found = [...(providerSelect?.options || [])].some(o => o.text === t.provider);
        if (found) {
            providerSelect.value = [...providerSelect.options].find(o => o.text === t.provider).value;
            providerInput.classList.add('hidden');
        } else {
            providerSelect.value = '__custom';
            providerInput.value = t.provider;
            providerInput.classList.remove('hidden');
        }

        // Tarifname: Custom-Feld, da Katalog-Tarife nur Anzeige-Values nutzen
        const nameInput = form.querySelector('[name="name"]');
        nameInput.value = t.name;
        nameInput.classList.remove('hidden');

        // Status-Hinweis
        const status = document.getElementById('tariff-form-status');
        if (status) {
            status.textContent = `Bearbeite ${t.provider} · ${t.name} — Speichern erzeugt eine neue Version.`;
            status.className = 'tariff-success';
        }

        form.scrollIntoView({ behavior: 'smooth', block: 'center' });
    } catch (e) {
        console.error('editTariff failed:', e);
    }
}

// reloadTariffList lädt die Tarif-Liste verlässlich vom Server neu.
// Umgeht Browser-Cache durch einen Timestamp-Parameter.
async function reloadTariffList() {
    try {
        const res = await fetch('/api/tariffs?_t=' + Date.now(), {
            headers: { 'HX-Request': 'true' }
        });
        if (!res.ok) return;
        const html = await res.text();
        const container = document.getElementById('active-tariffs');
        if (container) container.innerHTML = html;
    } catch (e) {
        console.error('reloadTariffList failed:', e);
    }
}

// deleteTariff fragt per confirm() nach, löscht den Tarif per fetch
// und lädt die Liste verlässlich vom Server neu.
async function deleteTariff(id, name) {
    if (!confirm(`Tarif "${name}" wirklich löschen?\nMessungen behalten ihre Snapshot-Werte.`)) return;
    try {
        const res = await fetch(`/api/tariffs/${id}`, { method: 'DELETE' });
        if (!res.ok) {
            const msg = await res.text();
            alert('Löschen fehlgeschlagen: ' + msg);
            return;
        }
        // Liste vom Server neu laden (verlässlich, umgeht Cache)
        await reloadTariffList();
    } catch (e) {
        alert('Löschen fehlgeschlagen: ' + e.message);
    }
}

// HX-Events: Nach Speichern Liste neu laden + Formular zurücksetzen
document.body.addEventListener('htmx:afterRequest', (e) => {
    const form = e.detail?.requestConfig?.elt;
    if (form && form.classList?.contains('tariff-form') && e.detail?.successful) {
        const status = document.getElementById('tariff-form-status');
        if (status) {
            status.textContent = 'Tarif gespeichert.';
            status.className = 'tariff-success';
        }
        // Liste zuverlässig neu laden
        reloadTariffList();
        // Nach kurzer Pause Formular zurücksetzen
        setTimeout(() => {
            form.reset();
            form.querySelectorAll('input[name="provider"], input[name="name"]').forEach(el => el.classList.add('hidden'));
            htmx.trigger(form.querySelector('[name="provider_select"]'), 'load');
            htmx.trigger(form.querySelector('[name="tariff_template"]'), 'load');
        }, 1500);
    }
});

// generateComplaint: Generiert ein §41 TKG-Beschwerdeschreiben.
// Prüft vorher die gesetzliche Schwelle (2+ konsekutive Werktage).
async function generateComplaint(profileId) {
    if (!confirm(
        'Es wird ein formales Beschwerdeschreiben mit Ihren Messdaten generiert.\n\n' +
        'Dies ist KEINE Rechtsberatung. Die Messwerte sind Näherungswerte und ' +
        'keine verbindliche Bandbreitenmessung. Nutzung auf eigene Verantwortung.\n\n' +
        'Fortfahren?'
    )) return;

    try {
        const res = await fetch(`/api/complaint?profile_id=${profileId}&days=30`);
        if (res.status === 409) {
            // Schwelle nicht erfüllt — klare Info statt generiertem Dokument
            const err = await res.json();
            showToast(err.error || 'Schwelle nicht erfüllt', 'warning');
            return;
        }
        if (!res.ok) {
            showToast('Fehler beim Generieren: ' + await res.text(), 'error');
            return;
        }
        // HTML in neuem Tab öffnen
        const html = await res.text();
        const blob = new Blob([html], { type: 'text/html' });
        window.open(URL.createObjectURL(blob), '_blank');
    } catch (e) {
        showToast('Netzwerkfehler: ' + e.message, 'error');
    }
}