(() => {
  const THEME_KEY = 'ui:theme';
  const SECTION_KEY_PREFIX = 'section:';
  const TOAST_ICONS = { success: '✓', error: '✕', info: 'ℹ' };
  const pending = {};

  let body, checkbox, knob;
  let siteTitle, siteSub;
  let toastsEl, toastTpl;
  let modal, modalPanel, modalTitle;
  let modalResolve = null, lastActive = null;
  let buttonsEl, buttonTpl;

  let authAreaEl, gearBtn, gearPanel;
  let loginModal;
  let userFilterEl, userSelectEl, loginPasswordEl, rememberMeEl, loginErrorEl, loginSubmitEl;
  let currentUser = null;   // {username, groups} | null
  let loginForUsers = null; // string[] | null — pre-filter when triggered from a service click
  let allUsernames = [];    // union of all allowedUsers across services

  // --- Theme ---

  function setTheme(name, save = true) {
    const isDark = name === 'dark';
    body.classList.toggle('light', !isDark);
    body.classList.toggle('dark', isDark);
    if (checkbox) checkbox.checked = isDark;
    if (knob) knob.textContent = isDark ? '🌙' : '☀️';
    if (save) localStorage.setItem(THEME_KEY, name);
  }

  // --- Toasts ---

  function showToast(message, type = 'info', timeout = 4000) {
    const t = toastTpl.cloneNode(true).firstElementChild;
    if (type === 'success' || type === 'error') t.classList.add(type);
    t.querySelector('.toast-icon').textContent = TOAST_ICONS[type] || TOAST_ICONS.info;
    t.querySelector('.toast-message').textContent = message;
    toastsEl.appendChild(t);
    setTimeout(() => t.remove(), timeout);
  }

  // --- Confirm modal ---

  function showConfirm(title) {
    return new Promise((resolve) => {
      modalTitle.textContent = title;
      lastActive = document.activeElement;
      modal.setAttribute('aria-hidden', 'false');
      body.classList.add('modal-open');
      setTimeout(() => modalPanel?.focus?.({ preventScroll: true }), 10);
      modalResolve = resolve;
    });
  }

  function closeModal(result) {
    if (modal.contains(document.activeElement)) document.activeElement.blur();
    modal.setAttribute('aria-hidden', 'true');
    body.classList.remove('modal-open');
    if (modalResolve) modalResolve(result);
    modalResolve = null;
    try { lastActive?.focus(); } catch (e) {}
    lastActive = null;
  }

  // --- Auth area ---

  function renderAuthArea() {
    gearBtn.classList.toggle('logged-in', !!currentUser);
    authAreaEl.innerHTML = '';
    if (currentUser) {
      const label = Object.assign(document.createElement('span'), { className: 'gear-label auth-username', textContent: currentUser.username });
      const btn = Object.assign(document.createElement('button'), { className: 'btn ghost auth-btn', textContent: 'Logout' });
      btn.addEventListener('click', handleLogout);
      authAreaEl.append(label, btn);
    } else {
      const label = Object.assign(document.createElement('span'), { className: 'gear-label', textContent: 'Account' });
      const btn = Object.assign(document.createElement('button'), { className: 'btn ghost auth-btn', textContent: 'Login' });
      btn.addEventListener('click', () => openLoginModal(null));
      authAreaEl.append(label, btn);
    }
  }

  // --- Login modal ---

  function populateUserSelect(filter) {
    const lower = filter.toLowerCase();
    const pool = loginForUsers || allUsernames;
    userSelectEl.innerHTML = '';
    pool.filter(u => u.toLowerCase().includes(lower)).forEach(u => {
      const opt = Object.assign(document.createElement('option'), { value: u, textContent: u });
      userSelectEl.appendChild(opt);
    });
    if (userSelectEl.options.length === 1) userSelectEl.selectedIndex = 0;
  }

  function openLoginModal(allowedUsers) {
    loginForUsers = allowedUsers;
    loginErrorEl.textContent = '';
    loginPasswordEl.value = '';
    userFilterEl.value = '';
    rememberMeEl.checked = true;
    populateUserSelect('');
    loginModal.setAttribute('aria-hidden', 'false');
    body.classList.add('modal-open');
    setTimeout(() => (userSelectEl.options.length === 1 ? loginPasswordEl : userFilterEl).focus(), 10);
  }

  function closeLoginModal() {
    if (loginModal.contains(document.activeElement)) document.activeElement.blur();
    loginModal.setAttribute('aria-hidden', 'true');
    body.classList.remove('modal-open');
    loginForUsers = null;
  }

  async function handleLogin() {
    const username = userSelectEl.value;
    const password = loginPasswordEl.value;
    if (!username) { loginErrorEl.textContent = 'Please select a user.'; return; }
    if (!password) { loginErrorEl.textContent = 'Please enter a password.'; return; }
    loginSubmitEl.disabled = true;
    loginErrorEl.textContent = '';
    try {
      const res = await fetch('/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password, rememberMe: rememberMeEl.checked }),
      });
      if (res.ok) {
        const data = await res.json();
        currentUser = { username: data.username };
        closeLoginModal();
        renderAuthArea();
        loadServices();
      } else {
        const data = await res.json().catch(() => ({}));
        loginErrorEl.textContent = data.error || 'Login failed.';
        loginPasswordEl.value = '';
        loginPasswordEl.focus();
      }
    } catch { loginErrorEl.textContent = 'Network error. Try again.'; }
    finally { loginSubmitEl.disabled = false; }
  }

  async function handleLogout() {
    await fetch('/logout', { method: 'POST' });
    currentUser = null;
    renderAuthArea();
    loadServices();
  }

  // --- API ---

  async function runService(svc, timeoutMs = 15000) {
    const controller = new AbortController();
    const id = setTimeout(() => controller.abort(), timeoutMs);
    try {
      const res = await fetch(`/run/${svc}`, { method: 'POST', signal: controller.signal });
      const txt = await res.text().catch(() => res.statusText || '');
      if (res.status === 401) throw new Error('Authentication required');
      if (res.status === 403) throw new Error('Access denied');
      if (!res.ok) throw new Error(txt || res.statusText || 'Server error');
      return txt || `${svc} done`;
    } catch (err) {
      if (err.name === 'AbortError') throw new Error('Request timed out');
      throw err;
    } finally { clearTimeout(id); }
  }

  // --- Buttons ---

  function setButtonPending(btn, isPending) {
    btn.toggleAttribute('disabled', isPending);
    if (isPending && !btn.querySelector('.spinner')) {
      const sp = document.createElement('span');
      sp.className = 'spinner';
      sp.setAttribute('aria-hidden', 'true');
      btn.appendChild(sp);
    } else if (!isPending) {
      btn.querySelector('.spinner')?.remove();
    }
  }

  function createRipple(event, button) {
    const rect = button.getBoundingClientRect();
    const ripple = document.createElement('span');
    ripple.className = 'ripple';
    const size = Math.max(rect.width, rect.height);
    ripple.style.cssText = `width:${size}px;height:${size}px;left:${event.clientX - rect.left - size / 2}px;top:${event.clientY - rect.top - size / 2}px`;
    button.appendChild(ripple);
    setTimeout(() => ripple.remove(), 600);
  }

  async function handleRun(btn, svc, title, timeout, confirm) {
    if (pending[svc]) return;
    if (confirm) {
      const ok = await showConfirm(title);
      if (!ok) { showToast('Cancelled', 'info'); return; }
    }

    pending[svc] = true;
    setButtonPending(btn, true);
    try {
      const text = await runService(svc, timeout);
      showToast(text, 'success', 6000);
    } catch (err) {
      showToast(err.message || 'Request failed', 'error', 8000);
    } finally {
      pending[svc] = false;
      setButtonPending(btn, false);
    }
  }

  function getSectionExpanded(title) {
    return localStorage.getItem(SECTION_KEY_PREFIX + title) === 'true';
  }
  function setSectionExpanded(title, expanded) {
    localStorage.setItem(SECTION_KEY_PREFIX + title, String(expanded));
  }

  function renderSection(title, buttons) {
    const expanded = getSectionExpanded(title);
    const wrapper = document.createElement('div');
    wrapper.className = 'section-group';
    const header = document.createElement('button');
    header.type = 'button';
    header.className = 'section-header';
    header.setAttribute('aria-expanded', String(expanded));
    header.innerHTML = '<span class="section-chevron" aria-hidden="true">▶</span>'
      + '<span class="section-title">' + title + '</span>';
    const body = document.createElement('div');
    body.className = 'section-body';
    if (!expanded) body.classList.add('section-body--collapsed');
    body.append(...buttons);
    header.addEventListener('click', () => {
      const next = body.classList.toggle('section-body--collapsed');
      header.setAttribute('aria-expanded', String(!next));
      setSectionExpanded(title, !next);
    });
    wrapper.append(header, body);
    return wrapper;
  }

  async function loadServices() {
    try {
      const res = await fetch('/services');
      if (!res.ok) throw new Error('Failed to load services');
      const services = await res.json();
      buttonsEl.innerHTML = '';
      const sectionOrder = [];
      const sectionMap = {};
      services.forEach(({ id, title, timeout, confirm, section, public: isPub, accessible, allowedUsers }) => {
        const btn = buttonTpl.cloneNode(true).firstElementChild;
        btn.dataset.svc = id;
        btn.id = `btn-${id}`;
        if (confirm) {
          btn.textContent = title;
        } else {
          btn.innerHTML = '<span class="btn-icon">⚡</span><span class="btn-label">' + title + '</span>';
        }
        if (!isPub && !accessible) {
          if (currentUser) {
            btn.classList.add('forbidden');
            btn.disabled = true;
          } else {
            btn.classList.add('needs-login');
            btn.textContent = confirm ? '🔒 ' + title : '🔒 ' + title + ' ⚡';
            btn.addEventListener('click', (e) => { createRipple(e, btn); openLoginModal(allowedUsers); });
          }
        } else {
          btn.addEventListener('click', (e) => { createRipple(e, btn); handleRun(btn, id, title, timeout, confirm); });
        }
        btn.addEventListener('keydown', (e) => {
          if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); btn.click(); }
        });
        const key = section || '';
        if (!sectionMap[key]) { sectionMap[key] = []; sectionOrder.push(key); }
        sectionMap[key].push(btn);
      });
      for (const key of sectionOrder) {
        if (key) {
          buttonsEl.appendChild(renderSection(key, sectionMap[key]));
        } else {
          sectionMap[key].forEach(btn => buttonsEl.appendChild(btn));
        }
      }
    } catch { buttonsEl.textContent = 'Failed to load services.'; }
  }

  // --- Init ---

  function init() {
    body     = document.body;
    checkbox = document.getElementById('themeToggle');
    knob     = document.querySelector('.theme-toggle .knob');
    toastsEl = document.getElementById('toasts');
    toastTpl = document.getElementById('tpl-toast').content;
    modal      = document.getElementById('confirmModal');
    modalPanel = modal?.querySelector('.modal-panel');
    modalTitle = document.getElementById('modalTitle');
    buttonsEl  = document.getElementById('buttons');
    buttonTpl  = document.getElementById('tpl-button').content;

    authAreaEl  = document.getElementById('authArea');
    gearBtn     = document.getElementById('gearBtn');
    gearPanel   = document.getElementById('gearPanel');
    loginModal  = document.getElementById('loginModal');

    gearBtn.addEventListener('click', () => {
      const open = !gearPanel.hidden;
      gearPanel.hidden = open;
      gearBtn.setAttribute('aria-expanded', String(!open));
    });
    document.addEventListener('click', (e) => {
      if (!gearPanel.hidden && !gearBtn.contains(e.target) && !gearPanel.contains(e.target)) {
        gearPanel.hidden = true;
        gearBtn.setAttribute('aria-expanded', 'false');
      }
    });
    userFilterEl    = document.getElementById('userFilter');
    userSelectEl    = document.getElementById('userSelect');
    loginPasswordEl = document.getElementById('loginPassword');
    rememberMeEl    = document.getElementById('rememberMe');
    loginErrorEl    = document.getElementById('loginError');
    loginSubmitEl   = document.getElementById('loginSubmit');

    const saved = localStorage.getItem(THEME_KEY);
    const prefersDark = window.matchMedia?.('(prefers-color-scheme: dark)').matches;
    setTheme(saved || (prefersDark ? 'dark' : 'light'), false);
    checkbox?.addEventListener('change', () => setTheme(checkbox.checked ? 'dark' : 'light'));

    document.getElementById('modalCancel')?.addEventListener('click', () => closeModal(false));
    document.getElementById('modalConfirm')?.addEventListener('click', () => closeModal(true));
    modal?.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeModal(false); });
    modal?.addEventListener('click', (e) => { if (e.target === modal) closeModal(false); });

    document.getElementById('loginModalClose')?.addEventListener('click', closeLoginModal);
    loginModal?.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeLoginModal(); });
    loginModal?.addEventListener('click', (e) => { if (e.target === loginModal) closeLoginModal(); });
    userFilterEl?.addEventListener('input', () => populateUserSelect(userFilterEl.value));
    loginSubmitEl?.addEventListener('click', handleLogin);
    loginPasswordEl?.addEventListener('keydown', (e) => { if (e.key === 'Enter') handleLogin(); });

    siteTitle = document.getElementById('site-title');
    siteSub   = document.getElementById('site-subtitle');

    Promise.all([
      fetch('/config').then(r => r.json()),
      fetch('/me').then(r => r.json()).catch(() => null),
    ]).then(([{ title, subtitle, users }, me]) => {
      siteTitle.textContent = title;
      if (subtitle) { siteSub.textContent = subtitle; } else { siteSub.style.display = 'none'; }
      allUsernames = users || [];
      currentUser = me;
      renderAuthArea();
      loadServices();
    });
  }

  window.__ui = { showToast, showConfirm, runService };
  init();
})();
