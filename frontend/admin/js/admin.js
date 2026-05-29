/**
 * IF Festival — Admin Application
 * Dashboard, gestion des commandes, scanner QR
 */

const API_BASE = '/api/v1';
let authToken = localStorage.getItem('admin_token');
let adminName = localStorage.getItem('admin_name');
let adminRole = localStorage.getItem('admin_role') || 'admin';
let searchTimeout = null;
let currentPage = 1;
let busOptionsCache = null;
let busTicketsCache = [];
let latestSalesStats = null;
let salesChartRange = '1j';
let salesChart = null;
const expandedOrderIds = new Set();
const orderTicketsCache = new Map();
let salesCustomStart = '';
let salesCustomEnd = '';
let compedTicketTypes = [];
const compedCategoriesCache = new Map();
let compedFormReady = false;

// ==========================================
// Initialisation
// ==========================================

document.addEventListener('DOMContentLoaded', () => {
    if (authToken) {
        showDashboard();
    }

    document.getElementById('login-form').addEventListener('submit', handleLogin);
    const changePwdForm = document.getElementById('change-password-form');
    if (changePwdForm) {
        changePwdForm.addEventListener('submit', handleChangePassword);
    }
    const staffPwdForm = document.getElementById('staff-password-form');
    if (staffPwdForm) {
        staffPwdForm.addEventListener('submit', handleSetStaffPassword);
    }

    // Le scanner QR écoute les entrées clavier (lecteur USB)
    document.getElementById('qr-input').addEventListener('keypress', (e) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            validateQR();
        }
    });
});

// ==========================================
// Auth
// ==========================================

async function handleLogin(e) {
    e.preventDefault();
    const username = document.getElementById('login-username').value;
    const password = document.getElementById('login-password').value;
    const errorEl = document.getElementById('login-error');

    try {
        const response = await fetch(`${API_BASE}/admin/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password }),
        });

        const data = await response.json();

        if (!response.ok) {
            errorEl.textContent = data.error || 'Identifiants invalides';
            errorEl.classList.remove('hidden');
            return;
        }

        authToken = data.token;
        adminName = data.display_name;
        adminRole = data.role || 'admin';
        localStorage.setItem('admin_token', authToken);
        localStorage.setItem('admin_name', adminName);
        localStorage.setItem('admin_role', adminRole);

        showDashboard();
    } catch (error) {
        errorEl.textContent = 'Erreur de connexion au serveur';
        errorEl.classList.remove('hidden');
    }
}

function logout() {
    authToken = null;
    adminName = null;
    adminRole = 'admin';
    localStorage.removeItem('admin_token');
    localStorage.removeItem('admin_name');
    localStorage.removeItem('admin_role');
    document.getElementById('dashboard').classList.add('hidden');
    document.getElementById('login-page').style.display = 'flex';
}

function showDashboard() {
    document.getElementById('login-page').style.display = 'none';
    document.getElementById('dashboard').classList.remove('hidden');
    document.getElementById('admin-name').textContent = adminName;

    // Masquer les onglets selon le rôle
    const isStaff = adminRole === 'staff';
    const isComm = adminRole === 'comm';
    const isSuperAdmin = adminRole === 'super-admin';
    const changePasswordBtn = document.getElementById('change-password-btn');
    if (changePasswordBtn) {
        changePasswordBtn.classList.toggle('hidden', !isSuperAdmin);
    }
    const passwordPanel = document.getElementById('password-panel');
    if (passwordPanel) {
        passwordPanel.classList.add('hidden');
        if (!isSuperAdmin) {
            passwordPanel.style.display = 'none';
        } else {
            passwordPanel.style.display = 'block';
        }
    }
    document.querySelectorAll('.tab').forEach(tab => {
        const tabName = tab.dataset.tab;
        tab.style.display = canAccessTab(tabName) ? '' : 'none';
    });

    if (isStaff) {
        // Staff → directement sur le scanner
        switchTab('scanner');
    } else if (isComm) {
        switchTab('stats');
    } else {
        switchTab('stats');
    }
}

function togglePasswordPanel() {
    if (adminRole !== 'super-admin') return;
    const panel = document.getElementById('password-panel');
    if (!panel) return;
    panel.classList.toggle('hidden');
}

async function handleChangePassword(e) {
    e.preventDefault();

    if (adminRole !== 'super-admin') {
        return;
    }

    const msg = document.getElementById('password-msg');
    const currentPassword = document.getElementById('current-password').value;
    const newPassword = document.getElementById('new-password').value;
    const confirmPassword = document.getElementById('confirm-password').value;

    msg.classList.add('hidden');

    if (!currentPassword || !newPassword) {
        msg.textContent = '❌ Mot de passe actuel et nouveau requis';
        msg.className = 'form-msg error-text';
        return;
    }

    if (newPassword.length < 8) {
        msg.textContent = '❌ Minimum 8 caractères';
        msg.className = 'form-msg error-text';
        return;
    }

    if (newPassword !== confirmPassword) {
        msg.textContent = '❌ La confirmation ne correspond pas';
        msg.className = 'form-msg error-text';
        return;
    }

    try {
        const response = await apiFetch(`${API_BASE}/admin/change-password`, {
            method: 'POST',
            body: JSON.stringify({
                current_password: currentPassword,
                new_password: newPassword,
            }),
        });

        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur lors du changement de mot de passe');
        }

        msg.textContent = '✅ Mot de passe mis à jour';
        msg.className = 'form-msg success-text';
        document.getElementById('change-password-form').reset();
        setTimeout(() => {
            alert('Mot de passe modifié. Vous allez être déconnecté.');
            logout();
        }, 400);
    } catch (error) {
        msg.textContent = `❌ ${error.message}`;
        msg.className = 'form-msg error-text';
    }
}

async function handleSetStaffPassword(e) {
    e.preventDefault();

    if (adminRole !== 'super-admin') {
        return;
    }

    const msg = document.getElementById('staff-password-msg');
    const username = document.getElementById('staff-username').value.trim();
    const newPassword = document.getElementById('staff-new-password').value;
    const confirmPassword = document.getElementById('staff-confirm-password').value;

    msg.classList.add('hidden');

    if (!username || !newPassword) {
        msg.textContent = '❌ Username staff et mot de passe requis';
        msg.className = 'form-msg error-text';
        return;
    }

    if (newPassword.length < 8) {
        msg.textContent = '❌ Minimum 8 caractères';
        msg.className = 'form-msg error-text';
        return;
    }

    if (newPassword !== confirmPassword) {
        msg.textContent = '❌ La confirmation ne correspond pas';
        msg.className = 'form-msg error-text';
        return;
    }

    try {
        const response = await apiFetch(`${API_BASE}/admin/staff/change-password`, {
            method: 'POST',
            body: JSON.stringify({
                username,
                new_password: newPassword,
            }),
        });

        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur lors du changement du mot de passe staff');
        }

        msg.textContent = '✅ Mot de passe staff mis à jour (sessions invalidées)';
        msg.className = 'form-msg success-text';
        document.getElementById('staff-password-form').reset();
        document.getElementById('staff-username').value = username;
    } catch (error) {
        msg.textContent = `❌ ${error.message}`;
        msg.className = 'form-msg error-text';
    }
}

function apiHeaders() {
    return {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${authToken}`,
    };
}

async function apiFetch(url, options = {}) {
    options.headers = { ...apiHeaders(), ...options.headers };
    const response = await fetch(url, options);

    if (response.status === 401) {
        logout();
        throw new Error('Session expirée');
    }

    return response;
}

// ==========================================
// Navigation
// ==========================================

function canAccessTab(tabName) {
    if (adminRole === 'staff') {
        return tabName === 'scanner';
    }
    if (adminRole === 'comm') {
        return tabName === 'stats' || tabName === 'kpi';
    }
    return true;
}

function switchTab(tabName) {
    if (!canAccessTab(tabName)) {
        return;
    }

    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));

    document.querySelector(`[data-tab="${tabName}"]`).classList.add('active');
    document.getElementById(`tab-${tabName}`).classList.add('active');

    switch (tabName) {
        case 'stats': loadStats(); break;
        case 'orders': loadOrders(); break;
        case 'tickets': loadTicketTypesAdmin(); break;
        case 'coupons': loadCoupons(); break;
        case 'bus': loadBusAdminData(); break;
        case 'referral': loadReferralLinks(); break;
        case 'kpi': loadKPI(); break;
        case 'scanner':
            document.getElementById('qr-input').focus();
            loadValidationStats();
            break;
    }
}

async function loadReferralLinks() {
    try {
        const response = await apiFetch(`${API_BASE}/admin/referrals`);
        const rows = await response.json();
        renderReferralLinks(rows || []);
    } catch (error) {
        console.error('Erreur chargement parrainage:', error);
    }
}

async function createReferralLink() {
    const input = document.getElementById('referral-name');
    const customCodeInput = document.getElementById('referral-custom-code');
    const msg = document.getElementById('referral-msg');
    if (!input || !msg) return;

    const name = input.value.trim();
    const customCode = customCodeInput ? customCodeInput.value.trim() : '';
    msg.classList.add('hidden');

    if (!name) {
        msg.textContent = '❌ Nom de lien requis';
        msg.className = 'form-msg error-text';
        return;
    }

    try {
        const body = { name };
        if (customCode) body.custom_code = customCode;
        const response = await apiFetch(`${API_BASE}/admin/referrals`, {
            method: 'POST',
            body: JSON.stringify(body),
        });
        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur création lien parrainage');
        }

        msg.textContent = '✅ Lien de parrainage créé';
        msg.className = 'form-msg success-text';
        input.value = '';
        if (customCodeInput) customCodeInput.value = '';
        await loadReferralLinks();
    } catch (error) {
        msg.textContent = `❌ ${error.message}`;
        msg.className = 'form-msg error-text';
    }
}

function renderReferralLinks(rows) {
    const container = document.getElementById('referral-links-table');
    if (!container) return;

    if (!rows.length) {
        container.innerHTML = '<p style="color:#718096;">Aucun lien de parrainage</p>';
        return;
    }

    let html = `<table>
        <thead><tr>
            <th>Nom</th><th>Lien</th><th>Clics</th><th>Visiteurs uniques</th><th>Commandes converties</th><th>Tickets convertis</th><th>CA converti</th><th>Détail jour</th><th>Action</th>
        </tr></thead><tbody>`;

    rows.forEach(row => {
        const dailyRows = Array.isArray(row.daily_sales_by_day) ? row.daily_sales_by_day : [];
        let dailyHtml = '<span style="color:#a0aec0;">Aucune conversion</span>';
        if (dailyRows.length > 0) {
            dailyHtml = `<table style="font-size:.8rem;min-width:260px;">
                <thead><tr><th>Date</th><th>Clicks</th><th>Tickets</th></tr></thead><tbody>
                ${dailyRows.map(d => `<tr>
                    <td>${formatDate(d.date)}</td>
                    <td>${d.click_count || 0}</td>
                    <td>${d.ticket_count || 0}</td>
                </tr>`).join('')}
                </tbody></table>`;
        }

        html += `<tr>
            <td><strong>${row.name}</strong><br><small>${formatDateTime(row.created_at)}</small></td>
            <td><a href="${row.share_url}" target="_blank" rel="noopener noreferrer">${row.share_url}</a></td>
            <td>${row.click_count}</td>
            <td>${row.unique_visitors}</td>
            <td>${row.converted_orders}</td>
            <td>${row.converted_tickets}</td>
            <td><strong>${formatPrice(row.converted_revenue_cents || 0)}</strong></td>
            <td>${dailyHtml}</td>
            <td><button class="btn btn-sm btn-primary" onclick="copyReferralLink('${escapeAttr(row.share_url)}')">Copier</button></td>
        </tr>`;
    });

    container.innerHTML = html + '</tbody></table>';
}

async function copyReferralLink(url) {
    try {
        await navigator.clipboard.writeText(url);
        alert('Lien copié');
    } catch (_) {
        prompt('Copiez ce lien :', url);
    }
}

// ==========================================
// Statistiques
// ==========================================

async function loadStats() {
    try {
        const [statsResponse, busTicketsResponse] = await Promise.all([
            apiFetch(`${API_BASE}/admin/stats`),
            apiFetch(`${API_BASE}/admin/bus/tickets`),
        ]);
        const stats = await statsResponse.json();
        const busTickets = await busTicketsResponse.json();
        latestSalesStats = stats;

        const testEmailCard = document.getElementById('test-email-card');
        if (testEmailCard) {
            testEmailCard.classList.toggle('hidden', !stats.test_email_enabled);
        }

        // KPIs
        document.getElementById('stat-orders').textContent = stats.total_orders || 0;
        document.getElementById('stat-tickets').textContent = stats.total_tickets_sold || 0;
        document.getElementById('stat-revenue').textContent = formatPrice(stats.total_revenue_cents || 0);
        document.getElementById('stat-validated').textContent = stats.total_validated || 0;
        document.getElementById('stat-camping').textContent = stats.total_camping || 0;
        document.getElementById('stat-refund-insurance').textContent = stats.total_refund_insurance || 0;

        // Stats par type
        renderTypeStats(stats.by_ticket_type || []);

        // Ventes par jour
        setSalesChartRange(salesChartRange);
        renderDailyStats(stats.sales_by_day || []);

        // Commandes récentes
        renderRecentOrders(stats.recent_orders || []);

        renderBusStats(busTickets || []);
    } catch (error) {
        console.error('Erreur chargement stats:', error);
    }
}

function switchStatsView(kind) {
    const festivalPanel = document.getElementById('stats-festival-panel');
    const busPanel = document.getElementById('stats-bus-panel');
    const festivalBtn = document.getElementById('stats-tab-festival');
    const busBtn = document.getElementById('stats-tab-bus');
    if (!festivalPanel || !busPanel || !festivalBtn || !busBtn) return;

    const showBus = kind === 'bus';
    festivalPanel.classList.toggle('hidden', showBus);
    busPanel.classList.toggle('hidden', !showBus);
    festivalBtn.classList.toggle('btn-primary', !showBus);
    busBtn.classList.toggle('btn-primary', showBus);
}

function setSalesChartRange(rangeKey) {
    salesChartRange = rangeKey;

    document.querySelectorAll('#sales-chart-range-tabs [data-range]').forEach(btn => {
        const isActive = btn.getAttribute('data-range') === rangeKey;
        btn.classList.toggle('btn-primary', isActive);
    });

    if (latestSalesStats) {
        if (rangeKey === 'custom') {
            applyCustomSalesRange();
            return;
        }
        renderDailySalesChart();
    }
}

async function applyCustomSalesRange() {
    const start = document.getElementById('sales-custom-start')?.value || '';
    const end = document.getElementById('sales-custom-end')?.value || '';

    if (!start || !end) {
        alert('Sélectionnez une date de début et de fin');
        return;
    }

    salesCustomStart = start;
    salesCustomEnd = end;
    salesChartRange = 'custom';

    document.querySelectorAll('#sales-chart-range-tabs [data-range]').forEach(btn => {
        const isActive = btn.getAttribute('data-range') === 'custom';
        btn.classList.toggle('btn-primary', isActive);
    });

    try {
        const params = new URLSearchParams({ start, end });
        const response = await apiFetch(`${API_BASE}/admin/stats/timeline?${params.toString()}`);
        const payload = await response.json();
        if (!response.ok) {
            throw new Error(payload.error || 'Erreur chargement timeline custom');
        }

        if (!latestSalesStats) {
            latestSalesStats = { sales_timeline: {} };
        }
        if (!latestSalesStats.sales_timeline) {
            latestSalesStats.sales_timeline = {};
        }

        latestSalesStats.sales_timeline.custom = Array.isArray(payload.points) ? payload.points : [];
        renderDailySalesChart();
    } catch (error) {
        alert(`❌ ${error.message}`);
    }
}

function renderBusStats(rows) {
    const tickets = rows.length;
    const validated = rows.filter(r => r.is_validated).length;
    const roundTrip = rows.filter(r => r.is_round_trip).length;
    const revenue = rows.reduce((sum, r) => sum + (r.order_total_cents || 0), 0);

    const ticketsEl = document.getElementById('bus-stat-tickets');
    const validatedEl = document.getElementById('bus-stat-validated');
    const revenueEl = document.getElementById('bus-stat-revenue');
    const roundTripEl = document.getElementById('bus-stat-roundtrip');

    if (ticketsEl) ticketsEl.textContent = tickets;
    if (validatedEl) validatedEl.textContent = validated;
    if (revenueEl) revenueEl.textContent = formatPrice(revenue);
    if (roundTripEl) roundTripEl.textContent = roundTrip;

    renderBusTicketsTable(rows, 'stats-bus-tickets');
}

async function exportCSV(url, buttonId, msgId, fallbackName) {
    const button = document.getElementById(buttonId);
    const msg = document.getElementById(msgId);
    if (!button || !msg) return;

    button.disabled = true;
    const initialLabel = button.textContent;
    button.textContent = 'Export en cours...';
    msg.classList.add('hidden');

    try {
        const response = await fetch(url, {
            method: 'GET',
            headers: apiHeaders(),
        });

        if (response.status === 401) {
            logout();
            throw new Error('Session expirée');
        }

        if (!response.ok) {
            let errMsg = 'Erreur export CSV';
            try {
                const data = await response.json();
                errMsg = data.error || errMsg;
            } catch (_) {}
            throw new Error(errMsg);
        }

        const blob = await response.blob();
        const urlBlob = window.URL.createObjectURL(blob);
        const link = document.createElement('a');
        const disposition = response.headers.get('Content-Disposition') || '';
        const match = disposition.match(/filename="?([^";]+)"?/i);
        const filename = (match && match[1]) ? match[1] : fallbackName;

        link.href = urlBlob;
        link.download = filename;
        document.body.appendChild(link);
        link.click();
        link.remove();
        window.URL.revokeObjectURL(urlBlob);

        msg.textContent = '✅ Export téléchargé';
        msg.className = 'form-msg success-text';
    } catch (error) {
        msg.textContent = `❌ ${error.message}`;
        msg.className = 'form-msg error-text';
    } finally {
        button.disabled = false;
        button.textContent = initialLabel;
    }
}

async function exportFestivalTicketsCSV() {
    return exportCSV(`${API_BASE}/admin/stats/export-festival-tickets`, 'btn-export-festival-tickets', 'export-csv-msg', 'festival_tickets.csv');
}

async function exportBusTicketsCSV() {
    return exportCSV(`${API_BASE}/admin/stats/export-bus-tickets`, 'btn-export-bus-tickets', 'export-csv-msg', 'bus_tickets.csv');
}

async function exportOrdersCSV() {
    return exportCSV(`${API_BASE}/admin/stats/export-orders`, 'btn-export-orders', 'export-csv-msg', 'orders.csv');
}

async function sendTestEmail() {
    const input = document.getElementById('test-email-to');
    const button = document.getElementById('btn-send-test-email');
    const msg = document.getElementById('send-test-email-msg');
    if (!input || !button || !msg) return;

    const to = input.value.trim();
    msg.classList.add('hidden');

    if (!to) {
        msg.textContent = '❌ Email destinataire requis';
        msg.className = 'form-msg error-text';
        return;
    }

    button.disabled = true;
    const initialLabel = button.textContent;
    button.textContent = 'Envoi...';

    try {
        const response = await apiFetch(`${API_BASE}/admin/test-email`, {
            method: 'POST',
            body: JSON.stringify({ to }),
        });

        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur envoi email de test');
        }

        msg.textContent = '✅ Email de test envoyé (voir logs backend)';
        msg.className = 'form-msg success-text';
    } catch (error) {
        msg.textContent = `❌ ${error.message}`;
        msg.className = 'form-msg error-text';
    } finally {
        button.disabled = false;
        button.textContent = initialLabel;
    }
}

function renderTypeStats(types) {
    const container = document.getElementById('stats-by-type');
    if (types.length === 0) {
        container.innerHTML = '<p style="color:#718096;">Aucune donnée</p>';
        return;
    }

    let html = `<table>
        <thead><tr>
            <th>Type</th><th>Prix</th><th>Vendus</th><th>Total</th><th>Validés</th><th>CA</th><th>Remplissage</th>
        </tr></thead><tbody>`;

    types.forEach(t => {
        const pct = t.quantity_total > 0 ? Math.round((t.quantity_sold / t.quantity_total) * 100) : 0;
        html += `<tr>
            <td><strong>${t.name}</strong></td>
            <td>${formatPrice(t.price_cents)}</td>
            <td>${t.quantity_sold}</td>
            <td>${t.quantity_total}</td>
            <td>${t.quantity_validated}</td>
            <td><strong>${formatPrice(t.revenue_cents)}</strong></td>
            <td>
                <div style="display:flex;align-items:center;gap:8px;">
                    <div class="progress-bar" style="width:80px;">
                        <div class="progress-bar-fill" style="width:${pct}%"></div>
                    </div>
                    <span>${pct}%</span>
                </div>
            </td>
        </tr>`;
    });

    container.innerHTML = html + '</tbody></table>';
}

function renderDailyStats(days) {
    const container = document.getElementById('stats-by-day');
    if (!container) return;
    if (days.length === 0) {
        container.innerHTML = '<p style="color:#718096;">Aucune vente</p>';
        return;
    }

    let html = `<table>
        <thead><tr>
            <th>Date</th><th>Commandes</th><th>Tickets</th><th>CA</th>
        </tr></thead><tbody>`;

    days.forEach(d => {
        html += `<tr>
            <td>${formatDate(d.date)}</td>
            <td>${d.order_count}</td>
            <td>${d.ticket_count}</td>
            <td><strong>${formatPrice(d.revenue_cents)}</strong></td>
        </tr>`;
    });

    container.innerHTML = html + '</tbody></table>';
}

function renderDailySalesChart() {
    const container = document.getElementById('sales-by-day-chart');
    if (!container) return;

    if (typeof Chart === 'undefined') {
        container.innerHTML = '<p style="color:#e53e3e;">Chart.js non chargé</p>';
        return;
    }

    const timeline = latestSalesStats?.sales_timeline || {};
    const points = Array.isArray(timeline[salesChartRange]) ? timeline[salesChartRange] : [];

    if (!points.length) {
        if (salesChart) {
            salesChart.destroy();
            salesChart = null;
        }
        container.innerHTML = '<p style="color:#718096;">Aucune donnée pour ce créneau</p>';
        return;
    }

    const ordered = [...points].sort((a, b) => new Date(a.bucket) - new Date(b.bucket));
    const labels = ordered.map(point => formatBucketLabel(point.bucket, salesChartRange));
    const revenueData = ordered.map(point => point.revenue_cents || 0);
    const ticketData = ordered.map(point => point.ticket_count || 0);
    const rawDates = ordered.map(point => point.bucket);

    container.innerHTML = `
        <div class="daily-chart-head">
            <span>Plage: <strong>${salesChartRange}</strong></span>
            <span>${ordered.length} points</span>
        </div>
        <div class="daily-chart-main chartjs-main">
            <div class="daily-line-chart-wrap chartjs-wrap">
                <canvas id="sales-chart-canvas" aria-label="Ventes par période"></canvas>
            </div>
        </div>
    `;

    const canvas = document.getElementById('sales-chart-canvas');
    if (!canvas) return;

    if (salesChart) {
        salesChart.destroy();
        salesChart = null;
    }

    salesChart = new Chart(canvas, {
        type: 'line',
        data: {
            labels,
            datasets: [
                {
                    label: 'CA',
                    data: revenueData,
                    borderColor: '#667eea',
                    backgroundColor: 'rgba(102,126,234,0.12)',
                    pointBackgroundColor: '#667eea',
                    pointRadius: 3,
                    tension: 0.3,
                    yAxisID: 'yRevenue',
                },
                {
                    label: 'Tickets',
                    data: ticketData,
                    borderColor: '#ed8936',
                    backgroundColor: 'rgba(237,137,54,0.12)',
                    pointBackgroundColor: '#ed8936',
                    pointRadius: 3,
                    tension: 0.3,
                    yAxisID: 'yTickets',
                },
            ],
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: {
                mode: 'index',
                intersect: false,
            },
            plugins: {
                legend: {
                    position: 'right',
                    labels: {
                        usePointStyle: true,
                    },
                },
                tooltip: {
                    callbacks: {
                        title: items => {
                            const idx = items?.[0]?.dataIndex ?? 0;
                            return formatTooltipDate(rawDates[idx], salesChartRange);
                        },
                        label: ctx => {
                            if (ctx.dataset.label === 'CA') {
                                return `CA: ${formatPrice(ctx.parsed.y || 0)}`;
                            }
                            return `Tickets: ${ctx.parsed.y || 0}`;
                        },
                    },
                },
            },
            scales: {
                x: {
                    ticks: {
                        maxRotation: 0,
                        autoSkip: true,
                        maxTicksLimit: 8,
                    },
                    grid: {
                        display: false,
                    },
                },
                yRevenue: {
                    type: 'linear',
                    position: 'left',
                    ticks: {
                        callback: value => formatPrice(Number(value) || 0),
                    },
                },
                yTickets: {
                    type: 'linear',
                    position: 'right',
                    grid: {
                        drawOnChartArea: false,
                    },
                    ticks: {
                        precision: 0,
                    },
                },
            },
        },
    });
}

function formatBucketLabel(bucket, rangeKey) {
    const date = new Date(bucket);
    if (Number.isNaN(date.getTime())) return '';

    if (rangeKey === '1h' || rangeKey === '1j' || rangeKey === 'custom') {
        return date.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' });
    }

    return date.toLocaleDateString('fr-FR', { day: '2-digit', month: '2-digit' });
}

function formatTooltipDate(bucket, rangeKey) {
    const date = new Date(bucket);
    if (Number.isNaN(date.getTime())) return '';

    if (rangeKey === '1h' || rangeKey === '1j' || rangeKey === 'custom') {
        return date.toLocaleString('fr-FR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
    }

    return date.toLocaleDateString('fr-FR', { day: '2-digit', month: '2-digit', year: 'numeric' });
}

function renderRecentOrders(orders) {
    const container = document.getElementById('recent-orders');
    if (orders.length === 0) {
        container.innerHTML = '<p style="color:#718096;">Aucune commande</p>';
        return;
    }
    container.innerHTML = renderOrdersTable(orders);
}

// ==========================================
// Commandes
// ==========================================

async function loadOrders() {
    await initCompedOrderForm();
    const search = document.getElementById('order-search').value;
    const status = document.getElementById('order-status-filter').value;

    try {
        const params = new URLSearchParams({
            page: currentPage,
            page_size: 20,
        });
        if (search) params.set('search', search);
        if (status) params.set('status', status);

        const response = await apiFetch(`${API_BASE}/admin/orders?${params}`);
        const data = await response.json();

        document.getElementById('orders-table').innerHTML = renderOrdersTable(data.orders || [], { withDetails: true });
        renderPagination(data.total_count, data.page, data.page_size);

        expandedOrderIds.forEach((orderID) => {
            loadOrderTickets(orderID);
        });
    } catch (error) {
        console.error('Erreur chargement commandes:', error);
    }
}

function renderOrdersTable(orders, options = {}) {
    const withDetails = !!options.withDetails;

    if (orders.length === 0) return '<p style="color:#718096;padding:20px;">Aucune commande</p>';

    let html = `<table>
        <thead><tr>
            <th>N°</th><th>Client</th><th>Email</th><th>Camping</th><th>Assurance</th><th>Total</th><th>Statut</th><th>Date</th>${withDetails ? '<th style="text-align:right;">Action</th>' : ''}
        </tr></thead><tbody>`;

    orders.forEach(o => {
        const canEdit = o.status === 'paid' || o.status === 'confirmed';
        const isExpanded = withDetails && expandedOrderIds.has(o.id);

        html += `<tr>
            <td><strong>${o.order_number}</strong></td>
            <td>${o.customer_first_name} ${o.customer_last_name}</td>
            <td>${o.customer_email}</td>
            <td>${o.wants_camping ? 'Oui' : 'Non'}</td>
            <td>${o.wants_refund_insurance ? 'Oui' : 'Non'}</td>
            <td>${formatPrice(o.total_cents)}</td>
            <td><span class="badge badge-${o.status}">${statusLabel(o.status)}</span></td>
            <td>${formatDateTime(o.created_at)}</td>
            ${withDetails ? `<td style="text-align:right;"><button class="btn btn-sm btn-primary" onclick="toggleOrderDetails('${o.id}')">${isExpanded ? 'Masquer' : 'Détails'}</button></td>` : ''}
        </tr>`;

        if (withDetails && isExpanded) {
            html += `<tr class="order-details-row">
                <td colspan="9">
                    <div class="order-details-panel">
                        ${canEdit ? `
                            <div class="form-row">
                                <div class="form-group"><label>Prénom</label><input type="text" id="order-first-name-${o.id}" value="${escapeAttr(o.customer_first_name || '')}"></div>
                                <div class="form-group"><label>Nom</label><input type="text" id="order-last-name-${o.id}" value="${escapeAttr(o.customer_last_name || '')}"></div>
                            </div>
                            <div class="form-group"><label>Email</label><input type="email" id="order-email-${o.id}" value="${escapeAttr(o.customer_email || '')}"></div>
                            <div class="form-row">
                                <label class="order-checkbox-row"><input type="checkbox" id="order-camping-${o.id}" ${o.wants_camping ? 'checked' : ''}> Camping</label>
                                <label class="order-checkbox-row"><input type="checkbox" id="order-insurance-${o.id}" ${o.wants_refund_insurance ? 'checked' : ''}> Assurance</label>
                            </div>
                            <div class="order-details-actions">
                                <button class="btn btn-sm btn-primary" onclick="saveOrderDetails('${o.id}')">Confirmer</button>
                                <button class="btn btn-sm" onclick="resendOrderEmailFromDetails('${o.id}')">Renvoyer</button>
                                <button class="btn btn-sm btn-danger" onclick="refundOrderTotalFromDetails('${o.id}', '${escapeAttr(o.order_number || '')}')">Rembourser</button>
                                <button class="btn btn-sm btn-warning" onclick="removeOrderLocalFromDetails('${o.id}', '${escapeAttr(o.order_number || '')}')">Supprimer local</button>
                            </div>
                            <div id="order-tickets-${o.id}" style="margin-top:16px;">
                                <p style="margin:0;color:#718096;">Chargement des tickets...</p>
                            </div>
                        ` : `
                            <p style="margin:0;color:#718096;">Cette commande n'est pas modifiable (statut: ${statusLabel(o.status)}).</p>
                        `}
                    </div>
                </td>
            </tr>`;
        }
    });

    return html + '</tbody></table>';
}

function toggleOrderDetails(orderID) {
    if (expandedOrderIds.has(orderID)) {
        expandedOrderIds.delete(orderID);
    } else {
        expandedOrderIds.add(orderID);
    }
    loadOrders();
}

async function saveOrderDetails(orderID) {
    const body = {
        customer_first_name: (document.getElementById(`order-first-name-${orderID}`)?.value || '').trim(),
        customer_last_name: (document.getElementById(`order-last-name-${orderID}`)?.value || '').trim(),
        customer_email: (document.getElementById(`order-email-${orderID}`)?.value || '').trim(),
        wants_camping: !!document.getElementById(`order-camping-${orderID}`)?.checked,
        wants_refund_insurance: !!document.getElementById(`order-insurance-${orderID}`)?.checked,
    };

    if (!body.customer_first_name || !body.customer_last_name || !body.customer_email) {
        alert('Prénom, nom et email sont requis');
        return;
    }

    try {
        const response = await apiFetch(`${API_BASE}/admin/orders/${orderID}`, {
            method: 'PUT',
            body: JSON.stringify(body),
        });
        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur lors de la mise à jour');
        }
        await loadOrders();
        alert('✅ Commande mise à jour');
    } catch (error) {
        alert(`❌ ${error.message}`);
    }
}

async function resendOrderEmailFromDetails(orderID) {
    try {
        const response = await apiFetch(`${API_BASE}/admin/orders/${orderID}/resend-email`, {
            method: 'POST',
        });
        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur lors du renvoi');
        }
        alert('✅ Email de confirmation renvoyé');
    } catch (error) {
        alert(`❌ ${error.message}`);
    }
}

async function refundOrderTotalFromDetails(orderID, orderNumber) {
    const label = orderNumber ? ` (${orderNumber})` : '';
    if (!confirm(`Confirmer le remboursement total de cette commande${label} ?\nLe remboursement gardera 1€ par ticket.\nCette action mettra le statut à "Remboursé" et les tickets deviendront invalides au scan.`)) {
        return;
    }

    try {
        const response = await apiFetch(`${API_BASE}/admin/orders/${orderID}/refund-total`, {
            method: 'POST',
        });
        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur lors du remboursement');
        }
        await loadOrders();
        alert('✅ Commande remboursée et statut mis à jour');
    } catch (error) {
        alert(`❌ ${error.message}`);
    }
}

async function loadOrderTickets(orderID) {
    const container = document.getElementById(`order-tickets-${orderID}`);
    if (!container) return;

    if (orderTicketsCache.has(orderID)) {
        container.innerHTML = orderTicketsCache.get(orderID);
        return;
    }

    try {
        const response = await apiFetch(`${API_BASE}/admin/orders/${orderID}/tickets`);
        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur chargement tickets');
        }

        if (!Array.isArray(data) || data.length === 0) {
            container.innerHTML = '<p style="margin:0;color:#718096;">Aucun ticket pour cette commande.</p>';
            orderTicketsCache.set(orderID, container.innerHTML);
            return;
        }

        const rows = data.map((t) => {
            const attendee = [t.attendee_first_name, t.attendee_last_name].filter(Boolean).join(' ').trim();
            const attendeeLabel = attendee || 'Participant';
            const statusBadge = t.is_refunded
                ? '<span class="badge badge-refunded">Remboursé</span>'
                : (t.is_validated ? '<span class="badge badge-confirmed">Validé</span>' : '<span class="badge badge-paid">Actif</span>');
            const disableRefund = t.is_refunded || t.is_validated || t.is_bus;
            const btnLabel = t.is_bus ? 'Navette' : 'Rembourser ce ticket';
            const btnAttrs = disableRefund ? 'disabled' : '';
            const categoryLabel = t.category_name ? ` · ${escapeHtml(t.category_name)}` : '';

            return `
                <div style="display:flex;align-items:center;justify-content:space-between;gap:10px;padding:8px 0;border-top:1px solid #edf2f7;">
                    <div style="flex:1;min-width:0;">
                        <div style="font-weight:700;">${escapeHtml(t.ticket_type_name || '')}${categoryLabel}</div>
                        <div style="font-size:.82rem;color:#718096;">
                            ${escapeHtml(attendeeLabel)}${t.attendee_email ? ` · ${escapeHtml(t.attendee_email)}` : ''}
                        </div>
                        <div style="font-size:.74rem;color:#a0aec0;">QR: ${escapeHtml(t.qr_token || '')}</div>
                    </div>
                    <div style="display:flex;align-items:center;gap:8px;">
                        ${statusBadge}
                        <button class="btn btn-sm btn-danger" ${btnAttrs} onclick="refundSingleTicket('${orderID}', '${t.ticket_id}', '${escapeAttr(t.ticket_type_name || '')}', ${t.price_cents || 0})">${btnLabel}</button>
                    </div>
                </div>
            `;
        }).join('');

        const html = `<div style="border:1px solid #e2e8f0;border-radius:10px;padding:10px 14px;background:#f9fafb;">
            <div style="font-size:.78rem;color:#718096;margin-bottom:6px;font-weight:700;letter-spacing:.3px;text-transform:uppercase;">Tickets</div>
            ${rows}
        </div>`;

        container.innerHTML = html;
        orderTicketsCache.set(orderID, html);
    } catch (error) {
        container.innerHTML = `<p style="margin:0;color:#e53e3e;">❌ ${escapeHtml(error.message)}</p>`;
    }
}

async function refundSingleTicket(orderID, ticketID, ticketTypeName, priceCents) {
    const label = ticketTypeName ? ` (${ticketTypeName})` : '';
    const priceEuros = (priceCents || 0) / 100;
    const refundEuros = Math.max(0, (priceCents || 0) - 100) / 100;
    const msg = `Confirmer le remboursement de ce ticket${label} ?\nLe remboursement gardera 1€ pour ce ticket.` +
        (priceCents ? `\nPrix: ${priceEuros.toFixed(2)} € → Remboursé: ${refundEuros.toFixed(2)} €` : '');

    if (!confirm(msg)) {
        return;
    }

    try {
        const response = await apiFetch(`${API_BASE}/admin/orders/${orderID}/tickets/${ticketID}/refund`, {
            method: 'POST',
        });
        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur lors du remboursement du ticket');
        }
        orderTicketsCache.delete(orderID);
        await loadOrders();
        alert('✅ Ticket remboursé');
    } catch (error) {
        alert(`❌ ${error.message}`);
    }
}

async function removeOrderLocalFromDetails(orderID, orderNumber) {
    const label = orderNumber ? ` (${orderNumber})` : '';
    if (!confirm(`Confirmer la suppression locale de cette commande${label} ?\nLa personne ne sera PAS remboursée. Les tickets deviendront invalides au scan.`)) {
        return;
    }

    try {
        const response = await apiFetch(`${API_BASE}/admin/orders/${orderID}/remove-local`, {
            method: 'POST',
        });
        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur lors de la suppression locale');
        }
        await loadOrders();
        alert('✅ Commande supprimée localement');
    } catch (error) {
        alert(`❌ ${error.message}`);
    }
}

function renderPagination(total, page, pageSize) {
    const totalPages = Math.ceil(total / pageSize);
    const container = document.getElementById('orders-pagination');

    if (totalPages <= 1) {
        container.innerHTML = '';
        return;
    }

    let html = '';
    for (let i = 1; i <= totalPages && i <= 10; i++) {
        html += `<button class="${i === page ? 'active' : ''}" onclick="goToPage(${i})">${i}</button>`;
    }
    container.innerHTML = html;
}

function goToPage(page) {
    currentPage = page;
    loadOrders();
}

function debounceSearch() {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(() => {
        currentPage = 1;
        loadOrders();
    }, 300);
}

async function initCompedOrderForm() {
    if (compedFormReady) return;
    const select = document.getElementById('comped-ticket-type');
    const categorySelect = document.getElementById('comped-ticket-category');
    const msg = document.getElementById('comped-order-msg');
    if (!select || !categorySelect) return;

    compedFormReady = true;
    select.disabled = true;
    select.innerHTML = '<option value="">Chargement...</option>';
    categorySelect.innerHTML = '<option value="">Aucune</option>';
    categorySelect.disabled = true;

    try {
        const response = await apiFetch(`${API_BASE}/admin/ticket-types`);
        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur chargement ticket types');
        }

        compedTicketTypes = (data || []).slice().sort((a, b) => (a.name || '').localeCompare(b.name || ''));
        if (compedTicketTypes.length === 0) {
            select.innerHTML = '<option value="">Aucun type disponible</option>';
            return;
        }

        const options = compedTicketTypes.map((tt) => {
            const masked = tt.is_masked ? ' (masque)' : '';
            return `<option value="${escapeAttr(tt.id)}">${escapeHtml(tt.name || '')}${masked}</option>`;
        });
        select.innerHTML = '<option value="">Selectionner...</option>' + options.join('');
        select.disabled = false;
        handleCompedTicketTypeChange();
    } catch (error) {
        select.innerHTML = '<option value="">Erreur chargement</option>';
        if (msg) {
            msg.textContent = `❌ ${error.message}`;
            msg.className = 'form-msg error-text';
            msg.classList.remove('hidden');
        }
    }
}

function handleCompedTicketTypeChange() {
    const select = document.getElementById('comped-ticket-type');
    const categorySelect = document.getElementById('comped-ticket-category');
    if (!select || !categorySelect) return;

    const ticketTypeID = select.value;
    if (!ticketTypeID) {
        categorySelect.innerHTML = '<option value="">Aucune</option>';
        categorySelect.disabled = true;
        return;
    }

    loadCompedCategories(ticketTypeID);
}

async function loadCompedCategories(ticketTypeID) {
    const categorySelect = document.getElementById('comped-ticket-category');
    if (!categorySelect) return;

    if (compedCategoriesCache.has(ticketTypeID)) {
        const cached = compedCategoriesCache.get(ticketTypeID);
        applyCompedCategoryOptions(cached);
        return;
    }

    categorySelect.disabled = true;
    categorySelect.innerHTML = '<option value="">Chargement...</option>';

    try {
        const response = await apiFetch(`${API_BASE}/admin/ticket-types/${ticketTypeID}/categories`);
        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur chargement categories');
        }

        const categories = Array.isArray(data) ? data : [];
        compedCategoriesCache.set(ticketTypeID, categories);
        applyCompedCategoryOptions(categories);
    } catch (error) {
        categorySelect.innerHTML = '<option value="">Erreur chargement</option>';
        categorySelect.disabled = true;
    }
}

function applyCompedCategoryOptions(categories) {
    const categorySelect = document.getElementById('comped-ticket-category');
    if (!categorySelect) return;

    if (!categories || categories.length === 0) {
        categorySelect.innerHTML = '<option value="">Aucune</option>';
        categorySelect.disabled = true;
        return;
    }

    const options = categories.map((cat) => {
        const masked = cat.is_masked ? ' (masque)' : '';
        return `<option value="${escapeAttr(cat.id)}">${escapeHtml(cat.name || '')}${masked}</option>`;
    });
    categorySelect.innerHTML = '<option value="">Aucune</option>' + options.join('');
    categorySelect.disabled = false;
}

async function createCompedOrder() {
    const msg = document.getElementById('comped-order-msg');
    const ticketTypeID = document.getElementById('comped-ticket-type')?.value || '';
    const categoryID = document.getElementById('comped-ticket-category')?.value || '';
    const quantityRaw = document.getElementById('comped-quantity')?.value || '1';
    const firstName = (document.getElementById('comped-first-name')?.value || '').trim();
    const lastName = (document.getElementById('comped-last-name')?.value || '').trim();
    const email = (document.getElementById('comped-email')?.value || '').trim();

    if (msg) {
        msg.classList.add('hidden');
    }

    const quantity = Math.max(1, parseInt(quantityRaw, 10) || 1);
    if (!ticketTypeID || !firstName || !lastName || !email) {
        if (msg) {
            msg.textContent = '❌ Type de ticket, prenom, nom et email requis';
            msg.className = 'form-msg error-text';
            msg.classList.remove('hidden');
        }
        return;
    }

    if (!confirm('Confirmer la creation de cette commande gratuite ? Un email sera envoye immediatement.')) {
        return;
    }

    try {
        const response = await apiFetch(`${API_BASE}/admin/orders/comped`, {
            method: 'POST',
            body: JSON.stringify({
                ticket_type_id: ticketTypeID,
                category_id: categoryID,
                quantity,
                email,
                first_name: firstName,
                last_name: lastName,
            }),
        });
        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur creation commande');
        }

        if (msg) {
            const orderNumber = data.order_number ? ` (${data.order_number})` : '';
            msg.textContent = `✅ Commande gratuite creee${orderNumber}`;
            msg.className = 'form-msg success-text';
            msg.classList.remove('hidden');
        }

        document.getElementById('comped-quantity').value = '1';
        document.getElementById('comped-first-name').value = '';
        document.getElementById('comped-last-name').value = '';
        document.getElementById('comped-email').value = '';

        orderTicketsCache.clear();
        currentPage = 1;
        await loadOrders();
    } catch (error) {
        if (msg) {
            msg.textContent = `❌ ${error.message}`;
            msg.className = 'form-msg error-text';
            msg.classList.remove('hidden');
        }
    }
}

// ==========================================
// Scanner QR
// ==========================================

async function validateQR() {
    const input = document.getElementById('qr-input');
    const qrToken = input.value.trim();
    const resultEl = document.getElementById('qr-result');

    if (!qrToken) return;

    try {
        const response = await apiFetch(`${API_BASE}/admin/validate-qr`, {
            method: 'POST',
            body: JSON.stringify({ qr_token: qrToken }),
        });

        const data = await response.json();

        resultEl.classList.remove('hidden', 'valid', 'invalid', 'warning');

        if (data.valid) {
            const busDetails = data.ride_type
                ? `<br>Destination : ${data.to_station || '-'}<br>Horaire : ${data.departure_at ? formatDateTime(data.departure_at) : '-'}${data.return_departure_at ? `<br>Horaire retour : ${formatDateTime(data.return_departure_at)}` : ''}`
                : '';
            const campingDetails = `<br>Camping : ${data.is_camping ? 'Oui' : 'Non'}`;
            resultEl.classList.add('valid');
            resultEl.innerHTML = `
                <div class="result-icon">✅</div>
                <strong>${data.message}</strong>
                <div class="result-details">
                    ${data.attendee_first_name} ${data.attendee_last_name}<br>
                    Ticket : ${data.ticket_type_name}<br>
                    Commande : ${data.order_number}
                    ${campingDetails}
                    ${busDetails}
                </div>`;
            // Son de validation (optionnel)
            playSound('success');
        } else if (data.already_validated) {
            const busDetails = data.ride_type
                ? `<br>Destination : ${data.to_station || '-'}<br>Horaire : ${data.departure_at ? formatDateTime(data.departure_at) : '-'}${data.return_departure_at ? `<br>Horaire retour : ${formatDateTime(data.return_departure_at)}` : ''}`
                : '';
            const campingDetails = `<br>Camping : ${data.is_camping ? 'Oui' : 'Non'}`;
            resultEl.classList.add('warning');
            resultEl.innerHTML = `
                <div class="result-icon">⚠️</div>
                <strong>${data.message}</strong>
                <div class="result-details">
                    Ticket : ${data.ticket_type_name}<br>
                    Commande : ${data.order_number}
                    ${campingDetails}
                    ${busDetails}
                </div>`;
            playSound('warning');
        } else {
            resultEl.classList.add('invalid');
            resultEl.innerHTML = `
                <div class="result-icon">❌</div>
                <strong>${data.message}</strong>`;
            playSound('error');
        }

        // Actualiser les stats
        loadValidationStats();
    } catch (error) {
        resultEl.classList.remove('hidden', 'valid', 'warning');
        resultEl.classList.add('invalid');
        resultEl.innerHTML = `
            <div class="result-icon">❌</div>
            <strong>Erreur de validation</strong>`;
    }

    // Reset input pour le prochain scan
    input.value = '';
    input.focus();
}

async function loadValidationStats() {
    try {
        const response = await apiFetch(`${API_BASE}/admin/stats`);
        const stats = await response.json();

        document.getElementById('validated-count').textContent = stats.total_validated || 0;
        document.getElementById('remaining-count').textContent =
            (stats.total_tickets_sold || 0) - (stats.total_validated || 0);
    } catch (e) {
        // Silencieux
    }
}

// ==========================================
// Gestion Tickets & Catégories
// ==========================================

let allTicketTypes = []; // cache for reallocation dropdowns

document.addEventListener('DOMContentLoaded', () => {
    const ttForm = document.getElementById('create-tt-form');
    if (ttForm) ttForm.addEventListener('submit', handleCreateTicketType);

    const couponForm = document.getElementById('create-coupon-form');
    if (couponForm) couponForm.addEventListener('submit', handleCreateCoupon);
});

async function handleCreateTicketType(e) {
    e.preventDefault();
    const msg = document.getElementById('tt-msg');
    msg.classList.add('hidden');

    const domainsRaw = document.getElementById('tt-domains').value.trim();
    const allowed = domainsRaw ? domainsRaw.split(',').map(d => d.trim().toLowerCase()).filter(Boolean) : [];

    const body = {
        name: document.getElementById('tt-name').value.trim(),
        description: document.getElementById('tt-desc').value.trim(),
        price_cents: Math.round(parseFloat(document.getElementById('tt-price').value) * 100),
        quantity_total: parseInt(document.getElementById('tt-qty').value, 10),
        one_ticket_per_email: !!document.getElementById('tt-one-per-email')?.checked,
        sale_start: new Date(`${document.getElementById('tt-start-date').value}T${document.getElementById('tt-start-time').value}:00`).toISOString(),
        sale_end: new Date(`${document.getElementById('tt-end-date').value}T${document.getElementById('tt-end-time').value}:00`).toISOString(),
        allowed_domains: allowed,
    };
    body.max_per_order = body.one_ticket_per_email ? 1 : 10;

    try {
        const res = await apiFetch(`${API_BASE}/admin/ticket-types`, { method: 'POST', body: JSON.stringify(body) });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        const created = await res.json();
        msg.textContent = `✅ "${created.name}" créé !`;
        msg.className = 'form-msg success-text';
        document.getElementById('create-tt-form').reset();
        loadTicketTypesAdmin();
    } catch (err) {
        msg.textContent = `❌ ${err.message}`;
        msg.className = 'form-msg error-text';
    }
}

async function loadTicketTypesAdmin() {
    try {
        const res = await apiFetch(`${API_BASE}/admin/ticket-types`);
        const types = await res.json();
        allTicketTypes = types || [];
        await renderTicketTypesAdmin(allTicketTypes);
    } catch (err) {
        console.error('Erreur chargement ticket types:', err);
    }
}

// ==========================================
// Coupons
// ==========================================

async function loadCoupons() {
    await loadCouponTicketTypes();
    try {
        const response = await apiFetch(`${API_BASE}/admin/coupons`);
        const rows = await response.json();
        renderCoupons(rows || []);
    } catch (error) {
        console.error('Erreur chargement coupons:', error);
    }
}

async function loadCouponTicketTypes() {
    try {
        const res = await apiFetch(`${API_BASE}/admin/ticket-types`);
        const types = await res.json();
        const select = document.getElementById('coupon-ticket-type');
        if (!select) return;
        const options = (types || []).map(tt => `<option value="${tt.id}">${escapeHtml(tt.name)}</option>`);
        select.innerHTML = options.join('') || '<option value="">Aucun ticket</option>';
    } catch (error) {
        console.error('Erreur chargement ticket types pour coupons:', error);
    }
}

async function handleCreateCoupon(e) {
    e.preventDefault();
    const msg = document.getElementById('coupon-msg');
    if (!msg) return;
    msg.classList.add('hidden');

    const name = document.getElementById('coupon-name').value.trim();
    const code = document.getElementById('coupon-code').value.trim();
    const maxUses = parseInt(document.getElementById('coupon-max-uses').value, 10);
    const discount = parseFloat(document.getElementById('coupon-discount').value);
    const ticketTypeID = document.getElementById('coupon-ticket-type').value;

    if (!name || !ticketTypeID || !maxUses || isNaN(discount)) {
        msg.textContent = '❌ Champs requis manquants';
        msg.className = 'form-msg error-text';
        return;
    }

    const body = {
        name,
        code,
        ticket_type_id: ticketTypeID,
        max_uses: maxUses,
        discount_cents: Math.round(discount * 100),
    };

    try {
        const response = await apiFetch(`${API_BASE}/admin/coupons`, {
            method: 'POST',
            body: JSON.stringify(body),
        });
        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur création coupon');
        }

        msg.textContent = `✅ Coupon créé (${data.code})`;
        msg.className = 'form-msg success-text';
        document.getElementById('create-coupon-form').reset();
        await loadCoupons();
    } catch (error) {
        msg.textContent = `❌ ${error.message}`;
        msg.className = 'form-msg error-text';
    }
}

function renderCoupons(rows) {
    const container = document.getElementById('coupons-table');
    if (!container) return;

    if (!rows.length) {
        container.innerHTML = '<p style="color:#718096;padding:20px;">Aucun coupon</p>';
        return;
    }

    let html = '<table><thead><tr><th>Nom</th><th>Ticket</th><th>Code</th><th>Alloués</th><th>Utilisés</th><th>Action</th></tr></thead><tbody>';
    rows.forEach(c => {
        const disabled = c.is_active === false;
        const actionBtn = disabled
            ? '<span style="color:#a0aec0;">Désactivé</span>'
            : `<button class="btn btn-sm btn-danger" onclick="disableCoupon('${c.id}', '${escapeAttr(c.code || '')}')">Désactiver</button>`;
        html += `<tr>
            <td>${escapeHtml(c.name || '')}</td>
            <td>${escapeHtml(c.ticket_type_name || '')}</td>
            <td><strong>${escapeHtml(c.code || '')}</strong></td>
            <td>${c.max_uses ?? 0}</td>
            <td>${c.used_count ?? 0}</td>
            <td>${actionBtn}</td>
        </tr>`;
    });
    container.innerHTML = html + '</tbody></table>';
}

async function disableCoupon(couponID, code) {
    const label = code ? ` (${code})` : '';
    if (!confirm(`Désactiver ce coupon${label} ?`)) {
        return;
    }

    try {
        const response = await apiFetch(`${API_BASE}/admin/coupons/${couponID}/disable`, {
            method: 'POST',
        });
        const data = await response.json();
        if (!response.ok) {
            throw new Error(data.error || 'Erreur désactivation coupon');
        }
        await loadCoupons();
    } catch (error) {
        alert(`❌ ${error.message}`);
    }
}

async function renderTicketTypesAdmin(types) {
    const container = document.getElementById('tt-list');
    if (!types || types.length === 0) {
        container.innerHTML = '<p style="color:#718096;">Aucun type de ticket</p>';
        populateReallocDropdowns([], {});
        return;
    }

    let html = '';
    const catsCache = {}; // { ticketTypeId: [cats] }

    // Load categories for each type
    for (const tt of types) {
        let cats = [];
        try {
            const catRes = await apiFetch(`${API_BASE}/admin/ticket-types/${tt.id}/categories`);
            cats = (await catRes.json()) || [];
        } catch (e) { /* ignore */ }
        catsCache[tt.id] = cats;

        const domains = renderAllowedEntries(tt.allowed_domains || []);

        const totalAllocated = cats.reduce((s, c) => s + c.quantity_allocated, 0);
        const unallocated = tt.quantity_total - totalAllocated;

        const maskedClass = tt.is_masked ? ' tt-masked' : '';
        const maskedBadge = tt.is_masked ? '<span class="badge badge-masked">MASQUÉ</span>' : '';
        const maskBtnLabel = tt.is_masked ? 'Démasquer' : 'Masquer';
        const maskBtnClass = tt.is_masked ? 'btn-success' : 'btn-warning';

        html += `<div class="tt-block${maskedClass}">
            <div class="tt-header">
                <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap;">
                    <strong>${tt.name}</strong> — ${formatPrice(tt.price_cents)}
                    ${maskedBadge}
                    <span style="color:#718096;font-size:0.85em;">
                        ${tt.quantity_sold}/${tt.quantity_total} vendus · Accès: ${domains} · ${tt.one_ticket_per_email ? '1 ticket / email' : `jusqu'à ${Math.max(2, tt.max_per_order || 0)} / commande`}
                    </span>
                </div>
                <div style="display:flex;gap:6px;margin-top:4px;">
                    <button class="btn btn-sm btn-primary" onclick="toggleEditForm('${tt.id}')">Modifier</button>
                    <button class="btn btn-sm ${maskBtnClass}" onclick="toggleTicketTypeMask('${tt.id}')">${maskBtnLabel}</button>
                </div>
            </div>`;

        // Inline edit form (hidden by default)
        const sStart = tt.sale_start ? new Date(tt.sale_start) : new Date();
        const sEnd = tt.sale_end ? new Date(tt.sale_end) : new Date();
        const startDate = sStart.toISOString().slice(0, 10);
        const startTime = sStart.toTimeString().slice(0, 5);
        const endDate = sEnd.toISOString().slice(0, 10);
        const endTime = sEnd.toTimeString().slice(0, 5);
        const domainsStr = (tt.allowed_domains || []).join(', ');

        html += `<div id="edit-form-${tt.id}" class="edit-form hidden" style="margin:10px 0;padding:12px;background:#f7fafc;border-radius:8px;border:1px solid #e2e8f0;">
            <div class="form-row">
                <div class="form-group"><label>Nom</label><input type="text" id="edit-name-${tt.id}" value="${escapeAttr(tt.name)}"></div>
                <div class="form-group"><label>Prix (€)</label><input type="number" id="edit-price-${tt.id}" value="${(tt.price_cents / 100).toFixed(2)}" step="0.01" min="0"></div>
            </div>
            <div class="form-group"><label>Description</label><input type="text" id="edit-desc-${tt.id}" value="${escapeAttr(tt.description || '')}"></div>
            <div class="form-group" style="margin-top:-4px;">
                <label style="display:flex;align-items:center;gap:8px;cursor:pointer;font-weight:700;">
                    <input type="checkbox" id="edit-one-per-email-${tt.id}" ${tt.one_ticket_per_email ? 'checked' : ''}>
                    1 ticket maximum par email
                </label>
            </div>
            <div class="form-row">
                <div class="form-group"><label>Quantité totale (min: ${tt.quantity_sold} vendus)</label><input type="number" id="edit-qty-${tt.id}" value="${tt.quantity_total}" min="${tt.quantity_sold}"></div>
            </div>
            <div class="form-row">
                <div class="form-group"><label>Début vente — Date</label><input type="date" id="edit-start-date-${tt.id}" value="${startDate}"></div>
                <div class="form-group"><label>Début vente — Heure</label><input type="text" id="edit-start-time-${tt.id}" value="${startTime}" pattern="([01]\\d|2[0-3]):[0-5]\\d" maxlength="5"></div>
            </div>
            <div class="form-row">
                <div class="form-group"><label>Fin vente — Date</label><input type="date" id="edit-end-date-${tt.id}" value="${endDate}"></div>
                <div class="form-group"><label>Fin vente — Heure</label><input type="text" id="edit-end-time-${tt.id}" value="${endTime}" pattern="([01]\\d|2[0-3]):[0-5]\\d" maxlength="5"></div>
            </div>
            <div class="form-group"><label>Emails/domaines autorisés</label><div style="display:flex;gap:8px;align-items:center;"><input type="text" id="edit-domains-${tt.id}" value="${escapeAttr(domainsStr)}" placeholder="@univ.fr, admin@gmail.com" style="flex:1;"><button type="button" class="btn btn-sm" onclick="importAllowedFromCSV('edit-domains-${tt.id}')">Import CSV</button></div><small>Séparés par des virgules. Exemple: @univ.fr, admin@gmail.com. Vide = accessible à tous.</small></div>
            <div style="display:flex;gap:8px;margin-top:8px;">
                <button class="btn btn-primary btn-sm" onclick="saveTicketType('${tt.id}')">Enregistrer</button>
                <button class="btn btn-sm" onclick="toggleEditForm('${tt.id}')">Annuler</button>
            </div>
            <span id="edit-msg-${tt.id}" class="form-msg hidden"></span>
        </div>`;

        // Category table
        if (cats.length > 0) {
            html += `<table class="cat-table">
                <thead><tr><th>Catégorie</th><th>Alloués</th><th>Vendus</th><th>Restants</th><th>Domaines</th><th>Actions</th></tr></thead><tbody>`;
            cats.forEach(c => {
                const cDomains = renderAllowedEntries(c.allowed_domains || []);
                const remaining = c.quantity_allocated - c.quantity_sold;
                const catMaskedClass = c.is_masked ? ' cat-masked' : '';
                const catMaskedBadge = c.is_masked ? ' <span class="badge badge-masked" style="font-size:0.7em;">MASQUÉ</span>' : '';
                const catCheckboxBadge = c.is_checkbox ? ' <span class="badge" style="font-size:0.7em;background:#3182ce;color:#fff;">CASE</span>' : '';
                const catMaskBtn = c.is_masked
                    ? `<button class="btn btn-sm btn-success" onclick="toggleCategoryMask('${c.id}')" title="Démasquer">👁</button>`
                    : `<button class="btn btn-sm btn-warning" onclick="toggleCategoryMask('${c.id}')" title="Masquer">🚫</button>`;
                const catCheckboxBtn = c.is_checkbox
                    ? `<button class="btn btn-sm btn-primary" onclick="toggleCategoryCheckbox('${c.id}')" title="Retirer de la case">☑</button>`
                    : `<button class="btn btn-sm" onclick="toggleCategoryCheckbox('${c.id}')" title="Rendre cette catégorie en case">☐</button>`;
                html += `<tr class="${catMaskedClass}">
                    <td><strong>${c.name}</strong>${catMaskedBadge}${catCheckboxBadge}</td>
                    <td>${c.quantity_allocated}</td>
                    <td>${c.quantity_sold}</td>
                    <td>${remaining}</td>
                    <td>${cDomains}</td>
                    <td style="display:flex;gap:4px;">${catMaskBtn}${catCheckboxBtn}${c.quantity_sold === 0 ? `<button class="btn btn-sm btn-danger" onclick="deleteCategory('${c.id}')">×</button>` : ''}</td>
                </tr>`;
            });
            html += '</tbody></table>';
        }

        // Unallocated info
        if (unallocated > 0) {
            html += `<p style="color:#e53e3e;font-size:0.85em;margin:6px 0;">⚠️ ${unallocated} places non allouées sur ${tt.quantity_total}</p>`;
        } else {
            html += `<p style="color:#38a169;font-size:0.85em;margin:6px 0;">✅ Toutes les places sont allouées</p>`;
        }

        // Add category form
        html += `<div class="add-cat-form" style="margin-top:8px;padding-top:8px;border-top:1px solid #e2e8f0;">
            <strong style="font-size:0.85em;">Ajouter une catégorie :</strong>
            <div class="form-row" style="margin-top:4px;">
                <input type="text" id="cat-name-${tt.id}" placeholder="Nom (ex: Pharmacie)" style="flex:2">
                <input type="number" id="cat-qty-${tt.id}" placeholder="Places" min="1" style="flex:1">
                <input type="text" id="cat-dom-${tt.id}" placeholder="Domaines (virgules)" style="flex:2">
                <button class="btn btn-primary btn-sm" onclick="addCategory('${tt.id}')">+</button>
            </div>
        </div>`;

        html += '</div>';
    }

    container.innerHTML = html;
    populateReallocDropdowns(types, catsCache);
}

function escapeAttr(str) {
    return str.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/'/g, '&#39;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function renderAllowedEntries(entries) {
    if (!entries || entries.length === 0) {
        return '<span style="color:#a0aec0;">Tous</span>';
    }

    const normalized = entries
        .map(e => (e || '').trim())
        .filter(Boolean);

    if (normalized.length === 0) {
        return '<span style="color:#a0aec0;">Tous</span>';
    }

    const limit = 4;
    const visible = normalized.slice(0, limit);
    const remaining = normalized.length - visible.length;
    const tags = visible.map(value => {
        const escaped = escapeAttr(value);
        return `<span class="domain-tag" title="${escaped}">${escaped}</span>`;
    }).join(' ');

    if (remaining <= 0) {
        return tags;
    }

    const all = escapeAttr(normalized.join(', '));
    return `${tags} <span class="domain-tag" title="${all}">+${remaining}</span>`;
}

function importAllowedFromCSV(targetInputId) {
    const target = document.getElementById(targetInputId);
    if (!target) return;

    const picker = document.createElement('input');
    picker.type = 'file';
    picker.accept = '.csv,text/csv,.txt';

    picker.addEventListener('change', () => {
        const file = picker.files && picker.files[0];
        if (!file) return;

        const reader = new FileReader();
        reader.onload = () => {
            const text = String(reader.result || '');
            const imported = parseAllowedCSVEntries(text);
            if (imported.length === 0) {
                alert('Aucune entrée email/domaine détectée dans le fichier.');
                return;
            }

            const existing = parseAllowedCSVEntries(target.value || '');
            const merged = Array.from(new Set([...existing, ...imported]));
            target.value = merged.join(', ');
            alert(`${imported.length} entrée(s) importée(s).`);
        };

        reader.onerror = () => {
            alert('Impossible de lire le fichier CSV.');
        };

        reader.readAsText(file, 'utf-8');
    });

    picker.click();
}

function parseAllowedCSVEntries(rawText) {
    const text = (rawText || '').replace(/^\uFEFF/, '');
    const chunks = text.split(/[\n\r,;\t]+/);

    return chunks
        .map(value => value.trim())
        .map(value => value.replace(/^"|"$/g, ''))
        .map(value => value.toLowerCase())
        .filter(Boolean);
}

function toggleEditForm(ticketTypeId) {
    const form = document.getElementById(`edit-form-${ticketTypeId}`);
    form.classList.toggle('hidden');
}

async function saveTicketType(ticketTypeId) {
    const msg = document.getElementById(`edit-msg-${ticketTypeId}`);
    msg.classList.add('hidden');

    const domainsRaw = document.getElementById(`edit-domains-${ticketTypeId}`).value.trim();
    const allowed = domainsRaw ? domainsRaw.split(',').map(d => d.trim().toLowerCase()).filter(Boolean) : [];

    const body = {
        name: document.getElementById(`edit-name-${ticketTypeId}`).value.trim(),
        description: document.getElementById(`edit-desc-${ticketTypeId}`).value.trim(),
        price_cents: Math.round(parseFloat(document.getElementById(`edit-price-${ticketTypeId}`).value) * 100),
        quantity_total: parseInt(document.getElementById(`edit-qty-${ticketTypeId}`).value, 10),
        one_ticket_per_email: !!document.getElementById(`edit-one-per-email-${ticketTypeId}`)?.checked,
        sale_start: new Date(`${document.getElementById(`edit-start-date-${ticketTypeId}`).value}T${document.getElementById(`edit-start-time-${ticketTypeId}`).value}:00`).toISOString(),
        sale_end: new Date(`${document.getElementById(`edit-end-date-${ticketTypeId}`).value}T${document.getElementById(`edit-end-time-${ticketTypeId}`).value}:00`).toISOString(),
        allowed_domains: allowed,
    };

    try {
        const res = await apiFetch(`${API_BASE}/admin/ticket-types/${ticketTypeId}`, {
            method: 'PUT',
            body: JSON.stringify(body),
        });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        msg.textContent = '✅ Enregistré !';
        msg.className = 'form-msg success-text';
        setTimeout(() => loadTicketTypesAdmin(), 500);
    } catch (err) {
        msg.textContent = `❌ ${err.message}`;
        msg.className = 'form-msg error-text';
    }
}

async function toggleTicketTypeMask(ticketTypeId) {
    try {
        const res = await apiFetch(`${API_BASE}/admin/ticket-types/${ticketTypeId}/mask`, { method: 'POST' });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        loadTicketTypesAdmin();
    } catch (err) {
        alert(`Erreur: ${err.message}`);
    }
}

async function toggleCategoryMask(categoryId) {
    try {
        const res = await apiFetch(`${API_BASE}/admin/categories/${categoryId}/mask`, { method: 'POST' });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        loadTicketTypesAdmin();
    } catch (err) {
        alert(`Erreur: ${err.message}`);
    }
}

async function toggleCategoryCheckbox(categoryId) {
    try {
        const res = await apiFetch(`${API_BASE}/admin/categories/${categoryId}/checkbox`, { method: 'POST' });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        loadTicketTypesAdmin();
    } catch (err) {
        alert(`Erreur: ${err.message}`);
    }
}

async function addCategory(ticketTypeID) {
    const name = document.getElementById(`cat-name-${ticketTypeID}`).value.trim();
    const qty = parseInt(document.getElementById(`cat-qty-${ticketTypeID}`).value, 10);
    const domRaw = document.getElementById(`cat-dom-${ticketTypeID}`).value.trim();
    const domains = domRaw ? domRaw.split(',').map(d => d.trim().toLowerCase()).filter(Boolean) : [];

    if (!name || !qty || qty < 1) { alert('Nom et quantité requis'); return; }

    try {
        const res = await apiFetch(`${API_BASE}/admin/ticket-types/${ticketTypeID}/categories`, {
            method: 'POST',
            body: JSON.stringify({ ticket_type_id: ticketTypeID, name, quantity: qty, allowed_domains: domains }),
        });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        loadTicketTypesAdmin();
    } catch (err) {
        alert(`Erreur: ${err.message}`);
    }
}

async function deleteCategory(categoryID) {
    if (!confirm('Supprimer cette catégorie ?')) return;
    try {
        const res = await apiFetch(`${API_BASE}/admin/categories/${categoryID}`, { method: 'DELETE' });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        loadTicketTypesAdmin();
    } catch (err) {
        alert(`Erreur: ${err.message}`);
    }
}

function populateReallocDropdowns(types, catsCache) {
    const srcSel = document.getElementById('realloc-src');
    const dstSel = document.getElementById('realloc-dst');
    srcSel.innerHTML = '<option value="">— Sélectionner source —</option>';
    dstSel.innerHTML = '<option value="">— Sélectionner cible —</option>';

    for (const tt of types) {
        const cats = catsCache[tt.id] || [];

        for (const c of cats) {
            const remaining = c.quantity_allocated - c.quantity_sold;
            const opt1 = document.createElement('option');
            opt1.value = c.id;
            opt1.textContent = `${tt.name} → ${c.name} (${remaining} dispo)`;
            opt1.dataset.typeId = tt.id;
            srcSel.appendChild(opt1);

            const opt2 = document.createElement('option');
            opt2.value = c.id;
            opt2.textContent = `${tt.name} → ${c.name}`;
            opt2.dataset.typeId = tt.id;
            dstSel.appendChild(opt2);
        }
    }
}

async function doReallocate() {
    const msg = document.getElementById('realloc-msg');
    msg.classList.add('hidden');

    const srcID = document.getElementById('realloc-src').value;
    const dstID = document.getElementById('realloc-dst').value;
    const qty = parseInt(document.getElementById('realloc-qty').value, 10);

    if (!srcID || !dstID || srcID === dstID || qty < 1) {
        msg.textContent = '❌ Source et cible doivent être différentes, quantité > 0';
        msg.className = 'form-msg error-text';
        return;
    }

    try {
        const res = await apiFetch(`${API_BASE}/admin/categories/reallocate`, {
            method: 'POST',
            body: JSON.stringify({ source_category_id: srcID, target_category_id: dstID, quantity: qty }),
        });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        msg.textContent = '✅ Réallocation effectuée';
        msg.className = 'form-msg success-text';
        loadTicketTypesAdmin();
    } catch (err) {
        msg.textContent = `❌ ${err.message}`;
        msg.className = 'form-msg error-text';
    }
}

async function loadBusAdminData() {
    try {
        const [optionsRes, ticketsRes] = await Promise.all([
            apiFetch(`${API_BASE}/admin/bus/options`),
            apiFetch(`${API_BASE}/admin/bus/tickets`),
        ]);

        busOptionsCache = await optionsRes.json();
        busTicketsCache = await ticketsRes.json();

        renderBusStationsSelects(busOptionsCache.stations || []);
        renderBusDeparturesTable([...(busOptionsCache.outbound_departures || []), ...(busOptionsCache.return_departures || [])]);
        populateBusTicketNavetteFilter();
        applyBusTicketsFilters();
    } catch (error) {
        console.error('Erreur chargement bus admin:', error);
    }
}

function renderBusStationsSelects(stations) {
    const stationSelect = document.getElementById('bus-dep-station');
    if (!stationSelect) return;
    stationSelect.innerHTML = '<option value="">Choisir une station</option>' + stations
        .filter(s => s.is_active)
        .map(s => `<option value="${s.id}">${s.name}</option>`)
        .join('');
}

function renderBusDeparturesTable(departures) {
    const container = document.getElementById('bus-departures-table');
    if (!container) return;
    if (!departures.length) {
        container.innerHTML = '<p style="color:#718096;">Aucun horaire</p>';
        return;
    }

    const stations = (busOptionsCache?.stations || []).filter(s => s.is_active);

    departures.sort((a, b) => new Date(a.departure_time) - new Date(b.departure_time));
    let html = `<table><thead><tr>
        <th>Station</th><th>Direction</th><th>Départ</th><th>Prix</th><th>Vendus</th><th>Capacité</th><th>Remplissage</th><th>Statut</th><th>Actions</th>
    </tr></thead><tbody>`;

    departures.forEach(d => {
        const isSoldOut = !!d.is_sold_out;
        const status = isSoldOut ? 'Complet' : (d.is_active ? 'Visible' : 'Masqué');
        const maskLabel = d.is_active ? 'Masquer' : 'Démasquer';
        const soldOutLabel = isSoldOut ? 'Annuler soldout' : 'Mettre en soldout';
        const departureLocalValue = toDateTimeLocalValue(d.departure_time);
        const stationOptions = stations.map(s => `<option value="${s.id}" ${s.id === d.station_id ? 'selected' : ''}>${s.name}</option>`).join('');
        const fillPercent = d.capacity > 0 ? Math.round((d.sold / d.capacity) * 100) : 0;
        const fillColor = getFillRateColor(fillPercent);
        html += `<tr>
            <td>${d.station_name}</td>
            <td>${d.direction === 'to_festival' ? 'Aller' : 'Retour'}</td>
            <td>${formatDateTime(d.departure_time)}</td>
            <td>${formatPrice(d.price_cents)}</td>
            <td>${d.sold}</td>
            <td>${d.capacity}</td>
            <td><span style="display:inline-flex;align-items:center;gap:8px;"><span style="width:16px;height:16px;border-radius:999px;background:${fillColor};display:inline-block;box-shadow:0 0 0 2px rgba(15,23,42,0.12);"></span><strong>${fillPercent}%</strong></span></td>
            <td>${status}</td>
            <td style="display:flex;gap:6px;flex-wrap:wrap;">
                <button class="btn btn-sm btn-primary" onclick="editBusDeparture('${d.id}')">Modifier</button>
                <button class="btn btn-sm" onclick="toggleBusDepartureSoldOut('${d.id}')">${soldOutLabel}</button>
                <button class="btn btn-sm btn-warning" onclick="toggleBusDepartureMask('${d.id}')">${maskLabel}</button>
                <button class="btn btn-sm btn-danger" onclick="deleteBusDeparture('${d.id}')">Supprimer</button>
            </td>
        </tr>`;

        html += `<tr id="bus-edit-row-${d.id}" class="hidden">
            <td colspan="9" style="background:#f8fafc;padding:0;">
                <div style="margin:10px 12px;padding:12px;border:1px solid #e2e8f0;border-radius:8px;background:#f7fafc;">
                    <div class="form-row">
                        <div class="form-group">
                            <label>Station</label>
                            <select id="bus-edit-station-${d.id}">${stationOptions}</select>
                        </div>
                        <div class="form-group">
                            <label>Direction</label>
                            <select id="bus-edit-direction-${d.id}">
                                <option value="to_festival" ${d.direction === 'to_festival' ? 'selected' : ''}>Aller vers festival</option>
                                <option value="from_festival" ${d.direction === 'from_festival' ? 'selected' : ''}>Retour depuis festival</option>
                            </select>
                        </div>
                    </div>
                    <div class="form-row">
                        <div class="form-group">
                            <label>Date/heure départ</label>
                            <input type="datetime-local" id="bus-edit-time-${d.id}" value="${departureLocalValue}">
                        </div>
                        <div class="form-group">
                            <label>Prix (€)</label>
                            <input type="number" id="bus-edit-price-${d.id}" min="0" step="0.01" value="${(d.price_cents / 100).toFixed(2)}">
                        </div>
                        <div class="form-group">
                            <label>Capacité</label>
                            <input type="number" id="bus-edit-capacity-${d.id}" min="1" value="${d.capacity}">
                        </div>
                    </div>
                    <input type="hidden" id="bus-edit-active-${d.id}" value="${d.is_active ? '1' : '0'}">
                    <div style="display:flex;gap:8px;align-items:center;">
                        <button class="btn btn-primary btn-sm" onclick="saveBusDeparture('${d.id}')">Enregistrer</button>
                        <button class="btn btn-sm" onclick="toggleBusDepartureEditForm('${d.id}')">Annuler</button>
                        <span id="bus-edit-msg-${d.id}" class="form-msg hidden"></span>
                    </div>
                </div>
            </td>
        </tr>`;
    });

    container.innerHTML = html + '</tbody></table>';
}

function renderBusTicketsTable(rows, containerId = 'bus-tickets-table') {
    const container = document.getElementById(containerId);
    if (!container) return;
    if (!rows.length) {
        container.innerHTML = '<p style="color:#718096;">Aucun ticket navette</p>';
        return;
    }

    const showActions = containerId === 'bus-tickets-table';

    let html = `<table><thead><tr>
        <th>Commande</th><th>Client</th><th>Trajet</th><th>Départ</th><th>Retour</th><th>Total</th><th>Scan</th>${showActions ? '<th>Action</th>' : ''}
    </tr></thead><tbody>`;

    rows.forEach(r => {
        const hasReturn = !!r.return_departure_id;
        const outboundLabel = r.outbound_direction === 'from_festival' ? 'Retour' : 'Aller';
        html += `<tr>
            <td>${r.order_number}</td>
            <td>${r.customer_first_name} ${r.customer_last_name}<br><small>${r.customer_email}</small></td>
            <td>${r.from_station} → ${r.to_station}</td>
            <td>${formatDateTime(r.departure_time)}</td>
            <td>${r.return_departure_time ? formatDateTime(r.return_departure_time) : '-'}</td>
            <td>${formatPrice(r.order_total_cents || 0)}</td>
            <td>${r.is_validated ? '✅' : '⏳'}</td>
            ${showActions ? `<td style="text-align:right;"><button class="btn btn-sm" onclick="toggleBusTicketEdit('${r.ticket_id}')">Modifier</button></td>` : ''}
        </tr>`;

        if (showActions) {
            html += `<tr id="bus-ticket-edit-${r.ticket_id}" class="hidden">
                <td colspan="8" style="background:#f8fafc;padding:0;">
                    <div style="margin:10px 12px;padding:12px;border:1px solid #e2e8f0;border-radius:8px;background:#f7fafc;">
                        <div style="display:grid;gap:10px;">
                            <div class="form-row" style="align-items:flex-end;">
                                <div class="form-group" style="min-width:260px;">
                                    <label>${outboundLabel}</label>
                                    <select id="bus-ticket-select-${r.ticket_id}-outbound">${buildBusDepartureOptions(r.outbound_direction, r.outbound_departure_id)}</select>
                                </div>
                                <div class="form-group" style="display:flex;align-items:flex-end;">
                                    <button class="btn btn-sm btn-primary" onclick="changeBusTicketDeparture('${r.ticket_id}', '${r.outbound_direction}', 'bus-ticket-select-${r.ticket_id}-outbound')">Changer de navette</button>
                                </div>
                            </div>
                            ${hasReturn ? `
                            <div class="form-row" style="align-items:flex-end;">
                                <div class="form-group" style="min-width:260px;">
                                    <label>Retour</label>
                                    <select id="bus-ticket-select-${r.ticket_id}-return">${buildBusDepartureOptions(r.return_direction || 'from_festival', r.return_departure_id)}</select>
                                </div>
                                <div class="form-group" style="display:flex;align-items:flex-end;">
                                    <button class="btn btn-sm btn-primary" onclick="changeBusTicketDeparture('${r.ticket_id}', 'from_festival', 'bus-ticket-select-${r.ticket_id}-return')">Changer de navette</button>
                                </div>
                            </div>` : ''}
                            <div style="display:flex;justify-content:flex-end;">
                                <button class="btn btn-sm" onclick="toggleBusTicketEdit('${r.ticket_id}')">Fermer</button>
                            </div>
                        </div>
                    </div>
                </td>
            </tr>`;
        }
    });

    container.innerHTML = html + '</tbody></table>';
}

function toggleBusTicketEdit(ticketID) {
    const row = document.getElementById(`bus-ticket-edit-${ticketID}`);
    if (!row) return;
    row.classList.toggle('hidden');
}

function buildBusDepartureOptions(direction, selectedID) {
    const stations = busOptionsCache?.stations || [];
    const stationByID = new Map(stations.map(s => [s.id, s.name]));
    const departures = [
        ...(busOptionsCache?.outbound_departures || []),
        ...(busOptionsCache?.return_departures || []),
    ].filter(d => d.direction === direction && d.is_active && (d.capacity - d.sold) > 0);

    const options = departures.map(d => {
        const stationName = stationByID.get(d.station_id) || '';
        const selected = d.id === selectedID ? 'selected' : '';
        return `<option value="${d.id}" ${selected}>${stationName} — ${formatDateTime(d.departure_time)} — ${formatPrice(d.price_cents)}</option>`;
    });

    if (selectedID && !departures.some(d => d.id === selectedID)) {
        options.unshift(`<option value="${selectedID}" selected>Horaire actuel</option>`);
    }

    if (options.length === 0) {
        return '<option value="" disabled>Aucun horaire disponible</option>';
    }

    return options.join('');
}

async function changeBusTicketDeparture(ticketID, direction, selectId) {
    const select = document.getElementById(selectId);
    if (!select) return;
    const departureId = select.value;
    if (!departureId) {
        alert('Choisissez un horaire');
        return;
    }

    if (!confirm('Changer la navette et envoyer le nouveau ticket par email ?')) return;

    try {
        const res = await apiFetch(`${API_BASE}/admin/bus/tickets/${ticketID}/change-departure`, {
            method: 'POST',
            body: JSON.stringify({ direction, departure_id: departureId }),
        });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        await loadBusAdminData();
    } catch (error) {
        alert(`Erreur modification navette: ${error.message}`);
    }
}

function populateBusTicketNavetteFilter() {
    const select = document.getElementById('bus-ticket-filter-navette');
    if (!select) return;

    const stations = busOptionsCache?.stations || [];
    const stationByID = new Map(stations.map(s => [s.id, s.name]));
    const allDepartures = [
        ...(busOptionsCache?.outbound_departures || []),
        ...(busOptionsCache?.return_departures || []),
    ];

    const entriesById = new Map();
    const toEntry = (id, label, timeValue) => {
        if (!id || entriesById.has(id)) return;
        entriesById.set(id, {
            id,
            label,
            time: Number.isFinite(timeValue) ? timeValue : 0,
        });
    };

    allDepartures.forEach(dep => {
        if (!dep?.id) return;
        const stationName = stationByID.get(dep.station_id) || dep.station_id;
        const directionLabel = dep.direction === 'to_festival' ? 'Aller' : 'Retour';
        const timeValue = new Date(dep.departure_time).getTime();
        toEntry(dep.id, `${directionLabel} ${stationName} — ${formatDateTime(dep.departure_time)}`, timeValue);
    });

    (busTicketsCache || []).forEach(row => {
        if (row.outbound_departure_id && row.departure_time) {
            const directionLabel = row.outbound_direction === 'from_festival' ? 'Retour' : 'Aller';
            const stationName = row.outbound_direction === 'from_festival'
                ? (row.to_station || '')
                : (row.from_station || '');
            const timeValue = new Date(row.departure_time).getTime();
            toEntry(row.outbound_departure_id, `${directionLabel} ${stationName} — ${formatDateTime(row.departure_time)}`, timeValue);
        }

        if (row.return_departure_id && row.return_departure_time) {
            const stationName = row.to_station || '';
            const timeValue = new Date(row.return_departure_time).getTime();
            toEntry(row.return_departure_id, `Retour ${stationName} — ${formatDateTime(row.return_departure_time)}`, timeValue);
        }
    });

    const entries = Array.from(entriesById.values());

    entries.sort((a, b) => a.time - b.time);
    const options = ['<option value="">Toutes les navettes</option>']
        .concat(entries.map(e => `<option value="${escapeAttr(e.id)}">${e.label}</option>`));
    select.innerHTML = options.join('');
}

function applyBusTicketsFilters() {
    const query = (document.getElementById('bus-ticket-search')?.value || '').trim().toLowerCase();
    const navetteId = document.getElementById('bus-ticket-filter-navette')?.value || '';

    const rows = (busTicketsCache || []).filter(r => {
        if (query) {
            const name = `${r.customer_first_name || ''} ${r.customer_last_name || ''}`.trim().toLowerCase();
            const email = (r.customer_email || '').trim().toLowerCase();
            if (!name.includes(query) && !email.includes(query)) {
                return false;
            }
        }

        if (navetteId) {
            const outboundID = r.outbound_departure_id;
            const returnID = r.return_departure_id;
            if (navetteId !== outboundID && navetteId !== returnID) {
                return false;
            }
        }

        return true;
    });

    renderBusTicketsTable(rows);
}

function resetBusTicketsFilters() {
    const search = document.getElementById('bus-ticket-search');
    const navette = document.getElementById('bus-ticket-filter-navette');
    if (search) search.value = '';
    if (navette) navette.value = '';
    applyBusTicketsFilters();
}

async function createBusStation() {
    const msg = document.getElementById('bus-station-msg');
    const name = document.getElementById('bus-station-name').value.trim();
    msg.classList.add('hidden');

    if (!name) {
        msg.textContent = '❌ Nom de station requis';
        msg.className = 'form-msg error-text';
        return;
    }

    try {
        const res = await apiFetch(`${API_BASE}/admin/bus/stations`, {
            method: 'POST',
            body: JSON.stringify({ name }),
        });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        msg.textContent = '✅ Station ajoutée';
        msg.className = 'form-msg success-text';
        document.getElementById('bus-station-name').value = '';
        loadBusAdminData();
    } catch (error) {
        msg.textContent = `❌ ${error.message}`;
        msg.className = 'form-msg error-text';
    }
}

async function createBusDeparture() {
    const msg = document.getElementById('bus-departure-msg');
    msg.classList.add('hidden');

    const stationID = document.getElementById('bus-dep-station').value;
    const direction = document.getElementById('bus-dep-direction').value;
    const departureTimeRaw = document.getElementById('bus-dep-time').value;
    const price = parseFloat(document.getElementById('bus-dep-price').value || '0');
    const capacity = parseInt(document.getElementById('bus-dep-capacity').value, 10);

    if (!stationID || !direction || !departureTimeRaw || !capacity) {
        msg.textContent = '❌ Champs incomplets';
        msg.className = 'form-msg error-text';
        return;
    }

    try {
        const res = await apiFetch(`${API_BASE}/admin/bus/departures`, {
            method: 'POST',
            body: JSON.stringify({
                station_id: stationID,
                direction,
                departure_time: new Date(departureTimeRaw).toISOString(),
                price_cents: Math.round(price * 100),
                capacity,
                is_active: true,
            }),
        });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        msg.textContent = '✅ Horaire ajouté';
        msg.className = 'form-msg success-text';
        loadBusAdminData();
    } catch (error) {
        msg.textContent = `❌ ${error.message}`;
        msg.className = 'form-msg error-text';
    }
}

async function editBusDeparture(departureID) {
    toggleBusDepartureEditForm(departureID);
}

function toggleBusDepartureEditForm(departureID) {
    const row = document.getElementById(`bus-edit-row-${departureID}`);
    if (!row) return;
    row.classList.toggle('hidden');
}

async function saveBusDeparture(departureID) {
    const msg = document.getElementById(`bus-edit-msg-${departureID}`);
    if (msg) msg.classList.add('hidden');

    const stationID = document.getElementById(`bus-edit-station-${departureID}`).value;
    const direction = document.getElementById(`bus-edit-direction-${departureID}`).value;
    const departureTimeRaw = document.getElementById(`bus-edit-time-${departureID}`).value;
    const priceRaw = document.getElementById(`bus-edit-price-${departureID}`).value;
    const capacityRaw = document.getElementById(`bus-edit-capacity-${departureID}`).value;
    const activeRaw = document.getElementById(`bus-edit-active-${departureID}`).value;

    if (!stationID || !direction || !departureTimeRaw || !priceRaw || !capacityRaw) {
        if (msg) {
            msg.textContent = '❌ Champs incomplets';
            msg.className = 'form-msg error-text';
        }
        return;
    }

    const departureDate = new Date(departureTimeRaw);
    if (Number.isNaN(departureDate.getTime())) {
        if (msg) {
            msg.textContent = '❌ Date/heure invalide';
            msg.className = 'form-msg error-text';
        }
        return;
    }

    const payload = {
        station_id: stationID,
        direction,
        departure_time: departureDate.toISOString(),
        price_cents: Math.round(parseFloat(priceRaw) * 100),
        capacity: parseInt(capacityRaw, 10),
        is_active: activeRaw === '1',
    };

    try {
        const res = await apiFetch(`${API_BASE}/admin/bus/departures/${departureID}`, {
            method: 'PUT',
            body: JSON.stringify(payload),
        });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        if (msg) {
            msg.textContent = '✅ Départ mis à jour';
            msg.className = 'form-msg success-text';
        }
        setTimeout(() => loadBusAdminData(), 300);
    } catch (error) {
        if (msg) {
            msg.textContent = `❌ ${error.message}`;
            msg.className = 'form-msg error-text';
        }
    }
}

async function toggleBusDepartureMask(departureID) {
    try {
        const res = await apiFetch(`${API_BASE}/admin/bus/departures/${departureID}/mask`, { method: 'POST' });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        await loadBusAdminData();
    } catch (error) {
        alert(`Erreur masquage: ${error.message}`);
    }
}

async function toggleBusDepartureSoldOut(departureID) {
    try {
        const res = await apiFetch(`${API_BASE}/admin/bus/departures/${departureID}/soldout`, { method: 'POST' });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        await loadBusAdminData();
    } catch (error) {
        alert(`Erreur soldout: ${error.message}`);
    }
}

async function resyncBusDeparturesSold() {
    if (!confirm('Resynchroniser le compteur vendus des navettes ?')) return;
    try {
        const res = await apiFetch(`${API_BASE}/admin/bus/departures/resync`, { method: 'POST' });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        await loadBusAdminData();
    } catch (error) {
        alert(`Erreur resync: ${error.message}`);
    }
}

async function deleteBusDeparture(departureID) {
    if (!confirm('Supprimer ce départ navette ?')) return;
    try {
        const res = await apiFetch(`${API_BASE}/admin/bus/departures/${departureID}`, { method: 'DELETE' });
        if (!res.ok) { const e = await res.json(); throw new Error(e.error); }
        await loadBusAdminData();
    } catch (error) {
        alert(`Erreur suppression: ${error.message}`);
    }
}

// ==========================================
// Utilitaires
// ==========================================

function formatPrice(cents) {
    return (cents / 100).toLocaleString('fr-FR', { style: 'currency', currency: 'EUR' });
}

function getFillRateColor(percent) {
    const clamped = Math.max(0, Math.min(100, percent));
    const hue = (120 * (100 - clamped)) / 100;
    return `hsl(${hue}, 70%, 45%)`;
}

function formatDate(dateStr) {
    if (!dateStr) return '-';
    return new Date(dateStr).toLocaleDateString('fr-FR');
}

function formatDateTime(dateStr) {
    if (!dateStr) return '-';
    return new Date(dateStr).toLocaleString('fr-FR', {
        day: '2-digit', month: '2-digit', year: 'numeric',
        hour: '2-digit', minute: '2-digit',
    });
}

function toDateTimeLocalValue(dateStr) {
    if (!dateStr) return '';
    const d = new Date(dateStr);
    if (Number.isNaN(d.getTime())) return '';
    const year = d.getFullYear();
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    const hours = String(d.getHours()).padStart(2, '0');
    const minutes = String(d.getMinutes()).padStart(2, '0');
    return `${year}-${month}-${day}T${hours}:${minutes}`;
}

function statusLabel(status) {
    const labels = {
        pending: 'En attente',
        paid: 'Payé',
        confirmed: 'Confirmé',
        cancelled: 'Annulé',
        refunded: 'Remboursé',
    };
    return labels[status] || status;
}

function playSound(type) {
    // Web Audio API pour un retour sonore lors du scan
    try {
        const ctx = new (window.AudioContext || window.webkitAudioContext)();
        const osc = ctx.createOscillator();
        const gain = ctx.createGain();
        osc.connect(gain);
        gain.connect(ctx.destination);

        switch (type) {
            case 'success':
                osc.frequency.value = 880;
                gain.gain.value = 0.3;
                osc.start();
                osc.stop(ctx.currentTime + 0.15);
                break;
            case 'warning':
                osc.frequency.value = 440;
                gain.gain.value = 0.3;
                osc.start();
                osc.stop(ctx.currentTime + 0.3);
                break;
            case 'error':
                osc.frequency.value = 220;
                gain.gain.value = 0.3;
                osc.start();
                osc.stop(ctx.currentTime + 0.5);
                break;
        }
    } catch (e) {
        // Pas de support audio, pas grave
    }
}

// Auto-format HH:MM time inputs
document.querySelectorAll('#create-tt-form input[id$="-time"]').forEach(input => {
    input.addEventListener('input', function () {
        let v = this.value.replace(/[^\d]/g, '').slice(0, 4);
        if (v.length >= 3) v = v.slice(0, 2) + ':' + v.slice(2);
        this.value = v;
    });
});

/* ── KPI Analytics ───────────────────────────────────────── */

let kpiSessionsChart = null;
let kpiClicksChart = null;
let kpiCurrentRange = '1j';

async function loadKPI(range) {
    range = range || kpiCurrentRange;
    kpiCurrentRange = range;

    // Toggle active range button
    ['1h', '1j', '1semaine', '1mois', 'custom'].forEach(r => {
        const btn = document.getElementById('kpi-range-' + r);
        if (btn) btn.classList.toggle('btn-primary', r === range);
    });

    const params = new URLSearchParams();
    if (range !== 'custom') {
        params.set('range', range);
    } else {
        const start = document.getElementById('kpi-custom-start')?.value || '';
        const end = document.getElementById('kpi-custom-end')?.value || '';
        if (!start || !end) {
            alert('Sélectionnez une date de début et de fin pour la période KPI');
            return;
        }
        params.set('range', 'custom');
        params.set('start', start);
        params.set('end', end);
    }

    try {
        const response = await apiFetch(`${API_BASE}/admin/analytics/kpi?${params.toString()}`);
        const kpi = await response.json();

        document.getElementById('kpi-sessions').textContent = kpi.total_sessions ?? '-';
        document.getElementById('kpi-clicks').textContent = kpi.total_clicks ?? '-';
        document.getElementById('kpi-avg-duration').textContent = formatDurationShort(kpi.avg_session_duration_s);

        renderKPIChart('kpi-sessions-chart', kpi.sessions_timeline || [], 'Sessions', '#667eea', c => { kpiSessionsChart = c; }, kpiSessionsChart);
        renderKPIChart('kpi-clicks-chart', kpi.clicks_timeline || [], 'Clics', '#ed8936', c => { kpiClicksChart = c; }, kpiClicksChart);
        renderKPITopPages(kpi.top_pages || []);
        renderKPITicketOrigins(kpi.ticket_origins || []);
    } catch (error) {
        console.error('Erreur chargement KPI:', error);
    }
}

function applyCustomKPIRange() {
    loadKPI('custom');
}

function formatDurationShort(seconds) {
    if (seconds == null || isNaN(seconds)) return '-';
    seconds = Math.round(seconds);
    if (seconds < 60) return seconds + 's';
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return m + 'min ' + (s > 0 ? s + 's' : '');
}

function renderKPIChart(containerId, points, label, color, setter, existing) {
    const container = document.getElementById(containerId);
    if (!container) return;

    if (typeof Chart === 'undefined') {
        container.innerHTML = '<p style="color:#e53e3e;">Chart.js non chargé</p>';
        return;
    }

    if (!points.length) {
        if (existing) { existing.destroy(); setter(null); }
        container.innerHTML = '<p style="color:#718096;">Aucune donnée</p>';
        return;
    }

    const ordered = [...points].sort((a, b) => new Date(a.bucket) - new Date(b.bucket));
    const labels = ordered.map(p => formatKPIBucketLabel(p.bucket));
    const data = ordered.map(p => p.count || 0);

    container.innerHTML = '<canvas></canvas>';
    const canvas = container.querySelector('canvas');

    if (existing) { existing.destroy(); setter(null); }

    const chart = new Chart(canvas, {
        type: 'line',
        data: {
            labels,
            datasets: [{
                label,
                data,
                borderColor: color,
                backgroundColor: color + '1f',
                pointBackgroundColor: color,
                pointRadius: 3,
                tension: 0.3,
                fill: true,
            }],
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            interaction: { mode: 'index', intersect: false },
            plugins: {
                legend: { display: false },
                tooltip: {
                    callbacks: {
                        title: items => items[0]?.label || '',
                    },
                },
            },
            scales: {
                x: { ticks: { maxTicksLimit: 12, font: { size: 11 } } },
                y: { beginAtZero: true, ticks: { precision: 0 } },
            },
        },
    });

    setter(chart);
}

function formatKPIBucketLabel(bucket) {
    const d = new Date(bucket);
    if (isNaN(d)) return bucket;
    if (kpiCurrentRange === '1h') return d.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' });
    if (kpiCurrentRange === '1j') return d.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' });
    return d.toLocaleDateString('fr-FR', { day: '2-digit', month: '2-digit' });
}

function renderKPITopPages(pages) {
    const container = document.getElementById('kpi-top-pages');
    if (!container) return;

    if (!pages.length) {
        container.innerHTML = '<p style="color:#718096;">Aucune donnée</p>';
        return;
    }

    let html = '<table><thead><tr><th>Page</th><th>Sessions</th><th>Clics</th></tr></thead><tbody>';
    pages.forEach(p => {
        html += `<tr><td>${escapeHtml(p.page)}</td><td>${p.sessions}</td><td>${p.clicks}</td></tr>`;
    });
    container.innerHTML = html + '</tbody></table>';
}

function renderKPITicketOrigins(rows) {
    const container = document.getElementById('kpi-ticket-origins');
    if (!container) return;

    if (!rows.length) {
        container.innerHTML = '<p style="color:#718096;">Aucune donnée</p>';
        return;
    }

    let html = '<table><thead><tr><th>Catégorie</th><th>Type</th><th>Tickets</th></tr></thead><tbody>';
    rows.forEach(r => {
        const category = r.category || 'sans catégorie';
        const ticketType = r.ticket_type || '-';
        html += `<tr><td>${escapeHtml(category)}</td><td>${escapeHtml(ticketType)}</td><td>${r.ticket_count ?? 0}</td></tr>`;
    });
    container.innerHTML = html + '</tbody></table>';
}

function escapeHtml(str) {
    const d = document.createElement('div');
    d.textContent = str;
    return d.innerHTML;
}
