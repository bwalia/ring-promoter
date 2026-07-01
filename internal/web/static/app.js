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

// ---- auth gate ----
// Validate a token by making a real authenticated request.
async function tokenIsValid(token) {
  try {
    const resp = await fetch("/api/apps", { headers: { Authorization: "Bearer " + token } });
    return resp.ok;
  } catch (_) {
    return false;
  }
}

function showGate(message) {
  $("#app-root").hidden = true;
  $("#gate").hidden = false;
  const err = $("#gate-error");
  if (message) { err.textContent = message; err.hidden = false; } else { err.hidden = true; }
  $("#gate-token").focus();
}

function unlock() {
  $("#gate").hidden = true;
  $("#app-root").hidden = false;
  loadApps();
}

function signOut() {
  state.token = "";
  localStorage.removeItem("rp_token");
  $("#gate-token").value = "";
  showGate();
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
  if (res.status === 401) { signOut(); return; } // token no longer valid -> re-lock
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

  // Promote — only shown when this ring has a version and a next ring to go to.
  if (v.can_promote_from) {
    const promoteBtn = document.createElement("button");
    promoteBtn.textContent = "Promote →";
    promoteBtn.className = "primary";
    promoteBtn.title = "Promote to the next ring";
    promoteBtn.onclick = () => act("promote", { from_ring: v.ring.name });
    wrap.append(promoteBtn);
  }

  // Rollback — only shown when there is a previous version to return to.
  if (v.previous_version) {
    const rbBtn = document.createElement("button");
    rbBtn.textContent = "Rollback";
    rbBtn.className = "danger";
    rbBtn.onclick = () => act("rollback", { ring: v.ring.name });
    wrap.append(rbBtn);
  }

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
  if (res.status !== 401) await loadApp();
}

// ---- wiring ----
$("#gate-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const token = $("#gate-token").value.trim();
  if (!token) return showGate("Please enter a token.");
  if (await tokenIsValid(token)) {
    state.token = token;
    localStorage.setItem("rp_token", token);
    unlock();
  } else {
    showGate("Invalid token. Please try again.");
  }
});
$("#signout").onclick = signOut;
$("#app").onchange = (e) => { state.app = e.target.value; loadApp(); };
$("#refresh").onclick = () => loadApp();

// ---- init ----
state.app = localStorage.getItem("rp_app") || "";
(async () => {
  if (state.token && (await tokenIsValid(state.token))) {
    unlock();
  } else {
    showGate();
  }
})();
