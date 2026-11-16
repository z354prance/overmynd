(() => {
  const output = document.getElementById("storageSummary");
  if (!output) return;
  const button = document.createElement("button");
  button.type = "button";
  button.className = "storage-trigger";
  button.setAttribute("popovertarget", "storagePopover");
  const popup = document.createElement("section");
  popup.id = "storagePopover";
  popup.setAttribute("popover", "auto");
  popup.setAttribute("aria-label", "Storage details");
  const heading = document.createElement("header");
  const title = document.createElement("strong"); title.textContent = "storage";
  const close = document.createElement("button"); close.type = "button";
  close.textContent = "[ close ]";
  close.setAttribute("popovertarget", "storagePopover");
  close.setAttribute("popovertargetaction", "hide");
  heading.append(title, close);
  const values = document.createElement("dl");
  const note = document.createElement("p");
  popup.append(heading, values, note);
  document.body.append(popup);
  output.append(button);
  function position() {
    if (!popup.matches(":popover-open")) return;
    const b = button.getBoundingClientRect();
    const p = popup.getBoundingClientRect();
    popup.style.left = `${Math.max(8, Math.min(innerWidth-p.width-8, b.left+(b.width-p.width)/2))}px`;
    popup.style.top = `${Math.max(8, Math.min(innerHeight-p.height-8, (b.bottom+8+p.height <= innerHeight-8 ? b.bottom+8 : b.top-p.height-8)))}px`;
  }
  popup.addEventListener("toggle", position);
  window.addEventListener("resize", position);
  window.addEventListener("scroll", position, true);
  function size(bytes) {
    if (bytes === null || bytes === undefined) return "unavailable";
    const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
    let value = Number(bytes), unit = 0;
    while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit++; }
    return `${value.toFixed(unit ? 1 : 0)} ${units[unit]}`;
  }
  async function refresh() {
    try {
      const response = await fetch("/api/v1/storage", {cache:"no-store"});
      if (!response.ok) throw new Error("unavailable");
      const data = await response.json();
      output.hidden = !data.enabled;
      if (!data.enabled) { popup.hidePopover(); return; }
      const known = Number.isFinite(data.total_bytes) && data.total_bytes > 0 && Number.isFinite(data.free_bytes);
      const free = known ? Math.max(0, Math.min(100, 100*data.free_bytes/data.total_bytes)) : null;
      if (known) {
        const total = data.total_bytes, used = Math.max(0, total - data.free_bytes);
        const groups = {movies:0, tv:0, music:0, books:0};
        let libraries = 0;
        for (const c of data.categories || []) {
          const n = Number(c.bytes) || 0, name = String(c.name).toLowerCase();
          const key = name === "movies" ? "movies" : name === "music" ? "music" : name === "books" ? "books" : (name === "tv" || name === "anime") ? "tv" : "";
          if (key) { groups[key] += n; libraries += n; }
        }
        const other = Math.max(0, used - libraries);
        const bar = document.createElement("span");
        bar.className = "cap-bar";
        const add = (cls, bytes, label) => {
          if (!(bytes > 0)) return;
          const seg = document.createElement("span");
          seg.className = "cap-seg " + cls;
          seg.style.flexGrow = String(bytes);
          seg.title = `${label}: ${size(bytes)}`;
          bar.append(seg);
        };
        add("cap-movies", groups.movies, "Movies"); add("cap-tv", groups.tv, "TV"); add("cap-music", groups.music, "Music"); add("cap-books", groups.books, "Books");
        add("cap-other", other, "Other"); add("cap-free", data.free_bytes, "Free");
        const left = document.createElement("span"); left.className = "cap-label"; left.textContent = "storage";
        const right = document.createElement("span"); right.className = "cap-free-text"; right.textContent = `${free.toFixed(1)}% free`;
        button.replaceChildren(left, bar, right);
        button.setAttribute("aria-label", `Storage: ${free.toFixed(1)} percent remaining. Show category sizes.`);
      } else {
        button.textContent = data.scanning ? "storage: scanning..." : "storage: unavailable";
        button.setAttribute("aria-label", button.textContent + ". Show storage details.");
      }
      values.replaceChildren();
      for (const item of [...(data.categories || []), {name:"Total capacity",bytes:data.total_bytes}, {name:"Remaining",bytes:data.free_bytes}]) {
        const row = document.createElement("div"), label = document.createElement("dt"), value = document.createElement("dd");
        label.textContent = item.name; value.textContent = size(item.bytes);
        row.append(label,value); values.append(row);
      }
      note.textContent = data.categories?.length ? `Categories are library file sizes. The bar shows overall filesystem usage, including other data. Updated ${new Date(data.updated_at).toLocaleString()}${data.scanning ? " (refreshing)" : ""}.` : "The first library scan is running.";
      position();
    } catch (_) { if (!output.hidden) { button.textContent = "storage: unavailable"; button.setAttribute("aria-label", "Storage unavailable"); note.textContent = "Refresh failed; any displayed values are from the previous update."; } }
    finally { setTimeout(refresh, 30000); }
  }
  refresh();
})();
