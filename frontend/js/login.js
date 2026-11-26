/* =====================================================================
   login.js — кіру және тіркелу
   ===================================================================== */

// Кіргеннен кейін қайтатын бет (тек өз сайтымыздағы жол — open redirect-тен қорғау)
function nextUrl() {
  const next = new URLSearchParams(location.search).get('next') || '';
  return next.startsWith('/') && !next.startsWith('//') ? next : 'index.html';
}

if (Auth.isLoggedIn()) location.replace(nextUrl());

const loginForm = document.getElementById('login-form');
const registerForm = document.getElementById('register-form');

document.querySelector('.tabs').addEventListener('click', (e) => {
  const b = e.target.closest('[data-tab]');
  if (!b) return;
  document.querySelectorAll('.tabs button').forEach((t) => t.classList.toggle('active', t === b));
  loginForm.classList.toggle('hidden', b.dataset.tab !== 'login');
  registerForm.classList.toggle('hidden', b.dataset.tab !== 'register');
});
if (location.hash === '#register') document.querySelector('[data-tab="register"]').click();

async function submit(form, path, body) {
  const btn = form.querySelector('button[type="submit"]');
  btn.disabled = true;
  showFieldErrors(form, {});
  try {
    const { token, user } = await api(path, { method: 'POST', body });
    Auth.save(token, user);
    toast('Қош келдіңіз, ' + user.full_name + '!', 'success');
    location.href = nextUrl();
  } catch (e) {
    showFieldErrors(form, e.fields);
    toast(e.message, 'error');
    btn.disabled = false;
  }
}

loginForm.addEventListener('submit', (e) => {
  e.preventDefault();
  submit(loginForm, '/auth/login', {
    email: loginForm.email.value,
    password: loginForm.password.value,
  });
});

registerForm.addEventListener('submit', (e) => {
  e.preventDefault();
  submit(registerForm, '/auth/register', {
    full_name: registerForm.full_name.value,
    email: registerForm.email.value,
    phone: registerForm.phone.value,
    password: registerForm.password.value,
  });
});
