(() => {
  // Theme handling
  const KEY = 'ui:theme';
  const body = document.body;
  const checkbox = document.getElementById('themeToggle');
  const knob = document.querySelector('.theme-toggle .knob');

  function setTheme(name, save = true){
    const isDark = name === 'dark';
    body.classList.toggle('light', !isDark);
    body.classList.toggle('dark', isDark);
    if (checkbox) checkbox.checked = isDark;
    if (knob) knob.textContent = isDark ? '🌙' : '☀️';
    if (save) localStorage.setItem(KEY, name);
  }

  const saved = localStorage.getItem(KEY);
  const prefersDark = window.matchMedia?.('(prefers-color-scheme: dark)').matches;
  setTheme(saved || (prefersDark ? 'dark' : 'light'), false);

  checkbox?.addEventListener('change', ()=> setTheme(checkbox.checked ? 'dark' : 'light'));

  // Toasts
  let toastsEl = document.getElementById('toasts');
  if (!toastsEl) {
    toastsEl = document.createElement('div');
    toastsEl.id = 'toasts';
    toastsEl.className = 'toasts';
    toastsEl.setAttribute('aria-live','polite');
    document.body.appendChild(toastsEl);
  }

  const toastIcons = { success: '✓', error: '✕', info: 'ℹ' };
  function showToast(message, type='info', timeout=4000){
    const t = document.createElement('div');
    t.className = 'toast ' + (type==='success' ? 'success' : type==='error' ? 'error' : '');
    const icon = toastIcons[type] || toastIcons.info;
    t.innerHTML = `<span class="toast-icon">${icon}</span><span class="toast-message">${message}</span>`;
    toastsEl.appendChild(t);
    setTimeout(()=> { t.remove(); }, timeout);
  }

  // Modal
  const modal = document.getElementById('confirmModal');
  const modalPanel = modal?.querySelector('.modal-panel');
  const modalTitle = document.getElementById('modalTitle');
  const modalBody = document.getElementById('modalBody');
  const modalCancel = document.getElementById('modalCancel');
  const modalConfirm = document.getElementById('modalConfirm');
  let modalResolve = null;
  let lastActive = null;

  function showConfirm(title, bodyText){
    return new Promise((resolve) => {
      modalTitle.textContent = title;
      modalBody.textContent = bodyText;
      lastActive = document.activeElement;
      modal.setAttribute('aria-hidden', 'false');
      document.body.classList.add('modal-open');
      setTimeout(()=> { modalPanel?.focus?.({preventScroll:true}); }, 10);
      modalResolve = resolve;
    });
  }

  function closeModal(result){
    modal.setAttribute('aria-hidden', 'true');
    document.body.classList.remove('modal-open');
    if (modalResolve) modalResolve(result);
    modalResolve = null;
    try { lastActive?.focus(); } catch(e){}
    lastActive = null;
  }
  modalCancel?.addEventListener('click', ()=> closeModal(false));
  modalConfirm?.addEventListener('click', ()=> closeModal(true));
  modal?.addEventListener('keydown', (e)=>{
    if (e.key === 'Escape') closeModal(false);
  });
  // close when clicking on backdrop (outside panel)
  modal?.addEventListener('click', (e) => {
    if (e.target === modal) closeModal(false);
  });

  // API wrapper with timeout and improved error handling
  async function postRestart(svc, timeoutMs=15000){
    const controller = new AbortController();
    const id = setTimeout(()=> controller.abort(), timeoutMs);
    try {
      const res = await fetch(`/restart/${svc}`,{ method:'POST', signal: controller.signal });
      const txt = await res.text().catch(()=>res.statusText || '');
      if (!res.ok) throw new Error(txt || res.statusText || 'Server error');
      return { ok:true, text: txt || `${svc} restarted` };
    } catch (err) {
      if (err.name === 'AbortError') throw new Error('Request timed out');
      throw err;
    } finally { clearTimeout(id); }
  }

  // Button handling: prevent double clicks and show spinner
  const buttons = Array.from(document.querySelectorAll('.btn[data-svc]'));
  const pending = {};

  function setButtonPending(svc, isPending){
    const btn = document.querySelector(`.btn[data-svc="${svc}"]`);
    if (!btn) return;
    btn.toggleAttribute('disabled', isPending);
    if (isPending && !btn.querySelector('.spinner')) {
      const sp = document.createElement('span');
      sp.className = 'spinner';
      sp.setAttribute('aria-hidden','true');
      btn.appendChild(sp);
    } else if (!isPending) {
      btn.querySelector('.spinner')?.remove();
    }
  }

  async function handleRestart(svc){
    if (pending[svc]) return;
    const name = { openclaw:'OpenClaw', ollama:'Ollama' }[svc] || svc;
    const ok = await showConfirm(`Restart ${name}`, `Are you sure you want to restart ${name}?`);
    if (!ok) { showToast('Cancelled', 'info'); return; }

    pending[svc] = true;
    setButtonPending(svc, true);
    try {
      const result = await postRestart(svc, 60000);
      showToast(result.text, 'success', 6000);
    } catch (err) {
      showToast(err.message || 'Request failed', 'error', 8000);
    } finally {
      pending[svc] = false;
      setButtonPending(svc, false);
    }
  }

  buttons.forEach(b => {
    b.addEventListener('click', (e) => {
      const svc = b.getAttribute('data-svc');
      createRipple(e, b);
      handleRestart(svc);
    });
    // keyboard activation
    b.addEventListener('keydown', (e)=>{
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); b.click(); }
    });
  });

  function createRipple(event, button){
    const rect = button.getBoundingClientRect();
    const ripple = document.createElement('span');
    ripple.className = 'ripple';
    const size = Math.max(rect.width, rect.height);
    ripple.style.cssText = `width:${size}px;height:${size}px;left:${event.clientX - rect.left - size/2}px;top:${event.clientY - rect.top - size/2}px`;
    button.appendChild(ripple);
    setTimeout(() => ripple.remove(), 600);
  }

  // expose for debugging
  window.__ui = { showToast, showConfirm, postRestart };
})();
