// settings.js — Absender-Adresse laden/speichern für Mängelmeldung

const SETTINGS_KEYS = ['complaint_name', 'complaint_street', 'complaint_city',
    'complaint_phone', 'complaint_email',
    'complaint_provider_street', 'complaint_provider_city'];

async function loadSettingsAddress() {
    const form = document.getElementById('settings-address-form');
    if (!form) return;
    try {
        const data = await API.get('/api/settings');
        SETTINGS_KEYS.forEach(key => {
            const el = form.elements[key];
            if (el) el.value = data[key] || '';
        });
    } catch (e) {
        console.error('Failed to load settings:', e);
    }
}

async function saveSettingsAddress(e) {
    e.preventDefault();
    const form = e.target;
    const status = document.getElementById('settings-address-status');
    const payload = {};
    SETTINGS_KEYS.forEach(key => {
        const el = form.elements[key];
        if (el) payload[key] = el.value.trim();
    });
    try {
        await API.put('/api/settings', payload);
        if (status) {
            status.textContent = 'Gespeichert.';
            status.className = 'tariff-success';
            setTimeout(() => { status.textContent = ''; }, 2500);
        }
        showToast('Absender-Adresse gespeichert', 'success');
    } catch (err) {
        if (status) {
            status.textContent = 'Fehler: ' + err.message;
            status.className = 'tariff-error';
        }
        showToast('Fehler beim Speichern: ' + err.message, 'error');
    }
}

document.addEventListener('DOMContentLoaded', () => {
    const form = document.getElementById('settings-address-form');
    if (form) {
        form.addEventListener('submit', saveSettingsAddress);
        loadSettingsAddress();
    }
});
