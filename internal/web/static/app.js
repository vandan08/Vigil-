// VIGIL incident console. One EventSource, one Map, idempotent re-render:
// `snapshot` replaces the board, `incident` upserts one strip. The server
// drops subscribers that fall behind by closing the stream; EventSource
// reconnects and the next snapshot resyncs — no client-side catch-up logic.
"use strict";

const strips = document.getElementById("strips");
const template = document.getElementById("strip-template");
const allQuiet = document.getElementById("all-quiet");
const tally = document.getElementById("tally");
const feed = document.querySelector(".feed");
const feedLabel = document.getElementById("feed-label");

const incidents = new Map(); // id -> incident
const expanded = new Set();  // ids with an open timeline, survives re-render

// Actions valid per state, mirroring the server's state machine. The server
// still validates; this only avoids offering dead buttons.
const ACTIONS = {
  triggered: ["ack", "resolve"],
  acknowledged: ["mitigate", "resolve"],
  mitigated: ["resolve"],
  resolved: [],
};
const ACTION_LABEL = { ack: "ACK", mitigate: "MITIGATE", resolve: "RESOLVE" };

/* ---------- rendering ---------- */

function render() {
  const order = [...incidents.values()].sort((a, b) => {
    const doneA = a.state === "resolved", doneB = b.state === "resolved";
    if (doneA !== doneB) return doneA ? 1 : -1; // active board above the resolved log
    return new Date(b.createdAt) - new Date(a.createdAt);
  });

  strips.replaceChildren(...order.map(buildStrip));
  allQuiet.hidden = incidents.size > 0;
  renderTally();
}

function buildStrip(inc) {
  const li = template.content.firstElementChild.cloneNode(true);
  li.dataset.id = inc.id;
  li.dataset.severity = inc.severity;
  li.classList.toggle("resolved", inc.state === "resolved");

  li.querySelector(".strip-id").textContent = inc.id;
  li.querySelector(".strip-sev").textContent = inc.severity;
  const state = li.querySelector(".strip-state");
  state.textContent = inc.state;
  state.dataset.state = inc.state;
  li.querySelector(".strip-title").textContent = inc.title;

  const age = li.querySelector(".strip-age");
  age.textContent = relativeTime(inc.createdAt);
  age.title = inc.createdAt;

  const actions = li.querySelector(".strip-actions");
  for (const action of ACTIONS[inc.state] ?? []) {
    const btn = document.createElement("button");
    btn.dataset.action = action;
    btn.textContent = ACTION_LABEL[action];
    btn.addEventListener("click", () => transition(inc.id, action, btn));
    actions.append(btn);
  }

  const toggle = li.querySelector(".strip-toggle");
  const pane = li.querySelector(".strip-timeline");
  const open = expanded.has(inc.id);
  pane.hidden = !open;
  toggle.setAttribute("aria-expanded", String(open));
  toggle.addEventListener("click", () => {
    const nowOpen = pane.hidden;
    pane.hidden = !nowOpen;
    toggle.setAttribute("aria-expanded", String(nowOpen));
    nowOpen ? expanded.add(inc.id) : expanded.delete(inc.id);
  });

  const timeline = li.querySelector(".timeline");
  for (const ev of inc.timeline ?? []) {
    const item = document.createElement("li");
    item.dataset.kind = ev.kind;
    const at = document.createElement("span");
    at.className = "timeline-at";
    at.textContent = clockTime(ev.at);
    const kind = document.createElement("span");
    kind.className = "timeline-kind";
    kind.textContent = ev.kind.replaceAll("_", " ");
    const msg = document.createElement("span");
    msg.className = "timeline-msg";
    msg.textContent = ev.message;
    item.append(at, kind, msg);
    timeline.append(item);
  }
  return li;
}

function renderTally() {
  const counts = { triggered: 0, acknowledged: 0, mitigated: 0, resolved: 0 };
  for (const inc of incidents.values()) counts[inc.state] = (counts[inc.state] ?? 0) + 1;
  for (const cell of tally.querySelectorAll(".tally-cell")) {
    const n = counts[cell.dataset.count] ?? 0;
    cell.querySelector("dd").textContent = n;
    cell.classList.toggle("nonzero", n > 0);
  }
}

function flash(id) {
  const strip = strips.querySelector(`[data-id="${CSS.escape(id)}"]`);
  if (strip) {
    strip.classList.remove("flash");
    void strip.offsetWidth; // restart the animation
    strip.classList.add("flash");
  }
}

/* ---------- actions ---------- */

async function transition(id, action, btn) {
  btn.disabled = true;
  try {
    const resp = await fetch(`/api/incidents/${encodeURIComponent(id)}/${action}`, { method: "POST" });
    if (resp.ok) {
      const inc = await resp.json();
      incidents.set(inc.id, inc); // SSE will confirm; render now for snappiness
      render();
      flash(id);
    }
    // Non-OK means the incident moved on under us (409) — the live feed
    // already carries the truth, nothing to do.
  } catch {
    btn.disabled = false; // network hiccup: let the operator retry
  }
}

/* ---------- live feed ---------- */

function connect() {
  const es = new EventSource("/api/events");

  es.addEventListener("snapshot", (e) => {
    incidents.clear();
    for (const inc of JSON.parse(e.data) ?? []) incidents.set(inc.id, inc);
    setFeed("live", "LIVE");
    render();
  });

  es.addEventListener("incident", (e) => {
    const { incident } = JSON.parse(e.data);
    incidents.set(incident.id, incident);
    render();
    flash(incident.id);
  });

  es.onerror = () => setFeed("down", "RECONNECTING");
}

function setFeed(cls, label) {
  feed.classList.remove("live", "down");
  if (cls) feed.classList.add(cls);
  feedLabel.textContent = label;
}

/* ---------- clocks ---------- */

function relativeTime(iso) {
  const s = Math.max(0, (Date.now() - new Date(iso)) / 1000);
  if (s < 60) return `${Math.floor(s)}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

function clockTime(iso) {
  return new Date(iso).toISOString().slice(11, 19) + "Z";
}

setInterval(() => {
  for (const li of strips.children) {
    const inc = incidents.get(li.dataset.id);
    if (inc) li.querySelector(".strip-age").textContent = relativeTime(inc.createdAt);
  }
}, 30_000);

const clock = document.getElementById("clock");
setInterval(() => { clock.textContent = new Date().toISOString().slice(11, 19) + "Z"; }, 1000);

connect();
