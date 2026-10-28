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
  let previous = '', admin = false, busy = false;
  window.addEventListener('overmynd-auth', event => {
    admin = event.detail === true;
    for (const controls of list.querySelectorAll('.approval-actions')) controls.hidden = !admin;
  });
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
      if (busy) return;
      list.replaceChildren();
      for (const item of pending) {
        const entry = document.createElement('li');
        entry.textContent = `${item.title || 'Untitled request'} (${item.media_type === 'tv' ? 'TV show' : 'Movie'})`;
        const controls = document.createElement('div');
        controls.className = 'approval-actions'; controls.hidden = !admin;
        const feedback = document.createElement('span'); feedback.setAttribute('role', 'status');
        for (const [action, label] of [['approve', 'Approve'], ['decline', 'Deny']]) {
          const control = document.createElement('button'); control.type = 'button'; control.textContent = `[ ${label} ]`;
          control.addEventListener('click', async () => {
            if (busy || !admin) return;
            busy = true;
            for (const b of list.querySelectorAll('button')) b.disabled = true;
            feedback.textContent = ' Saving...';
            try {
              await serviceAPIRequest(`/api/v1/requests/${encodeURIComponent(item.source_service_id)}/${encodeURIComponent(item.source_id)}/${action}`, {method:'POST'});
              entry.remove();
              const count = list.children.length;
              button.textContent = `${count} awaiting approval`; button.hidden = count === 0;
              status.textContent = `${item.title || 'Request'} ${action === 'approve' ? 'approved' : 'denied'}`;
              if (!count) popup.hidePopover();
            } catch (error) { feedback.textContent = ` ${error.message}`; }
            finally { busy = false; for (const b of list.querySelectorAll('button')) b.disabled = false; position(); }
          });
          controls.append(control);
        }
        entry.append(controls, feedback);
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
