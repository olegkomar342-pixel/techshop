/* =====================================================================
   admin.js — әкімші панелі: статистика, тапсырыстар, тауарлар, санаттар
   (Рұқсатты сервер тексереді; мұндағы тексеру тек интерфейс үшін.)
   ===================================================================== */

let categories = [];
let products = [];

if (requireLogin()) {
  if (!Auth.isAdmin()) {
    document.getElementById('admin-root').innerHTML =
      '<div class="panel empty"><div class="icon">🔒</div>Бұл бет тек әкімшіге қолжетімді.</div>';
  } else {
    init();
  }
}

async function init() {
  const filter = document.getElementById('order-filter');
  for (const [value, label] of Object.entries(STATUS_LABELS)) {
    filter.insertAdjacentHTML('beforeend', `<option value="${value}">${esc(label)}</option>`);
  }
  await loadCategories();
  loadStats();
  loadOrders();
  loadProducts();
}

// ---------- Табтар ----------
document.querySelector('.tabs').addEventListener('click', (e) => {
  const b = e.target.closest('[data-tab]');
  if (!b) return;
  document.querySelectorAll('.tabs button').forEach((t) => t.classList.toggle('active', t === b));
  for (const name of ['orders', 'products', 'categories']) {
    document.getElementById('tab-' + name).classList.toggle('hidden', b.dataset.tab !== name);
  }
});

// ---------- Статистика ----------
async function loadStats() {
  try {
    const s = await api('/admin/stats');
    const stat = (value, label) => `<div class="stat"><div class="value">${value}</div><div class="label">${label}</div></div>`;
    document.getElementById('stats').innerHTML =
      stat(money(s.revenue), 'Түсім (төленген)') +
      stat(s.orders_by_status.new, 'Жаңа тапсырыстар') +
      stat(`${s.active_products} / ${s.products}`, 'Белсенді тауарлар') +
      stat(s.low_stock, 'Аз қалған тауарлар (≤3)') +
      stat(s.users, 'Пайдаланушылар');
  } catch (e) {
    toast(e.message, 'error');
  }
}

// ---------- Тапсырыстар ----------
async function loadOrders() {
  const body = document.getElementById('orders-body');
  const status = document.getElementById('order-filter').value;
  try {
    const orders = await api('/admin/orders' + (status ? '?status=' + status : ''));
    body.innerHTML = orders.length ? orders.map(orderRow).join('')
      : '<tr><td colspan="6" class="empty">Тапсырыс жоқ</td></tr>';
  } catch (e) {
    body.innerHTML = `<tr><td colspan="6" class="field-error">${esc(e.message)}</td></tr>`;
  }
}

function orderRow(o) {
  const options = Object.entries(STATUS_LABELS)
    .map(([v, l]) => `<option value="${v}" ${v === o.status ? 'selected' : ''}>${esc(l)}</option>`).join('');
  return `
    <tr>
      <td><strong>${o.id}</strong></td>
      <td class="small">${formatDate(o.created_at)}</td>
      <td>${esc(o.full_name)}<div class="small muted">${esc(o.phone)}<br>${esc(o.user_email || '')}</div>
          <div class="small muted">${esc(DELIVERY_LABELS[o.delivery_method])}: ${esc(o.shipping_address)}</div>
          ${o.comment ? `<div class="small">💬 ${esc(o.comment)}</div>` : ''}</td>
      <td class="small">${o.items.map((i) => `${esc(i.product_name)} × ${i.quantity}`).join('<br>')}</td>
      <td class="num"><strong>${money(o.total)}</strong><div class="small muted">${esc(PAYMENT_LABELS[o.payment_method])}</div></td>
      <td>
        ${statusBadge(o.status)}
        ${o.status === 'cancelled' ? '' : `<select data-order="${o.id}" style="margin-top:6px;width:auto" aria-label="Күйді өзгерту">${options}</select>`}
      </td>
    </tr>`;
}

document.getElementById('order-filter').addEventListener('change', loadOrders);

document.getElementById('orders-body').addEventListener('change', async (e) => {
  const sel = e.target.closest('[data-order]');
  if (!sel) return;
  if (sel.value === 'cancelled' && !confirm('Тапсырысты болдырмау керек пе? Тауарлар қоймаға қайтарылады.')) {
    loadOrders();
    return;
  }
  try {
    await api(`/admin/orders/${sel.dataset.order}/status`, { method: 'PUT', body: { status: sel.value } });
    toast(`№${sel.dataset.order} күйі: ${STATUS_LABELS[sel.value]}`, 'success');
    loadOrders();
    loadStats();
    loadProducts(); // болдырылмаса, қалдық өзгереді
  } catch (err) {
    toast(err.message, 'error');
    loadOrders();
  }
});

// ---------- Тауарлар ----------
async function loadProducts() {
  const q = document.getElementById('product-search').value.trim();
  try {
    const page = await api('/admin/products?limit=100' + (q ? '&q=' + encodeURIComponent(q) : ''));
    products = page.items;
  } catch (e) {
    toast(e.message, 'error');
    return;
  }
  document.getElementById('products-body').innerHTML = products.length ? products.map((p) => `
    <tr class="${p.is_active ? '' : 'inactive'}">
      <td>${p.id}</td>
      <td><a href="product.html?id=${p.id}" target="_blank">${esc(p.name)}</a></td>
      <td>${esc(p.category_name || '—')}</td>
      <td class="num">${money(p.price)}</td>
      <td class="num">${p.stock <= 3 ? `<span class="stock ${p.stock ? 'low' : 'out'}">${p.stock}</span>` : p.stock}</td>
      <td>${p.is_active ? '<span class="status delivered">Белсенді</span>' : '<span class="status">Жасырылған</span>'}</td>
      <td style="white-space:nowrap">
        <button class="btn secondary sm" data-edit="${p.id}">Өңдеу</button>
        <button class="btn ghost sm" data-toggle="${p.id}">${p.is_active ? 'Жасыру' : 'Көрсету'}</button>
      </td>
    </tr>`).join('') : '<tr><td colspan="7" class="empty">Тауар жоқ</td></tr>';
}

let searchTimer;
document.getElementById('product-search').addEventListener('input', () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(loadProducts, 300);
});

const dialog = document.getElementById('product-dialog');
const pform = document.getElementById('product-form');

function categoryOptions(selected, emptyLabel) {
  return `<option value="">${emptyLabel}</option>` + categories.map((c) =>
    `<option value="${c.id}" ${c.id === selected ? 'selected' : ''}>${c.parent_id ? '— ' : ''}${esc(c.name)}</option>`).join('');
}

function openProductDialog(p) {
  showFieldErrors(pform, {});
  document.getElementById('product-dialog-title').textContent = p ? `Тауарды өңдеу (#${p.id})` : 'Жаңа тауар';
  pform.elements.id.value = p?.id || '';
  pform.elements.name.value = p?.name || '';
  pform.category_id.innerHTML = categoryOptions(p?.category_id ?? null, 'Санатсыз');
  pform.price.value = p?.price ?? '';
  pform.stock.value = p?.stock ?? 0;
  pform.image_url.value = p?.image_url || '';
  pform.description.value = p?.description || '';
  pform.is_active.checked = p ? p.is_active : true;
  dialog.showModal();
}

function productBody(p) {
  return {
    name: p.name,
    category_id: p.category_id,
    price: p.price,
    stock: p.stock,
    image_url: p.image_url || '',
    description: p.description,
    is_active: p.is_active,
  };
}

document.getElementById('new-product').addEventListener('click', () => openProductDialog(null));
document.getElementById('product-cancel').addEventListener('click', () => dialog.close());

document.getElementById('products-body').addEventListener('click', async (e) => {
  const edit = e.target.closest('[data-edit]');
  const toggle = e.target.closest('[data-toggle]');
  if (edit) {
    openProductDialog(products.find((p) => p.id === Number(edit.dataset.edit)));
  } else if (toggle) {
    const p = products.find((x) => x.id === Number(toggle.dataset.toggle));
    try {
      await api('/admin/products/' + p.id, { method: 'PUT', body: { ...productBody(p), is_active: !p.is_active } });
      toast(p.is_active ? 'Тауар каталогтан жасырылды' : 'Тауар каталогқа қайтарылды', 'success');
      loadProducts();
      loadStats();
    } catch (err) {
      toast(err.message, 'error');
    }
  }
});

pform.addEventListener('submit', async (e) => {
  e.preventDefault();
  showFieldErrors(pform, {});
  const id = pform.elements.id.value;
  const body = {
    name: pform.elements.name.value,
    category_id: pform.category_id.value ? Number(pform.category_id.value) : null,
    price: Number(pform.price.value),
    stock: Math.floor(Number(pform.stock.value)),
    image_url: pform.image_url.value,
    description: pform.description.value,
    is_active: pform.is_active.checked,
  };
  if (pform.price.value === '') return showFieldErrors(pform, { price: 'міндетті өріс' });
  try {
    await api(id ? '/admin/products/' + id : '/admin/products', { method: id ? 'PUT' : 'POST', body });
    dialog.close();
    toast(id ? 'Тауар сақталды' : 'Тауар қосылды', 'success');
    loadProducts();
    loadStats();
  } catch (err) {
    showFieldErrors(pform, err.fields);
    toast(err.message, 'error');
  }
});

// ---------- Санаттар ----------
async function loadCategories() {
  try {
    categories = await api('/categories');
  } catch (e) {
    toast(e.message, 'error');
    return;
  }
  const byId = Object.fromEntries(categories.map((c) => [c.id, c]));
  document.getElementById('categories-body').innerHTML = categories.map((c) => `
    <tr>
      <td>${c.id}</td>
      <td>${CATEGORY_ICONS[c.slug] || '📦'} ${esc(c.name)}</td>
      <td><code>${esc(c.slug)}</code></td>
      <td>${c.parent_id ? esc(byId[c.parent_id]?.name || '') : '—'}</td>
      <td class="num">${c.product_count}</td>
      <td><button class="btn ghost sm" data-delete-cat="${c.id}">Жою</button></td>
    </tr>`).join('');
  document.getElementById('c-parent').innerHTML =
    categoryOptions(null, 'Жоқ (негізгі санат)');
}

document.getElementById('categories-body').addEventListener('click', async (e) => {
  const b = e.target.closest('[data-delete-cat]');
  if (!b) return;
  const c = categories.find((x) => x.id === Number(b.dataset.deleteCat));
  if (!confirm(`«${c.name}» санатын жою керек пе? Оның тауарлары «Санатсыз» болып қалады.`)) return;
  try {
    await api('/admin/categories/' + c.id, { method: 'DELETE' });
    toast('Санат жойылды', 'success');
    await loadCategories();
    loadProducts();
  } catch (err) {
    toast(err.message, 'error');
  }
});

const cform = document.getElementById('category-form');
cform.addEventListener('submit', async (e) => {
  e.preventDefault();
  showFieldErrors(cform, {});
  try {
    await api('/admin/categories', {
      method: 'POST',
      body: {
        name: cform.elements.name.value,
        slug: cform.slug.value,
        parent_id: cform.parent_id.value ? Number(cform.parent_id.value) : null,
      },
    });
    cform.reset();
    toast('Санат қосылды', 'success');
    loadCategories();
  } catch (err) {
    showFieldErrors(cform, err.fields);
    toast(err.message, 'error');
  }
});
