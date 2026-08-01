// tariffs.js — Tarifkatalog, Tarifverwaltung und Soll/Ist-Vergleich

const CUSTOM_VALUE = '__custom';
let activeTariffs = [];
let tariffCatalog = { verified_at: '', note: '', providers: [] };

function tariffNumber(id) {
    const value = Number(document.getElementById(id)?.value || 0);
    return Number.isFinite(value) ? value : 0;
}

async function loadTariffs() {
    const profileSelect = document.getElementById('tariff-profile');
    if (!profileSelect) return;
    try {
        const selectedProfile = profileSelect.value;
        const selectedProvider = document.getElementById('tariff-provider-select')?.value || '';
        const [profiles, tariffs, catalog] = await Promise.all([
            API.get('/api/profiles'),
            API.get('/api/tariffs'),
            API.get('/api/tariff-catalog'),
        ]);
        profileSelect.innerHTML = '<option value="">Profil wählen …</option>' + profiles
            .map(p => `<option value="${p.id}">${escapeHtml(p.name)}</option>`).join('');
        if ([...profileSelect.options].some(o => o.value === selectedProfile)) profileSelect.value = selectedProfile;

        tariffCatalog = catalog || { verified_at: '', note: '', providers: [] };
        renderProviderOptions(selectedProvider);
        renderCatalogMeta();
        activeTariffs = tariffs || [];
        renderActiveTariffs(activeTariffs);
    } catch (error) {
        document.getElementById('active-tariffs').innerHTML = '<p class="tariff-error">Tarife oder Tarifkatalog konnten nicht geladen werden.</p>';
        console.error('Tariffs failed:', error);
    }
}

function renderProviderOptions(selected = '') {
    const select = document.getElementById('tariff-provider-select');
    if (!select) return;
    select.innerHTML = '<option value="">Anbieter wählen …</option>' +
        tariffCatalog.providers.map(p => `<option value="${p.id}">${escapeHtml(p.name)}</option>`).join('') +
        `<option value="${CUSTOM_VALUE}">Anderer Anbieter …</option>`;
    select.value = [...select.options].some(o => o.value === selected) ? selected : '';
    updateProviderFields(false);
}

function renderCatalogMeta(provider = null, plan = null) {
    const element = document.getElementById('tariff-catalog-meta');
    if (!element) return;
    const date = tariffCatalog.verified_at
        ? new Date(`${tariffCatalog.verified_at}T00:00:00Z`).toLocaleDateString('de-DE')
        : 'unbekannt';
    if (provider) {
        const supplement = plan?.requires_upload_input
            ? ' · Upload hängt von Anschluss/Adresse ab und muss ergänzt werden.'
            : '';
        element.innerHTML = `Vorlage geprüft am ${date} · <a href="${provider.source_url}" target="_blank" rel="noopener">offizielle Quelle</a>${supplement}`;
        return;
    }
    element.textContent = `Lokaler Tarifkatalog · geprüft am ${date}. Werte bitte mit deinem Produktinformationsblatt abgleichen.`;
}

function updateProviderFields(resetValues = true) {
    const providerID = document.getElementById('tariff-provider-select').value;
    const customInput = document.getElementById('tariff-provider');
    const custom = providerID === CUSTOM_VALUE;
    customInput.classList.toggle('hidden', !custom);
    customInput.required = custom;
    if (!custom && resetValues) customInput.value = '';
    renderTariffOptions(providerID);
}

function renderTariffOptions(providerID, selected = '') {
    const select = document.getElementById('tariff-template');
    const customName = document.getElementById('tariff-name');
    const provider = tariffCatalog.providers.find(p => p.id === providerID);
    if (provider) {
        select.innerHTML = '<option value="">Tarif wählen …</option>' + provider.tariffs
            .map(t => `<option value="${t.id}">${escapeHtml(t.name)}</option>`).join('') +
            `<option value="${CUSTOM_VALUE}">Eigener Tarif …</option>`;
    } else if (providerID === CUSTOM_VALUE) {
        select.innerHTML = `<option value="${CUSTOM_VALUE}">Eigener Tarif …</option>`;
        selected = CUSTOM_VALUE;
    } else {
        select.innerHTML = '<option value="">Zuerst Anbieter wählen …</option>';
    }
    select.value = [...select.options].some(o => o.value === selected) ? selected : '';
    const custom = select.value === CUSTOM_VALUE;
    customName.classList.toggle('hidden', !custom);
    customName.required = custom;
    renderCatalogMeta(provider || null, null);
}

function applyCatalogTariff() {
    const providerID = document.getElementById('tariff-provider-select').value;
    const tariffID = document.getElementById('tariff-template').value;
    const customName = document.getElementById('tariff-name');
    const provider = tariffCatalog.providers.find(p => p.id === providerID);
    const plan = provider?.tariffs.find(t => t.id === tariffID);
    const custom = tariffID === CUSTOM_VALUE;
    customName.classList.toggle('hidden', !custom);
    customName.required = custom;
    if (custom) {
        customName.value = '';
        renderCatalogMeta(provider || null, null);
        return;
    }
    if (!plan) return;

    document.getElementById('tariff-technology').value = plan.access_technology || '';
    document.getElementById('tariff-down-max').value = plan.advertised_down_mbps || '';
    document.getElementById('tariff-up-max').value = plan.advertised_up_mbps || '';
    for (const id of ['tariff-down-normal', 'tariff-down-min', 'tariff-up-normal', 'tariff-up-min']) {
        document.getElementById(id).value = '';
    }
    const status = document.getElementById('tariff-form-status');
    status.className = '';
    status.textContent = plan.requires_upload_input
        ? 'Download übernommen. Bitte Upload aus deinem Vertrag ergänzen.'
        : 'Tarifwerte aus dem lokalen Katalog übernommen und frei editierbar.';
    renderCatalogMeta(provider, plan);
}

function selectedProviderName() {
    const id = document.getElementById('tariff-provider-select').value;
    if (id === CUSTOM_VALUE) return document.getElementById('tariff-provider').value.trim();
    return tariffCatalog.providers.find(p => p.id === id)?.name || '';
}

function selectedTariffName() {
    const id = document.getElementById('tariff-template').value;
    if (id === CUSTOM_VALUE) return document.getElementById('tariff-name').value.trim();
    for (const provider of tariffCatalog.providers) {
        const plan = provider.tariffs.find(t => t.id === id);
        if (plan) return plan.name;
    }
    return '';
}

function renderActiveTariffs(tariffs) {
    const container = document.getElementById('active-tariffs');
    if (!container) return;
    if (!tariffs.length) {
        container.innerHTML = '<p class="coming-soon">Noch kein Tarif hinterlegt. Alte Messungen bleiben ohne Tarifvergleich.</p>';
        return;
    }
    container.innerHTML = tariffs.map(t => `
        <article class="tariff-card">
            <div>
                <span class="tariff-profile">${escapeHtml(t.profile_name)}</span>
                <h4>${escapeHtml(t.provider)} · ${escapeHtml(t.name)}</h4>
                <small>${escapeHtml(t.access_technology || 'Anschlussart offen')} · gültig seit ${formatTime(t.valid_from)}</small>
            </div>
            <div class="tariff-speeds">
                <span>↓ <strong>${t.advertised_down_mbps.toLocaleString('de-DE')} Mbps</strong>${tariffThresholdText(t.normal_down_mbps, t.minimum_down_mbps)}</span>
                <span>↑ <strong>${t.advertised_up_mbps.toLocaleString('de-DE')} Mbps</strong>${tariffThresholdText(t.normal_up_mbps, t.minimum_up_mbps)}</span>
            </div>
            <button type="button" class="btn btn-sm btn-secondary tariff-edit" data-tariff-id="${t.id}">Neue Version</button>
        </article>`).join('');
    container.querySelectorAll('.tariff-edit').forEach(button => {
        button.addEventListener('click', () => fillTariffForm(Number(button.dataset.tariffId)));
    });
}

function tariffThresholdText(normal, minimum) {
    const parts = [];
    if (normal > 0) parts.push(`normal ${normal.toLocaleString('de-DE')}`);
    if (minimum > 0) parts.push(`min. ${minimum.toLocaleString('de-DE')}`);
    return parts.length ? `<small>${parts.join(' · ')}</small>` : '';
}

function fillTariffForm(id) {
    const t = activeTariffs.find(item => item.id === id);
    if (!t) return;
    const provider = tariffCatalog.providers.find(p => p.name.toLowerCase() === t.provider.toLowerCase());
    const providerSelect = document.getElementById('tariff-provider-select');
    providerSelect.value = provider?.id || CUSTOM_VALUE;
    updateProviderFields(false);
    document.getElementById('tariff-provider').value = provider ? '' : t.provider;

    const template = provider?.tariffs.find(plan => plan.name.toLowerCase() === t.name.toLowerCase());
    renderTariffOptions(providerSelect.value, template?.id || CUSTOM_VALUE);
    document.getElementById('tariff-name').value = template ? '' : t.name;

    const values = {
        'tariff-profile': t.profile_id,
        'tariff-technology': t.access_technology || '',
        'tariff-down-max': t.advertised_down_mbps,
        'tariff-down-normal': t.normal_down_mbps || '',
        'tariff-down-min': t.minimum_down_mbps || '',
        'tariff-up-max': t.advertised_up_mbps,
        'tariff-up-normal': t.normal_up_mbps || '',
        'tariff-up-min': t.minimum_up_mbps || '',
    };
    Object.entries(values).forEach(([elementID, value]) => { document.getElementById(elementID).value = value; });
    document.getElementById('tariff-form-status').textContent = 'Vorlage geladen – Speichern erzeugt eine neue Version.';
    document.getElementById('tariff-provider-select').focus();
}

async function saveTariff(event) {
    event.preventDefault();
    const status = document.getElementById('tariff-form-status');
    const provider = selectedProviderName();
    const name = selectedTariffName();
    if (!provider || !name) {
        status.textContent = 'Bitte Anbieter und Tarif auswählen oder frei eingeben.';
        status.className = 'tariff-error';
        return;
    }
    const payload = {
        profile_id: Number(document.getElementById('tariff-profile').value),
        provider,
        name,
        access_technology: document.getElementById('tariff-technology').value,
        advertised_down_mbps: tariffNumber('tariff-down-max'),
        advertised_up_mbps: tariffNumber('tariff-up-max'),
        normal_down_mbps: tariffNumber('tariff-down-normal'),
        normal_up_mbps: tariffNumber('tariff-up-normal'),
        minimum_down_mbps: tariffNumber('tariff-down-min'),
        minimum_up_mbps: tariffNumber('tariff-up-min'),
        valid_from: new Date().toISOString(),
    };
    status.textContent = 'Speichere …';
    status.className = '';
    try {
        await API.post('/api/tariffs', payload);
        status.textContent = 'Tarif gespeichert. Er gilt für neue Messungen.';
        status.className = 'tariff-success';
        await loadTariffs();
    } catch (error) {
        status.textContent = 'Tarif ungültig. Maximum ≥ Normal ≥ Minimum und Upload prüfen.';
        status.className = 'tariff-error';
        console.error('Tariff save failed:', error);
    }
}

async function loadTariffComparison(result) {
    const download = document.getElementById('tariff-download');
    const upload = document.getElementById('tariff-upload');
    if (!download || !upload) return;
    download.classList.add('hidden');
    upload.classList.add('hidden');
    if (!result?.id || !result?.tariff_id) return;
    const response = await fetch(`/api/tariff-comparison?result_id=${result.id}`);
    if (!response.ok) return;
    const comparison = await response.json();
    renderTariffMetric(download, comparison.download, comparison.tariff);
    renderTariffMetric(upload, comparison.upload, comparison.tariff);
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

document.addEventListener('DOMContentLoaded', () => {
    document.getElementById('tariff-form')?.addEventListener('submit', saveTariff);
    document.getElementById('tariff-provider-select')?.addEventListener('change', () => updateProviderFields(true));
    document.getElementById('tariff-template')?.addEventListener('change', applyCatalogTariff);
});
