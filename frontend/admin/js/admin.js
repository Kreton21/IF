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
let latestSalesStats = null;
let salesChartRange = '1j';
let salesChart = null;
const expandedOrderIds = new Set();

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
    document.querySelectorAll('.tab[data-tab="stats"], .tab[data-tab="orders"], .tab[data-tab="tickets"], .tab[data-tab="bus"], .tab[data-tab="referral"]').forEach(tab => {
        tab.style.display = isStaff ? 'none' : '';
    });

    if (isStaff) {
        // Staff → directement sur le scanner
        switchTab('scanner');
    } else {
        loadStats();
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

function switchTab(tabName) {
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));

    document.querySelector(`[data-tab="${tabName}"]`).classList.add('active');
    document.getElementById(`tab-${tabName}`).classList.add('active');

    switch (tabName) {
        case 'stats': loadStats(); break;
        case 'orders': loadOrders(); break;
        case 'tickets': loadTicketTypesAdmin(); break;
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
        renderDailySalesChart();
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

async function exportDatabaseCSV() {
    const button = document.getElementById('btn-export-db-csv');
    const msg = document.getElementById('export-db-msg');
    if (!button || !msg) return;

    button.disabled = true;
    const initialLabel = button.textContent;
    button.textContent = 'Export en cours...';
    msg.classList.add('hidden');

    try {
        const response = await fetch(`${API_BASE}/admin/stats/export-csv`, {
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
        const url = window.URL.createObjectURL(blob);
        const link = document.createElement('a');
        const disposition = response.headers.get('Content-Disposition') || '';
        const match = disposition.match(/filename="?([^";]+)"?/i);
        const filename = (match && match[1]) ? match[1] : 'database_export.csv';

        link.href = url;
        link.download = filename;
        document.body.appendChild(link);
        link.click();
        link.remove();
        window.URL.revokeObjectURL(url);

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

    if (rangeKey === '1h' || rangeKey === '1j') {
        return date.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' });
    }

    return date.toLocaleDateString('fr-FR', { day: '2-digit', month: '2-digit' });
}

function formatTooltipDate(bucket, rangeKey) {
    const date = new Date(bucket);
    if (Number.isNaN(date.getTime())) return '';

    if (rangeKey === '1h' || rangeKey === '1j') {
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
        const busTickets = await ticketsRes.json();

        renderBusStationsSelects(busOptionsCache.stations || []);
        renderBusDeparturesTable([...(busOptionsCache.outbound_departures || []), ...(busOptionsCache.return_departures || [])]);
        renderBusTicketsTable(busTickets || []);
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
        <th>Station</th><th>Direction</th><th>Départ</th><th>Prix</th><th>Vendus</th><th>Capacité</th><th>Statut</th><th>Actions</th>
    </tr></thead><tbody>`;

    departures.forEach(d => {
        const status = d.is_active ? 'Visible' : 'Masqué';
        const maskLabel = d.is_active ? 'Masquer' : 'Démasquer';
        const departureLocalValue = toDateTimeLocalValue(d.departure_time);
        const stationOptions = stations.map(s => `<option value="${s.id}" ${s.id === d.station_id ? 'selected' : ''}>${s.name}</option>`).join('');
        html += `<tr>
            <td>${d.station_name}</td>
            <td>${d.direction === 'to_festival' ? 'Aller' : 'Retour'}</td>
            <td>${formatDateTime(d.departure_time)}</td>
            <td>${formatPrice(d.price_cents)}</td>
            <td>${d.sold}</td>
            <td>${d.capacity}</td>
            <td>${status}</td>
            <td style="display:flex;gap:6px;flex-wrap:wrap;">
                <button class="btn btn-sm btn-primary" onclick="editBusDeparture('${d.id}')">Modifier</button>
                <button class="btn btn-sm btn-warning" onclick="toggleBusDepartureMask('${d.id}')">${maskLabel}</button>
                <button class="btn btn-sm btn-danger" onclick="deleteBusDeparture('${d.id}')">Supprimer</button>
            </td>
        </tr>`;

        html += `<tr id="bus-edit-row-${d.id}" class="hidden">
            <td colspan="8" style="background:#f8fafc;padding:0;">
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

    let html = `<table><thead><tr>
        <th>Commande</th><th>Client</th><th>Trajet</th><th>Départ</th><th>Retour</th><th>Total</th><th>Scan</th>
    </tr></thead><tbody>`;

    rows.forEach(r => {
        html += `<tr>
            <td>${r.order_number}</td>
            <td>${r.customer_first_name} ${r.customer_last_name}<br><small>${r.customer_email}</small></td>
            <td>${r.from_station} → ${r.to_station}</td>
            <td>${formatDateTime(r.departure_time)}</td>
            <td>${r.return_departure_time ? formatDateTime(r.return_departure_time) : '-'}</td>
            <td>${formatPrice(r.order_total_cents || 0)}</td>
            <td>${r.is_validated ? '✅' : '⏳'}</td>
        </tr>`;
    });

    container.innerHTML = html + '</tbody></table>';
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
    ['1h', '1j', '1semaine', '1mois'].forEach(r => {
        const btn = document.getElementById('kpi-range-' + r);
        if (btn) btn.classList.toggle('btn-primary', r === range);
    });

    try {
        const response = await apiFetch(`${API_BASE}/admin/analytics/kpi?range=${encodeURIComponent(range)}`);
        const kpi = await response.json();

        document.getElementById('kpi-sessions').textContent = kpi.total_sessions ?? '-';
        document.getElementById('kpi-clicks').textContent = kpi.total_clicks ?? '-';
        document.getElementById('kpi-avg-duration').textContent = formatDurationShort(kpi.avg_session_duration_s);

        renderKPIChart('kpi-sessions-chart', kpi.sessions_timeline || [], 'Sessions', '#667eea', c => { kpiSessionsChart = c; }, kpiSessionsChart);
        renderKPIChart('kpi-clicks-chart', kpi.clicks_timeline || [], 'Clics', '#ed8936', c => { kpiClicksChart = c; }, kpiClicksChart);
        renderKPITopPages(kpi.top_pages || []);
    } catch (error) {
        console.error('Erreur chargement KPI:', error);
    }
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

function escapeHtml(str) {
    const d = document.createElement('div');
    d.textContent = str;
    return d.innerHTML;
}
