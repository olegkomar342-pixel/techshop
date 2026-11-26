/* =====================================================================
   account.js — жеке кабинет: тапсырыстар тарихы және профильді өңдеу
   ===================================================================== */

if (requireLogin()) {
  loadOrders();
  loadProfile();
}

document.querySelector('.tabs').addEventListener('click', (e) => {
  const b = e.target.closest('[data-tab]');
  if (!b) return;
  document.querySelectorAll('.tabs button').forEach((t) => t.classList.toggle('active', t === b));
  document.getElementById('tab-orders').classList.toggle('hidden', b.dataset.tab !== 'orders');
  document.getElementById('tab-profile').classList.toggle('hidden', b.dataset.tab !== 'profile');
});

async function loadOrders() {
  const box = document.getElementById('orders');
  let orders;
  try {
    orders = await api('/orders');
  } catch (e) {
    box.innerHTML = `<div class="alert error">${esc(e.message)}</div>`;
    return;
  }
  if (!orders.length) {
    box.innerHTML = `<div class="panel empty"><div class="icon">📦</div>
      <p>Сізде әлі тапсырыс жоқ.</p><a class="btn" href="index.html">Каталогқа өту</a></div>`;
    return;
  }
  box.innerHTML = orders.map(orderCard).join('');
}

function orderCard(o) {
  return `
    <div class="order panel">
      <div class="order-head">
        <div><strong>Тапсырыс №${o.id}</strong> <span class="muted small">· ${formatDate(o.created_at)}</span></div>
        ${statusBadge(o.status)}
      </div>
      <ul>
        ${o.items.map((i) => `<li>${i.product_id
          ? `<a href="product.html?id=${i.product_id}">${esc(i.product_name)}</a>`
          : esc(i.product_name)} × ${i.quantity} — ${money(i.subtotal)}</li>`).join('')}
      </ul>
      <div class="small muted">
        ${esc(DELIVERY_LABELS[o.delivery_method])}: ${esc(o.shipping_address)} ·
        Төлем: ${esc(PAYMENT_LABELS[o.payment_method])} ·
        Жеткізу: ${o.delivery_cost ? money(o.delivery_cost) : 'тегін'}
      </div>
      <div class="order-head" style="margin:10px 0 0">
        <strong>Барлығы: ${money(o.total)}</strong>
        ${o.status === 'new' ? `<button class="btn secondary sm" data-cancel="${o.id}">Тапсырысты болдырмау</button>` : ''}
      </div>
    </div>`;
}

document.getElementById('orders').addEventListener('click', async (e) => {
  const b = e.target.closest('[data-cancel]');
  if (!b || !confirm(`№${b.dataset.cancel} тапсырысты болдырмау керек пе?`)) return;
  b.disabled = true;
  try {
    await api(`/orders/${b.dataset.cancel}/cancel`, { method: 'POST' });
    toast('Тапсырыс болдырылмады', 'success');
    loadOrders();
  } catch (err) {
    toast(err.message, 'error');
    b.disabled = false;
  }
});

const form = document.getElementById('profile-form');

async function loadProfile() {
  try {
    const me = await api('/me');
    Auth.setUser(me);
    form.email.value = me.email;
    form.full_name.value = me.full_name;
    form.phone.value = me.phone || '';
  } catch (e) {
    toast(e.message, 'error');
  }
}

form.addEventListener('submit', async (e) => {
  e.preventDefault();
  showFieldErrors(form, {});
  try {
    const me = await api('/me', {
      method: 'PUT',
      body: {
        full_name: form.full_name.value,
        phone: form.phone.value,
        current_password: form.current_password.value,
        new_password: form.new_password.value,
      },
    });
    Auth.setUser(me);
    form.current_password.value = '';
    form.new_password.value = '';
    toast('Профиль сақталды', 'success');
  } catch (err) {
    showFieldErrors(form, err.fields);
    toast(err.message, 'error');
  }
});
