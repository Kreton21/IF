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

// ==========================================
// Initialisation
// ==========================================

document.addEventListener('DOMContentLoaded', () => {
    if (authToken) {
        showDashboard();
    }

    document.getElementById('login-form').addEventListener('submit', handleLogin);

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
    document.querySelectorAll('.tab[data-tab="stats"], .tab[data-tab="orders"], .tab[data-tab="tickets"]').forEach(tab => {
        tab.style.display = isStaff ? 'none' : '';
    });

    if (isStaff) {
        // Staff → directement sur le scanner
        switchTab('scanner');
    } else {
        loadStats();
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
        case 'scanner':
            document.getElementById('qr-input').focus();
            loadValidationStats();
            break;
    }
}

// ==========================================
// Statistiques
// ==========================================

async function loadStats() {
    try {
        const response = await apiFetch(`${API_BASE}/admin/stats`);
        const stats = await response.json();

        // KPIs
        document.getElementById('stat-orders').textContent = stats.total_orders || 0;
        document.getElementById('stat-tickets').textContent = stats.total_tickets_sold || 0;
        document.getElementById('stat-revenue').textContent = formatPrice(stats.total_revenue_cents || 0);
        document.getElementById('stat-validated').textContent = stats.total_validated || 0;

        // Stats par type
        renderTypeStats(stats.by_ticket_type || []);

        // Ventes par jour
        renderDailyStats(stats.sales_by_day || []);

        // Commandes récentes
        renderRecentOrders(stats.recent_orders || []);
    } catch (error) {
        console.error('Erreur chargement stats:', error);
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

        document.getElementById('orders-table').innerHTML = renderOrdersTable(data.orders || []);
        renderPagination(data.total_count, data.page, data.page_size);
    } catch (error) {
        console.error('Erreur chargement commandes:', error);
    }
}

function renderOrdersTable(orders) {
    if (orders.length === 0) return '<p style="color:#718096;padding:20px;">Aucune commande</p>';

    let html = `<table>
        <thead><tr>
            <th>N°</th><th>Client</th><th>Email</th><th>Total</th><th>Statut</th><th>Date</th>
        </tr></thead><tbody>`;

    orders.forEach(o => {
        html += `<tr>
            <td><strong>${o.order_number}</strong></td>
            <td>${o.customer_first_name} ${o.customer_last_name}</td>
            <td>${o.customer_email}</td>
            <td>${formatPrice(o.total_cents)}</td>
            <td><span class="badge badge-${o.status}">${statusLabel(o.status)}</span></td>
            <td>${formatDateTime(o.created_at)}</td>
        </tr>`;
    });

    return html + '</tbody></table>';
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
            resultEl.classList.add('valid');
            resultEl.innerHTML = `
                <div class="result-icon">✅</div>
                <strong>${data.message}</strong>
                <div class="result-details">
                    ${data.attendee_first_name} ${data.attendee_last_name}<br>
                    Ticket : ${data.ticket_type_name}<br>
                    Commande : ${data.order_number}
                </div>`;
            // Son de validation (optionnel)
            playSound('success');
        } else if (data.already_validated) {
            resultEl.classList.add('warning');
            resultEl.innerHTML = `
                <div class="result-icon">⚠️</div>
                <strong>${data.message}</strong>
                <div class="result-details">
                    Ticket : ${data.ticket_type_name}<br>
                    Commande : ${data.order_number}
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
        max_per_order: 1,
        sale_start: new Date(`${document.getElementById('tt-start-date').value}T${document.getElementById('tt-start-time').value}:00`).toISOString(),
        sale_end: new Date(`${document.getElementById('tt-end-date').value}T${document.getElementById('tt-end-time').value}:00`).toISOString(),
        allowed_domains: allowed,
    };

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

        const domains = (tt.allowed_domains && tt.allowed_domains.length > 0)
            ? tt.allowed_domains.map(d => `<span class="domain-tag">${d}</span>`).join(' ')
            : '<span style="color:#a0aec0;">Tous</span>';

        const totalAllocated = cats.reduce((s, c) => s + c.quantity_allocated, 0);
        const unallocated = tt.quantity_total - totalAllocated;

        html += `<div class="tt-block">
            <div class="tt-header">
                <strong>${tt.name}</strong> — ${formatPrice(tt.price_cents)}
                <span style="color:#718096;font-size:0.85em;margin-left:8px;">
                    ${tt.quantity_sold}/${tt.quantity_total} vendus · Domaines: ${domains}
                </span>
            </div>`;

        // Category table
        if (cats.length > 0) {
            html += `<table class="cat-table">
                <thead><tr><th>Catégorie</th><th>Alloués</th><th>Vendus</th><th>Restants</th><th>Domaines</th><th></th></tr></thead><tbody>`;
            cats.forEach(c => {
                const cDomains = (c.allowed_domains && c.allowed_domains.length > 0)
                    ? c.allowed_domains.map(d => `<span class="domain-tag">${d}</span>`).join(' ')
                    : '<span style="color:#a0aec0;">Tous</span>';
                const remaining = c.quantity_allocated - c.quantity_sold;
                html += `<tr>
                    <td><strong>${c.name}</strong></td>
                    <td>${c.quantity_allocated}</td>
                    <td>${c.quantity_sold}</td>
                    <td>${remaining}</td>
                    <td>${cDomains}</td>
                    <td>${c.quantity_sold === 0 ? `<button class="btn btn-sm btn-danger" onclick="deleteCategory('${c.id}')">×</button>` : ''}</td>
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
