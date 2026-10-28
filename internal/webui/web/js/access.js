// Access requests are separate from public media requests and administrator login.
(() => {
  const el = id => document.getElementById(id);
  const api = (path, body, method = "POST") => serviceAPIRequest(`/api/v1/access/${path}`, body === undefined ? {} : { method, body: JSON.stringify(body) });
  let authGeneration = 0;
  let setupToken = "";
  const message = (id, text) => { el(id).textContent = text; };
  async function busy(form, action) {
    const controls = [...form.querySelectorAll("button")];
    controls.forEach(b => { b.disabled = true; });
    try { await action(); } finally { controls.forEach(b => { b.disabled = false; }); }
  }
  async function availability() {
    try { el("requestAccessButton").hidden = !(await api("status")).enabled; }
    catch { el("requestAccessButton").hidden = true; }
  }
  el("requestAccessButton").addEventListener("click", () => {
    el("accessRequestForm").hidden = false;
    message("accessRequestMessage", "");
    el("accessRequestDialog").showModal();
  });
  for (const dialog of document.querySelectorAll(".access-dialog")) {
    dialog.querySelector("[data-close-access]").addEventListener("click", () => dialog.close());
    dialog.addEventListener("click", event => {
      const bounds = dialog.getBoundingClientRect();
      if (event.target === dialog && (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom)) dialog.close();
    });
  }
  el("accessRequestForm").addEventListener("submit", event => {
    event.preventDefault();
    const form = event.currentTarget;
    busy(form, async () => {
      message("accessRequestMessage", "Submitting your request...");
      try {
        const result = await api("request", Object.fromEntries(new FormData(form)));
        form.reset(); form.hidden = true;
        message("accessRequestMessage", result.message);
      } catch (error) { message("accessRequestMessage", error.message); }
    });
  });
  function readSetupLink() {
    if (!location.hash.startsWith("#access-setup=")) return;
    setupToken = location.hash.slice(14);
    // Keep the bearer token out of the address bar, history, and storage.
    history.replaceState(null, "", location.pathname + location.search);
    el("accessSetupForm").reset();
    el("accessSetupForm").hidden = !/^[a-f0-9]{64}$/.test(setupToken);
    message("accessSetupMessage", el("accessSetupForm").hidden ? "This setup link is invalid. Ask the server owner to resend it." : "");
    if (!el("accessSetupDialog").open) el("accessSetupDialog").showModal();
  }
  el("accessSetupDialog").addEventListener("close", () => { el("accessSetupForm").reset(); });
  el("accessSetupForm").addEventListener("submit", event => {
    event.preventDefault();
    const form = event.currentTarget;
    if (form.elements.password.value !== form.elements.confirm.value) {
      message("accessSetupMessage", "Passwords do not match."); return;
    }
    busy(form, async () => {
      message("accessSetupMessage", "Finishing setup...");
      try {
        const result = await api("setup", { token: setupToken, password: form.elements.password.value });
        setupToken = ""; form.hidden = true;
        message("accessSetupMessage", result.message);
      } catch (error) { message("accessSetupMessage", error.message); }
      finally { form.reset(); }
    });
  });
  const statuses = { pending: "Awaiting approval", incomplete: "Setup needs attention", provisioning: "Creating account", awaiting_setup: "Waiting for recipient setup", setting_up: "Finishing setup", active: "Active", declined: "Declined" };
  function queue(items) {
    const container = el("accessQueue");
    const historyOpen = container.querySelector(".access-history")?.open || false;
    container.replaceChildren();
    if (!items.length) { container.textContent = "No access requests yet."; return; }
    const completed = item => ["active", "declined"].includes(item.status) && !item.error && !item.mail_error;
    const count = items.filter(completed).length;
    const history = document.createElement("details"); history.className = "access-history"; history.open = historyOpen;
    const summary = document.createElement("summary"); summary.textContent = `Completed requests (${count})`; history.append(summary);
    if (count === items.length) { const empty = document.createElement("p"); empty.textContent = "No requests need attention."; container.append(empty); }
    for (const item of items) {
      const card = document.createElement("article"); card.className = "access-queue-card";
      const title = document.createElement("h4"); title.textContent = `${item.name} - ${statuses[item.status] || item.status}`; card.append(title);
      const info = document.createElement("p"); info.textContent = `${item.email} | Server: ${item.username} | Connect: ${item.connect_username}`; card.append(info);
      for (const text of [item.error, item.mail_error]) {
        if (text) { const p = document.createElement("p"); p.className = "access-error"; p.textContent = text; card.append(p); }
      }
      const actions = document.createElement("div"); actions.className = "access-actions"; card.append(actions);
      const options = item.status === "pending" ? [["approve", "Approve"], ["decline", "Decline"], ["resend", "Resend notices"]]
        : item.status === "incomplete" ? [["approve", "Retry setup"]]
        : ["awaiting_setup", "active", "declined"].includes(item.status) ? [["resend", item.status === "awaiting_setup" ? "Send fresh setup link" : item.status === "active" ? "Resend sign-in instructions" : "Resend notice"]] : [];
      if (item.emby_id && ["incomplete", "awaiting_setup", "active"].includes(item.status)) options.push(["reapply-template", "Reapply template"], ["reset", "Reset after Emby deletion"]);
      for (const [action, label] of options) {
        const button = document.createElement("button"); button.type = "button"; button.className = "service-secondary-button"; button.textContent = label;
        button.addEventListener("click", () => busy(card, async () => {
          try {
            message("accessAdminMessage", "Updating request...");
            const result = await api(`requests/${item.id}/${action}`, {});
            if (state.authenticated) message("accessAdminMessage", action === "reapply-template" ? "Template settings verified. Account is disabled until the recipient finishes the new setup email." : action === "reset" ? "Old setup links cleared. Approve this request again to create a new account." : result.message);
          } catch (error) { if (state.authenticated) message("accessAdminMessage", error.message); }
          await loadQueue();
        })); actions.append(button);
      }
      if (["pending", "incomplete"].includes(item.status)) {
        const details = document.createElement("details");
        const summary = document.createElement("summary"); summary.textContent = "Correct contact details"; details.append(summary);
        const form = document.createElement("form"); form.className = "access-correction";
        for (const [name, title, type, value] of [["email", "Notification email", "email", item.email], ["connect_username", "Emby Connect username", "text", item.connect_username]]) {
          const label = document.createElement("label"); label.className = "service-form-field";
          const span = document.createElement("span"); span.textContent = title;
          const input = document.createElement("input"); input.name = name; input.type = type; input.value = value; input.required = true;
          label.append(span, input); form.append(label);
        }
        const save = document.createElement("button"); save.type = "submit"; save.className = "service-secondary-button"; save.textContent = "Save correction"; form.append(save);
        form.addEventListener("submit", event => {
          event.preventDefault(); busy(card, async () => {
            try { await api(`requests/${item.id}/correct`, Object.fromEntries(new FormData(form))); if (state.authenticated) message("accessAdminMessage", "Details corrected. You can now approve or retry setup."); }
            catch (error) { if (state.authenticated) message("accessAdminMessage", error.message); }
            await loadQueue();
          });
        });
        details.append(form); card.append(details);
      }
      (completed(item) ? history : container).append(card);
    }
    if (count) container.append(history);
  }
  async function loadQueue() {
    if (!state.authenticated) return;
    const generation = authGeneration;
    try {
      const result = await api("requests");
      if (state.authenticated && generation === authGeneration) queue(result.requests);
    } catch (error) { if (state.authenticated && generation === authGeneration) message("accessAdminMessage", error.message); }
  }
  window.refreshAccessAdmin = async () => {
    const generation = ++authGeneration;
    el("accessAdminPanel").hidden = !state.authenticated;
    el("accessQueue").replaceChildren();
    el("accessSettingsForm").reset();
    message("accessAdminMessage", "");
    if (!state.authenticated) return;
    try {
      const settings = await api("settings");
      if (!state.authenticated || generation !== authGeneration) return;
      const form = el("accessSettingsForm");
      for (const control of form.elements) {
        if (!control.name) continue;
        if (control.type === "checkbox") control.checked = Boolean(settings[control.name]);
        else if (control.type !== "password") control.value = settings[control.name] ?? "";
      }
      form.elements.emby_key.placeholder = settings.has_emby_key ? "Saved - leave blank to keep" : "Emby API key";
      form.elements.smtp_password.placeholder = settings.has_smtp_password ? "Saved - leave blank to keep" : "Mailbox password";
      await loadQueue();
    } catch (error) { if (state.authenticated && generation === authGeneration) message("accessAdminMessage", error.message); }
  };
  el("accessSettingsForm").addEventListener("submit", event => {
    event.preventDefault();
    const form = event.currentTarget;
    busy(form, async () => {
      const data = Object.fromEntries(new FormData(form));
      data.enabled = form.elements.enabled.checked;
      data.smtp_port = Number(data.smtp_port);
      try {
        await api("settings", data, "PUT");
        form.elements.emby_key.value = ""; form.elements.smtp_password.value = "";
        if (state.authenticated) message("accessAdminMessage", "Access settings saved.");
        await availability();
      } catch (error) { if (state.authenticated) message("accessAdminMessage", error.message); }
    });
  });
  el("accessTestEmail").addEventListener("click", () => busy(el("accessSettingsForm"), async () => {
    message("accessAdminMessage", "Sending test using saved settings...");
    try { const result = await api("test-email", {}); if (state.authenticated) message("accessAdminMessage", result.message); }
    catch (error) { if (state.authenticated) message("accessAdminMessage", error.message); }
  }));
  el("accessRefresh").addEventListener("click", loadQueue);
  addEventListener("hashchange", readSetupLink);
  availability(); readSetupLink(); window.refreshAccessAdmin();
})();
