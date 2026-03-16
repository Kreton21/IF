/**
 * L'Interfilières 2026 — Application Client
 * Design team frontend + Dynamic ticket purchasing via API
 */

const API_BASE = '/api/v1';

// ══════════════════════════════════════
// App State
// ══════════════════════════════════════
const state = {
  ticketTypes: [],
  cart: {},       // { ticketTypeId: quantity }
  loading: false,
  customerEmail: '',
  selectedCategories: {}, // { ticketTypeId: categoryId }
  busOptions: null,
  busLoading: false,
};

// ══════════════════════════════════════
// Initialisation
// ══════════════════════════════════════
document.addEventListener('DOMContentLoaded', () => {
  // Check for order_id in URL (payment return)
  const urlParams = new URLSearchParams(window.location.search);
  const orderId = urlParams.get('order_id');
  
  if (orderId) {
    // Show success page with order details
    showOrderSuccess(orderId);
    return;
  }
  
  // Normal page — init everything
  initParticles();
  initCountdown();
  initNavScroll();
  initHamburger();
  initReveal();
  setupEmailGate();
  setupCheckoutForm();
  setupBusForm();
});

// ══════════════════════════════════════
// NAVIGATION (from design)
// ══════════════════════════════════════
function go(id) {
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  const page = document.getElementById('page-' + id);
  if (page) page.classList.add('active');
  window.scrollTo({ top: 0, behavior: 'smooth' });
  document.getElementById('navLinks').classList.remove('open');
  setTimeout(initReveal, 80);
}

function goTickets() {
  go('home');
  setTimeout(() => {
    const el = document.getElementById('tickets');
    if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }, 130);
}

// ══════════════════════════════════════
// NAV SCROLL EFFECT
// ══════════════════════════════════════
function initNavScroll() {
  window.addEventListener('scroll', () => {
    document.getElementById('nav').classList.toggle('scrolled', scrollY > 70);
  });
}

// ══════════════════════════════════════
// HAMBURGER MENU
// ══════════════════════════════════════
function initHamburger() {
  document.getElementById('burger').addEventListener('click', () => {
    document.getElementById('navLinks').classList.toggle('open');
  });
}

// ══════════════════════════════════════
// COUNTDOWN
// ══════════════════════════════════════
function initCountdown() {
  const cdTarget = new Date('2026-03-05T10:00:00');
  function updateCD() {
    const diff = cdTarget - new Date();
    if (diff <= 0) {
      document.getElementById('cD').textContent = '00';
      document.getElementById('cH').textContent = '00';
      document.getElementById('cM').textContent = '00';
      document.getElementById('cS').textContent = '00';
      return;
    }
    const pad = n => String(Math.floor(n)).padStart(2, '0');
    document.getElementById('cD').textContent = pad(diff / 864e5);
    document.getElementById('cH').textContent = pad((diff % 864e5) / 36e5);
    document.getElementById('cM').textContent = pad((diff % 36e5) / 6e4);
    document.getElementById('cS').textContent = pad((diff % 6e4) / 1e3);
  }
  updateCD();
  setInterval(updateCD, 1000);
}

// ══════════════════════════════════════
// SCROLL REVEAL
// ══════════════════════════════════════
const obs = new IntersectionObserver(entries => {
  entries.forEach(e => { if (e.isIntersecting) e.target.classList.add('in'); });
}, { threshold: 0.1, rootMargin: '0px 0px -40px 0px' });

function initReveal() {
  document.querySelectorAll('.page.active .rv, .page.active .rv2, .page.active .rv3')
    .forEach(el => obs.observe(el));
}

// ══════════════════════════════════════
// FAQ ACCORDION
// ══════════════════════════════════════
function faq(q) {
  const item = q.parentElement;
  const was = item.classList.contains('open');
  document.querySelectorAll('.faq-item').forEach(i => i.classList.remove('open'));
  if (!was) item.classList.add('open');
}

// ══════════════════════════════════════
// PARTICLES
// ══════════════════════════════════════
function initParticles() {
  const cont = document.getElementById('particles');
  if (!cont) return;
  for (let i = 0; i < 22; i++) {
    const p = document.createElement('div');
    p.className = 'sparkle';
    const s = Math.random() * 4 + 2;
    const colors = ['rgba(255,255,255,.8)', 'rgba(255,230,130,.8)', 'rgba(255,180,120,.7)', 'rgba(249,127,160,.7)'];
    p.style.cssText = `
      width:${s}px; height:${s}px;
      left:${Math.random() * 100}%;
      background:${colors[Math.floor(Math.random() * colors.length)]};
      animation-duration:${Math.random() * 14 + 8}s;
      animation-delay:-${Math.random() * 14}s;
    `;
    cont.appendChild(p);
  }
}

// ══════════════════════════════════════
// EMAIL GATE
// ══════════════════════════════════════
function setupEmailGate() {
  const form = document.getElementById('email-gate-form');
  if (!form) return;
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const email = document.getElementById('gate-email').value.trim();
    const errEl = document.getElementById('eg-error');
    errEl.classList.add('hidden');

    if (!email || !email.includes('@')) {
      errEl.textContent = 'Veuillez entrer une adresse email valide';
      errEl.classList.remove('hidden');
      return;
    }

    state.customerEmail = email;
    await loadTicketTypes(email);
  });
}

function resetEmailGate() {
  state.customerEmail = '';
  state.ticketTypes = [];
  state.cart = {};
  state.selectedCategories = {};
  document.getElementById('email-gate').classList.remove('hidden');
  document.getElementById('tkt-step2').classList.add('hidden');
  document.getElementById('checkout-section').classList.add('hidden');
  document.getElementById('gate-email').value = '';
}

// ══════════════════════════════════════
// TICKET LOADING (API)
// ══════════════════════════════════════
async function loadTicketTypes(email) {
  const grid = document.getElementById('tkt-grid');
  const errEl = document.getElementById('eg-error');

  grid.innerHTML = '<div class="tkt-loading"><div class="spinner"></div><p>Chargement des billets disponibles...</p></div>';

  try {
    const url = email
      ? `${API_BASE}/tickets/types?email=${encodeURIComponent(email)}`
      : `${API_BASE}/tickets/types`;
    const response = await fetch(url);
    if (!response.ok) throw new Error('Erreur chargement');

    state.ticketTypes = await response.json();

    if (!state.ticketTypes || state.ticketTypes.length === 0) {
      errEl.textContent = 'Aucun billet disponible pour cette adresse email. Vérifiez votre email ou réessayez plus tard.';
      errEl.classList.remove('hidden');
      grid.innerHTML = '';
      return;
    }

    // Show step 2, hide email gate
    document.getElementById('email-gate').classList.add('hidden');
    document.getElementById('tkt-step2').classList.remove('hidden');
    document.getElementById('current-email-display').textContent = email;

    // Pre-fill email in checkout form
    const emailField = document.getElementById('email');
    if (emailField) emailField.value = email;

    renderTickets();
  } catch (error) {
    console.error('Erreur chargement tickets:', error);
    errEl.textContent = 'Erreur de connexion. Veuillez réessayer.';
    errEl.classList.remove('hidden');
  }
}

function renderTickets() {
  const grid = document.getElementById('tkt-grid');
  grid.innerHTML = '';

  if (!state.ticketTypes || state.ticketTypes.length === 0) {
    grid.innerHTML = '<div class="tkt-empty">Aucun billet disponible pour le moment.</div>';
    return;
  }

  const sorted = [...state.ticketTypes].sort((a, b) => a.price_cents - b.price_cents);

  sorted.forEach((tt, idx) => {
    const categories = tt.categories || [];
    const selectedCatId = state.selectedCategories[tt.id];
    const selectedCat = categories.find(c => c.id === selectedCatId);

    // Calculate remaining based on selected category or ticket type total
    let remaining, totalQty;
    if (selectedCat) {
      remaining = selectedCat.quantity_allocated - selectedCat.quantity_sold;
      totalQty = selectedCat.quantity_allocated;
    } else {
      remaining = (tt.quantity_total || 0) - (tt.quantity_sold || 0);
      totalQty = tt.quantity_total || 0;
    }

    const now = new Date();
    const saleStart = tt.sale_start ? new Date(tt.sale_start) : null;
    const saleEnd = tt.sale_end ? new Date(tt.sale_end) : null;
    const isActive = tt.is_active !== undefined ? tt.is_active : true;
    const isOnSale = isActive && (!saleStart || now >= saleStart) && (!saleEnd || now <= saleEnd) && remaining > 0;
    const isSoldOut = remaining <= 0;
    const isLow = remaining > 0 && remaining <= 20;
    const notYet = isActive && saleStart && now < saleStart;
    const isBest = idx === 0 && !isSoldOut;
    const inCart = (state.cart[tt.id] || 0) > 0;

    const rvClass = idx === 0 ? 'rv' : idx === 1 ? 'rv2' : 'rv3';

    // Availability badge
    let availHtml = '';
    if (selectedCat) {
      if (isSoldOut) {
        availHtml = '<div class="tkt-avail sold-out">❌ Complet pour cette catégorie</div>';
      } else if (isLow) {
        availHtml = `<div class="tkt-avail low">⚡ Plus que ${remaining} places !</div>`;
      } else if (isOnSale) {
        availHtml = `<div class="tkt-avail available">${remaining} places disponibles</div>`;
      }
    } else if (notYet) {
      availHtml = '<div class="tkt-avail not-yet">🕐 Vente pas encore ouverte</div>';
    }

    // Category dropdown
    let catHtml = '';
    if (categories.length > 0) {
      catHtml = `<div class="tkt-cat-select">
        <label>Votre catégorie :</label>
        <select onchange="selectCategory('${tt.id}', this.value)">
          <option value="">— Sélectionner —</option>
          ${categories.map(c => {
            const cRemaining = c.quantity_allocated - c.quantity_sold;
            const disabled = cRemaining <= 0 ? 'disabled' : '';
            const sel = (selectedCatId === c.id) ? 'selected' : '';
            return `<option value="${c.id}" ${disabled} ${sel}>${c.name} (${cRemaining} places)</option>`;
          }).join('')}
        </select>
      </div>`;
    }

    const perks = getPerksForTicket(tt, idx);

    // No quantity controls — 1 ticket per email
    const canBuy = isOnSale && (categories.length === 0 || selectedCatId);

    // CTA button
    let btnHtml = '';
    if (categories.length > 0 && !selectedCatId) {
      btnHtml = '<button class="btn-otl" disabled>Sélectionnez une catégorie</button>';
    } else if (canBuy) {
      if (inCart) {
        btnHtml = `<button class="btn-full selected-btn" onclick="deselectTicket('${tt.id}')">✓ Sélectionné</button>`;
      } else if (isBest) {
        btnHtml = `<button class="btn-full" onclick="selectTicket('${tt.id}')">Prendre ce tarif</button>`;
      } else {
        btnHtml = `<button class="btn-otl" onclick="selectTicket('${tt.id}')">Prendre ce tarif</button>`;
      }
    } else if (isSoldOut) {
      btnHtml = '<button class="btn-otl" disabled>Complet</button>';
    } else if (notYet) {
      btnHtml = '<button class="btn-otl" disabled>Bientôt disponible</button>';
    }

    const card = document.createElement('div');
    card.className = `tkt-card ${rvClass} ${isBest ? 'best' : ''} ${inCart ? 'selected' : ''}`;
    card.innerHTML = `
      ${isBest ? '<div class="tkt-badge">⚡ Recommandé</div>' : ''}
      <p class="tkt-tier">${tt.name}</p>
      <div class="tkt-price">${formatPrice(tt.price_cents)}</div>
      <p class="tkt-desc">${tt.description || ''}</p>
      ${catHtml}
      ${availHtml}
      <ul class="tkt-perks">
        ${perks.map(p => `<li>${p}</li>`).join('')}
      </ul>
      ${btnHtml}
    `;

    grid.appendChild(card);
  });

  setTimeout(initReveal, 50);
  updateCheckoutVisibility();
}

function selectCategory(ticketTypeId, categoryId) {
  if (categoryId) {
    state.selectedCategories[ticketTypeId] = categoryId;
  } else {
    delete state.selectedCategories[ticketTypeId];
  }
  // Reset cart for this type when changing category
  delete state.cart[ticketTypeId];
  renderTickets();
}

/**
 * Generate perks list for a ticket type based on its position
 */
function getPerksForTicket(tt, idx) {
  // If the ticket description contains bullet-style info, use generic perks
  const basePerk = 'Accès festival complet';
  const animPerk = 'Accès toutes animations';

  if (idx === 0) {
    return [basePerk, animPerk, 'Meilleur prix garanti'];
  } else if (idx === 1) {
    return [basePerk, animPerk];
  } else {
    return [basePerk, 'Entrée dernière vague'];
  }
}

// ══════════════════════════════════════
// CART MANAGEMENT
// ══════════════════════════════════════
function selectTicket(id) {
  // One ticket per email — clear previous selection and set this one
  state.cart = {};
  state.cart[id] = 1;
  renderTickets();
  updateOrderSummary();
  // Scroll to checkout form
  setTimeout(() => {
    const cs = document.getElementById('checkout-section');
    if (cs && !cs.classList.contains('hidden')) {
      cs.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  }, 100);
}

function deselectTicket(id) {
  delete state.cart[id];
  renderTickets();
  updateOrderSummary();
  updateCheckoutVisibility();
}

function updateCheckoutVisibility() {
  const hasItems = Object.values(state.cart).some(q => q > 0);
  const cs = document.getElementById('checkout-section');
  if (hasItems) {
    cs.classList.remove('hidden');
    updateOrderSummary();
  } else {
    cs.classList.add('hidden');
  }
}

function updateOrderSummary() {
  const summaryItems = document.getElementById('summary-items');
  const totalEl = document.getElementById('summary-total-price');
  if (!summaryItems || !totalEl) return;

  let html = '';
  let total = 0;

  for (const [typeId, qty] of Object.entries(state.cart)) {
    if (qty <= 0) continue;
    const tt = state.ticketTypes.find(t => t.id === typeId);
    if (!tt) continue;

    const subtotal = tt.price_cents * qty;
    total += subtotal;
    html += `<div class="summary-item">
      <span>${qty}× ${tt.name}</span>
      <span>${formatPrice(subtotal)}</span>
    </div>`;
  }

  summaryItems.innerHTML = html;
  totalEl.textContent = formatPrice(total);
}

// ══════════════════════════════════════
// CHECKOUT
// ══════════════════════════════════════
function setupCheckoutForm() {
  const form = document.getElementById('checkout-form');
  if (!form) return;

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    if (state.loading) return;

    const items = Object.entries(state.cart)
      .filter(([, qty]) => qty > 0)
      .map(([typeId, qty]) => {
        const item = { ticket_type_id: typeId, quantity: qty };
        if (state.selectedCategories[typeId]) {
          item.category_id = state.selectedCategories[typeId];
        }
        return item;
      });

    if (items.length === 0) {
      showNotification('Veuillez sélectionner au moins un billet', 'warning');
      return;
    }

    const body = {
      customer_first_name: document.getElementById('firstName').value.trim(),
      customer_last_name: document.getElementById('lastName').value.trim(),
      customer_email: document.getElementById('email').value.trim(),
      customer_phone: document.getElementById('phone').value.trim(),
      items: items,
    };

    if (!body.customer_first_name || !body.customer_last_name || !body.customer_email) {
      showNotification('Veuillez remplir tous les champs obligatoires', 'warning');
      return;
    }

    state.loading = true;
    const btn = document.getElementById('checkout-btn');
    btn.disabled = true;
    btn.textContent = '⏳ Redirection vers le paiement...';

    try {
      const response = await fetch(`${API_BASE}/tickets/checkout`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });

      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.error || 'Erreur lors de la création du checkout');
      }

      // Store order data for success page
      localStorage.setItem('lastOrderId', data.order_id);
      localStorage.setItem('lastOrderNumber', data.order_number);

      // Redirect to HelloAsso
      if (data.checkout_url) {
        window.location.href = data.checkout_url;
      } else {
        showNotification('URL de paiement manquante', 'error');
      }
    } catch (error) {
      console.error('Erreur checkout:', error);
      showNotification(error.message, 'error');
    } finally {
      state.loading = false;
      btn.disabled = false;
      btn.textContent = '💳 Procéder au paiement';
    }
  });
}

function setupBusForm() {
  const form = document.getElementById('bus-form');
  if (!form) return;

  const roundTrip = document.getElementById('bus-round-trip');
  roundTrip.addEventListener('change', () => {
    const wrap = document.getElementById('bus-return-fields');
    wrap.classList.toggle('hidden', !roundTrip.checked);
  });

  const fromStation = document.getElementById('bus-from-station');
  fromStation.addEventListener('change', refreshOutboundDepartureOptions);

  form.addEventListener('submit', submitBusCheckout);
}

async function toggleBusSection() {
  const section = document.getElementById('bus-section');
  if (!section) return;

  const willShow = section.classList.contains('hidden');
  section.classList.toggle('hidden');

  if (willShow) {
    if (!state.busOptions) {
      await loadBusOptions();
    }
    section.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }
}

async function loadBusOptions() {
  if (state.busLoading) return;
  state.busLoading = true;
  try {
    const res = await fetch(`${API_BASE}/bus/options`);
    if (!res.ok) throw new Error('Erreur chargement navettes');
    state.busOptions = await res.json();
    populateBusFormOptions();
  } catch (error) {
    showNotification(error.message || 'Impossible de charger les navettes', 'error');
  } finally {
    state.busLoading = false;
  }
}

function populateBusFormOptions() {
  if (!state.busOptions) return;

  const stations = (state.busOptions.stations || []).filter(s => s.is_active);
  const outbound = (state.busOptions.outbound_departures || []).filter(d => d.is_active);
  const fromSelect = document.getElementById('bus-from-station');
  const returnStationSelect = document.getElementById('bus-return-station');

  const stationOptions = ['<option value="">Choisir une station</option>']
    .concat(stations.map(s => `<option value="${s.id}">${s.name}</option>`));

  fromSelect.innerHTML = stationOptions.join('');
  returnStationSelect.innerHTML = stationOptions.join('');

  if (stations.length > 0) {
    const stationWithOutbound = stations.find(s => outbound.some(d => d.station_id === s.id));
    const defaultStationID = stationWithOutbound ? stationWithOutbound.id : stations[0].id;
    fromSelect.value = defaultStationID;
    returnStationSelect.value = defaultStationID;
  }

  refreshOutboundDepartureOptions();
  refreshReturnDepartureOptions();
}

function refreshOutboundDepartureOptions() {
  const selectedStation = document.getElementById('bus-from-station').value;
  const select = document.getElementById('bus-outbound-time');
  const departures = (state.busOptions?.outbound_departures || []).filter(d => d.is_active && d.station_id === selectedStation);

  let html = '<option value="">Choisir un horaire aller</option>';
  if (departures.length === 0) {
    html += '<option value="" disabled>Aucun horaire disponible pour cette station</option>';
  } else {
    html += departures.map(d => `<option value="${d.id}">${formatDateTime(d.departure_time)} — ${formatPrice(d.price_cents)}</option>`).join('');
  }
  select.innerHTML = html;

  if (departures.length > 0) {
    select.value = departures[0].id;
  }
}

function refreshReturnDepartureOptions() {
  const select = document.getElementById('bus-return-time');
  const departures = (state.busOptions?.return_departures || []).filter(d => d.is_active);

  let html = '<option value="">Choisir un horaire retour</option>';
  html += departures.map(d => `<option value="${d.id}">${formatDateTime(d.departure_time)} — ${formatPrice(d.price_cents)}</option>`).join('');
  select.innerHTML = html;

  if (departures.length > 0) {
    select.value = departures[0].id;
  }
}

async function submitBusCheckout(e) {
  e.preventDefault();
  if (state.busLoading) return;

  const isRoundTrip = document.getElementById('bus-round-trip').checked;
  const body = {
    customer_first_name: document.getElementById('bus-first-name').value.trim(),
    customer_last_name: document.getElementById('bus-last-name').value.trim(),
    customer_email: document.getElementById('bus-email').value.trim(),
    customer_phone: document.getElementById('bus-phone').value.trim(),
    from_station_id: document.getElementById('bus-from-station').value,
    outbound_departure_id: document.getElementById('bus-outbound-time').value,
    round_trip: isRoundTrip,
  };

  if (isRoundTrip) {
    body.return_departure_id = document.getElementById('bus-return-time').value;
    body.return_station_id = document.getElementById('bus-return-station').value;
  }

  if (!body.customer_first_name || !body.customer_last_name || !body.customer_email || !body.customer_phone || !body.from_station_id || !body.outbound_departure_id) {
    showNotification('Veuillez remplir tous les champs obligatoires de la navette', 'warning');
    return;
  }
  if (isRoundTrip && (!body.return_departure_id || !body.return_station_id)) {
    showNotification('Veuillez renseigner les champs retour', 'warning');
    return;
  }

  const btn = document.getElementById('bus-checkout-btn');
  btn.disabled = true;
  btn.textContent = '⏳ Redirection vers le paiement...';

  try {
    const res = await fetch(`${API_BASE}/bus/checkout`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Erreur checkout navette');

    localStorage.setItem('lastOrderId', data.order_id);
    localStorage.setItem('lastOrderNumber', data.order_number);
    window.location.href = data.checkout_url;
  } catch (error) {
    showNotification(error.message || 'Erreur checkout navette', 'error');
  } finally {
    btn.disabled = false;
    btn.textContent = '💳 Payer la navette';
  }
}

// ══════════════════════════════════════
// RESULT PAGES
// ══════════════════════════════════════
function showResultPage(type, orderId) {
  // Hide ticket + checkout sections, show result
  const tktSec = document.getElementById('tickets');
  const checkoutSec = document.getElementById('checkout-section');
  const successSec = document.getElementById('success-section');
  const errorSec = document.getElementById('error-section');

  if (tktSec) tktSec.classList.add('hidden');
  if (checkoutSec) checkoutSec.classList.add('hidden');

  if (type === 'success') {
    if (successSec) successSec.classList.remove('hidden');
    const orderNumber = localStorage.getItem('lastOrderNumber') || orderId || '';
    const el = document.getElementById('success-order-number');
    if (el) el.textContent = orderNumber;
    if (orderId) pollOrderStatus(orderId);
  } else {
    if (errorSec) errorSec.classList.remove('hidden');
  }

  // Still init the page visuals
  initParticles();
  initCountdown();
  initNavScroll();
  initHamburger();
  initReveal();
}

async function pollOrderStatus(orderId) {
  try {
    const response = await fetch(`${API_BASE}/orders/${orderId}/status`);
    if (response.ok) {
      const order = await response.json();
      if (order.status === 'confirmed') {
        const el = document.getElementById('success-order-number');
        if (el) el.textContent = order.order_number;
      } else if (order.status === 'pending') {
        setTimeout(() => pollOrderStatus(orderId), 3000);
      }
    }
  } catch (e) {
    console.error('Erreur polling:', e);
  }
}

// Show order success page after payment redirect
async function showOrderSuccess(orderId) {
  try {
    const response = await fetch(`${API_BASE}/orders/${orderId}/status`);
    if (!response.ok) {
      throw new Error('Failed to fetch order');
    }
    
    const order = await response.json();
    
    // Switch pages: hide home, show success
    document.getElementById('page-home').classList.remove('active');
    document.getElementById('page-home').classList.add('hidden');
    document.getElementById('page-success').classList.remove('hidden');
    document.getElementById('page-success').classList.add('active');
    
    // Update order number
    const orderNumEl = document.getElementById('success-order-number');
    if (orderNumEl) {
      orderNumEl.textContent = order.order_number;
    }
    
    // Display QR codes for each ticket
    const qrContainer = document.getElementById('qr-codes-container');
    if (qrContainer && order.tickets && order.tickets.length > 0) {
      qrContainer.innerHTML = order.tickets.map(ticket => `
        <div class="qr-ticket">
          <p class="qr-ticket-name">${ticket.ticket_type_name || 'Billet'}</p>
          <p class="qr-ticket-attendee">${ticket.attendee_first_name} ${ticket.attendee_last_name}</p>
          <img class="qr-img" src="${API_BASE}/tickets/${ticket.qr_token}/qr" alt="QR Code" />
        </div>
      `).join('');
    }
    
    // Clear URL without reload
    window.history.replaceState({}, document.title, '/');
  } catch (error) {
    console.error('Error loading order:', error);
    // Switch pages: hide home, show error
    document.getElementById('page-home').classList.remove('active');
    document.getElementById('page-home').classList.add('hidden');
    document.getElementById('page-error').classList.remove('hidden');
    document.getElementById('page-error').classList.add('active');
  }
}

// ══════════════════════════════════════
// UTILITIES
// ══════════════════════════════════════
function formatPrice(cents) {
  return (cents / 100).toLocaleString('fr-FR', {
    style: 'currency',
    currency: 'EUR',
  });
}

function formatDateTime(dateStr) {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleString('fr-FR', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function showNotification(message, type = 'info') {
  const el = document.getElementById('notification');
  if (!el) return;
  el.textContent = message;
  el.className = `notification ${type} show`;

  setTimeout(() => {
    el.classList.remove('show');
  }, 4500);
}
