const state = {
  activity: [],
  missing: [],
  missingErrors: [],
  downloads: [],
  downloadErrors: [],
  processing: [],
  processingErrors: [],
  playback: [],
  services: [],
  managedServices: [],
  authenticated: false,
  setupRequired: false,
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

  $("pendingCount").textContent = lifecycles.filter(
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

// Match normalized records by their complete reference, never by title or ID alone.
function pipelineRecords(item, type, records) {
  return records.filter((record) => (item.references || []).some((ref) =>
    ref.record_type === type && ref.source === record.source &&
    Number(ref.source_service_id) === Number(record.source_service_id) &&
    String(ref.record_id) === String(record.id)
  ));
}

function pipelineProgress(item) {
  const problems = (item.problems || []).filter((problem) => problem !== "missing");
  const stages = {
    requested: "Requested", wanted: "Waiting for download", downloading: "Downloading",
    downloaded: "Download complete", processing: "Processing", importing: "Importing",
    available: "Ready to watch", playing: "Playing",
  };
  let label = stages[item.stage] || pretty(item.stage || "Waiting");
  let percent = null;
  let detail = "Progress not reported";
  let sources = [];
  let paused = false;
  if (item.stage === "downloading") {
    const downloads = pipelineRecords(item, "download", state.downloads);
    // Prefer the download client's byte counts over an ARR copy of the same queue.
    const clients = downloads.filter((d) => ["qbittorrent", "nzbget"].includes(d.source));
    const records = clients.length ? clients : downloads;
    sources = records;
    const measured = records.filter((d) => Number.isFinite(Number(d.size)) && Number(d.size) > 0);
    if (measured.length && measured.length === records.length) {
      const total = measured.reduce((sum, d) => sum + Number(d.size), 0);
      const remaining = measured.reduce((sum, d) => sum + Math.min(Number(d.size), Math.max(0, Number(d.size_left) || 0)), 0);
      percent = 100 * (total - remaining) / total;
      detail = `${formatBytes(total - remaining)} of ${formatBytes(total)}`;
    }
    paused = records.some((d) => /paused|queued/i.test(d.status || ""));
    if (paused) label = "Download paused / queued";
    if (records.length === 1 && records[0].time_left && !paused) detail += ` · ${records[0].time_left} remaining`;
  } else if (item.stage === "processing") {
    const jobs = pipelineRecords(item, "processing", state.processing);
    sources = jobs;
    paused = jobs.some((job) => ["held", "queued", "problem"].includes(job.state));
    if (jobs.length && jobs.every((job) => typeof job.progress === "number" && Number.isFinite(job.progress))) {
      percent = jobs.reduce((sum, job) => sum + Math.max(0, Math.min(100, job.progress)), 0) / jobs.length;
      detail = jobs.length > 1 ? `Average across ${jobs.length} processing jobs` : "Current processing job";
    }
    if (jobs.length === 1) {
      label = jobs[0].state === "queued" ? "Queued for processing" : jobs[0].state === "held" ? "Processing on hold" : label;
      if (jobs[0].stage) detail = pretty(jobs[0].stage);
    }
  } else if (["downloaded", "available", "playing"].includes(item.stage)) {
    percent = 100;
    detail = item.stage === "downloaded" ? "Waiting for the next stage" : "Media is available";
  } else if (["requested", "wanted"].includes(item.stage)) {
    detail = "Waiting for a download to start";
  }
  if (percent !== null) percent = Math.max(0, Math.min(100, percent));
  const serviceNames = [...new Set(sources.map((source) =>
    state.services.find((service) => Number(service.id) === Number(source.source_service_id))?.name || pretty(source.source)
  ))];
  return { label, percent, detail, problems, serviceNames, paused };
}

function pipelineCardMarkup(item) {
  const progress = pipelineProgress(item);
  const title = item.title || "Unknown media";
  const waiting = progress.percent === null;
  const steps = [
    ["requested", "Requested"], ["downloading", "Download"],
    ["processing", "Process"], ["importing", "Import"], ["available", "Ready"],
  ];
  const current = { wanted: "requested", downloaded: "downloading", playing: "available" }[item.stage] || item.stage;
  return `
    <div class="pipeline-card-heading">
      <div><h3>${escapeHTML(title)}</h3><p class="item-meta">${escapeHTML(mediaLabel(item))}</p></div>
      <span class="pipeline-stage">${escapeHTML(progress.label)}</span>
    </div>
    <div class="pipeline-progress-label"><span>${waiting ? "Awaiting progress" : "Current stage"}</span><strong>${waiting ? "—" : `${Math.round(progress.percent)}%`}</strong></div>
    <div class="pipeline-progress${waiting ? " unmeasured" : ""}${progress.problems.length || progress.paused ? " paused" : ""}" role="progressbar" aria-label="${escapeHTML(title)}: ${escapeHTML(progress.label)}" aria-valuemin="0" aria-valuemax="100" ${waiting ? 'aria-valuetext="Progress not reported"' : `aria-valuenow="${Math.round(progress.percent)}"`}>
      <div class="pipeline-progress-fill" style="width:${waiting ? 100 : progress.percent}%"></div>
    </div>
    <p class="pipeline-detail">${escapeHTML(progress.detail)}${progress.serviceNames.length ? ` · ${escapeHTML(progress.serviceNames.join(", "))}` : ""}</p>
    <ol class="pipeline-steps" aria-label="Workflow stages">${steps.map(([stage, name]) => `<li${stage === current ? ' aria-current="step"' : ""}>${name}</li>`).join("")}</ol>
    ${progress.problems.length ? `<p class="pipeline-problem">${escapeHTML(progress.problems.map(pretty).join(" · "))}</p>` : ""}
  `;
}

function renderPipeline() {
  const priority = { importing: 0, processing: 1, downloading: 2 };
  // Pending requests and completed downloads stay off the active dashboard.
  const items = state.activity.filter((item) => Object.hasOwn(priority, item.stage))
    .sort((a, b) => priority[a.stage] - priority[b.stage] || String(a.title).localeCompare(String(b.title)));
  $("pipelineCount").textContent = items.length;
  const list = $("pipelineList");
  if (!items.length) {
    list.innerHTML = '<div class="empty-state">No active downloads, processing, or imports right now.</div>';
    return;
  }
  list.querySelector(".empty-state")?.remove();
  const existing = new Map([...list.children].map((card) => [card.dataset.lifecycleId, card]));
  const keep = new Set(items.map((item) => String(item.id)));
  for (const card of [...list.children]) {
    if (!keep.has(card.dataset.lifecycleId)) card.remove();
  }
  for (const item of items) {
    const id = String(item.id);
    const card = existing.get(id) || document.createElement("article");
    card.className = "pipeline-card";
    card.dataset.lifecycleId = id;
    const markup = pipelineCardMarkup(item);
    if (card.innerHTML !== markup) card.innerHTML = markup;
    list.append(card);
  }
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
          ${playbackPoster(session)}
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


function formatPlaybackTime(milliseconds) {
  const totalSeconds = Math.max(0, Math.floor(Number(milliseconds || 0) / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  }

  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

function playbackMode(session) {
  if (
    session.is_transcode ||
    String(session.video_decision || "").toLowerCase() === "transcode" ||
    String(session.audio_decision || "").toLowerCase() === "transcode"
  ) {
    return "transcode";
  }

  return "direct";
}

function filteredPlayback() {
  const search = $("playbackSearch").value.trim().toLowerCase();
  const playbackState = $("playbackState").value;
  const mode = $("playbackMode").value;

  return state.playback.filter((session) => {
    if (playbackState && session.state !== playbackState) {
      return false;
    }

    if (mode && playbackMode(session) !== mode) {
      return false;
    }

    if (search) {
      const searchable = [
        session.media_title,
        session.show_title,
        session.artist_name,
        session.album_name,
        session.username,
        session.server_name,
        session.device,
        session.player,
        session.product,
        session.platform,
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();

      if (!searchable.includes(search)) {
        return false;
      }
    }

    return true;
  });
}

function renderPlaybackView() {
  const sessions = filteredPlayback();

  $("playbackViewCount").textContent = sessions.length;

  const playbackErrors = state.serviceErrors.filter(
    (item) => item.type === "tracearr"
  );

  if (playbackErrors.length) {
    $("playbackErrors").innerHTML = playbackErrors
      .map((item) => {
        const service = item.name || "Tracearr";
        return `<div>${escapeHTML(service)}: ${escapeHTML(item.error || "Unavailable")}</div>`;
      })
      .join("");
  } else {
    $("playbackErrors").innerHTML = "";
  }

  if (!sessions.length) {
    $("playbackViewList").innerHTML =
      state.playback.length === 0
        ? '<div class="empty-state">Nothing is playing right now.</div>'
        : '<div class="empty-state">No playback sessions match these filters.</div>';
    return;
  }

  $("playbackViewList").innerHTML = sessions
    .map((session) => {
      const isEpisode =
        session.media_type === "episode" && session.show_title;

      const title = isEpisode
        ? session.show_title
        : session.media_title || "Unknown media";

      const subtitle = [];

      if (isEpisode && session.season_number && session.episode_number) {
        subtitle.push(
          `S${String(session.season_number).padStart(2, "0")}E${String(
            session.episode_number
          ).padStart(2, "0")}`
        );
      }

      if (
        isEpisode &&
        session.media_title &&
        session.media_title !== title
      ) {
        subtitle.push(session.media_title);
      }

      if (session.year) {
        subtitle.push(String(session.year));
      }

      const client = [];
      if (session.player) client.push(session.player);
      if (session.device && session.device !== session.player) {
        client.push(session.device);
      }
      if (session.platform && !client.includes(session.platform)) {
        client.push(session.platform);
      }

      const duration = Number(session.duration_ms || 0);
      const progress = Number(session.progress_ms || 0);
      const percent =
        duration > 0
          ? Math.max(0, Math.min(100, (progress / duration) * 100))
          : 0;

      const mode = playbackMode(session);
      const modeLabel =
        mode === "transcode" ? "Transcode" : "Direct Play";

      const technical = [];

      if (session.video_decision) {
        technical.push(`Video: ${pretty(session.video_decision)}`);
      }

      if (session.audio_decision) {
        technical.push(`Audio: ${pretty(session.audio_decision)}`);
      }

      if (session.bitrate) {
        technical.push(`${Number(session.bitrate).toLocaleString()} kbps`);
      }

      const poster = playbackPoster(session);

      const progressMarkup =
        duration > 0
          ? `
            <div class="playback-detail-progress">
              <div class="progress-track">
                <div class="progress-bar" style="width:${percent.toFixed(1)}%"></div>
              </div>
              <div class="playback-time">
                <span>${escapeHTML(formatPlaybackTime(progress))}</span>
                <span>${escapeHTML(Math.round(percent))}%</span>
                <span>${escapeHTML(formatPlaybackTime(duration))}</span>
              </div>
            </div>
          `
          : `
            <div class="playback-live">
              ${session.media_type === "live" ? "LIVE" : "Duration unavailable"}
            </div>
          `;

      return `
        <article class="playback-card">
          ${poster}

          <div class="playback-card-body">
            <div class="playback-card-top">
              <div>
                <div class="item-title">${escapeHTML(title)}</div>
                ${
                  subtitle.length
                    ? `<div class="playback-subtitle">${escapeHTML(subtitle.join(" · "))}</div>`
                    : ""
                }
              </div>

              <div class="playback-card-badges">
                <span class="stage">${escapeHTML(pretty(session.state || "playing"))}</span>
                <span class="stage">${escapeHTML(modeLabel)}</span>
              </div>
            </div>

            <div class="playback-details">
              ${
                session.username
                  ? `<span><strong>User:</strong> ${escapeHTML(session.username)}</span>`
                  : ""
              }
              ${
                client.length
                  ? `<span><strong>Player:</strong> ${escapeHTML(client.join(" · "))}</span>`
                  : ""
              }
              ${
                session.server_name
                  ? `<span><strong>Server:</strong> ${escapeHTML(session.server_name)}</span>`
                  : ""
              }
              ${
                technical.length
                  ? `<span><strong>Stream:</strong> ${escapeHTML(technical.join(" · "))}</span>`
                  : ""
              }
            </div>

            ${progressMarkup}
          </div>
        </article>
      `;
    })
    .join("");
}

function renderServices() {
  $("serviceHealthPanel").hidden = !state.authenticated;
  if (!state.authenticated) {
    $("serviceList").replaceChildren();
    $("serviceHealthSummary").textContent = "";
    return;
  }
  const services = state.managedServices;
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

function activityProblems(item) {
  return Array.isArray(item.problems) ? item.problems : [];
}

function filteredActivity() {
  const search = $("activitySearch").value.trim().toLowerCase();
  const stage = $("activityStage").value;
  const problem = $("activityProblem").value;

  return state.activity.filter((item) => {
    const problems = activityProblems(item);

    if (stage && item.stage !== stage) {
      return false;
    }

    if (problem === "healthy" && problems.length) {
      return false;
    }

    if (problem === "problem" && !problems.length) {
      return false;
    }

    if (
      problem &&
      problem !== "healthy" &&
      problem !== "problem" &&
      !problems.includes(problem)
    ) {
      return false;
    }

    if (search) {
      const searchable = [
        item.title,
        item.kind,
        item.stage,
        item.year,
        item.season_number,
        item.episode_number,
        ...problems,
      ]
        .filter((value) => value !== undefined && value !== null)
        .join(" ")
        .toLowerCase();

      if (!searchable.includes(search)) {
        return false;
      }
    }

    return true;
  });
}

function renderActivity() {
  const items = filteredActivity();

  $("activityCount").textContent = items.length;

  if (!items.length) {
    $("activityList").innerHTML =
      '<div class="empty-state">No lifecycle activity matches these filters.</div>';
    return;
  }

  $("activityList").innerHTML = items
    .map((item) => {
      const problems = activityProblems(item);

      const problemBadges = problems
        .map(
          (problem) =>
            `<span class="activity-problem ${
              problem === "missing" ? "missing" : ""
            }">${escapeHTML(pretty(problem))}</span>`
        )
        .join("");

      return `
        <div class="activity-row">
          <div class="activity-main">
            <div class="item-title">${escapeHTML(item.title || "Unknown media")}</div>
            <div class="item-meta">
              <span>${escapeHTML(mediaLabel(item))}</span>
            </div>
          </div>
          <div class="activity-badges">
            ${problemBadges}
            <span class="stage">${escapeHTML(pretty(item.stage))}</span>
          </div>
        </div>
      `;
    })
    .join("");
}

function formatBytes(value) {
  const bytes = Number(value) || 0;

  if (bytes <= 0) {
    return "0 B";
  }

  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1
  );

  const amount = bytes / Math.pow(1024, index);

  return `${amount >= 10 || index === 0 ? amount.toFixed(0) : amount.toFixed(1)} ${units[index]}`;
}

function downloadProgress(item) {
  const size = Number(item.size) || 0;
  const left = Math.max(0, Number(item.size_left) || 0);

  if (size <= 0) {
    return null;
  }

  return Math.max(0, Math.min(100, ((size - left) / size) * 100));
}

function updateDownloadStatusOptions() {
  const select = $("downloadStatus");
  const selected = select.value;

  const statuses = [...new Set(
    state.downloads
      .filter((item) => item.source === "qbittorrent" || item.source === "nzbget")
      .map((item) => item.status)
      .filter(Boolean)
  )].sort();

  select.innerHTML =
    '<option value="">All status</option>' +
    statuses
      .map(
        (status) =>
          `<option value="${escapeHTML(status)}">${escapeHTML(pretty(status))}</option>`
      )
      .join("");

  if (statuses.includes(selected)) {
    select.value = selected;
  }
}

function filteredDownloads() {
  const search = $("downloadSearch").value.trim().toLowerCase();
  const source = $("downloadSource").value;
  const status = $("downloadStatus").value;

  return state.downloads.filter((item) => {
    if (item.source !== "qbittorrent" && item.source !== "nzbget") {
      return false;
    }

    if (source && item.source !== source) {
      return false;
    }

    if (status && item.status !== status) {
      return false;
    }

    if (search) {
      const searchable = [
        item.title,
        item.source,
        item.status,
        item.tracked_download_status,
        item.protocol,
        item.download_client,
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();

      if (!searchable.includes(search)) {
        return false;
      }
    }

    return true;
  });
}

function renderDownloads() {
  updateDownloadStatusOptions();

  const items = filteredDownloads();

  $("downloadCount").textContent = items.length;

  if (state.downloadErrors.length) {
    $("downloadErrors").innerHTML = state.downloadErrors
      .map((item) => {
        const service = item.name || pretty(item.type) || "Download service";
        return `<div>${escapeHTML(service)}: ${escapeHTML(item.error || "Unavailable")}</div>`;
      })
      .join("");
  } else {
    $("downloadErrors").innerHTML = "";
  }

  if (!items.length) {
    $("downloadList").innerHTML =
      state.downloads.length === 0
        ? '<div class="empty-state">No active downloads.</div>'
        : '<div class="empty-state">No downloads match these filters.</div>';
    return;
  }

  $("downloadList").innerHTML = items
    .map((item) => {
      const progress = downloadProgress(item);
      const progressText =
        progress === null ? "" : `${Math.round(progress)}%`;

      const source = pretty(item.source);
      const status = pretty(item.status);

      const details = [
        item.protocol ? pretty(item.protocol) : "",
        item.download_client || "",
        item.time_left ? `${item.time_left} remaining` : "",
      ].filter(Boolean);

      const size = Number(item.size) || 0;
      const left = Number(item.size_left) || 0;

      if (size > 0) {
        details.push(`${formatBytes(Math.max(0, left))} of ${formatBytes(size)} remaining`);
      }

      return `
        <div class="download-row">
          <div class="download-row-top">
            <div class="download-main">
              <div class="item-title">${escapeHTML(item.title || "Unknown download")}</div>
              <div class="item-meta">
                <span>${escapeHTML(details.join(" · ") || "Active download")}</span>
              </div>
            </div>

            <div class="download-badges">
              ${item.tracked_download_status
                ? `<span class="activity-problem">${escapeHTML(pretty(item.tracked_download_status))}</span>`
                : ""}
              <span class="stage">${escapeHTML(source)}</span>
              <span class="stage">${escapeHTML(status)}</span>
            </div>
          </div>

          ${progress === null
            ? ""
            : `
              <div class="download-progress" aria-label="Download progress ${escapeHTML(progressText)}">
                <div class="download-progress-bar" style="width:${progress.toFixed(2)}%"></div>
              </div>
              <div class="download-stats">
                <span>${escapeHTML(progressText)} complete</span>
              </div>
            `}
        </div>
      `;
    })
    .join("");
}

function filteredProcessing() {
  const search = $("processingSearch").value.trim().toLowerCase();
  const stage = $("processingStage").value;
  const processingState = $("processingState").value;

  return state.processing.filter((item) => {
    if (stage && item.stage !== stage) {
      return false;
    }

    if (processingState && item.state !== processingState) {
      return false;
    }

    if (search) {
      const searchable = [
        item.title,
        item.state,
        item.stage,
        item.health_check,
        item.transcode,
        item.node_name,
        item.worker_id,
        item.message,
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();

      if (!searchable.includes(search)) {
        return false;
      }
    }

    return true;
  });
}

function renderProcessing() {
  const items = filteredProcessing();

  $("processingCount").textContent = items.length;

  if (state.processingErrors.length) {
    $("processingErrors").innerHTML = state.processingErrors
      .map((item) => {
        const service = item.name || pretty(item.type) || "Tdarr";
        return `<div>${escapeHTML(service)}: ${escapeHTML(item.error || "Unavailable")}</div>`;
      })
      .join("");
  } else {
    $("processingErrors").innerHTML = "";
  }

  if (!items.length) {
    $("processingList").innerHTML =
      state.processing.length === 0
        ? '<div class="empty-state">No active processing jobs.</div>'
        : '<div class="empty-state">No processing jobs match these filters.</div>';
    return;
  }

  $("processingList").innerHTML = items
    .map((item) => {
      const details = [];

      if (item.health_check) {
        details.push(`Health: ${pretty(item.health_check)}`);
      }

      if (item.transcode) {
        details.push(`Transcode: ${pretty(item.transcode)}`);
      }

      if (item.node_name) {
        details.push(`Node: ${item.node_name}`);
      }

      if (item.worker_id) {
        details.push(`Worker: ${item.worker_id}`);
      }

      if (item.hold_until) {
        const hold = new Date(item.hold_until);
        if (!Number.isNaN(hold.getTime())) {
          details.push(`Held until ${hold.toLocaleString()}`);
        }
      }

      const progress = Number(item.progress) || 0;
      const showProgress =
        item.state === "processing" || progress > 0;

      const problemBadge =
        item.state === "problem"
          ? '<span class="activity-problem">Problem</span>'
          : item.state === "held"
            ? '<span class="activity-problem missing">Held</span>'
            : "";

      return `
        <div class="download-row">
          <div class="download-row-top">
            <div class="download-main">
              <div class="item-title">${escapeHTML(item.title || "Unknown processing job")}</div>
              <div class="item-meta">
                <span>${escapeHTML(details.join(" · ") || "Tdarr")}</span>
              </div>
              ${item.message
                ? `<div class="item-meta"><span>${escapeHTML(item.message)}</span></div>`
                : ""}
            </div>

            <div class="download-badges">
              ${problemBadge}
              <span class="stage">TDARR</span>
              ${item.stage
                ? `<span class="stage">${escapeHTML(pretty(item.stage))}</span>`
                : ""}
              <span class="stage">${escapeHTML(pretty(item.state))}</span>
            </div>
          </div>

          ${showProgress
            ? `
              <div class="download-progress" aria-label="Processing progress ${escapeHTML(Math.round(progress))}%">
                <div class="download-progress-bar" style="width:${Math.max(0, Math.min(100, progress)).toFixed(2)}%"></div>
              </div>
              <div class="download-stats">
                <span>${escapeHTML(Math.round(progress))}% complete</span>
              </div>
            `
            : ""}
        </div>
      `;
    })
    .join("");
}

function render() {
  renderSummary();
  renderPipeline();
  renderPlayback();
  renderPlaybackView();
  renderServices();
  renderServiceManagement();
  renderActivity();
  renderMissing();
  renderDownloads();
  renderProcessing();

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
    const [health, activity, playback, services, missing, downloads, processing] = await Promise.all([
      getJSON("/health"),
      getJSON("/api/v1/activity"),
      getJSON("/api/v1/playback"),
      getJSON("/api/v1/public/services"),
      getJSON("/api/v1/missing"),
      getJSON("/api/v1/downloads"),
      getJSON("/api/v1/processing"),
    ]);

    state.activity = activity.lifecycles || [];
    state.playback = playback.sessions || [];
    state.services = services.services || services || [];
    state.missing = missing.items || [];
    state.missingErrors = missing.errors || [];
    state.downloads = downloads.downloads || [];
    state.downloadErrors = downloads.errors || [];
    state.processing = processing.jobs || [];
    state.processingErrors = processing.errors || [];

    state.serviceErrors = [
      ...(activity.errors || []),
      ...(playback.errors || []),
      ...state.missingErrors,
      ...state.downloadErrors,
      ...state.processingErrors,
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

function setView(view) {
  if ($("navigationDrawer").open) $("navigationDrawer").close();
  if (view === "settings" && state.authenticated) loadRequestSettings();
  if (view === "services" && !state.authenticated) view = "settings";
  if (view === "services") {
    refreshServiceManagement().catch((error) => showServiceMessage(error.message, "error"));
  }
  const views = {
    dashboard: {
      element: "dashboardView",
      title: "Dashboard",
    },
    activity: {
      element: "activityView",
      title: "Activity",
    },
    missing: {
      element: "missingView",
      title: "Missing",
    },
    downloads: {
      element: "downloadsView",
      title: "Downloads",
    },
    processing: {
      element: "processingView",
      title: "Processing",
    },
    services: {
      element: "servicesView",
      title: "Services",
    },
    playback: {
      element: "playbackView",
      title: "Playback",
    },
    settings: {
      element: "settingsView",
      title: "Settings",
    },
  };

  if (!views[view]) {
    return;
  }

  Object.values(views).forEach((entry) => {
    $(entry.element).classList.remove("active");
  });

  $(views[view].element).classList.add("active");
  $("pageTitle").textContent = views[view].title;
  $("pageTitle").hidden = view === "dashboard";
  $("dashboardBrand").hidden = view !== "dashboard";
  $("pageHeader").classList.toggle("dashboard-topbar", view === "dashboard");

  document.querySelectorAll(".nav-item[data-view]").forEach((button) => {
    button.classList.toggle("active", button.dataset.view === view);
  });
}

document.querySelectorAll(".nav-item[data-view]").forEach((button) => {
  button.addEventListener("click", () => {
    setView(button.dataset.view);
  });
});

$("activitySearch").addEventListener("input", renderActivity);
$("activityStage").addEventListener("change", renderActivity);
$("activityProblem").addEventListener("change", renderActivity);

$("downloadSearch").addEventListener("input", renderDownloads);
$("downloadSource").addEventListener("change", renderDownloads);
$("downloadStatus").addEventListener("change", renderDownloads);

$("processingSearch").addEventListener("input", renderProcessing);
$("processingStage").addEventListener("change", renderProcessing);
$("processingState").addEventListener("change", renderProcessing);

$("playbackSearch").addEventListener("input", renderPlaybackView);
$("playbackState").addEventListener("change", renderPlaybackView);
$("playbackMode").addEventListener("change", renderPlaybackView);

$("logoutButton").addEventListener("click", async () => {
  try {
    await serviceAPIRequest("/api/v1/auth/logout", { method: "POST" });
    applyAuth({ authenticated: false, setup_required: false });
  } catch (error) {
    $("authMessage").textContent = error.message;
  }
});

function applyAuth(status) {
  state.authenticated = Boolean(status.authenticated);
  renderServices();
  $("requestSettingsPanel").hidden = !state.authenticated;
  if (state.authenticated) loadRequestSettings();
  else $("requestSettingsForm").reset();
  state.setupRequired = Boolean(status.setup_required);
  $("settingsUsername").textContent = state.authenticated ? `Signed in as ${status.username}` : "Public read-only access";
  $("logoutButton").hidden = !state.authenticated;
  $("authForm").hidden = state.authenticated;
  $("authSubmit").textContent = state.setupRequired ? "Create administrator" : "Sign in";
  $("authPassword").autocomplete = state.setupRequired ? "new-password" : "current-password";
  $("authPassword").minLength = state.setupRequired ? 12 : 1;
  $("authPassword").value = "";
  if (!state.authenticated) {
    state.managedServices = [];
    closeServiceEditor();
    $("serviceForm").reset();
    renderServiceManagement();
    if ($("servicesView").classList.contains("active")) setView("settings");
  }
}

async function refreshAuth() {
  try {
    applyAuth(await getJSON("/api/v1/auth/status"));
  } catch {
    applyAuth({});
    $("authMessage").textContent = "Unable to check your session. Try again.";
  }
}

$("authForm").addEventListener("submit", async (event) => {
  event.preventDefault();
  $("authSubmit").disabled = true;
  $("authMessage").textContent = "";
  try {
    const result = await serviceAPIRequest(`/api/v1/auth/${state.setupRequired ? "setup" : "login"}`, {
      method: "POST",
      body: JSON.stringify({ username: $("authUsername").value, password: $("authPassword").value }),
    });
    applyAuth(result);
    setView("services");
  } catch (error) {
    $("authMessage").textContent = error.message;
    await refreshAuth();
  } finally {
    $("authPassword").value = "";
    $("authSubmit").disabled = false;
  }
});

refreshAuth();
refreshDashboard();
setInterval(refreshDashboard, 10000);

function missingAvailability(item) {
  if (!item.air_date) {
    return "unknown";
  }

  const date = new Date(item.air_date);
  if (Number.isNaN(date.getTime())) {
    return "unknown";
  }

  const releaseDay = new Date(
    date.getFullYear(),
    date.getMonth(),
    date.getDate(),
  );

  const now = new Date();
  const today = new Date(
    now.getFullYear(),
    now.getMonth(),
    now.getDate(),
  );

  return releaseDay <= today ? "available" : "upcoming";
}

function missingDateLabel(item) {
  if (!item.air_date) {
    return "Date unknown";
  }

  const date = new Date(item.air_date);
  if (Number.isNaN(date.getTime())) {
    return "Date unknown";
  }

  return date.toLocaleDateString([], {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function missingMediaLabel(item) {
  if (item.kind === "episode") {
    const season = Number(item.season_number || 0);
    const episode = Number(item.episode_number || 0);

    if (season > 0 && episode > 0) {
      return `S${String(season).padStart(2, "0")}E${String(episode).padStart(2, "0")}`;
    }
  }

  if (item.kind === "movie" && item.year) {
    return String(item.year);
  }

  return pretty(item.kind || "media");
}

function filteredMissing() {
  const query = $("missingSearch").value.trim().toLowerCase();
  const kind = $("missingKind").value;
  const source = $("missingSource").value;
  const availability = $("missingAvailability").value;

  return state.missing.filter((item) => {
    if (kind && item.kind !== kind) {
      return false;
    }

    if (source && item.source !== source) {
      return false;
    }

    if (availability && missingAvailability(item) !== availability) {
      return false;
    }

    if (query) {
      const haystack = [
        item.title,
        item.year,
        item.kind,
        item.source,
        item.season_number,
        item.episode_number,
      ]
        .filter((value) => value !== undefined && value !== null)
        .join(" ")
        .toLowerCase();

      if (!haystack.includes(query)) {
        return false;
      }
    }

    return true;
  });
}

function renderMissing() {
  const items = filteredMissing();
  const list = $("missingList");
  const errors = $("missingErrors");

  $("missingViewCount").textContent = items.length;

  if (state.missingErrors.length) {
    errors.innerHTML = state.missingErrors
      .map((error) => `
        <div class="download-error">
          ${escapeHTML(error.message || error.error || "Missing-media source error")}
        </div>
      `)
      .join("");
  } else {
    errors.innerHTML = "";
  }

  if (!items.length) {
    list.innerHTML = `
      <div class="empty-state">
        No missing media matches these filters.
      </div>
    `;
    return;
  }

  const sorted = [...items].sort((a, b) => {
    const aAvailability = missingAvailability(a);
    const bAvailability = missingAvailability(b);

    const rank = {
      available: 0,
      upcoming: 1,
      unknown: 2,
    };

    if (rank[aAvailability] !== rank[bAvailability]) {
      return rank[aAvailability] - rank[bAvailability];
    }

    const aDate = a.air_date ? new Date(a.air_date).getTime() : 0;
    const bDate = b.air_date ? new Date(b.air_date).getTime() : 0;

    if (aAvailability === "upcoming" && aDate !== bDate) {
      return aDate - bDate;
    }

    if (aAvailability === "available" && aDate !== bDate) {
      return bDate - aDate;
    }

    return String(a.title || "").localeCompare(String(b.title || ""));
  });

  list.innerHTML = sorted
    .map((item) => {
      const availability = missingAvailability(item);

      const availabilityLabel = {
        available: "Available now",
        upcoming: "Upcoming",
        unknown: "Unknown date",
      }[availability];

      return `
        <article class="activity-row missing-row">
          <div class="activity-row-main">
            <div class="activity-row-title">
              ${escapeHTML(item.title || "Unknown media")}
            </div>

            <div class="activity-row-meta">
              <span>${escapeHTML(pretty(item.source))}</span>
              <span>${escapeHTML(pretty(item.kind))}</span>
              <span>${escapeHTML(missingMediaLabel(item))}</span>
              <span>${escapeHTML(missingDateLabel(item))}</span>
            </div>
          </div>

          <div class="activity-row-side">
            <span class="status-badge missing-${escapeHTML(availability)}">
              ${escapeHTML(availabilityLabel)}
            </span>
          </div>
        </article>
      `;
    })
    .join("");
}

["missingSearch", "missingKind", "missingSource", "missingAvailability"]
  .forEach((id) => {
    $(id).addEventListener("input", renderMissing);
    $(id).addEventListener("change", renderMissing);
  });

function serviceUsesLogin(type) {
  return type === "qbittorrent" || type === "nzbget";
}

function serviceNeedsNoCredential(type) {
  return type === "tdarr";
}

function serviceDefaultName(type) {
  const names = {
    radarr: "Radarr",
    sonarr: "Sonarr",
    lidarr: "Lidarr",
    seerr: "Seerr",
    tdarr: "Tdarr",
    qbittorrent: "qBittorrent",
    nzbget: "NZBGet",
    tracearr: "Tracearr",
  };

  return names[type] || pretty(type);
}

function renderServiceManagement() {
  const list = $("serviceManagementList");

  if (!state.managedServices.length) {
    list.innerHTML = `
      <div class="panel empty-state">
        No services configured. Add a service to get started.
      </div>
    `;
    return;
  }

  list.innerHTML = state.managedServices
    .map((service) => {
      const failed = state.serviceErrors.some(
        (error) => Number(error.service_id) === Number(service.id)
      );

      const stateLabel = !service.enabled
        ? "Disabled"
        : failed
          ? "Error"
          : "Healthy";

      return `
        <article class="panel service-management-card">
          <div class="service-management-card-header">
            <div>
              <div class="service-management-name">
                ${escapeHTML(service.name)}
              </div>
              <div class="service-management-type">
                ${escapeHTML(serviceDefaultName(service.type))}
              </div>
            </div>

            <span class="service-management-state ${
              failed ? "bad" : ""
            } ${!service.enabled ? "disabled" : ""}">
              <span class="status-dot ${failed ? "bad" : ""}"></span>
              ${escapeHTML(stateLabel)}
            </span>
          </div>

          <div class="service-management-details">
            <div>
              <span>URL</span>
              <strong>${escapeHTML(service.base_url)}</strong>
            </div>
            <div>
              <span>Credential</span>
              <strong>${
                serviceNeedsNoCredential(service.type)
                  ? "Not required"
                  : service.has_credential
                    ? "Configured"
                    : "Not configured"
              }</strong>
            </div>
          </div>

          <div class="service-management-actions">
            <button
              class="service-secondary-button"
              type="button"
              data-service-action="edit"
              data-service-id="${Number(service.id)}"
            >
              Edit
            </button>

            <button
              class="service-secondary-button"
              type="button"
              data-service-action="test"
              data-service-id="${Number(service.id)}"
            >
              Test
            </button>

            <button
              class="service-danger-button"
              type="button"
              data-service-action="delete"
              data-service-id="${Number(service.id)}"
            >
              Delete
            </button>
          </div>
        </article>
      `;
    })
    .join("");
}

function updateServiceCredentialFields() {
  const type = $("serviceType").value;
  const editing = Boolean($("serviceID").value);

  const apiFields = $("serviceAPIKeyFields");
  const loginFields = $("serviceLoginFields");
  const credentialSection = $("serviceCredentialSection");

  if (serviceNeedsNoCredential(type)) {
    credentialSection.hidden = true;
    return;
  }

  credentialSection.hidden = false;

  if (serviceUsesLogin(type)) {
    apiFields.hidden = true;
    loginFields.hidden = false;

    $("serviceLoginHelp").textContent = editing
      ? "Leave username and password blank to keep the saved credentials."
      : "Enter the username and password used by this service.";
  } else {
    apiFields.hidden = false;
    loginFields.hidden = true;

    $("serviceCredentialHelp").textContent = editing
      ? "Leave this blank to keep the saved credential."
      : "Enter the API key or access token for this service.";
  }
}

function closeServiceEditor() {
  $("serviceEditor").hidden = true;
  $("serviceForm").reset();
  $("serviceID").value = "";
  $("serviceEnabled").checked = true;
}

function openServiceEditor(service = null) {
  $("serviceForm").reset();

  $("serviceID").value = service ? service.id : "";
  $("serviceType").value = service ? service.type : "";
  $("serviceName").value = service ? service.name : "";
  $("serviceBaseURL").value = service ? service.base_url : "";
  $("serviceEnabled").checked = service ? Boolean(service.enabled) : true;

  $("serviceCredential").value = "";
  $("serviceUsername").value = "";
  $("servicePassword").value = "";

  $("serviceEditorKicker").textContent =
    service ? "EDIT INTEGRATION" : "NEW INTEGRATION";

  $("serviceEditorTitle").textContent =
    service ? `Edit ${service.name}` : "Add Service";

  $("serviceSaveButton").textContent =
    service ? "Save Changes" : "Add Service";

  updateServiceCredentialFields();

  $("serviceEditor").hidden = false;
  $("serviceEditor").scrollIntoView({
    behavior: "smooth",
    block: "start",
  });
}

function showServiceMessage(message, kind = "") {
  const element = $("serviceManagementMessage");
  element.textContent = message;
  element.className = `service-management-message ${kind}`.trim();
}

$("addServiceButton").addEventListener("click", () => {
  showServiceMessage("");
  openServiceEditor();
});

$("serviceEditorClose").addEventListener("click", closeServiceEditor);
$("serviceCancelButton").addEventListener("click", closeServiceEditor);

$("serviceType").addEventListener("change", () => {
  if (!$("serviceID").value && !$("serviceName").value.trim()) {
    $("serviceName").value = serviceDefaultName($("serviceType").value);
  }

  updateServiceCredentialFields();
});

$("serviceManagementList").addEventListener("click", (event) => {
  const button = event.target.closest("[data-service-action]");
  if (!button) {
    return;
  }

  const serviceID = Number(button.dataset.serviceId);
  const service = state.managedServices.find(
    (item) => Number(item.id) === serviceID
  );

  if (!service) {
    return;
  }

  if (button.dataset.serviceAction === "edit") {
    showServiceMessage("");
    openServiceEditor(service);
  }
});

async function serviceAPIRequest(url, options = {}) {
  const response = await fetch(url, {
    ...options,
    credentials: "same-origin",
    headers: {
      "X-Overmynd-Request": "1",
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...(options.headers || {}),
    },
  });

  if (response.status === 401) applyAuth({});
  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`;

    try {
      const body = await response.json();

      if (body && typeof body.error === "string" && body.error) {
        message = body.error;
      } else if (body && typeof body.message === "string" && body.message) {
        message = body.message;
      }
    } catch {
      // Keep the HTTP status text when the response is not JSON.
    }

    throw new Error(message);
  }

  if (response.status === 204) {
    return null;
  }

  return response.json();
}

async function refreshServiceManagement() {
  const services = await serviceAPIRequest("/api/v1/services");
  if (!state.authenticated) return;
  state.managedServices = Array.isArray(services) ? services : [];
  renderServices();

  renderServiceManagement();
}

function serviceFormPayload() {
  const id = $("serviceID").value;
  const type = $("serviceType").value;
  const editing = Boolean(id);

  const payload = {
    type,
    name: $("serviceName").value.trim(),
    enabled: $("serviceEnabled").checked,
    base_url: $("serviceBaseURL").value.trim(),
    update_credential: false,
  };

  if (serviceNeedsNoCredential(type)) {
    if (editing) {
      payload.update_credential = false;
    }

    return payload;
  }

  if (serviceUsesLogin(type)) {
    const username = $("serviceUsername").value;
    const password = $("servicePassword").value;
    const replacingCredential = username !== "" || password !== "";

    if (replacingCredential) {
      payload.username = username;
      payload.password = password;
      payload.update_credential = true;
    }

    return payload;
  }

  const credential = $("serviceCredential").value;

  if (credential !== "") {
    payload.credential = credential;
    payload.update_credential = true;
  }

  return payload;
}

$("serviceForm").addEventListener("submit", async (event) => {
  event.preventDefault();

  const id = $("serviceID").value;
  const editing = Boolean(id);
  const saveButton = $("serviceSaveButton");
  const originalLabel = saveButton.textContent;

  saveButton.disabled = true;
  saveButton.textContent = editing ? "Saving…" : "Adding…";
  showServiceMessage("");

  try {
    const payload = serviceFormPayload();

    if (!payload.type) {
      throw new Error("Select a service type.");
    }

    if (!payload.name) {
      throw new Error("Enter a service name.");
    }

    if (!payload.base_url) {
      throw new Error("Enter the service base URL.");
    }

    if (!editing && !serviceNeedsNoCredential(payload.type)) {
      if (
        serviceUsesLogin(payload.type) &&
        !payload.update_credential
      ) {
        throw new Error("Enter the service username and password.");
      }

      if (
        !serviceUsesLogin(payload.type) &&
        !payload.update_credential
      ) {
        throw new Error("Enter the service API key or token.");
      }
    }

    await serviceAPIRequest(
      editing
        ? `/api/v1/services/${Number(id)}`
        : "/api/v1/services",
      {
        method: editing ? "PUT" : "POST",
        body: JSON.stringify(payload),
      }
    );

    closeServiceEditor();
    await refreshServiceManagement();

    showServiceMessage(
      editing
        ? "Service updated successfully."
        : "Service added successfully.",
      "success"
    );
  } catch (error) {
    showServiceMessage(
      error instanceof Error ? error.message : "Unable to save service.",
      "error"
    );
  } finally {
    saveButton.disabled = false;
    saveButton.textContent = originalLabel;
  }
});

async function testManagedService(service, button) {
  const originalLabel = button.textContent;

  button.disabled = true;
  button.textContent = "Testing…";
  showServiceMessage("");

  try {
    const result = await serviceAPIRequest(
      `/api/v1/services/${Number(service.id)}/test`,
      {
        method: "POST",
      }
    );

    const detail =
      result && typeof result.message === "string" && result.message
        ? ` ${result.message}`
        : "";

    showServiceMessage(
      `${service.name} connection successful.${detail}`,
      "success"
    );
  } catch (error) {
    showServiceMessage(
      `${service.name}: ${
        error instanceof Error ? error.message : "connection test failed"
      }`,
      "error"
    );
  } finally {
    button.disabled = false;
    button.textContent = originalLabel;
  }
}

async function deleteManagedService(service, button) {
  const confirmed = window.confirm(
    `Delete ${service.name} from Overmynd?\n\nThis removes only the Overmynd integration. It does not change the service itself.`
  );

  if (!confirmed) {
    return;
  }

  const originalLabel = button.textContent;

  button.disabled = true;
  button.textContent = "Deleting…";
  showServiceMessage("");

  try {
    await serviceAPIRequest(
      `/api/v1/services/${Number(service.id)}`,
      {
        method: "DELETE",
      }
    );

    if (Number($("serviceID").value) === Number(service.id)) {
      closeServiceEditor();
    }

    await refreshServiceManagement();

    showServiceMessage(
      `${service.name} removed from Overmynd.`,
      "success"
    );
  } catch (error) {
    showServiceMessage(
      `${service.name}: ${
        error instanceof Error ? error.message : "unable to delete service"
      }`,
      "error"
    );

    button.disabled = false;
    button.textContent = originalLabel;
  }
}

$("serviceManagementList").addEventListener("click", async (event) => {
  const button = event.target.closest("[data-service-action]");

  if (!button) {
    return;
  }

  const action = button.dataset.serviceAction;

  if (action !== "test" && action !== "delete") {
    return;
  }

  const serviceID = Number(button.dataset.serviceId);
  const service = state.managedServices.find(
    (item) => Number(item.id) === serviceID
  );

  if (!service) {
    showServiceMessage("Service could not be found.", "error");
    return;
  }

  if (action === "test") {
    await testManagedService(service, button);
    return;
  }

  await deleteManagedService(service, button);
});


// Native modal navigation supplies focus trapping, Escape handling and backdrop.
$("menuToggle").addEventListener("click", () => {
  $("navigationDrawer").showModal();
  $("menuToggle").setAttribute("aria-expanded", "true");
});
$("menuClose").addEventListener("click", () => $("navigationDrawer").close());
$("navigationDrawer").addEventListener("close", () => {
  $("menuToggle").setAttribute("aria-expanded", "false");
  $("menuToggle").focus();
});
$("navigationDrawer").addEventListener("click", (event) => {
  if (event.target !== $("navigationDrawer")) return;
  const bounds = event.target.getBoundingClientRect();
  if (event.clientX > bounds.right || event.clientY > bounds.bottom) event.target.close();
});

function playbackPoster(session) {
  const src = typeof session.poster_url === "string" && session.poster_url.startsWith("/api/v1/playback/poster?") ? session.poster_url : "";
  return `<div class="playback-art"><span class="poster-fallback" aria-label="Poster unavailable">▶</span>${src ? `<img src="${escapeHTML(src)}" alt="" loading="lazy" class="playback-poster">` : ""}</div>`;
}
document.addEventListener("error", (event) => {
  if (event.target.matches?.(".playback-poster, .request-poster")) event.target.hidden = true;
}, true);

let mediaSearchResults = [];
let selectedMediaRequest = null;
let searchGeneration = 0;
async function refreshRequestStatus() {
  try {
    const status = await getJSON("/api/v1/media-request/status");
    $("mediaSearchButton").disabled = !status.enabled;
    $("mediaRequestMessage").textContent = status.enabled ? "" : "Requests are not enabled. An administrator can configure Seerr under Settings.";
  } catch {
    $("mediaSearchButton").disabled = true;
    $("mediaRequestMessage").textContent = "Unable to check Seerr. Refresh the page to try again.";
  }
}

async function loadRequestSettings() {
  try {
    const [settings, integrations] = await Promise.all([
      serviceAPIRequest("/api/v1/request-settings"), serviceAPIRequest("/api/v1/services"),
    ]);
    if (!state.authenticated) return;
    $("requestServiceID").innerHTML = '<option value="">Select Seerr</option>' + integrations.filter(s => s.type === "seerr" && s.enabled).map(s => `<option value="${Number(s.id)}">${escapeHTML(s.name)}</option>`).join("");
    $("requestsEnabled").checked = settings.enabled;
    $("requestServiceID").value = settings.service_id || "";
    $("requestUserID").value = settings.user_id || "";
    requestSettingsRequired();
  } catch (error) { $("requestSettingsMessage").textContent = error.message; }
}
function requestSettingsRequired() {
  $("requestServiceID").required = $("requestsEnabled").checked;
  $("requestUserID").required = $("requestsEnabled").checked;
}
$("requestsEnabled").addEventListener("change", requestSettingsRequired);
$("requestSettingsForm").addEventListener("submit", async event => {
  event.preventDefault();
  $("requestSettingsSave").disabled = true;
  try {
    await serviceAPIRequest("/api/v1/request-settings", { method:"PUT", body:JSON.stringify({
      enabled:$("requestsEnabled").checked, service_id:Number($("requestServiceID").value), user_id:Number($("requestUserID").value),
    }) });
    $("requestSettingsMessage").textContent = "Saved. Requests will use this Seerr account and its approval permissions.";
    await refreshRequestStatus();
  } catch (error) { $("requestSettingsMessage").textContent = error.message; }
  finally { $("requestSettingsSave").disabled = false; }
});

$("mediaSearchForm").addEventListener("submit", async event => {
  event.preventDefault();
  const generation = ++searchGeneration;
  $("mediaSearchButton").disabled = true;
  $("mediaRequestMessage").textContent = "Searching Seerr…";
  $("mediaSearchResults").replaceChildren();
  try {
    const response = await serviceAPIRequest(`/api/v1/media-request/search?query=${encodeURIComponent($("mediaSearchQuery").value.trim())}`);
    if (generation !== searchGeneration) return;
    mediaSearchResults = response.results || [];
    $("mediaSearchResults").innerHTML = mediaSearchResults.map((item,index) => {
      const title = item.title || item.name || "Untitled";
      const existing = [2,3,5].includes(item.mediaInfo?.status);
      const availability = item.mediaInfo?.status === 5 ? "Available" : "Already requested";
      const poster = /^\/[A-Za-z0-9_.-]+$/.test(item.posterPath || "") ? `https://image.tmdb.org/t/p/w185${item.posterPath}` : "";
      return `<article class="request-result"><div class="request-art">${poster ? `<img class="request-poster" src="${escapeHTML(poster)}" alt="" loading="lazy">` : ""}</div><div class="request-result-content"><h3>${escapeHTML(title)}</h3><p>${item.mediaType === "tv" ? "TV show" : "Movie"} · ${escapeHTML((item.releaseDate || item.firstAirDate || "").slice(0,4))}</p><p class="request-overview">${escapeHTML(item.overview || "No description available.")}</p><button class="service-primary-button" type="button" data-request-index="${index}" ${existing ? "disabled" : ""}>${existing ? availability : "Request"}</button></div></article>`;
    }).join("");
    $("mediaRequestMessage").textContent = mediaSearchResults.length ? `${mediaSearchResults.length} results. Select a title to confirm your request.` : "No movies or TV shows found. Try another title.";
  } catch (error) { $("mediaRequestMessage").textContent = error.message; }
  finally { $("mediaSearchButton").disabled = false; }
});
$("mediaSearchResults").addEventListener("click", event => {
  const button = event.target.closest("[data-request-index]");
  if (!button || button.disabled) return;
  selectedMediaRequest = { item:mediaSearchResults[Number(button.dataset.requestIndex)], button };
  const item = selectedMediaRequest.item;
  $("requestConfirmTitle").textContent = `Request ${item.title || item.name}?`;
  $("requestConfirmDescription").textContent = item.mediaType === "tv" ? "This requests all seasons through Seerr." : "This sends a movie request to Seerr.";
  $("requestSeasonsLabel").hidden = item.mediaType !== "tv";
  $("requestAllSeasons").checked = false;
  $("requestConfirmMessage").textContent = "";
  $("mediaRequestDialog").showModal();
});
$("requestCancel").addEventListener("click", () => $("mediaRequestDialog").close());
$("requestConfirm").addEventListener("click", async () => {
  if (!selectedMediaRequest) return;
  const {item,button} = selectedMediaRequest;
  if (item.mediaType === "tv" && !$("requestAllSeasons").checked) {
    $("requestConfirmMessage").textContent = "Confirm all seasons before sending this request.";
    return;
  }
  $("requestConfirm").disabled = true;
  $("requestCancel").disabled = true;
  try {
    const result = await serviceAPIRequest("/api/v1/media-request", { method:"POST", body:JSON.stringify({ media_id:item.id, media_type:item.mediaType, all_seasons:item.mediaType === "tv" }) });
    button.disabled = true; button.textContent = "Requested";
    $("mediaRequestMessage").textContent = result.status === 1 ? "Request sent. Waiting for approval in Seerr." : "Request accepted by Seerr.";
    $("mediaRequestDialog").close();
  } catch (error) { $("requestConfirmMessage").textContent = error.message; }
  finally { $("requestConfirm").disabled = false; $("requestCancel").disabled = false; }
});
$("mediaRequestDialog").addEventListener("cancel", event => { if ($("requestConfirm").disabled) event.preventDefault(); });
refreshRequestStatus();
