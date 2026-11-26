/* =====================================================================
   product.js — тауар беті: сипаттама, себетке қосу, пікірлер мен бағалар
   ===================================================================== */

const productId = Number(new URLSearchParams(location.search).get('id'));
const root = document.getElementById('product-root');
let product = null;

async function loadProduct() {
  if (!productId) {
    root.innerHTML = '<div class="empty"><div class="icon">❓</div>Тауар көрсетілмеген. <a href="index.html">Каталогқа оралу</a></div>';
    return;
  }
  try {
    product = await api('/products/' + productId);
  } catch (e) {
    root.innerHTML = `<div class="empty"><div class="icon">${e.status === 404 ? '🔍' : '⚠️'}</div>
      ${esc(e.message)}. <a href="index.html">Каталогқа оралу</a></div>`;
    return;
  }
  document.title = product.name + ' — TechShop';
  render();
}

function render() {
  const p = product;
  const me = Auth.user;
  const myReview = me ? p.reviews.find((r) => r.user_id === me.id) : null;

  root.innerHTML = `
    <div class="breadcrumbs">
      <a href="index.html">Каталог</a>
      ${p.category_id ? ` / <a href="index.html?category=${p.category_id}">${esc(p.category_name)}</a>` : ''}
      / ${esc(p.name)}
    </div>

    <div class="product">
      ${productThumb(p)}
      <div>
        <h1>${esc(p.name)}</h1>
        <div>
          ${p.reviews_count
            ? `${stars(p.avg_rating)} <strong>${p.avg_rating}</strong> <span class="muted">· ${p.reviews_count} пікір</span>`
            : '<span class="muted">Әзірге пікір жоқ</span>'}
        </div>
        <div class="price">${money(p.price)}</div>
        ${stockLabel(p.stock)}

        <div class="buy">
          <div class="qty">
            <button type="button" data-step="-1" aria-label="Азайту">−</button>
            <input type="number" id="qty" value="1" min="1" max="${Math.max(1, Math.min(p.stock, 99))}" aria-label="Саны">
            <button type="button" data-step="1" aria-label="Көбейту">+</button>
          </div>
          <button class="btn" id="add-btn" ${p.stock <= 0 ? 'disabled' : ''}>Себетке қосу</button>
        </div>

        <div class="panel">
          <h3>Сипаттамасы</h3>
          <p class="description" style="margin:0">${esc(p.description) || '<span class="muted">Сипаттама жоқ</span>'}</p>
        </div>
      </div>
    </div>

    <section class="reviews panel">
      <h2>Пікірлер (${p.reviews.length})</h2>
      ${me ? reviewForm(myReview) : `<p class="muted"><a href="login.html?next=${encodeURIComponent(location.pathname + location.search)}">Кіріңіз</a>, пікір қалдыру үшін.</p>`}
      <div id="reviews-list">
        ${p.reviews.length ? p.reviews.map((r) => reviewItem(r, me)).join('') : '<p class="muted">Бірінші болып пікір қалдырыңыз!</p>'}
      </div>
    </section>`;

  bindEvents(myReview);
}

function reviewForm(myReview) {
  return `
    <form id="review-form" class="panel" style="box-shadow:none;margin-bottom:16px">
      <h3>${myReview ? 'Пікіріңізді өзгерту' : 'Пікір қалдыру'}</h3>
      <div class="field">
        <label>Бағаңыз</label>
        <div class="star-input" id="star-input">
          ${[1, 2, 3, 4, 5].map((i) => `<button type="button" data-rating="${i}" aria-label="${i} жұлдыз">★</button>`).join('')}
        </div>
        <input type="hidden" name="rating" value="${myReview?.rating || 0}">
      </div>
      <div class="field">
        <label for="comment">Пікір</label>
        <textarea id="comment" name="comment" maxlength="2000" placeholder="Тауар туралы әсеріңіз...">${esc(myReview?.comment || '')}</textarea>
      </div>
      <button class="btn" type="submit">${myReview ? 'Сақтау' : 'Жіберу'}</button>
    </form>`;
}

function reviewItem(r, me) {
  const canDelete = me && (me.id === r.user_id || me.role === 'admin');
  return `
    <div class="review">
      <div class="review-head">
        <div><strong>${esc(r.author_name)}</strong> ${stars(r.rating)}</div>
        <div class="small muted">
          ${formatDate(r.created_at)}
          ${canDelete ? `<button class="btn ghost sm" data-delete-review="${r.id}">Жою</button>` : ''}
        </div>
      </div>
      ${r.comment ? `<p style="margin:6px 0 0">${esc(r.comment)}</p>` : ''}
    </div>`;
}

function bindEvents() {
  const qty = document.getElementById('qty');
  root.querySelectorAll('[data-step]').forEach((b) =>
    b.addEventListener('click', () => {
      const max = Number(qty.max);
      qty.value = Math.min(max, Math.max(1, Number(qty.value) + Number(b.dataset.step)));
    }));

  document.getElementById('add-btn').addEventListener('click', async (e) => {
    const n = Math.max(1, Math.floor(Number(qty.value) || 1));
    e.target.disabled = true;
    await addToCart(product.id, n);
    e.target.disabled = product.stock <= 0;
  });

  const form = document.getElementById('review-form');
  if (form) {
    const paintStars = (value) =>
      form.querySelectorAll('[data-rating]').forEach((s) => s.classList.toggle('on', Number(s.dataset.rating) <= value));
    paintStars(Number(form.rating.value));
    form.querySelectorAll('[data-rating]').forEach((s) =>
      s.addEventListener('click', () => {
        form.rating.value = s.dataset.rating;
        paintStars(Number(s.dataset.rating));
      }));

    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const rating = Number(form.rating.value);
      if (!rating) return toast('Жұлдызшамен баға қойыңыз', 'error');
      try {
        await api(`/products/${product.id}/reviews`, {
          method: 'POST',
          body: { rating, comment: form.comment.value },
        });
        toast('Пікіріңіз сақталды. Рахмет!', 'success');
        await loadProduct();
      } catch (err) {
        toast(err.message, 'error');
      }
    });
  }

  document.getElementById('reviews-list').addEventListener('click', async (e) => {
    const b = e.target.closest('[data-delete-review]');
    if (!b || !confirm('Пікірді жою керек пе?')) return;
    try {
      await api('/reviews/' + b.dataset.deleteReview, { method: 'DELETE' });
      toast('Пікір жойылды');
      await loadProduct();
    } catch (err) {
      toast(err.message, 'error');
    }
  });
}

loadProduct();
