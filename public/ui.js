(() => {
  const STORAGE = {
    theme:        'ui:theme',
    section:     (title) => 'section:' + title,
    fab:          'fab:pos',
    showDisabled: 'ui:show-disabled',
  };
  const FOCUS_DELAY_MS = 10;
  const RIPPLE_DURATION_MS = 600;
  const TOAST_ICONS = { success: '✓', error: '✕', info: 'ℹ' };
  const TOAST_MAX_CHARS = 120;
  const pending = new Map();

  function makeEl(tag, props) {
    return Object.assign(document.createElement(tag), props);
  }

  let body, checkbox, knob;
  let toastsEl, toastTpl;
  let modal, modalPanel, modalTitle;
  let logsModalEl, logsModalPanel, logsStdoutEl, logsStderrEl, logsTabBtns;
  let modalResolve = null, lastActive = null;
  let buttonsEl, buttonTpl;

  let authAreaEl, gearBtn, gearPanel, disabledToggle;
  let loginModal;
  let userFilterEl, userSelectEl, loginPasswordEl, rememberMeEl, loginErrorEl, loginSubmitEl;
  let currentUser = null;   // {username, groups} | null
  let loginForUsers = null; // string[] | null — pre-filter when triggered from a service click
  let allUsernames = [];    // union of all allowedUsers across services

  // --- Theme ---

  function setTheme(name, save = true) {
    const isDark = name === 'dark';
    document.documentElement.classList.toggle('light', !isDark);
    if (checkbox) checkbox.checked = isDark;
    if (knob) knob.textContent = isDark ? '🌙' : '☀️';
    if (save) localStorage.setItem(STORAGE.theme, name);
  }

  // --- Toasts ---

  function attachSwipeToDismiss(el) {
    let swipeStartX = null, swipePointerId = null;
    el.addEventListener('pointerdown', (e) => {
      if (e.target.closest('button')) return;
      swipeStartX = e.clientX;
      swipePointerId = e.pointerId;
      el.style.animation = 'none';
      el.style.opacity = '1';
      e.target.setPointerCapture(e.pointerId);
    });
    el.addEventListener('pointermove', (e) => {
      if (swipeStartX === null || e.pointerId !== swipePointerId) return;
      const dx = e.clientX - swipeStartX;
      el.style.transform = `translateX(${dx}px)`;
      el.style.opacity = String(Math.max(0, 1 - Math.abs(dx) / 150));
    });
    el.addEventListener('pointerup', (e) => {
      if (swipeStartX === null || e.pointerId !== swipePointerId) return;
      const dx = e.clientX - swipeStartX;
      swipeStartX = swipePointerId = null;
      if (Math.abs(dx) > 60) { el.remove(); return; }
      el.style.transition = 'transform 0.2s ease, opacity 0.2s ease';
      el.style.transform = 'translateY(0)';
      el.style.opacity = '1';
      el.addEventListener('transitionend', () => { el.style.transition = ''; }, { once: true });
    });
    el.addEventListener('pointercancel', (e) => {
      if (e.pointerId !== swipePointerId) return;
      swipeStartX = swipePointerId = null;
      el.style.transform = 'translateY(0)';
      el.style.opacity = '1';
    });
  }

  function showToast(message, type = 'info', { jobId } = {}) {
    const t = toastTpl.cloneNode(true).firstElementChild;
    if (type === 'success' || type === 'error') t.classList.add(type);
    t.querySelector('.toast-icon').textContent = TOAST_ICONS[type] || TOAST_ICONS.info;
    const truncated = message.length > TOAST_MAX_CHARS ? message.slice(0, TOAST_MAX_CHARS) + '…' : message;
    t.querySelector('.toast-message').textContent = truncated;
    const logsBtn = t.querySelector('.toast-logs-btn');
    if (jobId) {
      logsBtn.addEventListener('click', () => openLogsModal(jobId));
    } else {
      logsBtn.remove();
    }
    t.querySelector('.toast-close-btn').addEventListener('click', () => t.remove());
    attachSwipeToDismiss(t);
    toastsEl.appendChild(t);
  }

  // --- Modal helpers ---

  function blurIfInside(el) {
    if (el.contains(document.activeElement)) document.activeElement.blur();
  }

  function openModalEl(el, focusEl) {
    lastActive = document.activeElement;
    el.setAttribute('aria-hidden', 'false');
    document.documentElement.classList.add('modal-open');
    setTimeout(() => focusEl?.focus?.({ preventScroll: true }), FOCUS_DELAY_MS);
  }

  function closeModalEl(el) {
    blurIfInside(el);
    el.setAttribute('aria-hidden', 'true');
    document.documentElement.classList.remove('modal-open');
    lastActive?.focus({ preventScroll: true });
    lastActive = null;
  }

  function setupModalEvents(el, closeCallback) {
    el?.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeCallback(); });
    el?.addEventListener('click', (e) => { if (e.target === el) closeCallback(); });
  }

  // --- Confirm modal ---

  function showConfirm(title) {
    return new Promise((resolve) => {
      modalTitle.textContent = title;
      openModalEl(modal, modalPanel);
      modalResolve = resolve;
    });
  }

  function closeModal(result) {
    closeModalEl(modal);
    if (modalResolve) modalResolve(result);
    modalResolve = null;
  }

  // --- Auth area ---

  function renderAuthArea() {
    gearBtn.classList.toggle('logged-in', !!currentUser);
    authAreaEl.innerHTML = '';
    if (currentUser) {
      const label = makeEl('span', { className: 'gear-label auth-username', textContent: currentUser.username });
      const btn = makeEl('button', { className: 'btn ghost auth-btn', textContent: 'Logout' });
      btn.addEventListener('click', handleLogout);
      authAreaEl.append(label, btn);
    } else {
      const btn = makeEl('button', { className: 'btn ghost auth-btn auth-btn--full', textContent: 'Login' });
      btn.addEventListener('click', () => openLoginModal(null));
      authAreaEl.append(btn);
    }
  }

  // --- Login modal ---

  function populateUserSelect(filter) {
    const lower = filter.toLowerCase();
    const pool = loginForUsers || allUsernames;
    userSelectEl.innerHTML = '';
    pool.filter(u => u.toLowerCase().includes(lower)).forEach(u => {
      const opt = makeEl('option', { value: u, textContent: u });
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
    const focusEl = userSelectEl.options.length === 1 ? loginPasswordEl : userFilterEl;
    openModalEl(loginModal, focusEl);
  }

  function closeLoginModal() {
    closeModalEl(loginModal);
    loginForUsers = null;
  }

  function refreshAuth() {
    renderAuthArea();
    loadServices();
  }

  function onLoginSuccess(data) {
    currentUser = { username: data.username, groups: [] };
    closeLoginModal();
    refreshAuth();
  }

  function onLoginError(err) {
    loginErrorEl.textContent = err || 'Login failed.';
    loginPasswordEl.value = '';
    loginPasswordEl.focus();
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
        onLoginSuccess(await res.json());
      } else {
        const data = await res.json().catch(() => ({}));
        onLoginError(data.error);
      }
    } catch { loginErrorEl.textContent = 'Network error. Try again.'; }
    finally { loginSubmitEl.disabled = false; }
  }

  async function handleLogout() {
    try {
      await fetch('/logout', { method: 'POST' });
    } catch {
      showToast('Logout failed. Try again.', 'error');
      return;
    }
    currentUser = null;
    refreshAuth();
  }

  // --- API ---

  async function runService(svc, timeoutMs = 15000) {
    const startRes = await fetch(`/run/${svc}`, { method: 'POST' });
    if (startRes.status === 401) throw new Error('Authentication required');
    if (startRes.status === 403) throw new Error('Access denied');
    if (!startRes.ok) {
      const errData = await startRes.json().catch(() => null);
      throw new Error(errData?.error || startRes.statusText || 'Server error');
    }
    const { jobId } = await startRes.json();

    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      await new Promise(r => setTimeout(r, 2000));
      const pollRes = await fetch(`/jobs/${jobId}`);
      if (!pollRes.ok) throw new Error('Job lost');
      const job = await pollRes.json();
      if (job.status === 'done') return { message: `${job.title} done`, jobId };
      if (job.status === 'error') throw Object.assign(new Error('Command failed'), { jobId });
    }
    throw Object.assign(new Error('Request timed out'), { jobId });
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
    setTimeout(() => ripple.remove(), RIPPLE_DURATION_MS);
  }

  async function handleRun(btn, svc, title, timeout, confirm) {
    if (pending.has(svc)) return;
    if (confirm) {
      const ok = await showConfirm(title);
      if (!ok) { showToast('Cancelled', 'info'); return; }
    }

    pending.set(svc, true);
    setButtonPending(btn, true);
    try {
      const { message, jobId } = await runService(svc, timeout);
      showToast(message, 'success', { jobId });
    } catch (err) {
      showToast(err.message || 'Request failed', 'error', { jobId: err.jobId });
    } finally {
      pending.delete(svc);
      setButtonPending(btn, false);
    }
  }

  function toggleSection(header, sectionBody, title, expand) {
    header.setAttribute('aria-expanded', String(expand));
    sectionBody.classList.toggle('section-body--collapsed', !expand);
    localStorage.setItem(STORAGE.section(title), String(expand));
  }

  function getSectionCols(configured) {
    const w = window.innerWidth;
    if (w >= 1024) return configured;
    if (w >= 640) return Math.min(configured, 2);
    return 1;
  }

  function renderSection(title, buttons, collapsable, columns) {
    const storedExpanded = localStorage.getItem(STORAGE.section(title));
    const expanded = !collapsable || storedExpanded === 'true';
    const wrapper = document.createElement('div');
    wrapper.className = 'section-group';

    const header = document.createElement('button');
    header.type = 'button';
    header.className = 'section-header';
    header.setAttribute('aria-expanded', String(expanded));

    if (!collapsable) {
      header.classList.add('section-header--pinned');
      const titleSpan = document.createElement('span');
      titleSpan.className = 'section-title';
      titleSpan.textContent = title;
      header.appendChild(titleSpan);
    } else {
      const chevron = document.createElement('span');
      chevron.className = 'section-chevron';
      chevron.setAttribute('aria-hidden', 'true');
      chevron.textContent = '▶';
      const titleSpan = document.createElement('span');
      titleSpan.className = 'section-title';
      titleSpan.textContent = title;
      header.append(chevron, titleSpan);
      header.addEventListener('click', () => {
        toggleSection(header, sectionBody, title, header.getAttribute('aria-expanded') !== 'true');
      });
    }

    const sectionBody = document.createElement('div');
    sectionBody.className = 'section-body';
    if (!expanded) sectionBody.classList.add('section-body--collapsed');
    sectionBody.style.setProperty('--cols', getSectionCols(columns));

    sectionBody.append(...buttons);
    wrapper.append(header, sectionBody);
    return { wrapper, header, sectionBody, collapsable, title, columns };
  }

  function applyButtonAccessState(btn, { public: isPub, accessible, allowedUsers, title, confirm }) {
    if (!isPub && !accessible) {
      if (currentUser) {
        btn.classList.add('forbidden');
        btn.disabled = true;
      } else {
        btn.classList.add('needs-login');
        btn.textContent = '🔒 ' + title + (confirm ? '' : ' ⚡');
        btn.addEventListener('click', (e) => { createRipple(e, btn); openLoginModal(allowedUsers); });
      }
    }
  }

  function buildServiceButton(svc) {
    const { id, title, timeout, confirm, public: isPub, accessible, enabled } = svc;
    const btn = buttonTpl.cloneNode(true).firstElementChild;
    btn.dataset.svc = id;
    btn.id = `btn-${id}`;

    if (confirm) {
      btn.textContent = title;
    } else {
      const icon = document.createElement('span');
      icon.className = 'btn-icon';
      icon.textContent = '⚡';
      const label = document.createElement('span');
      label.className = 'btn-label';
      label.textContent = title;
      btn.append(icon, label);
    }

    if (!enabled) {
      btn.classList.add('service-disabled');
      btn.disabled = true;
    } else if (isPub || accessible) {
      btn.addEventListener('click', (e) => { createRipple(e, btn); handleRun(btn, id, title, timeout, confirm); });
    } else {
      applyButtonAccessState(btn, svc);
    }

    btn.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); btn.click(); }
    });
    return btn;
  }

  // track rendered sections for collapse/expand all
  let renderedSections = [];

  function groupServicesBySection(services, showDisabled) {
    const sectionOrder = [];
    const sectionMap = {};
    const sectionMeta = {};
    const sectionsAllDisabled = {};
    services.forEach((svc) => {
      if (!svc.enabled && !showDisabled) return;
      const btn = buildServiceButton(svc);
      const key = svc.section || '';
      if (!sectionMap[key]) {
        sectionMap[key] = [];
        sectionOrder.push(key);
        sectionMeta[key] = { collapsable: svc.sectionCollapsable, columns: svc.sectionColumns };
        sectionsAllDisabled[key] = { meta: sectionMeta[key], allDisabled: true };
      }
      sectionMap[key].push(btn);
      if (svc.enabled) {
        sectionsAllDisabled[key].allDisabled = false;
      }
    });
    return { sectionOrder, sectionMap, sectionsAllDisabled };
  }

  async function loadServices() {
    try {
      const res = await fetch('/services');
      if (!res.ok) throw new Error('Failed to load services');
      const services = await res.json();
      const showDisabled = localStorage.getItem(STORAGE.showDisabled) === 'true';
      buttonsEl.innerHTML = '';
      renderedSections = [];
      const { sectionOrder, sectionMap, sectionsAllDisabled } = groupServicesBySection(services, showDisabled);
      for (const key of sectionOrder) {
        if (key) {
          const { meta, allDisabled } = sectionsAllDisabled[key];
          const section = renderSection(key, sectionMap[key], meta.collapsable !== false, meta.columns || 3);
          if (allDisabled) {
            section.wrapper.classList.add('section-disabled');
          }
          buttonsEl.appendChild(section.wrapper);
          renderedSections.push(section);
        } else {
          buttonsEl.append(...sectionMap[key]);
        }
      }
    } catch (err) {
      console.error('Failed to load services:', err);
      buttonsEl.textContent = 'Failed to load services.';
    }
  }

  // --- Collapse / Expand all FABs ---

  const SVG_COLLAPSE = '<svg width="16" height="20" viewBox="0 0 16 20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3,3 8,8 13,3"/><line x1="3" y1="10" x2="13" y2="10" stroke-dasharray="2,2"/><polyline points="3,17 8,12 13,17"/></svg>';
  const SVG_EXPAND   = '<svg width="16" height="20" viewBox="0 0 16 20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3,8 8,3 13,8"/><line x1="3" y1="10" x2="13" y2="10" stroke-dasharray="2,2"/><polyline points="3,12 8,17 13,12"/></svg>';
  const DRAG_THRESHOLD = 5;

  function initFab() {
    const fab = document.createElement('div');
    fab.className = 'fab-group';

    const collapseBtn = makeEl('button', { className: 'fab-btn', type: 'button', title: 'Collapse all', innerHTML: SVG_COLLAPSE });
    const expandBtn   = makeEl('button', { className: 'fab-btn', type: 'button', title: 'Expand all',   innerHTML: SVG_EXPAND });
    fab.append(expandBtn, collapseBtn);
    document.body.appendChild(fab);

    const saved = JSON.parse(localStorage.getItem(STORAGE.fab) || 'null');
    let side = saved?.side || 'right';
    let topPct = saved?.topPct ?? 50;

    const containerEl = document.querySelector('.container');
    let fabW = 44, fabH = 88;

    function applyPos() {
      const vw = document.documentElement.clientWidth;
      const cr = containerEl?.getBoundingClientRect() ?? { left: 0, right: vw };
      topPct = Math.max((fabH / 2 / window.innerHeight) * 100, Math.min(((window.innerHeight - fabH / 2) / window.innerHeight) * 100, topPct));
      fab.style.top = topPct + '%';
      fab.style.transform = 'translateY(-50%)';
      fab.style.removeProperty('right');
      const gap = 8;
      const leftPos = side === 'left'
        ? (cr.left >= fabW + gap ? cr.left - fabW - gap : 0)
        : (vw - cr.right >= fabW + gap ? cr.right + gap : vw - fabW);
      fab.style.left = leftPos + 'px';
    }
    requestAnimationFrame(() => {
      fabW = fab.offsetWidth || 44;
      fabH = fab.offsetHeight || 88;
      applyPos();
    });
    window.addEventListener('resize', applyPos, { passive: true });

    function animateFabToPos() {
      fab.classList.add('fab-group--animating');
      applyPos();
      fab.addEventListener('transitionend', () => fab.classList.remove('fab-group--animating'), { once: true });
    }

    function setAllSections(expand) {
      renderedSections.forEach(({ header, sectionBody, collapsable, title }) => {
        if (!expand && !collapsable) return;
        toggleSection(header, sectionBody, title, expand);
      });
    }

    // Capture pointer to allow dragging outside button bounds; short tap fires action on target button
    let downTarget = null, startX = 0, startY = 0, startTopPx = 0, didDrag = false;

    fab.addEventListener('pointerdown', (e) => {
      downTarget = e.target;
      startX = e.clientX;
      startY = e.clientY;
      startTopPx = (topPct / 100) * window.innerHeight;
      didDrag = false;
      fab.classList.remove('fab-group--animating');
      fab.setPointerCapture(e.pointerId);
      e.preventDefault();
    });

    fab.addEventListener('pointermove', (e) => {
      if (!fab.hasPointerCapture(e.pointerId)) return;
      const dx = e.clientX - startX, dy = e.clientY - startY;
      if (!didDrag && Math.hypot(dx, dy) < DRAG_THRESHOLD) return;
      didDrag = true;

      const newTopPx = Math.max(fabH / 2, Math.min(window.innerHeight - fabH / 2, startTopPx + dy));
      topPct = (newTopPx / window.innerHeight) * 100;
      fab.style.top = topPct + '%';
      const vw = document.documentElement.clientWidth;
      const newSide = e.clientX < vw / 2 ? 'left' : 'right';
      if (Math.abs(dx) > 30) {
        // horizontal intent — follow cursor and animate to side
        fab.style.left = Math.max(0, Math.min(vw - fabW, e.clientX - fabW / 2)) + 'px';
        if (newSide !== side) {
          side = newSide;
          animateFabToPos();
        }
      } else {
        side = newSide;
      }
    });

    function stopDrag(e) {
      if (!fab.hasPointerCapture(e.pointerId)) return;
      fab.releasePointerCapture(e.pointerId);
      if (!didDrag) {
        if (collapseBtn.contains(downTarget)) { setAllSections(false); collapseBtn.blur(); }
        else if (expandBtn.contains(downTarget)) { setAllSections(true); expandBtn.blur(); }
      } else {
        localStorage.setItem(STORAGE.fab, JSON.stringify({ side, topPct }));
        animateFabToPos();
      }
      downTarget = null;
      didDrag = false;
    }
    fab.addEventListener('pointerup', stopDrag);
    fab.addEventListener('pointercancel', stopDrag);
  }

  // --- Logs modal ---

  function activateLogsTab(name) {
    logsTabBtns.forEach(btn => btn.classList.toggle('active', btn.dataset.tab === name));
    logsStdoutEl.hidden = name !== 'stdout';
    logsStderrEl.hidden = name !== 'stderr';
  }

  async function openLogsModal(jobId) {
    let job = {};
    try {
      const res = await fetch(`/jobs/${jobId}`);
      if (res.ok) job = await res.json();
    } catch {
      showToast('Failed to load logs.', 'error');
      return;
    }
    logsStdoutEl.textContent = job.stdout || '(empty)';
    logsStderrEl.textContent = job.stderr || '(empty)';
    activateLogsTab('stdout');
    openModalEl(logsModalEl, logsModalPanel);
  }

  function onBootstrap({ title, subtitle, users }, me) {
    const siteTitle = document.getElementById('site-title');
    const siteSub   = document.getElementById('site-subtitle');
    siteTitle.textContent = title;
    document.title = title;
    if (subtitle) { siteSub.textContent = subtitle; } else { siteSub.style.display = 'none'; }
    allUsernames = users || [];
    currentUser = me;
    refreshAuth();
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
    disabledToggle = document.getElementById('disabledToggle');
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

    document.getElementById('togglePassword')?.addEventListener('click', () => {
      const isPassword = loginPasswordEl.type === 'password';
      loginPasswordEl.type = isPassword ? 'text' : 'password';
      const icon = document.getElementById('eyeIcon');
      if (icon) icon.innerHTML = isPassword
        ? '<path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94"/><path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19"/><line x1="1" y1="1" x2="23" y2="23"/>'
        : '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>';
    });

    const saved = localStorage.getItem(STORAGE.theme);
    const prefersDark = window.matchMedia?.('(prefers-color-scheme: dark)').matches;
    setTheme(saved || (prefersDark ? 'dark' : 'light'), false);
    checkbox?.addEventListener('change', () => setTheme(checkbox.checked ? 'dark' : 'light'));

    const showDisabled = localStorage.getItem(STORAGE.showDisabled) === 'true';
    if (disabledToggle) disabledToggle.checked = showDisabled;
    disabledToggle?.addEventListener('change', () => {
      localStorage.setItem(STORAGE.showDisabled, String(disabledToggle.checked));
      loadServices();
    });

    document.getElementById('modalCancel')?.addEventListener('click', () => closeModal(false));
    document.getElementById('modalConfirm')?.addEventListener('click', () => closeModal(true));
    setupModalEvents(modal, () => closeModal(false));

    document.getElementById('loginModalClose')?.addEventListener('click', closeLoginModal);
    setupModalEvents(loginModal, closeLoginModal);

    logsModalEl    = document.getElementById('logsModal');
    logsModalPanel = logsModalEl?.querySelector('.modal-panel');
    logsStdoutEl   = document.getElementById('logsStdout');
    logsStderrEl   = document.getElementById('logsStderr');
    logsTabBtns    = logsModalEl?.querySelectorAll('.logs-tab') ?? [];
    logsTabBtns.forEach(btn => btn.addEventListener('click', () => activateLogsTab(btn.dataset.tab)));
    document.getElementById('logsModalClose')?.addEventListener('click', () => closeModalEl(logsModalEl));
    setupModalEvents(logsModalEl, () => closeModalEl(logsModalEl));
    userFilterEl?.addEventListener('input', () => populateUserSelect(userFilterEl.value));
    loginSubmitEl?.addEventListener('click', handleLogin);
    loginPasswordEl?.addEventListener('keydown', (e) => { if (e.key === 'Enter') handleLogin(); });

    window.addEventListener('resize', () => {
      renderedSections.forEach(({ sectionBody, columns }) => {
        sectionBody.style.setProperty('--cols', getSectionCols(columns));
      });
    }, { passive: true });

    initFab();

    Promise.all([
      fetch('/config').then(r => r.json()),
      fetch('/me').then(r => r.json()).catch(() => null),
    ]).then(([config, me]) => onBootstrap(config, me))
      .catch(() => { buttonsEl.textContent = 'Failed to connect to server.'; });
  }

  window.__ui = { showToast, showConfirm, runService };
  init();
})();
