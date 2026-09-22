const state = {
  activity: [],
  playback: [],
  services: [],
  serviceErrors: [],
};

const $ = (id) => document.getElementById(id);

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function pretty(value) {
  return String(value ?? "")
    .replaceAll("_", " ")
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function mediaLabel(item) {
  const parts = [];

  if (item.kind) {
    parts.push(pretty(item.kind));
  }

  if (item.season_number && item.episode_number) {
    parts.push(
      `S${String(item.season_number).padStart(2, "0")}E${String(item.episode_number).padStart(2, "0")}`
    );
  } else if (item.year) {
    parts.push(item.year);
  }

  return parts.join(" · ");
}

function problemCount(lifecycles) {
  return lifecycles.filter(
    (item) =>
      Array.isArray(item.problems) &&
      item.problems.some((problem) => problem !== "missing")
  ).length;
}

function renderSummary() {
  const lifecycles = state.activity;

  $("missingCount").textContent = lifecycles.filter(
    (item) => Array.isArray(item.problems) && item.problems.includes("missing")
  ).length;

  $("downloadingCount").textContent = lifecycles.filter(
    (item) =>
      item.stage === "downloading" ||
      item.stage === "downloaded"
  ).length;

  $("processingCount").textContent = lifecycles.filter(
    (item) =>
      item.stage === "processing" ||
      item.stage === "importing"
  ).length;

  $("problemCount").textContent = problemCount(lifecycles);
}

function renderPipeline() {
  const priority = {
    importing: 0,
    processing: 1,
    downloading: 2,
    downloaded: 3,
    requested: 4,
    wanted: 5,
    playing: 6,
    available: 7,
  };

  const items = [...state.activity]
    .sort(
      (a, b) =>
        (priority[a.stage] ?? 99) - (priority[b.stage] ?? 99)
    )
    .slice(0, 12);

  $("pipelineCount").textContent = state.activity.length;

  if (!items.length) {
    $("pipelineList").innerHTML =
      '<div class="empty-state">Nothing currently needs attention.</div>';
    return;
  }

  $("pipelineList").innerHTML = items
    .map((item) => {
      const problems = Array.isArray(item.problems)
        ? item.problems.filter((problem) => problem !== "missing")
        : [];

      return `
        <div class="pipeline-row">
          <div>
            <div class="item-title">${escapeHTML(item.title || "Unknown media")}</div>
            <div class="item-meta">
              <span>${escapeHTML(mediaLabel(item))}</span>
              ${
                problems.length
                  ? `<span>${escapeHTML(problems.map(pretty).join(", "))}</span>`
                  : ""
              }
            </div>
          </div>
          <span class="stage">${escapeHTML(pretty(item.stage))}</span>
        </div>
      `;
    })
    .join("");
}

function renderProblems() {
  const items = state.activity.filter(
    (item) =>
      Array.isArray(item.problems) &&
      item.problems.some((problem) => problem !== "missing")
  );

  $("problemsPanelCount").textContent = items.length;

  if (!items.length) {
    $("problemList").innerHTML =
      '<div class="empty-state">No active problems.</div>';
    return;
  }

  $("problemList").innerHTML = items
    .slice(0, 10)
    .map((item) => {
      const problems = item.problems.filter(
        (problem) => problem !== "missing"
      );

      return `
        <div class="problem-row">
          <div>
            <div class="item-title">${escapeHTML(item.title || "Unknown media")}</div>
            <div class="item-meta">
              <span>${escapeHTML(mediaLabel(item))}</span>
              <span>${escapeHTML(pretty(item.stage))}</span>
            </div>
          </div>
          <span class="problem-badge">${escapeHTML(
            problems.map(pretty).join(", ")
          )}</span>
        </div>
      `;
    })
    .join("");
}

function renderPlayback() {
  $("playbackCount").textContent = state.playback.length;

  if (!state.playback.length) {
    $("playbackList").innerHTML =
      '<div class="empty-state">Nothing is playing right now.</div>';
    return;
  }

  $("playbackList").innerHTML = state.playback
    .slice(0, 6)
    .map((session) => {
      const title =
        session.media_type === "episode" && session.show_title
          ? session.show_title
          : session.media_title || "Unknown media";

      const subtitleParts = [];

      if (session.season_number && session.episode_number) {
        subtitleParts.push(
          `S${String(session.season_number).padStart(2, "0")}E${String(
            session.episode_number
          ).padStart(2, "0")}`
        );
      }

      if (
        session.media_type === "episode" &&
        session.media_title &&
        session.media_title !== title
      ) {
        subtitleParts.push(session.media_title);
      }

      if (session.username) {
        subtitleParts.push(session.username);
      }

      const duration = Number(session.duration_ms || 0);
      const progress = Number(session.progress_ms || 0);
      const percent =
        duration > 0
          ? Math.max(0, Math.min(100, (progress / duration) * 100))
          : 0;

      return `
        <div class="playback-row">
          <div class="play-icon">▶</div>
          <div>
            <div class="item-title">${escapeHTML(title)}</div>
            <div class="item-meta">
              <span>${escapeHTML(subtitleParts.join(" · "))}</span>
              <span>${escapeHTML(pretty(session.state || "playing"))}</span>
            </div>
            <div class="progress-track">
              <div class="progress-bar" style="width:${percent.toFixed(1)}%"></div>
            </div>
          </div>
        </div>
      `;
    })
    .join("");
}

function renderServices() {
  const services = state.services;
  const errorsByID = new Set(
    state.serviceErrors.map((error) => Number(error.service_id))
  );

  const healthy = services.filter(
    (service) => service.enabled && !errorsByID.has(Number(service.id))
  ).length;

  const enabled = services.filter((service) => service.enabled).length;

  $("serviceHealthSummary").textContent = `${healthy} / ${enabled} HEALTHY`;

  if (!services.length) {
    $("serviceList").innerHTML =
      '<div class="empty-state">No services configured.</div>';
    return;
  }

  $("serviceList").innerHTML = services
    .filter((service) => service.enabled)
    .map((service) => {
      const failed = errorsByID.has(Number(service.id));

      return `
        <div class="service-row">
          <div>
            <div class="service-name">${escapeHTML(service.name)}</div>
            <div class="service-type">${escapeHTML(service.type)}</div>
          </div>
          <div class="service-state">
            <span class="status-dot ${failed ? "bad" : ""}"></span>
            ${failed ? "ERROR" : "HEALTHY"}
          </div>
        </div>
      `;
    })
    .join("");
}

function render() {
  renderSummary();
  renderPipeline();
  renderProblems();
  renderPlayback();
  renderServices();

  $("lastUpdated").textContent =
    `Updated ${new Date().toLocaleTimeString([], {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    })}`;
}

async function getJSON(path) {
  const response = await fetch(path, {
    cache: "no-store",
    headers: {
      Accept: "application/json",
    },
  });

  if (!response.ok) {
    throw new Error(`${path} returned ${response.status}`);
  }

  return response.json();
}

async function refreshDashboard() {
  try {
    const [health, activity, playback, services] = await Promise.all([
      getJSON("/health"),
      getJSON("/api/v1/activity"),
      getJSON("/api/v1/playback"),
      getJSON("/api/v1/services"),
    ]);

    state.activity = activity.lifecycles || [];
    state.playback = playback.sessions || [];
    state.services = services.services || services || [];

    state.serviceErrors = [
      ...(activity.errors || []),
      ...(playback.errors || []),
    ];

    $("sidebarHealth").textContent = "Online";
    $("sidebarHealthDot").classList.remove("bad");

    render();

    void health;
  } catch (error) {
    console.error("Dashboard refresh failed:", error);

    $("sidebarHealth").textContent = "Error";
    $("sidebarHealthDot").classList.add("bad");
    $("lastUpdated").textContent = "Refresh failed";
  }
}

refreshDashboard();
setInterval(refreshDashboard, 10000);
