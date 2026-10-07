(() => {
  const output = document.getElementById("storageSummary");
  if (!output) return;
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
      if (!data.enabled) return;
      output.replaceChildren();
      if (!data.categories?.length) { output.textContent = "storage: scanning..."; return; }
      for (const item of [...data.categories, {name:"Free",bytes:data.free_bytes}]) {
        const span = document.createElement("span");
        span.textContent = `${item.name.toLowerCase()}: ${size(item.bytes)}`;
        output.append(span);
      }
      output.title = `Library file sizes; free space on the media filesystem. Updated ${new Date(data.updated_at).toLocaleString()}${data.scanning ? " (refreshing)" : ""}.`;
    } catch (_) { if (!output.hidden) output.textContent = "storage: unavailable"; }
    finally { setTimeout(refresh, 30000); }
  }
  refresh();
})();
