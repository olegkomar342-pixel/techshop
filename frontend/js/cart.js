/* =====================================================================
   cart.js — себет және тапсырысты рәсімдеу
   ===================================================================== */

// Бэкендтегі шарттармен бірдей (orders.go: CourierCost, FreeDeliveryFrom)
const COURIER_COST = 1500;
const FREE_DELIVERY_FROM = 50000;

const root = document.getElementById('cart-root');
let cart = null;
let me = null;

async function init() {
  if (!requireLogin()) return;
  try {
    [cart, me] = await Promise.all([api('/cart'), api('/me')]);
    Auth.setUser(me);
  } catch (e) {
    root.innerHTML = `<div class="empty"><div class="icon">⚠️</div>${esc(e.message)}</div>`;
    return;
  }
  render();
}

function deliveryCost(method, subtotal) {
  return method === 'courier' && subtotal < FREE_DELIVERY_FROM ? COURIER_COST : 0;
}

function render() {
  setCartBadge(cart.total_items);
  if (!cart.items.length) {
    root.innerHTML = `
      <h1>Себет</h1>
      <div class="panel empty"><div class="icon">🛒</div>
        <p>Себет бос.</p>
        <a class="btn" href="index.html">Каталогқа өту</a>
      </div>`;
    return;
  }

  const problems = cart.items.filter((i) => !i.is_active || i.quantity > i.stock);

  root.innerHTML = `
    <h1>Себет</h1>
    <div class="cart-layout">
      <div>
        <div class="panel" id="items">${cart.items.map(cartRow).join('')}
          <div style="text-align:right;margin-top:8px">
            <button class="btn ghost sm" id="clear-cart">Себетті тазалау</button>
          </div>
        </div>
      </div>

      <form class="panel" id="checkout" novalidate>
        <h2>Тапсырысты рәсімдеу</h2>
        ${problems.length ? `<div class="alert error">Кейбір тауарлар қоймада жеткіліксіз немесе сатылымнан алынған. Санын азайтыңыз немесе жойыңыз.</div>` : ''}
        <div class="field">
          <label for="full_name">Аты-жөні</label>
          <input id="full_name" name="full_name" required value="${esc(me.full_name)}">
        </div>
        <div class="field">
          <label for="phone">Телефон</label>
          <input id="phone" name="phone" type="tel" required placeholder="+7 700 000 00 00" value="${esc(me.phone || '')}">
        </div>

        <div class="field">
          <label>Жеткізу тәсілі</label>
          <div class="choice">
            <label><input type="radio" name="delivery_method" value="courier" checked>
              <span>Курьер<small>${money(COURIER_COST)}, ${money(FREE_DELIVERY_FROM)}-ден тегін</small></span></label>
            <label><input type="radio" name="delivery_method" value="pickup">
              <span>Өзі алып кету<small>Тегін, дүкеннен</small></span></label>
          </div>
        </div>
        <div class="field" id="address-field">
          <label for="shipping_address">Жеткізу мекенжайы</label>
          <textarea id="shipping_address" name="shipping_address" rows="2" placeholder="Қала, көше, үй, пәтер"></textarea>
        </div>

        <div class="field">
          <label>Төлем тәсілі</label>
          <div class="choice">
            <label><input type="radio" name="payment_method" value="cash" checked>
              <span>Қолма-қол<small>Алған кезде</small></span></label>
            <label><input type="radio" name="payment_method" value="card">
              <span>Карта<small>Алған кезде терминал арқылы</small></span></label>
          </div>
        </div>
        <div class="field">
          <label for="comment">Түсініктеме <span class="muted">(міндетті емес)</span></label>
          <textarea id="comment" name="comment" rows="2" maxlength="500"></textarea>
        </div>

        <div id="summary"></div>
        <button class="btn block" type="submit" id="submit-order" ${problems.length ? 'disabled' : ''}>Тапсырыс беру</button>
      </form>
    </div>`;

  renderSummary();
  bindEvents();
}

function cartRow(i) {
  let warn = '';
  if (!i.is_active) warn = '<div class="stock out">Сатылымнан алынды</div>';
  else if (i.quantity > i.stock) warn = `<div class="stock out">Қоймада тек ${i.stock} дана бар</div>`;
  return `
    <div class="cart-item" data-id="${i.product_id}">
      ${productThumb(i, 'sm')}
      <div class="info">
        <a href="product.html?id=${i.product_id}"><strong>${esc(i.name)}</strong></a>
        <div class="muted small">${money(i.price)} / дана</div>
        ${warn}
      </div>
      <div class="qty sm">
        <button type="button" data-step="-1" aria-label="Азайту">−</button>
        <input type="number" value="${i.quantity}" min="1" max="${Math.max(1, Math.min(99, i.stock))}" data-qty aria-label="Саны">
        <button type="button" data-step="1" aria-label="Көбейту">+</button>
      </div>
      <div class="sum">${money(i.subtotal)}</div>
      <button class="btn ghost sm" data-remove aria-label="Жою" title="Жою">✕</button>
    </div>`;
}

function renderSummary() {
  const form = document.getElementById('checkout');
  const method = form.delivery_method.value;
  const delivery = deliveryCost(method, cart.total);
  document.getElementById('address-field').classList.toggle('hidden', method === 'pickup');
  document.getElementById('summary').innerHTML = `
    <div class="summary-line"><span>Тауарлар (${cart.total_items} дана)</span><span>${money(cart.total)}</span></div>
    <div class="summary-line"><span>Жеткізу</span><span>${delivery ? money(delivery) : 'Тегін'}</span></div>
    <div class="summary-line total"><span>Барлығы</span><span>${money(cart.total + delivery)}</span></div>
    ${method === 'courier' && delivery ? `<p class="small muted">Тегін жеткізуге дейін ${money(FREE_DELIVERY_FROM - cart.total)} қалды</p>` : ''}`;
}

async function updateQty(productId, quantity) {
  try {
    cart = await api('/cart/' + productId, { method: 'PUT', body: { quantity } });
    render();
  } catch (e) {
    toast(e.message, 'error');
    render(); // мәнді бұрынғысына қайтарамыз
  }
}

function bindEvents() {
  const items = document.getElementById('items');

  items.addEventListener('click', async (e) => {
    const row = e.target.closest('.cart-item');
    if (e.target.closest('[data-step]') && row) {
      const input = row.querySelector('[data-qty]');
      const next = Number(input.value) + Number(e.target.closest('[data-step]').dataset.step);
      if (next >= 1) updateQty(row.dataset.id, next);
    } else if (e.target.closest('[data-remove]') && row) {
      try {
        cart = await api('/cart/' + row.dataset.id, { method: 'DELETE' });
        render();
      } catch (err) { toast(err.message, 'error'); }
    }
  });

  items.addEventListener('change', (e) => {
    const input = e.target.closest('[data-qty]');
    if (!input) return;
    const n = Math.floor(Number(input.value));
    if (n >= 1) updateQty(input.closest('.cart-item').dataset.id, n);
    else render();
  });

  document.getElementById('clear-cart').addEventListener('click', async () => {
    if (!confirm('Себеттегі барлық тауарды жою керек пе?')) return;
    try {
      cart = await api('/cart', { method: 'DELETE' });
      render();
    } catch (err) { toast(err.message, 'error'); }
  });

  const form = document.getElementById('checkout');
  form.querySelectorAll('[name="delivery_method"]').forEach((r) => r.addEventListener('change', renderSummary));

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const btn = document.getElementById('submit-order');
    btn.disabled = true;
    btn.textContent = 'Жіберілуде...';
    showFieldErrors(form, {});
    try {
      const order = await api('/orders', {
        method: 'POST',
        body: {
          full_name: form.full_name.value,
          phone: form.phone.value,
          delivery_method: form.delivery_method.value,
          shipping_address: form.shipping_address.value,
          payment_method: form.payment_method.value,
          comment: form.comment.value,
        },
      });
      showSuccess(order);
    } catch (err) {
      showFieldErrors(form, err.fields);
      toast(err.message, 'error');
      btn.disabled = false;
      btn.textContent = 'Тапсырыс беру';
      // Қалдық өзгерген болуы мүмкін — себетті жаңартамыз
      if (err.status === 400 && !Object.keys(err.fields).length) {
        cart = await api('/cart').catch(() => cart);
        render();
      }
    }
  });
}

function showSuccess(order) {
  setCartBadge(0);
  root.innerHTML = `
    <div class="panel" style="max-width:640px;margin:0 auto;text-align:center">
      <div style="font-size:56px">✅</div>
      <h1>Тапсырыс №${order.id} қабылданды!</h1>
      <p class="muted">Менеджер жақын арада ${esc(order.phone)} нөміріне хабарласады.</p>
      <div style="text-align:left;margin:20px 0">
        ${order.items.map((i) => `<div class="summary-line"><span>${esc(i.product_name)} × ${i.quantity}</span><span>${money(i.subtotal)}</span></div>`).join('')}
        <div class="summary-line"><span>Жеткізу (${esc(DELIVERY_LABELS[order.delivery_method])})</span><span>${order.delivery_cost ? money(order.delivery_cost) : 'Тегін'}</span></div>
        <div class="summary-line total"><span>Барлығы</span><span>${money(order.total)}</span></div>
      </div>
      <a class="btn" href="account.html">Менің тапсырыстарым</a>
      <a class="btn secondary" href="index.html">Сатып алуды жалғастыру</a>
    </div>`;
}

init();
