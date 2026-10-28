(() => {
  const button = document.getElementById('pendingApprovals');
  const popup = document.getElementById('approvalPopover');
  const list = document.getElementById('approvalList');
  const status = document.getElementById('approvalStatus');
  function position() {
    if (!popup.matches(':popover-open')) return;
    const anchor = button.getBoundingClientRect(), bounds = popup.getBoundingClientRect();
    popup.style.left = `${Math.max(8, Math.min(innerWidth - bounds.width - 8, anchor.right - bounds.width))}px`;
    popup.style.top = `${Math.max(8, Math.min(innerHeight - bounds.height - 8, anchor.bottom + 8))}px`;
  }
  popup.addEventListener('toggle', position);
  window.addEventListener('resize', position);
  window.addEventListener('scroll', position, true);
  let previous = '';
  async function refresh() {
    try {
      const response = await fetch('/api/v1/requests', {cache:'no-store', signal:AbortSignal.timeout(20000)});
      if (!response.ok) throw new Error('unavailable');
      const data = await response.json();
      if (data.errors?.length || !Array.isArray(data.requests)) throw new Error('unavailable');
      const pending = [...new Map(data.requests.filter(item => item.source === 'seerr' && item.status === 'pending').map(item => [item.id, item])).values()];
      button.hidden = pending.length === 0;
      const message = `${pending.length} awaiting approval`;
      button.textContent = message;
      if (previous !== message) { status.textContent = pending.length ? message : 'No requests awaiting approval'; previous = message; }
      list.replaceChildren();
      for (const item of pending) {
        const entry = document.createElement('li');
        entry.textContent = `${item.title || 'Untitled request'} (${item.media_type === 'tv' ? 'TV show' : 'Movie'})`;
        list.append(entry);
      }
      if (!pending.length) popup.hidePopover();
      position();
    } catch (_) {
      button.hidden = false;
      button.textContent = 'Approval status unavailable';
      list.replaceChildren();
      const entry = document.createElement('li');
      entry.textContent = 'Could not refresh requests. Check Seerr; retrying automatically.';
      list.append(entry);
    } finally { setTimeout(refresh, 30000); }
  }
  refresh();
})();
