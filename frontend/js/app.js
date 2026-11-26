/* =====================================================================
   app.js — барлық беттерге ортақ функциялар:
   API-ға сұраныс (fetch), авторизация, тақырып (header), форматтау.
   ===================================================================== */

const API_BASE = '/api';

// ---------- Авторизация (токен localStorage-да сақталады) ----------
const Auth = {
  get token() {
    try { return localStorage.getItem('token'); } catch { return null; }
  },
  get user() {
    try { return JSON.parse(localStorage.getItem('user') || 'null'); } catch { return null; }
  },
  save(token, user) {
    try {
      localStorage.setItem('token', token);
      localStorage.setItem('user', JSON.stringify(user));
    } catch { /* жеке режимде localStorage жұмыс істемеуі мүмкін */ }
  },
  setUser(user) {
    try { localStorage.setItem('user', JSON.stringify(user)); } catch {}
  },
  clear() {
    try { localStorage.removeItem('token'); localStorage.removeItem('user'); } catch {}
  },
  isLoggedIn() { return !!this.token; },
  isAdmin() { return this.user?.role === 'admin'; },
};

class ApiError extends Error {
  constructor(message, status, fields) {
    super(message);
    this.status = status;
    this.fields = fields || {};
  }
}

/**
 * API-ға сұраныс жібереді. Қате болса ApiError лақтырады ({ error: "..." } форматы).
 *   await api('/products?q=samsung')
 *   await api('/cart', { method: 'POST', body: { product_id: 1, quantity: 2 } })
 */
async function api(path, { method = 'GET', body } = {}) {
  const headers = { Accept: 'application/json' };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (Auth.token) headers.Authorization = 'Bearer ' + Auth.token;

  let res;
  try {
    res = await fetch(API_BASE + path, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new ApiError('Серверге қосылу мүмкін болмады. Интернетті немесе серверді тексеріңіз.', 0);
  }

  if (res.status === 204) return null;
  let data = null;
  try { data = await res.json(); } catch { /* JSON емес жауап */ }

  if (!res.ok) {
    if (res.status === 401 && Auth.token) {
      Auth.clear(); // токен ескірген — қайта кіру керек
      renderHeader();
    }
    throw new ApiError(data?.error || `Қате: ${res.status}`, res.status, data?.fields);
  }
  return data;
}

// ---------- Көмекші функциялар ----------

/** HTML-ге қауіпсіз енгізу үшін мәтінді экрандау (XSS-тен қорғау). */
function esc(value) {
  return String(value ?? '').replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  })[c]);
}

const moneyFormat = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 2 });
function money(n) {
  return moneyFormat.format(Number(n) || 0) + ' ₸';
}

function formatDate(iso) {
  return new Date(iso).toLocaleString('ru-RU', {
    day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit',
  });
}

function stars(rating, outOf = 5) {
  const r = Math.round(Number(rating) || 0);
  let html = '<span class="stars" aria-label="Бағасы ' + r + '/5">';
  for (let i = 1; i <= outOf; i++) html += i <= r ? '★' : '<span class="off">★</span>';
  return html + '</span>';
}

const CATEGORY_ICONS = {
  smartphones: '📱', laptops: '💻', accessories: '🔌', headphones: '🎧',
};

/** Тауар суреті; сурет жоқ болса — санатқа сай белгіше. */
function productThumb(p, extraClass = '') {
  const icon = CATEGORY_ICONS[p.category_slug] || '📦';
  const img = p.image_url
    ? `<img src="${esc(p.image_url)}" alt="${esc(p.name)}" loading="lazy" onerror="this.replaceWith(document.createTextNode('${icon}'))">`
    : icon;
  return `<div class="thumb ${extraClass}">${img}</div>`;
}

function stockLabel(stock) {
  if (stock <= 0) return '<span class="stock out">Қоймада жоқ</span>';
  if (stock <= 3) return `<span class="stock low">Аз қалды: ${stock} дана</span>`;
  return '<span class="stock in">Қоймада бар</span>';
}

const STATUS_LABELS = {
  new: 'Жаңа', paid: 'Төленді', shipped: 'Жіберілді', delivered: 'Жеткізілді', cancelled: 'Болдырылмады',
};
const DELIVERY_LABELS = { courier: 'Курьер', pickup: 'Өзі алып кету' };
const PAYMENT_LABELS = { cash: 'Қолма-қол', card: 'Карта (алған кезде)' };

function statusBadge(status) {
  return `<span class="status ${esc(status)}">${esc(STATUS_LABELS[status] || status)}</span>`;
}

function toast(message, type = '') {
  let box = document.querySelector('.toasts');
  if (!box) {
    box = document.createElement('div');
    box.className = 'toasts';
    document.body.appendChild(box);
  }
  const el = document.createElement('div');
  el.className = 'toast ' + type;
  el.textContent = message;
  box.appendChild(el);
  setTimeout(() => el.remove(), 3500);
}

/** Кіру қажет беттерде: кірмеген болса login.html-ге жібереді. */
function requireLogin() {
  if (!Auth.isLoggedIn()) {
    location.href = 'login.html?next=' + encodeURIComponent(location.pathname + location.search);
    return false;
  }
  return true;
}

/** Формадағы өріс қателерін көрсету (сервер қайтарған fields бойынша). */
function showFieldErrors(form, fields = {}) {
  form.querySelectorAll('.field-error').forEach((el) => el.remove());
  for (const [name, msg] of Object.entries(fields)) {
    const input = form.elements[name];
    if (!input || !input.closest) continue;
    const div = document.createElement('div');
    div.className = 'field-error';
    div.textContent = msg;
    (input.closest('.field') || input.parentElement).appendChild(div);
  }
}

/** Тауарды себетке қосу (кірмеген болса — кіру бетіне). */
async function addToCart(productId, quantity = 1) {
  if (!requireLogin()) return false;
  try {
    const cart = await api('/cart', { method: 'POST', body: { product_id: productId, quantity } });
    setCartBadge(cart.total_items);
    toast('Себетке қосылды', 'success');
    return true;
  } catch (e) {
    toast(e.message, 'error');
    return false;
  }
}

// ---------- Header & footer ----------

function setCartBadge(n) {
  const el = document.getElementById('cart-count');
  if (!el) return;
  el.textContent = n;
  el.classList.toggle('hidden', !n);
}

async function refreshCartBadge() {
  if (!Auth.isLoggedIn()) return setCartBadge(0);
  try {
    const cart = await api('/cart');
    setCartBadge(cart.total_items);
  } catch { /* бейдж маңызды емес */ }
}

function renderHeader() {
  const header = document.getElementById('site-header');
  if (!header) return;
  const page = location.pathname.split('/').pop() || 'index.html';
  const user = Auth.user;
  const q = new URLSearchParams(location.search).get('q') || '';
  const active = (name) => (page === name ? ' class="active"' : '');

  header.className = 'site-header';
  header.innerHTML = `
    <div class="container">
      <a href="index.html" class="logo">Tech<span>Shop</span></a>
      <form class="search" id="search-form" role="search">
        <input type="search" name="q" placeholder="Тауарды іздеу..." value="${esc(q)}" aria-label="Іздеу">
        <button class="btn" type="submit">Іздеу</button>
      </form>
      <nav class="nav">
        <a href="index.html"${active('index.html')}>Каталог</a>
        <a href="cart.html"${active('cart.html')}>Себет<span id="cart-count" class="badge hidden">0</span></a>
        ${user ? `
          ${user.role === 'admin' ? `<a href="admin.html"${active('admin.html')}>Әкімші</a>` : ''}
          <a href="account.html"${active('account.html')} title="${esc(user.email)}">Кабинет</a>
          <button class="link" id="logout-btn" type="button">Шығу</button>
        ` : `<a href="login.html"${active('login.html')}>Кіру</a>`}
      </nav>
    </div>`;

  header.querySelector('#search-form').addEventListener('submit', (e) => {
    e.preventDefault();
    const value = e.target.q.value.trim();
    // Каталог бетінде catalog.js өзі өңдейді (window.onSearch), басқа беттерде — каталогқа өтеміз
    if (typeof window.onSearch === 'function') window.onSearch(value);
    else location.href = 'index.html' + (value ? '?q=' + encodeURIComponent(value) : '');
  });
  header.querySelector('#logout-btn')?.addEventListener('click', () => {
    Auth.clear();
    location.href = 'index.html';
  });
  refreshCartBadge();
}

function renderFooter() {
  const footer = document.getElementById('site-footer');
  if (!footer) return;
  footer.className = 'site-footer';
  footer.innerHTML = `
    <div class="container small muted">
      <span>© ${new Date().getFullYear()} TechShop — электроника және аксессуарлар</span>
      <span>Жеткізу: курьер ${money(1500)}, ${money(50000)}-ден бастап тегін</span>
    </div>`;
}

document.addEventListener('DOMContentLoaded', () => {
  renderHeader();
  renderFooter();
});
