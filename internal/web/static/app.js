"use strict";

const $ = (s) => document.querySelector(s);
const enc = encodeURIComponent;
const state = {
  token: localStorage.getItem("rp_token") || "",
  app: localStorage.getItem("rp_app") || "",
  pipelineRings: [],
  rings: [],
  history: [],
  activeJob: null,
};

// ---------- API ----------
async function api(path, method = "GET", body = null) {
  const opts = { method, headers: {} };
  if (state.token) opts.headers["Authorization"] = "Bearer " + state.token;
  if (body) { opts.headers["Content-Type"] = "application/json"; opts.body = JSON.stringify(body); }
  let resp;
  try { resp = await fetch(path, opts); } catch (_) { return { ok: false, status: 0, data: null }; }
  let data = null;
  try { data = await resp.json(); } catch (_) {}
  return { ok: resp.ok, status: resp.status, data };
}

// ---------- helpers ----------
function esc(s) {
  return String(s == null ? "" : s).replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}
function timeAgo(ts) {
  if (!ts) return "";
  const d = new Date(ts).getTime();
  if (isNaN(d)) return "";
  const s = Math.max(0, (Date.now() - d) / 1000);
  if (s < 60) return Math.floor(s) + "s ago";
  if (s < 3600) return Math.floor(s / 60) + "m ago";
  if (s < 86400) return Math.floor(s / 3600) + "h ago";
  return Math.floor(s / 86400) + "d ago";
}
function dur(a, b) {
  if (!a) return "";
  const start = new Date(a).getTime();
  const end = b ? new Date(b).getTime() : Date.now();
  const ms = Math.max(0, end - start);
  return ms < 1000 ? ms + "ms" : (ms / 1000).toFixed(1) + "s";
}
function nextRing(name) {
  const r = state.pipelineRings, i = r.findIndex((x) => x.name === name);
  return i >= 0 && i + 1 < r.length ? r[i + 1] : null;
}
function ringLabel(name) {
  const r = state.pipelineRings.find((x) => x.name === name);
  return r ? r.label : name;
}
let toastTimer = null;
function toast(msg, ok) {
  const el = $("#toast");
  el.textContent = msg;
  el.className = "toast " + (ok ? "ok" : "bad");
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (el.hidden = true), 6000);
}
function showStartFailure(res) {
  if (res.status === 401) return signOut();
  const m = (res.data && (res.data.error || res.data.message)) || "Request failed (" + res.status + ")";
  toast(m, false);
}

// ---------- auth gate ----------
async function tokenValid(tok) {
  try { const r = await fetch("/api/apps", { headers: { Authorization: "Bearer " + tok } }); return r.ok; }
  catch (_) { return false; }
}
function showGate(msg) {
  $("#app-root").hidden = true; $("#gate").hidden = false;
  const e = $("#gate-error"); if (msg) { e.textContent = msg; e.hidden = false; } else e.hidden = true;
  $("#gate-token").focus();
}
function unlock() { $("#gate").hidden = true; $("#app-root").hidden = false; loadApps(); }
function signOut() {
  state.token = ""; localStorage.removeItem("rp_token"); $("#gate-token").value = ""; showGate();
}

// ---------- loaders ----------
async function loadApps() {
  const res = await api("/api/apps");
  if (!res.ok) return showStartFailure(res);
  state.pipelineRings = res.data.rings || [];
  const sel = $("#app");
  sel.innerHTML = "";
  (res.data.apps || []).forEach((n) => {
    const o = document.createElement("option"); o.value = n; o.textContent = n; sel.appendChild(o);
  });
  if (state.app && res.data.apps.includes(state.app)) sel.value = state.app;
  state.app = sel.value;
  if (state.app) await loadApp();
}
async function loadApp() {
  if (!state.app) return;
  localStorage.setItem("rp_app", state.app);
  await Promise.all([loadRings(), loadHistory()]);
  renderAll();
}
async function loadRings() {
  const res = await api(`/api/apps/${enc(state.app)}/rings`);
  if (res.ok) state.rings = res.data.rings || [];
}
async function loadHistory() {
  const res = await api(`/api/apps/${enc(state.app)}/history`);
  if (res.ok) state.history = res.data.history || [];
}

// ---------- render ----------
function renderAll() { renderSummary(); renderPipeline(); renderHistory(); }

function renderSummary() {
  const rings = state.rings;
  const configured = rings.filter((r) => r.configured);
  const deployed = configured.filter((r) => r.current_version);
  const healthy = deployed.filter((r) => r.live_healthy);
  const anyBad = deployed.some((r) => !r.live_healthy);
  const prod = rings[rings.length - 1];
  const cards = [
    { label: "Environments", value: configured.length, sub: deployed.length + " deployed" },
    { label: "Healthy", value: healthy.length + "/" + (deployed.length || 0), cls: anyBad ? "bad" : (deployed.length ? "good" : ""), sub: anyBad ? "attention needed" : "all green" },
    { label: "Production", value: (prod && prod.current_version) || "—", mono: true, sub: prod ? prod.ring.label : "" },
  ];
  $("#summary").innerHTML = cards.map((c) =>
    `<div class="stat ${c.cls || ""}"><span class="stat-label">${esc(c.label)}</span>` +
    `<span class="stat-value ${c.mono ? "mono" : ""}" style="${c.mono ? "font-size:1.15rem" : ""}">${esc(String(c.value))}</span>` +
    `<span class="stat-sub">${esc(c.sub || "")}</span></div>`).join("");
}

function renderPipeline() {
  const el = $("#pipeline"); el.innerHTML = "";
  $("#pipeline-sub").textContent = state.app || "";
  state.rings.forEach((v, i) => {
    if (i > 0) { const c = document.createElement("div"); c.className = "connector"; el.appendChild(c); }
    el.appendChild(stageEl(v));
  });
}

function stageEl(v) {
  const d = document.createElement("div");
  const configured = v.configured, hasVer = !!v.current_version;
  const healthy = configured && hasVer && v.live_healthy;
  const busy = !!state.activeJob;
  const isTarget = busy && state.activeJob.app === state.app && state.activeJob.targetRing === v.ring.name;

  let cls = "stage";
  if (!configured || !hasVer) cls += " empty";
  else cls += healthy ? " healthy" : " unhealthy";
  if (isTarget) cls += " active";
  d.className = cls;

  const dot = !configured || !hasVer ? "na" : (healthy ? "ok" : "bad");
  const pill = !configured ? '<span class="pill na">not configured</span>'
    : !hasVer ? '<span class="pill na">not deployed</span>'
    : healthy ? '<span class="pill ok">● healthy</span>'
    : '<span class="pill bad">● unhealthy</span>';
  const meta = [];
  if (v.live_version && v.live_version !== v.current_version) meta.push("live: " + v.live_version);
  if (v.previous_version) meta.push("prev: " + v.previous_version);

  d.innerHTML =
    `<div class="stage-top"><div><div class="stage-title">${esc(v.ring.label)}</div>` +
    `<div class="stage-ring">${esc(v.ring.name)}</div></div><span class="dot ${dot}"></span></div>` +
    `<div class="stage-version"><span class="v-cur">${esc(v.current_version || "—")}</span>` +
    `<span class="v-meta">${esc(meta.join(" · "))}</span></div>` +
    `<div>${pill}</div><div class="stage-actions"></div>`;

  const actions = d.querySelector(".stage-actions");
  if (isTarget) {
    actions.className = "stage-running";
    actions.innerHTML = '<span class="dot accent"></span> deploying…'; // card already pulses; no spinner here
  } else if (configured) {
    fillActions(actions, v, busy);
  } else {
    actions.innerHTML = '<span class="muted" style="font-size:.8rem">—</span>';
  }
  return d;
}

function fillActions(actions, v, busy) {
  const row = document.createElement("div"); row.className = "seed-row";
  const input = document.createElement("input"); input.placeholder = "version"; input.disabled = busy;
  const seed = document.createElement("button"); seed.className = "btn btn-sm"; seed.textContent = "Seed"; seed.disabled = busy;
  seed.onclick = () => {
    const ver = input.value.trim();
    if (!ver) return toast("Enter a version to seed.", false);
    run("seed", { ring: v.ring.name, version: ver }, `Seed ${ver} → ${v.ring.label}`, v.ring.name);
  };
  row.append(input, seed);
  actions.appendChild(row);

  const brow = document.createElement("div");
  brow.style.cssText = "display:flex;gap:.4rem;width:100%";
  if (v.can_promote_from) {
    const nx = nextRing(v.ring.name);
    const p = document.createElement("button"); p.className = "btn btn-primary btn-sm"; p.textContent = "Promote →"; p.disabled = busy;
    p.onclick = () => run("promote", { from_ring: v.ring.name }, `Promote ${v.ring.label} → ${nx ? nx.label : ""}`, nx ? nx.name : null);
    brow.appendChild(p);
  }
  if (v.previous_version) {
    const rb = document.createElement("button"); rb.className = "btn btn-danger btn-sm"; rb.textContent = "Rollback"; rb.disabled = busy;
    rb.onclick = () => run("rollback", { ring: v.ring.name }, `Rollback ${v.ring.label}`, v.ring.name);
    brow.appendChild(rb);
  }
  if (brow.children.length) actions.appendChild(brow);
}

function renderHistory() {
  const el = $("#history"), h = state.history || [];
  if (!h.length) { el.innerHTML = '<div class="history-empty">No activity yet.</div>'; return; }
  el.innerHTML = h.slice(0, 30).map((x) => {
    const ok = x.result === "success";
    const ic = ok ? '<span class="ic success">✓</span>' : '<span class="ic failed">✕</span>';
    return `<div class="hrow">${ic}<span class="h-action">${esc(x.action)}</span>` +
      `<span class="h-detail">${esc(ringLabel(x.ring))} · <span class="mono">${esc(x.from_version || "—")}</span> → ` +
      `<span class="mono">${esc(x.to_version || "—")}</span>${x.message ? " · " + esc(x.message) : ""}</span>` +
      `<span class="h-time" title="${esc(x.created_at)}">${esc(timeAgo(x.created_at))}</span></div>`;
  }).join("");
}

// ---------- progress panel ----------
function stepIcon(st) {
  if (st === "running") return '<span class="spinner"></span>';
  if (st === "success") return '<span class="ic success">✓</span>';
  if (st === "failed") return '<span class="ic failed">✕</span>';
  return '<span class="ic skipped">–</span>';
}
function runIcon(st) {
  if (st === "success") return '<span class="ic success">✓</span>';
  if (st === "failed") return '<span class="ic failed">✕</span>';
  return '<span class="dot accent"></span>'; // static (the active step shows the only spinner)
}
function renderProgress() {
  const wrap = $("#progress-wrap"), a = state.activeJob;
  if (!a) { wrap.innerHTML = ""; return; }
  const job = a.job || { status: "pending", steps: [] };
  const status = job.status || "pending";
  const finished = status === "success" || status === "failed";
  const bar = status === "success" ? "done-ok" : status === "failed" ? "done-bad" : "running";
  const steps = (job.steps || []).map((s) => {
    const logs = (s.logs && s.logs.length) ? `<div class="step-logs">${s.logs.map(esc).join("\n")}</div>` : "";
    return `<div class="step"><div class="step-row">${stepIcon(s.status)}` +
      `<span class="step-title">${esc(s.title)}</span><span class="step-dur">${esc(dur(s.started_at, s.finished_at))}</span>` +
      `</div>${logs}</div>`;
  }).join("") || '<div class="muted" style="padding:.4rem .2rem">Starting…</div>';

  wrap.innerHTML =
    `<div class="run"><div class="run-head"><div class="run-title">${runIcon(status)} ${esc(a.title)}</div>` +
    `<div style="display:flex;gap:.6rem;align-items:center"><span class="badge ${status}">${esc(status)}</span>` +
    `${finished ? '<button class="btn btn-ghost btn-sm" id="run-dismiss">Dismiss</button>' : ""}</div></div>` +
    `<div class="run-bar ${bar}"><span></span></div><div class="steps">${steps}</div></div>`;

  if (finished) $("#run-dismiss").onclick = () => { state.activeJob = null; renderProgress(); renderAll(); };
}

// ---------- run an action (async job + polling) ----------
async function run(action, body, title, targetRing) {
  if (state.activeJob) return;
  state.activeJob = { action, title, targetRing, app: state.app, id: null, job: { status: "pending", steps: [] } };
  renderProgress();  // instant feedback
  renderAll();       // disable buttons + pulse target

  const res = await api(`/api/apps/${enc(state.app)}/${action}?async=1`, "POST", body);
  if (res.status !== 202 || !res.data || !res.data.job_id) {
    state.activeJob = null; renderProgress(); renderAll(); showStartFailure(res); return;
  }
  state.activeJob.id = res.data.job_id;
  pollJob();
}
async function pollJob() {
  const a = state.activeJob;
  if (!a || !a.id) return;
  const res = await api(`/api/apps/${enc(a.app)}/jobs/${a.id}`);
  if (!res.ok || !res.data) { setTimeout(pollJob, 1000); return; }
  a.job = res.data;
  renderProgress();
  if (res.data.status === "running" || res.data.status === "pending") { setTimeout(pollJob, 700); return; }
  // finished
  if (state.app === a.app) { await Promise.all([loadRings(), loadHistory()]); }
  renderAll(); renderProgress();
  const ok = res.data.status === "success";
  toast((res.data.result && res.data.result.message) || res.data.error || (ok ? "Done" : "Failed"), ok);
}

// ---------- wiring ----------
$("#gate-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const tok = $("#gate-token").value.trim();
  if (!tok) return showGate("Please enter a token.");
  if (await tokenValid(tok)) { state.token = tok; localStorage.setItem("rp_token", tok); unlock(); }
  else showGate("Invalid token. Please try again.");
});
$("#signout").onclick = signOut;
$("#refresh").onclick = () => loadApp();
$("#app").onchange = (e) => { state.app = e.target.value; state.activeJob = null; loadApp(); };

// ---------- init ----------
(async () => {
  if (state.token && (await tokenValid(state.token))) unlock();
  else showGate();
})();
