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
  attendees: {},  // { ticketTypeId: [{first_name,last_name,email}] }
  loading: false,
  customerEmail: '',
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
  initHeroVideo();
  setupEmailGate();
  setupCheckoutForm();
  setupBusForm();
  setupCampingClaimForm();
});

// ══════════════════════════════════════
// NAVIGATION (from design)
// ══════════════════════════════════════
function go(id) {
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  const page = document.getElementById('page-' + id);
  if (page) {
    page.classList.remove('hidden');
    page.classList.add('active');
  }
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

function goCampingForm() {
  go('camping');
  setTimeout(() => {
    const el = document.getElementById('camping-section');
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
  const cdTarget = new Date('2026-05-30T15:00:00');
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

function initHeroVideo() {
  const video = document.querySelector('.hero-video');
  if (!video) return;

  const ensurePlay = () => {
    const playPromise = video.play();
    if (playPromise && typeof playPromise.catch === 'function') {
      playPromise.catch(() => {});
    }
  };

  video.muted = true;
  video.playsInline = true;

  if (video.readyState >= 2) {
    ensurePlay();
  } else {
    video.addEventListener('canplay', ensurePlay, { once: true });
  }

  document.addEventListener('visibilitychange', () => {
    if (!document.hidden && video.paused) {
      ensurePlay();
    }
  });

  video.addEventListener('stalled', () => {
    if (!document.hidden) ensurePlay();
  });
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
  state.attendees = {};
  document.getElementById('email-gate').classList.remove('hidden');
  document.getElementById('tkt-step2').classList.add('hidden');
  document.getElementById('checkout-section').classList.add('hidden');
  document.getElementById('gate-email').value = '';
  const emailField = document.getElementById('email');
  if (emailField) {
    emailField.value = '';
    emailField.readOnly = false;
  }
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
    if (emailField) {
      emailField.value = email;
      emailField.readOnly = true;
    }
    const claimEmailField = document.getElementById('camping-claim-email');
    if (claimEmailField && !claimEmailField.value) claimEmailField.value = email;

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
    const qty = state.cart[tt.id] || 0;

    const remaining = categories.length > 0
      ? getTotalCategoryRemaining(tt)
      : ((tt.quantity_total || 0) - (tt.quantity_sold || 0));

    const now = new Date();
    const saleStart = tt.sale_start ? new Date(tt.sale_start) : null;
    const saleEnd = tt.sale_end ? new Date(tt.sale_end) : null;
    const isActive = tt.is_active !== undefined ? tt.is_active : true;
    const isOnSale = isActive && (!saleStart || now >= saleStart) && (!saleEnd || now <= saleEnd) && remaining > 0;
    const isSoldOut = remaining <= 0;
    const notYet = isActive && saleStart && now < saleStart;
    const isBest = idx === 0 && !isSoldOut;
    const inCart = qty > 0;
    const onePerEmail = !!tt.one_ticket_per_email;
    const maxPerOrder = onePerEmail ? 1 : Math.max(1, tt.max_per_order || 1);
    const maxQty = Math.max(0, Math.min(maxPerOrder, remaining));

    const rvClass = idx === 0 ? 'rv' : idx === 1 ? 'rv2' : 'rv3';

    // Availability badge
    let availHtml = '';
    if (categories.length > 0) {
      if (isSoldOut) {
        availHtml = '<div class="tkt-avail sold-out">❌ Plus de places disponibles</div>';
      } else if (isOnSale) {
        availHtml = '<div class="tkt-avail available">✅ Catégorie à choisir pour chaque billet</div>';
      }
    } else if (notYet) {
      availHtml = '<div class="tkt-avail not-yet">🕐 Vente pas encore ouverte</div>';
    }

    const canBuy = isOnSale;

    // CTA button
    let btnHtml = '';
    if (canBuy) {
      if (onePerEmail) {
        if (inCart) {
          btnHtml = `<button class="btn-full selected-btn" onclick="deselectTicket('${tt.id}')">✓ Sélectionné</button>`;
        } else if (isBest) {
          btnHtml = `<button class="btn-full" onclick="selectTicket('${tt.id}')">Prendre ce tarif</button>`;
        } else {
          btnHtml = `<button class="btn-otl" onclick="selectTicket('${tt.id}')">Prendre ce tarif</button>`;
        }
      } else {
        btnHtml = `<div class="tkt-qty">
          <button type="button" onclick="decreaseTicket('${tt.id}')" ${qty <= 0 ? 'disabled' : ''}>−</button>
          <span class="qty-val">${qty}</span>
          <button type="button" onclick="increaseTicket('${tt.id}')" ${qty >= maxQty ? 'disabled' : ''}>+</button>
        </div>`;
      }
    } else if (isSoldOut) {
      btnHtml = '<button class="btn-otl" disabled>Complet</button>';
    } else if (notYet) {
      btnHtml = '<button class="btn-otl" disabled>Bientôt disponible</button>';
    }

    const description = (tt.description || '').trim();
    const descHtml = description ? `<p class="tkt-desc">${description}</p>` : '';

    const card = document.createElement('div');
    card.className = `tkt-card ${rvClass} ${isBest ? 'best' : ''} ${inCart ? 'selected' : ''}`;
    card.innerHTML = `
      ${isBest ? '<div class="tkt-badge">⚡ Recommandé</div>' : ''}
      <p class="tkt-tier">${tt.name}</p>
      <div class="tkt-price">${formatPrice(tt.price_cents)}</div>
      ${descHtml}
      ${availHtml}
      ${btnHtml}
    `;

    grid.appendChild(card);
  });

  setTimeout(initReveal, 50);
  updateCheckoutVisibility();
}

// ══════════════════════════════════════
// CART MANAGEMENT
// ══════════════════════════════════════
function selectTicket(id) {
  state.cart[id] = 1;
  syncAttendeesForTicket(id, 1);
  renderTickets();
  updateOrderSummary();
  renderAttendeeForms();
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
  delete state.attendees[id];
  renderTickets();
  updateOrderSummary();
  renderAttendeeForms();
  updateCheckoutVisibility();
}

function increaseTicket(id) {
  const tt = state.ticketTypes.find(t => t.id === id);
  if (!tt || tt.one_ticket_per_email) return;

  const categories = tt.categories || [];
  const remaining = categories.length > 0
    ? getTotalCategoryRemaining(tt)
    : ((tt.quantity_total || 0) - (tt.quantity_sold || 0));

  const maxPerOrder = Math.max(1, tt.max_per_order || 1);
  const maxQty = Math.max(0, Math.min(maxPerOrder, remaining));
  const current = state.cart[id] || 0;
  if (current >= maxQty) return;

  state.cart[id] = current + 1;
  syncAttendeesForTicket(id, state.cart[id]);
  renderTickets();
  updateOrderSummary();
  renderAttendeeForms();
  updateCheckoutVisibility();
}

function decreaseTicket(id) {
  const current = state.cart[id] || 0;
  if (current <= 0) return;

  const next = current - 1;
  if (next <= 0) {
    delete state.cart[id];
    delete state.attendees[id];
  } else {
    state.cart[id] = next;
    syncAttendeesForTicket(id, next);
  }

  renderTickets();
  updateOrderSummary();
  renderAttendeeForms();
  updateCheckoutVisibility();
}

function updateCheckoutVisibility() {
  const hasItems = Object.values(state.cart).some(q => q > 0);
  const cs = document.getElementById('checkout-section');
  if (hasItems) {
    cs.classList.remove('hidden');
    updateOrderSummary();
    renderAttendeeForms();
  } else {
    cs.classList.add('hidden');
    const attendeesWrap = document.getElementById('attendees-forms');
    if (attendeesWrap) {
      attendeesWrap.innerHTML = '';
      attendeesWrap.classList.add('hidden');
    }
  }
}

function syncAttendeesForTicket(typeId, qty) {
  if (qty <= 0) {
    delete state.attendees[typeId];
    return;
  }

  const tt = state.ticketTypes.find(t => t.id === typeId);
  const hasCategories = Array.isArray(tt?.categories) && tt.categories.length > 0;
  const existing = Array.isArray(state.attendees[typeId]) ? [...state.attendees[typeId]] : [];

  while (existing.length < qty) {
    existing.push({
      first_name: document.getElementById('firstName')?.value.trim() || '',
      last_name: document.getElementById('lastName')?.value.trim() || '',
      email: (state.customerEmail || document.getElementById('email')?.value || '').trim().toLowerCase(),
      category_id: hasCategories ? '' : undefined,
    });
  }

  if (hasCategories) {
    for (const attendee of existing) {
      if (typeof attendee.category_id !== 'string') attendee.category_id = '';
    }
  }

  state.attendees[typeId] = existing.slice(0, qty);
}

function updateAttendeeField(typeId, index, field, value) {
  const attendees = Array.isArray(state.attendees[typeId]) ? state.attendees[typeId] : [];
  if (!attendees[index]) {
    attendees[index] = { first_name: '', last_name: '', email: '', category_id: '' };
  }
  attendees[index][field] = value;
  state.attendees[typeId] = attendees;
}

function renderAttendeeForms() {
  const wrap = document.getElementById('attendees-forms');
  if (!wrap) return;

  const lines = [];
  for (const [typeId, qty] of Object.entries(state.cart)) {
    if (!qty || qty < 1) continue;
    const tt = state.ticketTypes.find(t => t.id === typeId);
    if (!tt) continue;
    const categories = tt.categories || [];

    syncAttendeesForTicket(typeId, qty);
    const attendees = state.attendees[typeId] || [];

    for (let idx = 0; idx < qty; idx++) {
      const attendee = attendees[idx] || { first_name: '', last_name: '', email: '' };
      const fieldsetTitle = `${tt.name} — Billet ${idx + 1}`;
      const lockedEmail = !!tt.one_ticket_per_email;
      const emailValue = lockedEmail
        ? (state.customerEmail || document.getElementById('email')?.value || '').trim().toLowerCase()
        : (attendee.email || '').trim().toLowerCase();

      lines.push(`
        <div class="attendee-card">
          <h4>${fieldsetTitle}</h4>
          <div class="form-row">
            <div class="form-group">
              <label>Prénom du participant *</label>
              <input type="text" value="${escapeHTML(attendee.first_name || '')}" oninput="updateAttendeeField('${typeId}', ${idx}, 'first_name', this.value)" required>
            </div>
            <div class="form-group">
              <label>Nom du participant *</label>
              <input type="text" value="${escapeHTML(attendee.last_name || '')}" oninput="updateAttendeeField('${typeId}', ${idx}, 'last_name', this.value)" required>
            </div>
          </div>
          <div class="form-group">
            <label>Email du participant *</label>
            <input type="email" value="${escapeHTML(emailValue)}" oninput="updateAttendeeField('${typeId}', ${idx}, 'email', this.value)" ${lockedEmail ? 'readonly' : ''} required>
            ${lockedEmail ? '<small>Ce billet est limité à 1 ticket par email: adresse figée sur l\'email validé.</small>' : ''}
          </div>
          ${categories.length > 0 ? `<div class="form-group">
            <label>Catégorie du participant *</label>
            <select onchange="updateAttendeeField('${typeId}', ${idx}, 'category_id', this.value)" required>
              <option value="">— Sélectionner une catégorie —</option>
              ${categories.map(c => {
                const selected = attendee.category_id === c.id ? 'selected' : '';
                const remaining = getCategoryRemaining(c);
                const disabled = remaining <= 0 ? 'disabled' : '';
                const suffix = remaining <= 0 ? ' (Complet)' : ` (${remaining} place${remaining > 1 ? 's' : ''} restantes)`;
                return `<option value="${c.id}" ${selected} ${disabled}>${escapeHTML(c.name)}${suffix}</option>`;
              }).join('')}
            </select>
          </div>` : ''}
        </div>
      `);
    }
  }

  wrap.innerHTML = lines.join('');
  wrap.classList.toggle('hidden', lines.length === 0);
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

    const grouped = new Map();

    for (const [typeId, qty] of Object.entries(state.cart)) {
      if (!qty || qty < 1) continue;
      const tt = state.ticketTypes.find(t => t.id === typeId);
      syncAttendeesForTicket(typeId, qty);

      const attendees = (state.attendees[typeId] || []).slice(0, qty).map(a => ({
        first_name: (a.first_name || '').trim(),
        last_name: (a.last_name || '').trim(),
        email: (a.email || '').trim().toLowerCase(),
        category_id: (a.category_id || '').trim(),
      }));

      for (const attendee of attendees) {
        const categoryId = attendee.category_id;
        const key = `${typeId}::${categoryId}`;
        if (!grouped.has(key)) {
          const base = { ticket_type_id: typeId, quantity: 0, attendees: [] };
          if (categoryId) base.category_id = categoryId;
          grouped.set(key, base);
        }

        const item = grouped.get(key);
        item.quantity += 1;
        item.attendees.push({
          first_name: attendee.first_name,
          last_name: attendee.last_name,
          email: attendee.email,
        });
      }

      if (tt?.one_ticket_per_email) {
        const forcedEmail = (state.customerEmail || document.getElementById('email').value).trim().toLowerCase();
        for (const item of grouped.values()) {
          if (item.ticket_type_id !== typeId) continue;
          if (item.attendees[0]) item.attendees[0].email = forcedEmail;
        }
      }
    }

    const items = Array.from(grouped.values());

    if (items.length === 0) {
      showNotification('Veuillez sélectionner au moins un billet', 'warning');
      return;
    }

    const body = {
      customer_first_name: document.getElementById('firstName').value.trim(),
      customer_last_name: document.getElementById('lastName').value.trim(),
      customer_email: (state.customerEmail || document.getElementById('email').value).trim(),
      customer_phone: document.getElementById('phone').value.trim(),
      date_of_birth: document.getElementById('dateOfBirth').value,
      wants_camping: document.getElementById('wants-camping')?.checked || false,
      items: items,
    };

    if (!body.customer_first_name || !body.customer_last_name || !body.customer_email) {
      showNotification('Veuillez remplir tous les champs obligatoires', 'warning');
      return;
    }

    for (const item of items) {
      if (!Array.isArray(item.attendees) || item.attendees.length !== item.quantity) {
        showNotification('Veuillez remplir les informations nominatives de tous les billets', 'warning');
        return;
      }

      const tt = state.ticketTypes.find(t => t.id === item.ticket_type_id);
      const categories = tt?.categories || [];
      if (categories.length > 0 && !item.category_id) {
        showNotification(`Choisissez une catégorie pour chaque billet "${tt.name}"`, 'warning');
        return;
      }

      for (const attendee of item.attendees) {
        if (!attendee.first_name || !attendee.last_name || !attendee.email || !attendee.email.includes('@')) {
          showNotification('Chaque billet doit avoir prénom, nom et email valide', 'warning');
          return;
        }
      }

      if (tt?.one_ticket_per_email && item.attendees[0]?.email.toLowerCase() !== body.customer_email.toLowerCase()) {
        showNotification(`Le ticket "${tt.name}" doit utiliser l'email validé`, 'warning');
        return;
      }
    }

    if (state.customerEmail && body.customer_email.toLowerCase() !== state.customerEmail.toLowerCase()) {
      showNotification('L\'email utilisé pour les billets doit rester celui validé au début', 'warning');
      return;
    }

    if (!body.date_of_birth) {
      showNotification('Veuillez renseigner votre date de naissance', 'warning');
      return;
    }

    if (!isAtLeast18(body.date_of_birth)) {
      showNotification('Accès réservé aux personnes de 18 ans et plus', 'error');
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

function getCategoryRemaining(category) {
  return Math.max(0, (category?.quantity_allocated || 0) - (category?.quantity_sold || 0));
}

function getTotalCategoryRemaining(ticketType) {
  const categories = ticketType?.categories || [];
  return categories.reduce((sum, category) => sum + getCategoryRemaining(category), 0);
}

function setupCampingClaimForm() {
  const form = document.getElementById('camping-claim-form');
  const submitBtn = document.getElementById('camping-claim-btn');
  const msg = document.getElementById('camping-claim-msg');
  if (!form || !msg) return;

  const submitClaim = async () => {
    const email = document.getElementById('camping-claim-email').value.trim();

    if (!email || !email.includes('@')) {
      msg.textContent = 'Veuillez entrer une adresse email valide.';
      msg.style.color = '#e53e3e';
      msg.classList.remove('hidden');
      return;
    }

    msg.classList.add('hidden');

    try {
      const response = await fetch(`${API_BASE}/camping/claim`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email }),
      });

      const data = await response.json();
      if (!response.ok) {
        throw new Error(data.error || 'Impossible d\'activer le camping');
      }

      msg.textContent = data.message || '';
      msg.style.color = (data.updated_tickets || 0) > 0 ? '#38a169' : '#dd6b20';
      msg.classList.remove('hidden');
    } catch (error) {
      msg.textContent = `❌ ${error.message}`;
      msg.style.color = '#e53e3e';
      msg.classList.remove('hidden');
    }
  };

  form.addEventListener('submit', (e) => {
    e.preventDefault();
    submitClaim();
  });

  if (submitBtn) {
    submitBtn.addEventListener('click', (e) => {
      e.preventDefault();
      submitClaim();
    });
  }
}

function setupBusForm() {
  const form = document.getElementById('bus-form');
  if (!form) return;

  const tripType = document.getElementById('bus-trip-type');
  if (tripType) {
    tripType.addEventListener('change', refreshBusTripFields);
  }

  const fromStation = document.getElementById('bus-from-station');
  if (fromStation) {
    fromStation.addEventListener('change', refreshOutboundDepartureOptions);
  }
  const returnStation = document.getElementById('bus-return-station');
  if (returnStation) {
    returnStation.addEventListener('change', refreshReturnDepartureOptions);
  }

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
  if (returnStationSelect) {
    returnStationSelect.innerHTML = stationOptions.join('');
  }

  if (stations.length > 0) {
    const stationWithOutbound = stations.find(s => outbound.some(d => d.station_id === s.id));
    const defaultStationID = stationWithOutbound ? stationWithOutbound.id : stations[0].id;
    fromSelect.value = defaultStationID;
    if (returnStationSelect) {
      const returnDepartures = (state.busOptions.return_departures || []).filter(d => d.is_active);
      const stationWithReturn = stations.find(s => returnDepartures.some(d => d.station_id === s.id));
      const defaultReturnStationID = stationWithReturn ? stationWithReturn.id : stations[0].id;
      returnStationSelect.value = defaultReturnStationID;
    }
  }

  refreshOutboundDepartureOptions();
  refreshReturnDepartureOptions();
  refreshBusTripFields();
}

function refreshBusTripFields() {
  const tripType = document.getElementById('bus-trip-type')?.value || 'outbound';
  const outboundWrap = document.getElementById('bus-outbound-fields');
  const returnWrap = document.getElementById('bus-return-fields');

  if (outboundWrap) {
    outboundWrap.classList.toggle('hidden', tripType === 'return');
  }
  if (returnWrap) {
    returnWrap.classList.toggle('hidden', tripType === 'outbound');
  }
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
  const selectedStation = document.getElementById('bus-return-station')?.value;
  const departures = (state.busOptions?.return_departures || []).filter(d => d.is_active && (!selectedStation || d.station_id === selectedStation));

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

  const tripType = document.getElementById('bus-trip-type')?.value || 'outbound';
  const needsOutbound = tripType === 'outbound' || tripType === 'round_trip';
  const needsReturn = tripType === 'return' || tripType === 'round_trip';

  const phoneInput = document.getElementById('bus-phone');
  const normalizedPhone = normalizeFrenchPhone(phoneInput.value);
  if (!normalizedPhone) {
    showNotification('Numéro invalide. Utilisez un format FR valide (ex: 06 12 34 56 78 ou +33...).', 'warning');
    phoneInput.focus();
    return;
  }
  phoneInput.value = normalizedPhone;

  const body = {
    customer_first_name: document.getElementById('bus-first-name').value.trim(),
    customer_last_name: document.getElementById('bus-last-name').value.trim(),
    customer_email: document.getElementById('bus-email').value.trim(),
    customer_phone: normalizedPhone,
    trip_type: tripType,
  };

  if (needsOutbound) {
    body.from_station_id = document.getElementById('bus-from-station').value;
    body.outbound_departure_id = document.getElementById('bus-outbound-time').value;
  }

  if (needsReturn) {
    body.return_station_id = document.getElementById('bus-return-station').value;
    body.return_departure_id = document.getElementById('bus-return-time').value;
  }

  if (!body.customer_first_name || !body.customer_last_name || !body.customer_email || !body.customer_phone) {
    showNotification('Veuillez remplir tous les champs obligatoires de la navette', 'warning');
    return;
  }

  if (needsOutbound && (!body.from_station_id || !body.outbound_departure_id)) {
    showNotification('Veuillez renseigner les informations d\'aller', 'warning');
    return;
  }

  if (needsReturn && (!body.return_station_id || !body.return_departure_id)) {
    showNotification('Veuillez renseigner la gare et l\'horaire retour', 'warning');
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
    document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
    document.getElementById('page-home').classList.remove('active');
    document.getElementById('page-home').classList.add('hidden');
    document.getElementById('page-success').classList.remove('hidden');
    document.getElementById('page-success').classList.add('active');
    document.getElementById('page-error').classList.add('hidden');
    
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
    document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
    document.getElementById('page-home').classList.remove('active');
    document.getElementById('page-home').classList.add('hidden');
    document.getElementById('page-error').classList.remove('hidden');
    document.getElementById('page-error').classList.add('active');
    document.getElementById('page-success').classList.add('hidden');
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

function isAtLeast18(dateOfBirth) {
  if (!dateOfBirth) return false;
  const dob = new Date(`${dateOfBirth}T00:00:00`);
  if (Number.isNaN(dob.getTime())) return false;

  const now = new Date();
  let age = now.getFullYear() - dob.getFullYear();
  const monthDiff = now.getMonth() - dob.getMonth();
  if (monthDiff < 0 || (monthDiff === 0 && now.getDate() < dob.getDate())) {
    age -= 1;
  }
  return age >= 18;
}

function normalizeFrenchPhone(raw) {
  if (!raw) return '';
  let value = raw.trim();
  value = value.replace(/[\s().-]/g, '');

  if (value.startsWith('00')) {
    value = '+' + value.slice(2);
  }

  if (value.startsWith('+')) {
    const digits = value.slice(1).replace(/\D/g, '');
    if (digits.length < 9 || digits.length > 12) return '';
    if (digits.startsWith('33')) {
      const rest = digits.slice(2);
      if (rest.length !== 9) return '';
      if (rest[0] === '0') return '';
      return '+33' + rest;
    }
    return '+' + digits;
  }

  const digits = value.replace(/\D/g, '');
  if (digits.length === 10 && digits.startsWith('0')) {
    return '+33' + digits.slice(1);
  }
  if (digits.length === 9 && /^[1-9]/.test(digits)) {
    return '+33' + digits;
  }
  return '';
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

function escapeHTML(value) {
  return String(value || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}
