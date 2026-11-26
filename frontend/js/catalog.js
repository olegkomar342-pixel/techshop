/* =====================================================================
   catalog.js — каталог беті: санаттар, іздеу, сүзгілер, сұрыптау, беттерге бөлу
   ===================================================================== */

// Беттің күйі URL параметрлерінде сақталады — сілтемені бөлісуге және «Артқа» батырмасына ыңғайлы.
const state = readState();
let categories = [];

function readState() {
  const p = new URLSearchParams(location.search);
  return {
    q: p.get('q') || '',
    category: p.get('category') || '',
    min_price: p.get('min_price') || '',
    max_price: p.get('max_price') || '',
    in_stock: p.get('in_stock') || '',
    sort: p.get('sort') || 'newest',
    page: Number(p.get('page')) || 1,
  };
}

function stateToQuery(extra = {}) {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries({ ...state, ...extra })) {
    if (v !== '' && v !== null && !(k === 'page' && v === 1) && !(k === 'sort' && v === 'newest')) p.set(k, v);
  }
  return p.toString();
}

function syncUrl() {
  const qs = stateToQuery();
  history.replaceState(null, '', qs ? '?' + qs : location.pathname);
}

async function loadCategories() {
  try {
    categories = await api('/categories');
  } catch (e) {
    document.getElementById('categories').innerHTML = `<li class="field-error">${esc(e.message)}</li>`;
    return;
  }
  renderCategories();
}

function renderCategories() {
  const total = categories.reduce((sum, c) => sum + c.product_count, 0);
  // Ата-санаттың санына ішкі санаттардағы тауарлар да кіреді (API де солай сүзеді)
  const countWithChildren = (c) =>
    c.product_count + categories.filter((x) => x.parent_id === c.id).reduce((s, x) => s + countWithChildren(x), 0);
  const item = (c) => `
    <li class="${c.parent_id ? 'sub' : ''}">
      <a href="#" data-id="${c.id}" class="${String(c.id) === state.category ? 'active' : ''}">
        <span>${CATEGORY_ICONS[c.slug] || '📦'} ${esc(c.name)}</span>
        <span class="count">${countWithChildren(c)}</span>
      </a>
    </li>`;
  document.getElementById('categories').innerHTML = `
    <li><a href="#" data-id="" class="${state.category === '' ? 'active' : ''}">
      <span>Барлығы</span><span class="count">${total}</span></a></li>
    ${categories.map(item).join('')}`;
}

async function loadProducts() {
  const grid = document.getElementById('products');
  grid.innerHTML = '<div class="loading">Жүктелуде...</div>';
  syncUrl();
  updateTitle();

  let data;
  try {
    data = await api('/products?' + stateToQuery({ limit: 12 }));
  } catch (e) {
    grid.innerHTML = `<div class="empty"><div class="icon">⚠️</div>${esc(e.message)}</div>`;
    document.getElementById('pagination').innerHTML = '';
    return;
  }

  document.getElementById('result-info').textContent =
    data.total ? `Табылды: ${data.total} тауар` : '';

  if (!data.items.length) {
    grid.innerHTML = `<div class="empty" style="grid-column:1/-1"><div class="icon">🔍</div>
      Тауарлар табылмады. Сүзгілерді өзгертіп көріңіз.</div>`;
  } else {
    grid.innerHTML = data.items.map(productCard).join('');
  }
  renderPagination(data.page, data.pages);
}

function productCard(p) {
  return `
    <article class="card">
      <a class="card-link" href="product.html?id=${p.id}">
        ${productThumb(p)}
        <div class="card-body">
          <span class="card-cat">${esc(p.category_name || 'Санатсыз')}</span>
          <h3 class="card-title">${esc(p.name)}</h3>
          <div class="small">
            ${p.reviews_count ? `${stars(p.avg_rating)} <span class="muted">${p.avg_rating} (${p.reviews_count})</span>`
                              : '<span class="muted">Пікір жоқ</span>'}
          </div>
          ${stockLabel(p.stock)}
        </div>
      </a>
      <div class="card-footer">
        <span class="price">${money(p.price)}</span>
        <button class="btn sm" data-add="${p.id}" ${p.stock <= 0 ? 'disabled' : ''}>Себетке</button>
      </div>
    </article>`;
}

function renderPagination(page, pages) {
  const box = document.getElementById('pagination');
  if (pages <= 1) { box.innerHTML = ''; return; }
  let html = `<button data-page="${page - 1}" ${page <= 1 ? 'disabled' : ''} aria-label="Алдыңғы">‹</button>`;
  for (let i = 1; i <= pages; i++) {
    // Көп бет болса, тек айналасындағыларды көрсетеміз
    if (pages > 7 && i !== 1 && i !== pages && Math.abs(i - page) > 1) {
      if (i === 2 || i === pages - 1) html += '<button disabled>…</button>';
      continue;
    }
    html += `<button data-page="${i}" class="${i === page ? 'active' : ''}">${i}</button>`;
  }
  html += `<button data-page="${page + 1}" ${page >= pages ? 'disabled' : ''} aria-label="Келесі">›</button>`;
  box.innerHTML = html;
}

function updateTitle() {
  const cat = categories.find((c) => String(c.id) === state.category);
  let title = cat ? cat.name : 'Барлық тауарлар';
  if (state.q) title = `«${state.q}» бойынша іздеу`;
  document.getElementById('catalog-title').textContent = title;
  document.title = title + ' — TechShop';
}

function fillFilterForm() {
  const f = document.getElementById('filters');
  f.min_price.value = state.min_price;
  f.max_price.value = state.max_price;
  f.in_stock.checked = state.in_stock === '1';
  document.getElementById('sort').value = state.sort;
}

// ---------- Оқиғалар ----------

window.onSearch = (q) => {
  state.q = q;
  state.page = 1;
  loadProducts();
};

document.getElementById('categories').addEventListener('click', (e) => {
  const a = e.target.closest('a[data-id]');
  if (!a) return;
  e.preventDefault();
  state.category = a.dataset.id;
  state.page = 1;
  renderCategories();
  loadProducts();
});

document.getElementById('filters').addEventListener('submit', (e) => {
  e.preventDefault();
  const f = e.target;
  state.min_price = f.min_price.value;
  state.max_price = f.max_price.value;
  state.in_stock = f.in_stock.checked ? '1' : '';
  state.page = 1;
  loadProducts();
});

document.getElementById('reset-filters').addEventListener('click', () => {
  Object.assign(state, { q: '', category: '', min_price: '', max_price: '', in_stock: '', sort: 'newest', page: 1 });
  fillFilterForm();
  const search = document.querySelector('#search-form input');
  if (search) search.value = '';
  renderCategories();
  loadProducts();
});

document.getElementById('sort').addEventListener('change', (e) => {
  state.sort = e.target.value;
  state.page = 1;
  loadProducts();
});

document.getElementById('pagination').addEventListener('click', (e) => {
  const b = e.target.closest('button[data-page]');
  if (!b || b.disabled) return;
  state.page = Number(b.dataset.page);
  loadProducts();
  window.scrollTo({ top: 0, behavior: 'smooth' });
});

document.getElementById('products').addEventListener('click', async (e) => {
  const b = e.target.closest('button[data-add]');
  if (!b) return;
  b.disabled = true;
  await addToCart(Number(b.dataset.add), 1);
  b.disabled = false;
});

fillFilterForm();
loadCategories().then(updateTitle);
loadProducts();
