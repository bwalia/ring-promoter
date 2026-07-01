"use strict";

const $ = (sel) => document.querySelector(sel);
const state = { token: localStorage.getItem("rp_token") || "", app: "" };

// ---- API client ----
async function api(path, method = "GET", body = null) {
  const opts = { method, headers: {} };
  if (state.token) opts.headers["Authorization"] = "Bearer " + state.token;
  if (body) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const resp = await fetch(path, opts);
  let data = null;
  try { data = await resp.json(); } catch (_) { /* no body */ }
  return { ok: resp.ok, status: resp.status, data };
}

// ---- feedback ----
function feedback(msg, ok) {
  const el = $("#feedback");
  el.textContent = msg;
  el.className = "feedback " + (ok ? "ok" : "bad");
  el.hidden = false;
}

function showResult(res) {
  const d = res.data || {};
  if (res.status === 401) return feedback("Unauthorized — check your API token.", false);
  if (d.message) return feedback(d.message, res.ok);
  if (d.error) return feedback(d.error, false);
  feedback(res.ok ? "Done." : "Request failed (" + res.status + ").", res.ok);
}

// ---- badges / formatting ----
function healthBadge(view) {
  if (!view.configured) return '<span class="badge na">n/a</span>';
  return view.live_healthy
    ? '<span class="badge ok">healthy</span>'
    : '<span class="badge bad" title="' + esc(view.live_health_error || "") + '">unhealthy</span>';
}
function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}
function fmtTime(t) {
  if (!t) return "";
  const d = new Date(t);
  return isNaN(d) ? "" : d.toLocaleString();
}

// ---- loaders ----
async function loadApps() {
  const res = await api("/api/apps");
  if (!res.ok) return showResult(res);
  const sel = $("#app");
  sel.innerHTML = "";
  (res.data.apps || []).forEach((name) => {
    const o = document.createElement("option");
    o.value = name; o.textContent = name;
    sel.appendChild(o);
  });
  if (state.app && res.data.apps.includes(state.app)) sel.value = state.app;
  state.app = sel.value;
  if (state.app) await loadApp();
}

async function loadApp() {
  if (!state.app) return;
  localStorage.setItem("rp_app", state.app);
  await Promise.all([loadRings(), loadHistory()]);
}

async function loadRings() {
  const res = await api(`/api/apps/${encodeURIComponent(state.app)}/rings`);
  if (!res.ok) return showResult(res);
  const tbody = $("#rings tbody");
  tbody.innerHTML = "";
  (res.data.rings || []).forEach((v) => {
    const tr = document.createElement("tr");
    const cur = v.current_version || "—";
    const live = v.live_version || "—";
    const prev = v.previous_version || "—";
    tr.innerHTML = `
      <td><strong>${esc(v.ring.name)}</strong><div class="muted">${esc(v.ring.label)}</div></td>
      <td class="version">${esc(cur)}</td>
      <td class="version">${esc(live)}</td>
      <td class="version muted">${esc(prev)}</td>
      <td>${healthBadge(v)}</td>
      <td></td>`;
    tr.querySelector("td:last-child").appendChild(actionCell(v));
    tbody.appendChild(tr);
  });
}

function actionCell(v) {
  const wrap = document.createElement("div");
  wrap.className = "actions";
  if (!v.configured) {
    wrap.innerHTML = '<span class="muted">not configured</span>';
    return wrap;
  }

  // Seed
  const seedInput = document.createElement("input");
  seedInput.placeholder = "version";
  const seedBtn = document.createElement("button");
  seedBtn.textContent = "Seed";
  seedBtn.onclick = () => act("seed", { ring: v.ring.name, version: seedInput.value.trim() });
  wrap.append(seedInput, seedBtn);

  // Promote (from this ring to the next)
  const promoteBtn = document.createElement("button");
  promoteBtn.textContent = "Promote →";
  promoteBtn.className = "primary";
  promoteBtn.disabled = !v.can_promote_from;
  promoteBtn.title = v.can_promote_from ? "Promote to the next ring" : "Nothing to promote or last ring";
  promoteBtn.onclick = () => act("promote", { from_ring: v.ring.name });
  wrap.append(promoteBtn);

  // Rollback
  const rbBtn = document.createElement("button");
  rbBtn.textContent = "Rollback";
  rbBtn.className = "danger";
  rbBtn.disabled = !v.previous_version;
  rbBtn.onclick = () => act("rollback", { ring: v.ring.name });
  wrap.append(rbBtn);

  return wrap;
}

async function loadHistory() {
  const res = await api(`/api/apps/${encodeURIComponent(state.app)}/history`);
  if (!res.ok) return showResult(res);
  const tbody = $("#history tbody");
  tbody.innerHTML = "";
  (res.data.history || []).forEach((h) => {
    const tr = document.createElement("tr");
    const ok = h.result === "success";
    tr.innerHTML = `
      <td class="muted">${esc(fmtTime(h.created_at))}</td>
      <td>${esc(h.ring)}</td>
      <td>${esc(h.action)}</td>
      <td class="version">${esc(h.from_version || "—")} → ${esc(h.to_version || "—")}</td>
      <td><span class="badge ${ok ? "ok" : "bad"}">${esc(h.result)}</span></td>
      <td class="muted">${esc(h.message || "")}</td>`;
    tbody.appendChild(tr);
  });
}

// ---- actions ----
async function act(kind, body) {
  if (kind === "seed" && !body.version) return feedback("Enter a version to seed.", false);
  const res = await api(`/api/apps/${encodeURIComponent(state.app)}/${kind}`, "POST", body);
  showResult(res);
  await loadApp();
}

// ---- wiring ----
$("#save-token").onclick = () => {
  state.token = $("#token").value.trim();
  localStorage.setItem("rp_token", state.token);
  feedback("Token saved.", true);
  loadApps();
};
$("#app").onchange = (e) => { state.app = e.target.value; loadApp(); };
$("#refresh").onclick = () => loadApp();

// ---- init ----
$("#token").value = state.token;
state.app = localStorage.getItem("rp_app") || "";
loadApps();
