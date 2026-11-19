/* awesome_labs web interface. No build step: plain DOM and xterm.js. */
"use strict";

// ---------- helpers ----------
const $ = (sel, root = document) => root.querySelector(sel);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
function h(tag, attrs = {}, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v == null || v === false) continue;
    if (k === "class") el.className = v;
    else if (k === "html") el.innerHTML = v;
    else if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else el.setAttribute(k, v === true ? "" : v);
  }
  for (const kid of kids.flat()) if (kid != null && kid !== false) el.append(kid.nodeType ? kid : document.createTextNode(kid));
  return el;
}
const store = {
  get(k, d) { try { const v = localStorage.getItem("al." + k); return v == null ? d : JSON.parse(v); } catch { return d; } },
  set(k, v) { try { localStorage.setItem("al." + k, JSON.stringify(v)); } catch { /* private mode */ } },
};
async function api(method, path, body) {
  const res = await fetch(path, {
    method, headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : undefined, credentials: "same-origin",
  });
  if (res.status === 401) { location.reload(); throw new Error("signed out"); }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}
function toast(msg, kind = "info", ms = 4200) {
  const t = h("div", { class: "toast " + kind }, msg);
  $("#toasts").append(t);
  setTimeout(() => t.remove(), ms);
}
function confirmDialog(title, body, yes = "OK", danger = false, no = "Cancel") {
  return new Promise((resolve) => {
    const m = $("#modal");
    $("#modal-title").textContent = title;
    $("#modal-body").innerHTML = body;
    const y = $("#modal-yes"), n = $("#modal-no");
    y.textContent = yes;
    n.textContent = no;
    y.className = "btn " + (danger ? "btn-danger" : "btn-primary");
    m.hidden = false;
    y.focus();
    const done = (v) => { m.hidden = true; y.onclick = n.onclick = m.onclick = document.onkeydown = null; resolve(v); };
    y.onclick = () => done(true);
    n.onclick = () => done(false);
    m.onclick = (e) => { if (e.target === m) done(false); };
    // Enter does what the focused button says: Cancel stays Cancel
    document.onkeydown = (e) => {
      if (e.key === "Escape") { e.preventDefault(); done(false); }
      else if (e.key === "Enter") { e.preventDefault(); done(document.activeElement !== n); }
    };
  });
}
const dots = (d) => h("span", { class: "dots d" + d, title: ["", "Beginner", "Easy", "Medium", "Hard", "Expert"][d] },
  ...[1, 2, 3, 4, 5].map((i) => h("i", { class: i <= d ? "on" : "" })));
function ago(t) {
  const s = (Date.now() - new Date(t)) / 1000;
  if (s < 60) return "just now";
  if (s < 3600) return Math.floor(s / 60) + " min ago";
  if (s < 86400) return Math.floor(s / 3600) + " h ago";
  return Math.floor(s / 86400) + " d ago";
}
const fmtDur = (s) => { s = Math.max(0, Math.round(s)); const m = Math.floor(s / 60); return m ? `${m}m ${String(s % 60).padStart(2, "0")}s` : `${s}s`; };

// ---------- state ----------
const S = {
  catalog: null, byId: new Map(), progress: {}, state: {},
  filtered: [], ex: null, tab: store.get("tab", "scenario"),
  logs: new Map(), // target -> {op, lines, partial}
  route: { name: "home" },
  lastResult: {}, // exercise id -> "passed" | "failed"
};

// ---------- sidebar list ----------
function setupFilters() {
  const track = $("#track");
  track.append(h("option", { value: "" }, `All tracks (${S.catalog.exercises.length})`));
  for (const t of S.catalog.tracks) track.append(h("option", { value: t.id }, `${t.name} (${t.count})`));
  const path = $("#path");
  path.append(h("option", { value: "" }, "No learning path"));
  for (const p of S.catalog.paths) path.append(h("option", { value: p.id }, `Path: ${p.title}`));
  track.value = store.get("track", "");
  $("#show").value = store.get("show", "all");
  path.value = store.get("path", "");
  if (track.value === "" && track.selectedIndex < 0) track.value = "";
  $("#q").value = store.get("q", "");
  const changed = () => {
    store.set("track", track.value); store.set("show", $("#show").value);
    store.set("path", path.value); store.set("q", $("#q").value);
    renderList();
  };
  track.onchange = () => { if (track.value) path.value = ""; changed(); };
  path.onchange = () => { if (path.value) track.value = ""; changed(); };
  $("#show").onchange = changed;
  let t; $("#q").oninput = () => { clearTimeout(t); t = setTimeout(changed, 80); };
  $("#q").onkeydown = (e) => {
    if (e.key === "Escape") { $("#q").value = ""; changed(); $("#q").blur(); }
    if (e.key === "Enter" && S.filtered[0]) go("ex/" + S.filtered[0].id);
  };
}

// tiers: each module runs Beginner → Intermediate → Advanced and ends with its
// capstone or incident (the big multi-part lab)
const TIERS = ["Beginner", "Intermediate", "Advanced", "Capstone"];
function capstoneTag(e) { return (e.tags || []).find((t) => t === "capstone" || t === "incident") || null; }
function tierOf(e) { return capstoneTag(e) ? 3 : e.difficulty <= 2 ? 0 : e.difficulty === 3 ? 1 : 2; }

function statusOf(id) {
  if (S.state.active && S.state.active.id === id) return "active";
  return S.progress[id] || "";
}

function renderList() {
  const q = $("#q").value.trim().toLowerCase();
  const track = $("#track").value, show = $("#show").value, pathId = $("#path").value;
  const path = pathId && S.catalog.paths.find((p) => p.id === pathId);
  const inPath = path ? new Set(path.ids) : null;
  const words = q.split(/\s+/).filter(Boolean);
  const diffWords = ["", "beginner", "easy", "medium", "hard", "expert"];
  S.filtered = S.catalog.exercises.filter((e) => {
    if (inPath && !inPath.has(e.id)) return false;
    if (track && e.track !== track) return false;
    const st = S.progress[e.id] || "";
    if (show === "solved" && st !== "solved") return false;
    if (show === "started" && st !== "started") return false;
    if (show === "none" && st !== "") return false;
    if (!words.length) return true;
    const hay = `${e.id} ${e.title} ${e.module} ${e.kind} ${(e.tags || []).join(" ")} ${diffWords[e.difficulty]} ${TIERS[tierOf(e)]}`.toLowerCase();
    return words.every((w) => hay.includes(w));
  });
  // within a module, easy to hard by tier (stable: a module's own order otherwise)
  S.filtered.sort((a, b) => (S.modRank.get(a.track + "/" + a.module) - S.modRank.get(b.track + "/" + b.module)) || (tierOf(a) - tierOf(b)));
  const list = $("#list");
  list.replaceChildren();
  const solved = S.filtered.filter((e) => S.progress[e.id] === "solved").length;
  $("#side-count").replaceChildren(h("span", {}, `${S.filtered.length} exercises`), h("span", {}, `✔ ${solved} solved`));
  if (!S.filtered.length) {
    list.append(h("div", { class: "empty" }, "Nothing matches.", h("br"), h("button", { class: "link-btn", onclick: () => {
      $("#q").value = ""; $("#show").value = "all"; $("#track").value = ""; $("#path").value = "";
      store.set("q", ""); store.set("show", "all"); store.set("track", ""); store.set("path", ""); renderList();
    } }, "Clear the filters")));
    return;
  }
  const frag = document.createDocumentFragment();
  let mod = null, head = null, modCount = 0, modSolved = 0, tier = -1;
  const flush = () => { if (head) head.lastChild.textContent = `${modSolved}/${modCount}`; };
  for (const e of S.filtered) {
    const key = e.track + "/" + e.module;
    if (key !== mod) {
      flush();
      mod = key; modCount = modSolved = 0;
      head = h("div", { class: "mod-head" }, h("span", { title: e.module }, (track ? "" : S.trackName[e.track] + " · ") + e.module), h("span", { class: "mod-prog" }));
      frag.append(head);
      tier = -1;
    }
    if (tierOf(e) !== tier) {
      tier = tierOf(e);
      frag.append(h("div", { class: "tier-head t" + tier }, TIERS[tier]));
    }
    modCount++;
    const st = statusOf(e.id);
    if (S.progress[e.id] === "solved") modSolved++;
    const mark = st === "active" ? "▶" : st === "solved" ? "✔" : st === "started" ? "●" : "";
    frag.append(h("a", {
      class: "ex-row" + (S.ex && S.ex.id === e.id ? " sel" : ""), href: "#/ex/" + e.id, "data-id": e.id, title: e.title,
    }, h("span", { class: "mark " + st }, mark), h("span", { class: "id" }, e.id), h("span", { class: "title" }, e.title),
      h("span", { class: "meta" }, capstoneTag(e) ? h("span", { class: "cap-badge", title: capstoneTag(e) === "incident" ? "A multi-problem incident" : "The module's big real-world lab" }, capstoneTag(e)) : null,
        h("span", { class: "kind" }, e.kind), dots(e.difficulty))));
  }
  flush();
  list.append(frag);
  markSelected(false);
}

function markSelected(scroll = true) {
  for (const r of document.querySelectorAll(".ex-row.sel")) r.classList.remove("sel");
  if (!S.ex) return;
  const row = document.querySelector(`.ex-row[data-id="${CSS.escape(S.ex.id)}"]`);
  if (row) { row.classList.add("sel"); if (scroll) row.scrollIntoView({ block: "nearest" }); }
}

// ---------- top bar ----------
function renderTop() {
  const total = S.catalog.exercises.length;
  const solved = Object.values(S.progress).filter((v) => v === "solved").length;
  const started = Object.values(S.progress).filter((v) => v === "started").length;
  $("#score-chip").replaceChildren(h("span", {}, "✔ "), h("b", {}, String(solved)), `/${total}`,
    started ? h("span", { style: "color:var(--yellow)" }, ` · ● ${started}`) : "");
  const a = S.state.active, chip = $("#active-chip");
  chip.hidden = !a;
  if (a) {
    chip.href = "#/ex/" + a.id;
    chip.title = "The exercise set up on the VMs";
    chip.replaceChildren(h("span", { class: "dot" }), h("b", {}, a.id), " " + a.title);
  }
  const ex = S.state.exam, ec = $("#exam-chip");
  ec.hidden = !ex;
  if (ex) ec.textContent = `Exam · ${ex.title} · ${ex.passed}/${ex.ids.length} · ${fmtDur(ex.remainingSeconds)} left`;
}

// ---------- routing ----------
function go(path) { location.hash = "#/" + path; }
function route() {
  const p = location.hash.replace(/^#\/?/, "");
  document.body.classList.remove("show-list");
  for (const [id, r] of [["exam-link", "exam"], ["weak-link", "weak"], ["envs-link", "envs"]]) $("#" + id).classList.toggle("on", p === r);
  if (p !== "exam") clearInterval(examTimer);
  if (p.startsWith("ex/")) return openExercise(decodeURIComponent(p.slice(3)));
  S.ex = null;
  markSelected();
  if (p === "envs") return renderEnvs();
  if (p === "exam") return renderExam();
  if (p === "weak") return renderWeak();
  if (p === "settings") return renderSettings();
  if (p === "list") { document.body.classList.add("show-list"); return; }
  renderHome();
}

// ---------- home ----------
function renderHome() {
  S.route = { name: "home" };
  const c = $("#content");
  const page = h("div", { class: "page" });
  page.append(h("div", { class: "hero" },
    h("h1", {}, "Hands-on labs on real VMs"),
    h("p", {}, "Pick an exercise, start its lab, solve it in the terminal below and check your work. Every exercise has a scenario, progressive hints and a full solution."),
    h("div", { class: "quick" }, h("a", { class: "btn btn-sm", href: "#/exam" }, "⏱ Take an exam"), h("a", { class: "btn btn-sm", href: "#/weak" }, "Weak spots"),
      h("a", { class: "btn btn-sm", href: "#/envs" }, "Environments & VMs"), h("a", { class: "btn btn-sm btn-ghost", href: "#/settings" }, "Settings"))));
  const a = S.state.active;
  if (a) {
    page.append(h("div", { class: "banner info" }, h("span", { class: "grow" }, "You're working on ", h("b", {}, a.id), " — " + a.title + " (started " + ago(a.since) + ")"),
      h("a", { class: "btn btn-primary btn-sm", href: "#/ex/" + a.id }, "Continue")));
  }
  page.append(h("div", { class: "section-title" }, "Tracks"));
  const grid = h("div", { class: "grid" });
  for (const t of S.catalog.tracks) {
    const ids = S.catalog.exercises.filter((e) => e.track === t.id);
    const s = ids.filter((e) => S.progress[e.id] === "solved").length, st = ids.filter((e) => S.progress[e.id] === "started").length;
    grid.append(h("a", { class: "card", href: "#/", onclick: (ev) => { ev.preventDefault(); pickTrack(t.id); } },
      h("h4", {}, t.name), h("div", { class: "sub" }, `${s}/${t.count} solved` + (st ? ` · ${st} in progress` : "")),
      h("div", { class: "bar" }, h("i", { class: "s", style: `width:${(100 * s) / t.count}%` }), h("i", { class: "p", style: `width:${(100 * st) / t.count}%` }))));
  }
  page.append(grid);
  page.append(h("div", { class: "section-title" }, "Learning paths"));
  const pg = h("div", { class: "grid" });
  for (const p of S.catalog.paths) {
    const s = p.ids.filter((id) => S.progress[id] === "solved").length;
    pg.append(h("a", { class: "card path-card", href: "#/", onclick: (ev) => { ev.preventDefault(); pickPath(p.id); } },
      h("h4", {}, p.title), h("div", { class: "sub" }, `${s}/${p.ids.length} solved`), h("p", {}, p.description),
      h("div", { class: "bar" }, h("i", { class: "s", style: `width:${(100 * s) / Math.max(1, p.ids.length)}%` }))));
  }
  page.append(pg);
  page.append(h("p", { class: "muted small", style: "margin-top:28px" },
    `awesome_labs ${S.state.version} · `, h("a", { href: S.state.repo, target: "_blank", rel: "noopener noreferrer" }, S.state.repo.replace(/^https:\/\//, "")),
    ` · when you stop the server (ctrl+c) the lab VMs are handled by on_exit: ${S.state.onExit} (`, h("a", { href: "#/settings" }, "Settings"), ")."));
  c.replaceChildren(page);
  c.scrollTop = 0;
}
function pickTrack(id) {
  $("#track").value = id; $("#path").value = ""; store.set("track", id); store.set("path", "");
  renderList(); firstInList();
}
function pickPath(id) {
  $("#path").value = id; $("#track").value = ""; store.set("path", id); store.set("track", "");
  renderList(); firstInList();
}
function firstInList() {
  const next = S.filtered.find((e) => S.progress[e.id] !== "solved") || S.filtered[0];
  if (next) go("ex/" + next.id);
}

// ---------- exercise ----------
const TABS = [["scenario", "Scenario"], ["learn", "Learn"], ["hints", "Hints"], ["solution", "Solution"], ["output", "Output"], ["notes", "Notes"]];

async function openExercise(id, keepTab = false) {
  if (!S.byId.has(id)) { toast(`No exercise ${id}`, "bad"); return go(""); }
  const same = S.ex && S.ex.id === id;
  try {
    S.ex = await api("GET", "/api/exercises/" + encodeURIComponent(id));
  } catch (e) { toast(e.message, "bad"); return; }
  S.route = { name: "ex", id };
  // a new exercise opens on what you read first; the tabs that hold another exercise's state don't carry over
  if (!same && !keepTab && ["output", "solution", "hints"].includes(S.tab)) S.tab = "scenario";
  if (S.tab === "learn" && !S.ex.tutorial) S.tab = "scenario";
  if (!S.logs.has(id)) {
    api("GET", "/api/ops/last?target=" + encodeURIComponent(id)).then((r) => {
      if (r.op && !S.logs.has(id)) { S.logs.set(id, logFrom(r.op)); if (S.ex?.id === id && S.tab === "output") renderTabBody(); }
    }).catch(() => {});
  }
  renderExercise(!same);
  markSelected();
}

function busyOp() { return S.state.op && !S.state.op.done ? S.state.op : null; }

function exButtons(ex) {
  const busy = busyOp();
  const other = S.state.active && S.state.active.id !== ex.id ? S.state.active : null;
  const btn = (label, cls, why, fn, key) => h("button", {
    class: "btn " + cls, "aria-disabled": why ? "true" : null, title: why || null,
    onclick: () => (why ? toast(why, "info") : fn()),
  }, label, key ? h("kbd", {}, key) : null);
  const busyWhy = busy ? "busy: " + busy.label : null;
  const out = [];
  if (!ex.active) {
    out.push(btn("▶ Start lab", "btn-primary", busyWhy, () => startEx(ex, other)));
  } else if (ex.running === false) {
    out.push(btn("⏻ Boot the lab again", "btn-primary", busyWhy, () => runOp(ex, "boot")));
  }
  const changing = busy && busy.kind !== "check" && (!busy.env || busy.env === ex.profile) ? busy : null;
  const termWhy = !ex.active ? "start the lab first: only the exercise set up on the VMs has one"
    : ex.running === false ? "the lab's VMs are stopped: boot them again first"
    : changing ? "busy: " + changing.label : null;
  for (const t of ex.terminals) {
    out.push(btn("⌨ " + (ex.terminals.length > 1 ? t.label : "Terminal"), "", termWhy, () => terms.open(t.target, t.label)));
  }
  if (ex.notebook) out.push(btn("📓 Notebook", "", termWhy || (busy && busy.kind === "jupyter" ? "starting JupyterLab…" : null), () => terms.openNotebook()));
  out.push(btn("✓ Check", ex.active ? "btn-green" : "", busyWhy || (!ex.active ? "start the lab first" : null), () => runOp(ex, "check")));
  out.push(h("span", { class: "grow" }));
  const lock = ex.locked ? "locked during this exam" : null;
  out.push(btn("Hint", "btn-ghost", lock || (ex.hints.length >= ex.hintCount ? (ex.hintCount ? "all hints shown" : "no hints for this exercise") : null), () => revealHint(ex)));
  out.push(btn("Solution", "btn-ghost", lock, () => revealSolution(ex)));
  if (ex.active) {
    out.push(btn("End", "btn-ghost", busyWhy || (ex.inExam ? "during an exam the lab carries on to the next question" : null), () => endEx(ex)));
    out.push(btn("Restart", "btn-ghost", busyWhy, () => startEx(ex, null, true)));
  }
  const hasProg = ex.status || ex.progress.checks || ex.progress.hintsUsed || ex.progress.sawSolution;
  out.push(btn("Reset progress", "btn-ghost", !hasProg ? "no progress to reset" : ex.inExam ? "not during an exam on this question" : null, () => forget(ex)));
  return out;
}

function renderExercise(scrollTop) {
  const ex = S.ex, c = $("#content");
  const page = h("div", { class: "page" });
  const badges = [];
  if (ex.active) badges.push(h("span", { class: "badge active" }, "▶ set up on the VMs"));
  if (ex.status === "solved") badges.push(h("span", { class: "badge solved" }, "✔ Solved"));
  else if (ex.status === "started") badges.push(h("span", { class: "badge started" }, "● Started"));
  if (ex.destructive) badges.push(h("span", { class: "badge warn", title: "A reset can't undo this exercise; the environment is rebuilt before the next one" }, "rebuilds the environment"));
  if (ex.inExam) badges.push(h("span", { class: "badge" }, "exam question"));
  const cap = (ex.tags || []).find((t) => t === "capstone" || t === "incident");
  if (cap) badges.push(h("span", { class: "badge cap", title: cap === "incident" ? "A multi-problem incident" : "The module's big real-world lab" }, cap === "incident" ? "Incident" : "Capstone"));
  page.append(h("div", { class: "ex-head" }, h("div", { style: "flex:1;min-width:0" },
    h("div", { class: "eid" }, ex.id),
    h("h1", {}, ex.title),
    h("div", { class: "crumbs" }, dots(ex.difficulty), h("span", {}, ex.difficultyLabel), h("span", { class: "sep" }, "·"),
      h("span", {}, ex.kind === "fix" ? "Fix something" : "Build something"), h("span", { class: "sep" }, "·"),
      h("span", {}, ex.trackName + " — " + ex.module), h("span", { class: "sep" }, "·"), h("span", {}, ex.envTitle),
      ...badges))));
  page.append(h("div", { class: "actions" }, ...exButtons(ex)));

  const res = S.lastResult[ex.id];
  if (res === "passed") {
    const next = nextUnsolved(ex.id);
    page.append(h("div", { class: "banner ok" }, h("span", { class: "grow" }, "✔ Solved — great work. The VMs stay up, so the next exercise on them starts quickly."),
      next ? h("a", { class: "btn btn-sm", href: "#/ex/" + next.id }, "Next: " + next.id + " →") : null));
  } else if (res === "failed") {
    page.append(h("div", { class: "banner bad" }, h("span", { class: "grow" }, "Not yet — the Output tab shows which checks failed."),
      h("button", { class: "btn btn-sm", onclick: () => setTab("output") }, "Show output")));
  }
  const op = busyOp();
  if (op && op.target === ex.id) {
    page.append(h("div", { class: "banner info" }, h("span", { class: "spinner" }), h("span", { class: "grow" }, op.label + "…"),
      h("button", { class: "btn btn-sm", onclick: () => setTab("output") }, "Watch")));
  }
  const tabs = h("div", { class: "tabs", role: "tablist" });
  for (const [k, label] of TABS) {
    if (k === "learn" && !ex.tutorial) continue;
    const n = k === "hints" ? h("span", { class: "n" }, `${ex.hints.length}/${ex.hintCount}`) : null;
    tabs.append(h("button", { class: "tab" + (S.tab === k ? " on" : ""), role: "tab", onclick: () => setTab(k) }, label, n));
  }
  page.append(tabs, h("div", { class: "tab-body", id: "tab-body" }));
  c.replaceChildren(page);
  renderTabBody();
  if (scrollTop) c.scrollTop = 0;
}

function setTab(k) { S.tab = k; if (k !== "output") store.set("tab", k); renderExercise(false); }

function renderTabBody() {
  const body = $("#tab-body");
  if (!body || !S.ex) return;
  const ex = S.ex;
  let tab = S.tab;
  if (tab === "learn" && !ex.tutorial) tab = "scenario";
  body.replaceChildren();
  switch (tab) {
    case "scenario":
      body.append(mdBlock(ex.scenario));
      if (ex.notebook) body.append(notebookGuide(ex));
      body.append(h("p", { class: "muted small", style: "margin-top:22px" }, "Inside the VM the task is also in /lab/TASK.md, and /lab is your working directory."));
      break;
    case "learn":
      body.append(mdBlock(ex.tutorial));
      break;
    case "hints":
      if (ex.locked) { body.append(h("div", { class: "locked" }, h("p", {}, "Hints are locked during this exam."))); break; }
      if (!ex.hintCount) { body.append(h("div", { class: "locked" }, h("p", {}, "This exercise has no hints."))); break; }
      ex.hints.forEach((html, i) => body.append(h("div", { class: "hint" }, h("div", { class: "hint-n" }, `Hint ${i + 1} of ${ex.hintCount}`), mdBlock(html))));
      if (ex.hints.length < ex.hintCount) {
        body.append(h("div", { class: "locked" }, h("p", {}, ex.hints.length ? "Still stuck?" : "Try on your own first — hints are revealed one at a time."),
          h("button", { class: "btn", onclick: () => revealHint(ex) }, `Reveal hint ${ex.hints.length + 1}`)));
      }
      break;
    case "solution":
      if (ex.locked) { body.append(h("div", { class: "locked" }, h("p", {}, "The solution is locked during this exam."))); break; }
      if (ex.solution == null) {
        body.append(h("div", { class: "locked" }, h("p", {}, "The full solution is hidden until you ask for it."), h("button", { class: "btn", onclick: () => revealSolution(ex) }, "Reveal the solution")));
        break;
      }
      body.append(mdBlock(ex.solution));
      body.append(h("div", { class: "subhead" }, "Your commands"));
      body.append(ex.history ? h("pre", { class: "log" }, ex.history) : h("p", { class: "muted small" }, "Shells opened from awesome_labs keep a history per exercise; it's collected at every check."));
      break;
    case "output":
      body.append(renderLog(ex.id));
      break;
    case "notes": {
      const ta = h("textarea", { class: "notes", placeholder: "Your notes on this exercise — saved automatically, kept when progress is reset.", spellcheck: "false" });
      ta.value = ex.notes || "";
      const foot = h("span", {}, ex.notes ? "Saved" : "");
      let t;
      const save = async () => {
        try { const r = await api("PUT", `/api/exercises/${encodeURIComponent(ex.id)}/notes`, { text: ta.value }); ex.notes = r.notes; foot.textContent = "Saved"; }
        catch (e) { foot.textContent = "Not saved: " + e.message; }
      };
      ta.oninput = () => { foot.textContent = "Editing…"; clearTimeout(t); t = setTimeout(save, 600); };
      ta.onblur = () => { clearTimeout(t); if (ta.value !== (ex.notes || "")) save(); };
      body.append(ta, h("div", { class: "notes-foot" }, h("span", {}, "~/.awesome_labs/notes.json"), foot));
      break;
    }
  }
}

function mdBlock(html, { terminal = true } = {}) {
  const d = h("div", { class: "md", html });
  for (const pre of d.querySelectorAll("pre")) {
    const code = pre.innerText.replace(/\n$/, "");
    const tools = h("div", { class: "code-tools" },
      h("button", { title: "Copy", onclick: async (e) => { e.stopPropagation(); await copy(code); e.target.textContent = "Copied"; setTimeout(() => (e.target.textContent = "Copy"), 1200); } }, "Copy"),
      terminal ? h("button", { title: "Paste into the open terminal (you press Enter)", onclick: (e) => { e.stopPropagation(); terms.paste(code); } }, "▸ Terminal") : null);
    pre.append(tools);
  }
  return d;
}

// notebookGuide: how a notebook exercise is graded, and how to save each file it must write
function notebookGuide(ex) {
  const files = ex.guide.length ? ex.guide.map((g) => "/lab/" + g.file).join(", ") : null;
  const box = h("div", { class: "nb-guide" },
    h("div", { class: "nb-guide-head" }, "📓 This exercise is done in the notebook ", h("code", {}, "/lab/" + ex.starter),
      " (Notebook opens it once the lab is started). ", h("b", {}, "Check runs it from top to bottom in a fresh kernel"),
      ", on a copy of /lab",
      files ? [" without ", h("code", {}, files), ", so the notebook itself must write them: files made by hand don't count."]
        : ", then calls the functions the task names with its own data.",
      " Save the notebook before you press Check; Kernel → Restart Kernel and Run All Cells tries it the same way."));
  for (const it of ex.guide) {
    const item = h("div", { class: "nb-guide-item" }, h("div", {}, h("code", { class: "nb-file" }, "/lab/" + it.file), " — " + it.how));
    if (it.code) {
      const pre = h("pre", {}, h("code", {}, it.code));
      item.append(mdBlock(pre.outerHTML, { terminal: false }));
    }
    box.append(item);
  }
  return box;
}

async function copy(text) {
  try { await navigator.clipboard.writeText(text); }
  catch {
    const ta = h("textarea", { style: "position:fixed;opacity:0" }); ta.value = text; document.body.append(ta); ta.select();
    document.execCommand("copy"); ta.remove();
  }
}

function renderLog(target) {
  const wrap = h("div");
  const L = S.logs.get(target);
  if (!L) { wrap.append(h("div", { class: "locked" }, h("p", {}, "Nothing has run for this exercise yet in this session."))); return wrap; }
  const o = L.op;
  const state = !o.done ? [h("span", { class: "spinner" }), " " + o.label + "…"]
    : o.error ? [h("span", { style: "color:var(--red)" }, "✘ " + o.error)]
    : o.kind === "check" ? [h("span", { style: `color:var(--${o.passed ? "green" : "red"})` }, o.passed ? "✔ all checks pass" : "✘ some checks fail")]
    : [h("span", { style: "color:var(--green)" }, "✔ done")];
  wrap.append(h("div", { class: "log-head" }, ...state, h("span", { class: "grow" }),
    h("span", { class: "small", id: o.done ? null : "log-elapsed" }, o.done && o.finished ? `took ${fmtDur((new Date(o.finished) - new Date(o.started)) / 1000)}` : `${fmtDur((Date.now() - new Date(o.started)) / 1000)} so far`)));
  const pre = h("pre", { class: "log", id: "log-" + target });
  fillLog(pre, L);
  wrap.append(pre);
  requestAnimationFrame(() => (pre.scrollTop = pre.scrollHeight));
  return wrap;
}
function logLine(l) {
  const cls = /^==>/.test(l) ? "step" : /^\s*(✔|PASS|ok\b)/.test(l) ? "ok" : /^\s*(✘|FAIL|error)/i.test(l) ? "bad" : "";
  return cls ? h("span", { class: cls }, l + "\n") : document.createTextNode(l + "\n");
}
function fillLog(pre, L) {
  pre.replaceChildren(...L.lines.map(logLine));
  if (L.base > 0) pre.prepend(h("span", { class: "muted" }, `… ${L.base} earlier lines not kept\n`));
  if (L.partial) pre.append(document.createTextNode(L.partial));
}

function nextUnsolved(after) {
  const list = S.filtered.length ? S.filtered : S.catalog.exercises;
  const i = list.findIndex((e) => e.id === after);
  for (let j = i + 1; j < list.length; j++) if (S.progress[list[j].id] !== "solved") return list[j];
  return null;
}

// ---------- exercise actions ----------
async function runOp(ex, action) {
  try {
    await api("POST", `/api/exercises/${encodeURIComponent(ex.id)}/${action}`);
    delete S.lastResult[ex.id];
    S.tab = "output";
    await refreshState();
    if (S.ex && S.ex.id === ex.id) renderExercise(false);
  } catch (e) { toast(e.message, "bad"); }
}
async function startEx(ex, other, again) {
  if (other) {
    const ok = await confirmDialog("Replace the lab of " + other.id + "?",
      `<b>${esc(other.id)}</b> — ${esc(other.title)} is set up on the VMs. Starting <b>${esc(ex.id)}</b> resets the environment, so its work is lost.`, "Start " + ex.id, true);
    if (!ok) return;
  } else if (again) {
    const ok = await confirmDialog("Start " + ex.id + " again?", "This resets its environment and sets the scenario up from scratch: your work in the lab is lost.", "Restart", true);
    if (!ok) return;
  }
  if (!S.seenBuildNote) {
    S.seenBuildNote = true;
    toast("The first start of an environment builds its VMs (5–15 min); later starts take seconds.", "info", 7000);
  }
  runOp(ex, "start");
}
async function endEx(ex) {
  const ok = await confirmDialog("End " + ex.id + "?", "Its lab is forgotten and the environment is reset to pristine, ready for the next exercise. Open terminals on it are closed.", "End exercise");
  if (ok) runOp(ex, "end");
}
async function revealHint(ex) {
  try { S.ex = await api("POST", `/api/exercises/${encodeURIComponent(ex.id)}/hint`); S.tab = "hints"; renderExercise(false); }
  catch (e) { toast(e.message, "bad"); }
}
async function revealSolution(ex) {
  if (ex.solution == null) {
    const ok = await confirmDialog("Reveal the solution?", "It's recorded in your progress (and counts in Weak spots).", "Show it");
    if (!ok) return;
  }
  try { S.ex = await api("POST", `/api/exercises/${encodeURIComponent(ex.id)}/solution`); S.tab = "solution"; renderExercise(false); }
  catch (e) { toast(e.message, "bad"); }
}
async function forget(ex) {
  const ok = await confirmDialog("Reset progress on " + ex.id + "?", "Forgets started, solved, checks, hints and the solution view. Your notes are kept.", "Reset progress", true);
  if (!ok) return;
  try { S.ex = await api("POST", `/api/exercises/${encodeURIComponent(ex.id)}/forget`); delete S.lastResult[ex.id]; await refreshProgress(); renderExercise(false); }
  catch (e) { toast(e.message, "bad"); }
}

// ---------- environments ----------
async function renderEnvs() {
  S.route = { name: "envs" };
  const c = $("#content");
  if (!$("#envs-page")) c.replaceChildren(h("div", { class: "page", id: "envs-page" }, h("div", { class: "muted" }, h("span", { class: "spinner", style: "display:inline-block;vertical-align:-2px;margin-right:8px" }), "Asking Lima…")));
  let d;
  try { d = await api("GET", "/api/envs"); } catch (e) { toast(e.message, "bad"); return; }
  if (S.route.name !== "envs") return;
  const busy = busyOp();
  const act = (label, path, cls = "", confirmText) => h("button", {
    class: "btn btn-sm " + cls, "aria-disabled": busy ? "true" : null, title: busy ? "busy: " + busy.label : null,
    onclick: async () => {
      if (busy) return toast("busy: " + busy.label);
      if (confirmText && !(await confirmDialog(label + "?", confirmText, label, cls.includes("danger")))) return;
      try { await api("POST", path); await refreshState(); renderEnvs(); } catch (e) { toast(e.message, "bad"); }
    },
  }, label);
  const page = h("div", { class: "page", id: "envs-page" });
  page.append(h("div", { class: "hero" }, h("h1", {}, "Environments & VMs"),
    h("p", {}, "Only one environment runs at a time: starting an exercise deletes the VMs of the others. A saved copy makes the next start take seconds instead of a full build.")));
  if (d.error) page.append(h("div", { class: "banner bad" }, d.error));
  const et = h("table", { class: "tbl" }, h("tr", {}, h("th", {}, "Environment"), h("th", {}, "Status"), h("th", {}, "Saved copy"), h("th", {})));
  const days = d.deleteStoppedAfterDays;
  for (const e of d.envs) {
    const run = e.status === "ready" ? "run" : e.status === "not created" ? "none" : "stop";
    et.append(h("tr", {}, h("td", {}, h("b", {}, e.title), e.tainted ? h("div", { class: "small", style: "color:var(--red)" }, "rebuilt before the next exercise") : null),
      h("td", {}, h("span", { class: "st " + run }, e.status)),
      h("td", { class: "muted" }, e.saved ? "✔ up to date" : "—", e.savedDisk ? h("div", { class: "small" }, fmtBytes(e.savedDisk) + " on disk") : null),
      h("td", { class: "btns" },
        e.status !== "ready" ? act("Start", `/api/envs/${e.profile}/up`) : null,
        e.status !== "not created" && e.status !== "stopped" ? act("Stop", `/api/envs/${e.profile}/stop`) : null,
        act("Reset", `/api/envs/${e.profile}/reset`, "", "Restores every VM of the " + esc(e.title) + " to pristine. The exercise set up on it is forgotten."),
        !e.saved ? act("Prepare", `/api/envs/${e.profile}/prepare`, "", "Builds the " + esc(e.title) + " now and keeps a saved copy (5–15 min the first time); its VMs are deleted afterwards.") : null,
        act("Rebuild", `/api/envs/${e.profile}/rebuild`, "", "Deletes the " + esc(e.title) + " and its saved copy, then builds it from scratch."),
        e.status !== "not created" ? act("Delete", `/api/envs/${e.profile}/destroy`, "btn-danger", "Deletes the VMs of the " + esc(e.title) + ". The saved copy is kept.") : null)));
  }
  page.append(h("div", { class: "tbl-wrap" }, et));
  page.append(h("div", { class: "section-title", style: "display:flex;align-items:center;gap:8px" }, h("span", { style: "flex:1" }, "Virtual machines"),
    act("Stop all", "/api/vms/all/stop"), act("Delete all", "/api/vms/all/delete", "btn-danger", "Deletes every lab VM, running or stopped. Saved copies are kept.")));
  const vt = h("table", { class: "tbl" }, h("tr", {}, h("th", {}, "VM"), h("th", {}, "Environment"), h("th", {}, "Status"), h("th", {}, "CPU"), h("th", {}, "Memory"), h("th", { title: "Space its files really take on this machine" }, "Disk"), h("th", {})));
  let stoppedDisk = 0;
  for (const v of d.vms || []) {
    const st = v.status || "not created";
    if (v.status === "Stopped") stoppedDisk += v.disk || 0;
    const since = v.status === "Stopped" && v.stoppedSince ? h("div", { class: "small muted" }, "stopped " + ago(v.stoppedSince)) : null;
    vt.append(h("tr", {}, h("td", { class: "mono" }, v.name), h("td", { class: "muted" }, v.env || "(stray)"),
      h("td", {}, h("span", { class: "st " + (st === "Running" ? "run" : v.status ? "stop" : "none") }, st), since),
      h("td", { class: "muted" }, v.cpus || "–"), h("td", { class: "muted" }, v.memory), h("td", { class: "muted" }, fmtBytes(v.disk)),
      h("td", { class: "btns" }, v.status === "Running" ? act("Stop", `/api/vms/${v.name}/stop`) : null,
        v.status ? act("Delete", `/api/vms/${v.name}/delete`, "btn-danger", "Deletes " + esc(v.name) + ". The next exercise on its environment re-creates it.") : null)));
  }
  page.append(h("div", { class: "tbl-wrap" }, vt));
  page.append(h("p", { class: "muted small" }, stoppedDisk ? `Stopped lab VMs take ${fmtBytes(stoppedDisk)}. ` : "",
    days > 0 ? `Lab VMs stopped for more than ${days} days are deleted automatically (saved copies are kept). ` : "Stopped lab VMs are kept until you delete them. ",
    h("a", { href: "#/settings" }, days > 0 ? "Change in Settings" : "Delete them after a while (Settings)")));
  const L = S.logs.get("env");
  if (L) { page.append(h("div", { class: "section-title" }, "Last operation")); page.append(renderLog("env")); }
  page.append(h("div", { class: "section-title" }, "Image cache"), h("pre", { class: "log" }, d.cache || "empty"),
    h("p", { class: "muted small" }, "Detailed logs: " + d.logPath));
  c.replaceChildren(page);
}

// ---------- exam ----------
const fmtClock = (s) => { s = Math.max(0, Math.round(s)); const hh = Math.floor(s / 3600), mm = Math.floor(s / 60) % 60, ss = s % 60;
  return (hh ? hh + ":" + String(mm).padStart(2, "0") : String(mm).padStart(2, "0")) + ":" + String(ss).padStart(2, "0"); };
let examTimer;
async function renderExam() {
  S.route = { name: "exam" };
  clearInterval(examTimer);
  let d, setup;
  try { [d, setup] = await Promise.all([api("GET", "/api/exam"), api("GET", "/api/exam/setup")]); } catch (e) { toast(e.message, "bad"); return; }
  if (S.route.name !== "exam") return;
  const page = h("div", { class: "page" });
  if (d.exam) page.append(...examRunning(d.exam));
  else {
    if (S.examResult) page.append(...examResult(S.examResult));
    page.append(...examSetup(setup, !S.examResult && d.last ? d.last : null));
  }
  $("#content").replaceChildren(page);
  await refreshState();
}

function qmark(q) { return q.passed ? h("span", { class: "mark solved" }, "✔") : q.checks ? h("span", { class: "mark bad" }, "✘") : h("span", { class: "mark" }, "·"); }

function examRunning(e) {
  const left = h("b", { class: "exam-clock" });
  const deadline = Date.now() + e.remainingSeconds * 1000;
  const tick = async () => {
    const s = (deadline - Date.now()) / 1000;
    left.textContent = fmtClock(s);
    left.className = "exam-clock" + (s < 600 ? " low" : s < 1800 ? " mid" : "");
    if (s <= 0) { clearInterval(examTimer); toast("Time's up — here is your result", "info"); await showExamEnd(null); }
  };
  tick(); examTimer = setInterval(tick, 1000);
  const rows = e.questions.map((q, i) => h("a", { class: "q-row", href: "#/ex/" + q.id },
    h("span", { class: "n" }, String(i + 1) + "."), qmark(q), h("span", { class: "id" }, q.id), dots(q.difficulty), h("span", { class: "title" }, q.title),
    h("span", { class: "muted small" }, q.checks ? `${q.checks} check${q.checks > 1 ? "s" : ""}` : "not attempted")));
  return [
    h("div", { class: "hero" }, h("h1", {}, e.title),
      h("p", {}, "Open a question, start its lab, solve it and check: your last check per question counts. ",
        e.allowHints ? "Hints and solutions are allowed in this exam." : "Hints and solutions are locked until the exam ends.")),
    h("div", { class: "exam-bar" }, h("span", {}, "Time left "), left, h("span", { class: "muted" }, `· passing so far ${e.passed}/${e.total} · pass mark ${Math.round(e.passMark)}%`),
      h("span", { class: "grow" }),
      h("button", { class: "btn btn-primary", onclick: () => finishExam(false) }, "Finish exam"),
      h("button", { class: "btn btn-ghost btn-danger", onclick: () => finishExam(true) }, "Abandon")),
    h("div", { class: "q-list" }, ...rows),
  ];
}

async function finishExam(abandon) {
  const ok = abandon
    ? await confirmDialog("Abandon the exam?", "It's recorded in your exam history as abandoned, and hints and solutions unlock.", "Abandon", true)
    : await confirmDialog("Finish the exam now?", "Checking stops counting and you get your result.", "Finish");
  if (ok) await showExamEnd(abandon ? "abandon" : "finish");
}
async function showExamEnd(action) {
  try {
    const d = action ? await api("POST", "/api/exam/" + action) : await api("GET", "/api/exam");
    S.examResult = d.last || null;
  } catch (e) { toast(e.message, "bad"); }
  await refreshState();
  if (S.route.name === "exam") renderExam(); else go("exam");
}

function examResult(e, title) {
  const pct = Math.round(e.percent);
  const verdict = e.abandoned ? h("div", { class: "verdict" }, `ABANDONED · ${e.passed}/${e.total} · ${pct}%`)
    : h("div", { class: "verdict " + (e.pass ? "ok" : "bad") }, `${e.pass ? "PASSED" : "NOT PASSED"} · ${e.passed}/${e.total} · ${pct}%`);
  const rows = e.questions.map((q, i) => h("a", { class: "q-row", href: "#/ex/" + q.id },
    h("span", { class: "n" }, String(i + 1) + "."), qmark(q), h("span", { class: "id" }, q.id), dots(q.difficulty), h("span", { class: "title" }, q.title),
    h("span", { class: q.passed ? "res ok" : q.checks ? "res bad" : "res" }, q.passed ? "solved" : q.checks ? "not solved" : "not attempted")));
  return [
    h("div", { class: "section-title" }, title || "Result"),
    h("h2", { class: "exam-title" }, e.title), verdict,
    h("p", { class: "muted" }, `Time used ${fmtClock(e.timeUsedSeconds)} of ${fmtClock(e.durationSeconds)} · pass mark ${Math.round(e.passMark)}%`,
      e.previousBest != null ? ` · your previous best on this pool: ${Math.round(e.previousBest)}%` : ""),
    h("div", { class: "q-list" }, ...rows),
    h("p", { class: "muted small" }, "Open a question to see its solution: they're unlocked now."),
  ];
}

function examSetup(d, last) {
  const pool = h("select", { id: "ex-pool" }, ...d.sources.map((s) => h("option", { value: s.index }, `${s.label}  (${s.count})`)));
  const count = h("select", { id: "ex-count" }, ...d.counts.map((n) => h("option", { value: n }, `${n} questions`)));
  const mins = h("select", { id: "ex-mins" }, ...d.minutes.map((n) => h("option", { value: n }, `${n} minutes`)));
  const hints = h("input", { type: "checkbox", id: "ex-hints" });
  pool.value = store.get("examPool", "0"); count.value = store.get("examCount", String(d.counts[1])); mins.value = store.get("examMins", String(d.minutes[1]));
  if (pool.selectedIndex < 0) pool.selectedIndex = 0;
  if (count.selectedIndex < 0) count.selectedIndex = 0;
  if (mins.selectedIndex < 0) mins.selectedIndex = 0;
  const start = async () => {
    store.set("examPool", pool.value); store.set("examCount", count.value); store.set("examMins", mins.value);
    try {
      await api("POST", "/api/exam", { source: Number(pool.value), count: Number(count.value), minutes: Number(mins.value), hints: hints.checked });
      S.examResult = null;
      toast("Exam started — good luck! Open a question to begin.", "ok");
      renderExam();
    } catch (e) { toast(e.message, "bad"); }
  };
  const out = [
    h("div", { class: "hero" }, h("h1", {}, "New exam"), h("p", {}, "A timed set of exercises drawn at random from one pool, graded by their checks.")),
    h("div", { class: "form" },
      h("label", {}, h("span", {}, "Pool"), pool),
      h("label", {}, h("span", {}, "Questions"), count),
      h("label", {}, h("span", {}, "Time limit"), mins),
      h("label", { class: "check" }, hints, h("span", {}, "Allow hints and solutions (otherwise locked: exam conditions)")),
      h("div", {}, h("button", { class: "btn btn-primary", onclick: start }, "Start exam"))),
    h("ul", { class: "muted how" },
      h("li", {}, "Questions are drawn at random: about a third easy, a third medium, a third hard. Exercises that rebuild their environment are left out."),
      h("li", {}, "Each question gets a fresh lab when you start it — finish one before moving on."),
      h("li", {}, "Your last check per question counts. Starting a question again resets its lab."),
      h("li", {}, `Pass mark: ${Math.round(d.passMark * 100)}%. When time runs out, checking stops and you get your result.`)),
  ];
  if (last) out.push(...examResult(last, "Last exam"));
  return out;
}

// ---------- weak spots ----------
async function renderWeak() {
  S.route = { name: "weak" };
  let d;
  try { d = await api("GET", "/api/weakspots"); } catch (e) { toast(e.message, "bad"); return; }
  if (S.route.name !== "weak") return;
  const page = h("div", { class: "page" }, h("div", { class: "hero" }, h("h1", {}, "Weak spots"),
    h("p", {}, "The modules where you needed the most help — hints, solutions and failed checks — over the exercises you tried.")));
  if (!d.any) page.append(h("div", { class: "locked" }, h("p", {}, "Nothing to go on yet: this fills in as you solve exercises.")));
  else if (!d.modules.length) page.append(h("div", { class: "banner ok" }, "No weak spots: every exercise you tried passed first time, without help."));
  else {
    const t = h("table", { class: "tbl" }, h("tr", {}, h("th", {}, "Module"), h("th", {}, "Solved"), h("th", {}, "Hints"), h("th", {}, "Solutions"), h("th", {}, "Retries")));
    for (const m of d.modules) t.append(h("tr", {}, h("td", {}, m.name), h("td", {}, `${m.solved}/${m.tried}`), h("td", {}, String(m.hints)), h("td", {}, String(m.solutions)), h("td", {}, String(m.retries))));
    page.append(h("div", { class: "tbl-wrap" }, t));
  }
  if (d.retry.length) {
    page.append(h("div", { class: "section-title" }, "Worth doing again without the solution"),
      h("div", { class: "q-list" }, ...d.retry.map((r) => h("a", { class: "q-row two", href: "#/ex/" + r.id }, h("span", { class: "id" }, r.id), h("span", { class: "title" }, r.title)))),
      h("p", { class: "muted small" }, "Reset progress on one (on its page) to try it fresh."));
  }
  $("#content").replaceChildren(page);
}

// ---------- settings ----------
async function renderSettings() {
  S.route = { name: "settings" };
  let d;
  try { d = await api("GET", "/api/settings"); } catch (e) { toast(e.message, "bad"); return; }
  if (S.route.name !== "settings") return;
  const page = h("div", { class: "page" }, h("div", { class: "hero" }, h("h1", {}, "Settings"),
    h("p", {}, "Saved to ", h("code", {}, d.path), " — its comments and VM sizes are kept.")));
  if (d.error) page.append(h("div", { class: "banner bad" }, d.error));
  const inputs = {};
  const applies = { now: "applies now", "next start": "from the next start", "terminal UI": "terminal UI" };
  const form = h("div", { class: "settings" });
  for (const st of d.settings) {
    let input;
    if (st.type === "bool") { input = h("input", { type: "checkbox" }); input.checked = !!st.value; }
    else if (st.type === "int") { input = h("input", { type: "number", min: "0", step: "1", class: "num" }); input.value = st.value; }
    else { input = h("select", {}, ...st.options.map((o) => h("option", { value: o }, o))); input.value = st.value; }
    inputs[st.key] = { input, st };
    form.append(h("div", { class: "setting" },
      h("div", { class: "setting-head" }, h("label", {}, h("b", {}, st.label), " ", h("code", { class: "small muted" }, st.key)),
        h("span", { class: "badge" + (st.applies === "now" ? " solved" : ""), title: st.applies === "terminal UI" ? "Only the terminal UI (awesome_labs tui) uses this" : null }, applies[st.applies] || st.applies)),
      h("div", { class: "setting-row" }, input, h("span", { class: "muted small" }, st.help))));
  }
  const save = async () => {
    const body = {};
    for (const [k, { input, st }] of Object.entries(inputs)) {
      const v = st.type === "bool" ? input.checked : st.type === "int" ? Number(input.value) : input.value;
      if (v !== st.value) body[k] = v;
    }
    if (!Object.keys(body).length) return toast("Nothing changed", "info");
    try {
      const r = await api("PUT", "/api/settings", body);
      toast("✔ " + r.saved, "ok", 4000);
      renderSettings();
    } catch (e) { toast(e.message, "bad"); }
  };
  page.append(form, h("div", { class: "actions" }, h("button", { class: "btn btn-primary", onclick: save }, "Save"),
    h("span", { class: "muted small" }, `Stopping the server now: on_exit ${d.runningOnExit}.`)));
  $("#content").replaceChildren(page);
}

// ---------- resume: the exercise set up on the (usually stopped) VMs ----------
async function offerResume() {
  const a = S.state.active;
  if (!a || store.get("resumeDismissed", "") === a.id + a.since) return;
  let ex;
  try { ex = await api("GET", "/api/exercises/" + encodeURIComponent(a.id)); } catch { return; }
  const box = $("#resume");
  const close = () => { box.hidden = true; };
  const cont = async () => {
    close();
    go("ex/" + a.id);
    if (ex.running === false) {
      S.openTermAfterBoot = a.id;
      try { await api("POST", `/api/exercises/${encodeURIComponent(a.id)}/boot`); await refreshState(); toast("Booting the lab — the terminal opens when it's up", "info"); }
      catch (e) { S.openTermAfterBoot = null; toast(e.message, "bad"); }
    } else if (ex.terminals[0]) terms.open(ex.terminals[0].target, ex.terminals[0].label);
  };
  box.replaceChildren(
    h("div", { class: "resume-text" }, h("b", {}, "Continue " + a.id), " — " + a.title + ": your lab is kept",
      h("div", { class: "muted small" }, ex.running === false ? "Its VMs are stopped; Continue boots them and opens a terminal." : "Its VMs are running.")),
    h("button", { class: "btn btn-primary btn-sm", onclick: cont }, ex.running === false ? "⏻ Continue" : "Continue"),
    h("button", { class: "icon-btn", title: "Not now", onclick: () => { store.set("resumeDismissed", a.id + a.since); close(); } }, "×"));
  box.hidden = false;
}

const fmtBytes = (n) => !n ? "–" : n >= 1 << 30 ? (n / (1 << 30)).toFixed(1) + " GiB" : n >= 1 << 20 ? Math.round(n / (1 << 20)) + " MiB" : Math.round(n / 1024) + " KiB";

// ---------- operations stream ----------
function connectOps() {
  const es = new EventSource("/api/ops/stream");
  let opened = false;
  es.onopen = () => {
    $("#conn").className = "conn"; $("#conn").textContent = "connected";
    if (opened) resync();
    opened = true;
  };
  es.onerror = () => { $("#conn").className = "conn down"; $("#conn").textContent = "reconnecting…"; };
  es.addEventListener("op", (ev) => {
    const o = JSON.parse(ev.data);
    S.logs.set(o.target, { op: o, base: 0, lines: [], partial: "" });
    if (o.kind !== "check" && o.kind !== "jupyter" && (!o.env || o.env === "docker") && terms.list.some((x) => x.kind === "nb")) {
      terms.closeNotebook(false);
      toast("The notebook tab closed: " + o.label.toLowerCase() + " removes JupyterLab. Your notebooks in /lab stay until the lab is reset.", "info", 7000);
    }
    S.state.op = o;
    renderStatus();
    if (S.route.name === "ex" && S.ex?.id === o.target) renderExercise(false);
    if (S.route.name === "envs" && o.target === "env") renderEnvs();
  });
  es.addEventListener("lines", (ev) => {
    const d = JSON.parse(ev.data);
    const L = [...S.logs.values()].find((x) => x.op.id === d.id);
    if (!L) return;
    // d.from is an absolute line number; L.lines starts at line L.base
    if (d.from < L.base || d.from > L.base + L.lines.length) { L.base = d.from; L.lines = []; }
    else L.lines.length = d.from - L.base;
    L.lines.push(...d.lines);
    if (L.lines.length > MAX_LOG_LINES) { const n = L.lines.length - MAX_LOG_LINES; L.lines.splice(0, n); L.base += n; }
    L.partial = d.partial;
    renderStatus(L);
    const pre = document.getElementById("log-" + L.op.target);
    if (pre) {
      const stick = pre.scrollHeight - pre.scrollTop - pre.clientHeight < 40;
      fillLog(pre, L);
      if (stick) pre.scrollTop = pre.scrollHeight;
    }
  });
  es.addEventListener("done", (ev) => opDone(JSON.parse(ev.data)));
}

const MAX_LOG_LINES = 20000;
// logFrom makes a log from an op snapshot that carries its output
function logFrom(o) { return { op: o, base: o.dropped || 0, lines: o.lines || [], partial: o.partial || "" }; }

// resync catches up after the stream reconnects: an op this page watched may
// have finished while it was away, and the stream doesn't repeat that news
async function resync() {
  await refreshState();
  for (const [target, L] of S.logs) {
    if (L.op.done) continue;
    try {
      const r = await api("GET", "/api/ops/last?target=" + encodeURIComponent(target));
      if (r.op && r.op.id === L.op.id && r.op.done) { S.logs.set(target, logFrom(r.op)); await opDone(r.op); }
    } catch { /* next reconnect */ }
  }
}

async function opDone(o) {
  const L = S.logs.get(o.target);
  if (L && L.op.id === o.id) L.op = o;
  S.state.op = o;
  renderStatus();
  if (o.error) toast("✘ " + o.label + ": " + o.error, "bad", 8000);
  else if (o.kind === "check") {
    S.lastResult[o.target] = o.passed ? "passed" : "failed";
    toast(o.passed ? "✔ " + o.target + " solved!" : "Not yet — some checks of " + o.target + " fail", o.passed ? "ok" : "bad");
  } else if (o.kind === "start") toast("Lab ready — open a terminal and solve the task", "ok");
  else if (o.kind === "end") toast(o.target + " ended — its VMs are clean for the next exercise", "ok");
  else if (o.kind === "jupyter") { toast("JupyterLab is ready", "ok"); terms.frameNotebook(o.target); }
  else toast("✔ " + o.label, "ok");
  if (o.kind === "start" && !o.error && S.ex?.id === o.target) {
    S.tab = "scenario";
  }
  await refreshState();
  await refreshProgress();
  if (S.route.name === "ex" && S.ex) await openExercise(S.ex.id, true);
  if (o.kind === "start" && !o.error && S.ex?.id === o.target && S.ex.terminals[0]) terms.open(S.ex.terminals[0].target, S.ex.terminals[0].label);
  if (o.kind === "boot" && S.openTermAfterBoot === o.target) {
    S.openTermAfterBoot = null;
    if (!o.error && S.ex?.id === o.target && S.ex.terminals[0]) terms.open(S.ex.terminals[0].target, S.ex.terminals[0].label);
  }
  if (S.route.name === "envs") renderEnvs();
  if (S.route.name === "exam" && o.kind === "check") renderExam();
  if (S.route.name === "home") renderHome();
}

let statusTimer;
function renderStatus(L) {
  const o = busyOp(), el = $("#op-status");
  clearInterval(statusTimer);
  if (!o) { el.replaceChildren(h("span", {}, S.state.active ? `▶ ${S.state.active.id} is set up on the VMs` : "Ready")); return; }
  const last = (L || S.logs.get(o.target));
  const tail = last ? (last.partial || last.lines[last.lines.length - 1] || "") : "";
  const time = h("span", {});
  const tick = () => {
    time.textContent = fmtDur((Date.now() - new Date(o.started)) / 1000);
    const el = document.getElementById("log-elapsed");
    if (el) el.textContent = time.textContent + " so far";
  };
  tick(); statusTimer = setInterval(tick, 1000);
  el.replaceChildren(h("span", { class: "spinner" }), h("b", {}, o.label), time, h("span", { class: "last" }, tail));
}

async function refreshState() {
  // absent keys mean "none": don't let a stale active exercise, exam or op survive the merge
  try { S.state = { active: null, exam: null, op: null, ...(await api("GET", "/api/state")) }; } catch { return; }
  renderTop(); renderStatus();
  if (S.state.repo) $("#repo-link").href = S.state.repo;
  if (!S.state.active) $("#resume").hidden = true;
}
async function refreshProgress() {
  try { S.progress = await api("GET", "/api/progress"); } catch { return; }
  renderTop(); renderList();
}

// ---------- terminals ----------
const termTheme = {
  background: "#0a0c0f", foreground: "#d8dce4", cursor: "#7aa2ff", cursorAccent: "#0a0c0f", selectionBackground: "#2b3a5c",
  black: "#1b1f26", red: "#f0646a", green: "#46c37b", yellow: "#e0b04a", blue: "#7aa2ff", magenta: "#c08cf5", cyan: "#5cc8d7", white: "#d8dce4",
  brightBlack: "#5d6574", brightRed: "#ff8a8f", brightGreen: "#6fdc9b", brightYellow: "#f2c96e", brightBlue: "#a3c0ff", brightMagenta: "#d6b1fa", brightCyan: "#8ce0ea", brightWhite: "#ffffff",
};
const terms = {
  list: [], cur: null, seq: 0,
  panel: $("#termpanel"),
  open(target, label) {
    let t = this.list.find((x) => x.target === target);
    if (!t) t = this.create(target, label);
    else if (t.state === "dead") this.reconnect(t);
    this.show(t);
  },
  create(target, label) {
    const id = ++this.seq;
    const body = h("div", { class: "term-body" });
    $("#term-bodies").append(body);
    const term = new Terminal({
      fontFamily: getComputedStyle(document.body).getPropertyValue("--mono"), fontSize: 13, lineHeight: 1.15,
      cursorBlink: true, scrollback: 10000, theme: termTheme, allowProposedApi: false, macOptionIsMeta: true,
    });
    const fit = new FitAddon.FitAddon();
    term.loadAddon(fit);
    try { term.loadAddon(new WebLinksAddon.WebLinksAddon()); } catch { /* optional */ }
    term.open(body);
    const t = { id, target, label, term, fit, body, ws: null, state: "connecting", tab: null };
    t.tab = h("div", { class: "term-tab", role: "tab", onclick: () => this.show(t) },
      h("span", { class: "sdot" }), label, h("button", { class: "x", title: "Close this terminal", onclick: (e) => { e.stopPropagation(); this.close(t); } }, "×"));
    $("#term-tablist").append(t.tab);
    const enc = new TextEncoder();
    term.onData((d) => { if (t.ws?.readyState === 1) t.ws.send(enc.encode(d)); else if (t.state === "dead" && d === "\r") this.reconnect(t); });
    term.onBinary((d) => { if (t.ws?.readyState === 1) t.ws.send(Uint8Array.from(d, (c) => c.charCodeAt(0))); });
    term.onResize(({ cols, rows }) => { if (t.ws?.readyState === 1) t.ws.send(JSON.stringify({ type: "resize", cols, rows })); });
    new ResizeObserver(() => { if (t === this.cur) this.fit(t); }).observe(body);
    this.list.push(t);
    this.connect(t);
    return t;
  },
  connect(t) {
    this.panelShown(true);
    this.fit(t);
    const { cols, rows } = t.term;
    const ws = new WebSocket(`${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws/term?target=${encodeURIComponent(t.target)}&cols=${cols}&rows=${rows}`);
    ws.binaryType = "arraybuffer";
    t.ws = ws; this.setState(t, "connecting");
    t.term.write(`\x1b[2m connecting to ${t.label}…\x1b[0m\r\n`);
    ws.onmessage = (ev) => {
      if (typeof ev.data !== "string") { t.term.write(new Uint8Array(ev.data)); return; }
      const m = JSON.parse(ev.data);
      if (m.type === "ready") this.setState(t, "live");
      else if (m.type === "error") { t.term.write(`\r\n\x1b[31m✘ ${m.message}\x1b[0m\r\n`); t.failed = true; }
      else if (m.type === "exit") t.exitCode = m.code;
    };
    ws.onclose = () => {
      if (t.state === "closed") return;
      this.setState(t, "dead");
      const how = t.failed ? "" : t.exitCode != null ? ` (exit ${t.exitCode})` : " (connection closed)";
      t.term.write(`\r\n\x1b[2m── shell ended${how} · press Enter to reconnect ──\x1b[0m\r\n`);
      t.failed = false; t.exitCode = null;
    };
  },
  reconnect(t) { t.term.write("\r\n"); this.connect(t); },
  setState(t, st) { t.state = st; t.tab.querySelector(".sdot").className = "sdot " + (st === "live" ? "live" : st === "dead" ? "dead" : ""); },
  show(t) {
    this.panelShown(true);
    for (const x of this.list) { x.body.hidden = x !== t; x.tab.classList.toggle("on", x === t); }
    this.cur = t;
    requestAnimationFrame(() => { this.fit(t); (t.term || t.frame).focus(); });
  },
  // the notebook tab: JupyterLab, proxied by the server at /jupyter/
  async openNotebook() {
    const nb = this.list.find((x) => x.kind === "nb");
    if (nb) return this.show(nb);
    try {
      await api("POST", "/api/jupyter/start");
      toast("Starting JupyterLab — the first time on a VM it builds its image (about a minute)", "info", 6000);
      await refreshState();
      if (S.ex) renderExercise(false);
    } catch (e) { toast(e.message, "bad"); }
  },
  async frameNotebook(exId) {
    let t = this.list.find((x) => x.kind === "nb");
    if (!t) {
      // open the exercise's starter notebook when it's there
      const name = S.ex?.id === exId && S.ex.starter;
      let src = "/jupyter/lab";
      if (name) {
        try { if ((await fetch("/jupyter/api/contents/" + encodeURIComponent(name) + "?content=0", { credentials: "same-origin" })).ok) src = "/jupyter/lab/tree/" + encodeURIComponent(name); }
        catch { /* JupyterLab's own start page then */ }
      }
      const frame = h("iframe", { class: "nb-frame", src, title: "JupyterLab", allow: "clipboard-read; clipboard-write" });
      const body = h("div", { class: "term-body nb-body" }, frame);
      $("#term-bodies").append(body);
      t = { kind: "nb", target: "jupyter", label: "notebook", body, frame, state: "live", term: null };
      t.tab = h("div", { class: "term-tab", role: "tab", onclick: () => this.show(t) },
        h("span", { class: "sdot live" }), "📓 notebook", h("button", { class: "x", title: "Close the notebook (stops JupyterLab; your notebooks stay in /lab)", onclick: (e) => { e.stopPropagation(); this.close(t); } }, "×"));
      $("#term-tablist").append(t.tab);
      this.list.push(t);
    }
    this.show(t);
  },
  // closing the tab stops JupyterLab (nothing else needs it; notebooks stay in /lab), unless the
  // tab closes because an operation is removing it anyway
  closeNotebook(stop = false) { const t = this.list.find((x) => x.kind === "nb"); if (t) this.close(t, stop); },
  fit(t) { try { if (t.term && !t.body.hidden && t.body.clientWidth > 0) t.fit.fit(); } catch { /* not visible yet */ } },
  close(t, stopNotebook = true) {
    if (t.kind === "nb" && stopNotebook) api("POST", "/api/jupyter/stop").catch(() => {});
    t.state = "closed";
    try { t.ws?.close(); } catch { /* gone */ }
    t.term?.dispose(); t.body.remove(); t.tab.remove();
    this.list = this.list.filter((x) => x !== t);
    if (this.cur === t) this.cur = null;
    if (this.list.length) this.show(this.list[this.list.length - 1]);
    else this.panelShown(false);
  },
  paste(text) {
    let t = this.cur && this.cur.term && this.cur.state === "live" ? this.cur : this.list.find((x) => x.term && x.state === "live");
    if (!t) {
      if (S.ex?.active && S.ex.running !== false) {
        const first = S.ex.terminals[0];
        this.open(first.target, first.label);
        t = this.cur;
        setTimeout(() => t.state === "live" ? t.term.paste(text) : toast("The terminal isn't ready yet", "info"), 1500);
        return;
      }
      return toast("Open a terminal first — start the lab, then Terminal", "info");
    }
    this.show(t);
    t.term.paste(text);
  },
  panelShown(on) {
    const main = $("#main");
    this.panel.hidden = !on;
    $("#splitter").hidden = !on;
    $("#term-show").hidden = on || !this.list.length;
    if (!on) main.classList.remove("term-max");
    if (on && this.cur) requestAnimationFrame(() => this.fit(this.cur));
  },
};

// the list's width: drag the edge between it and the page; double-click resets it
function setupSidebar() {
  const grip = $("#side-grip"), root = document.documentElement;
  const set = (px) => root.style.setProperty("--side-w", Math.round(Math.min(Math.max(px, 260), window.innerWidth * 0.6)) + "px");
  const saved = store.get("sideW", null);
  if (saved) set(saved);
  grip.onpointerdown = (e) => {
    e.preventDefault();
    grip.setPointerCapture(e.pointerId);
    grip.classList.add("drag");
    document.body.classList.add("resizing");
    const move = (ev) => set(ev.clientX);
    const up = () => {
      grip.classList.remove("drag"); document.body.classList.remove("resizing");
      grip.removeEventListener("pointermove", move); grip.removeEventListener("pointerup", up);
      store.set("sideW", parseInt(getComputedStyle(root).getPropertyValue("--side-w"), 10));
      if (terms.cur) terms.fit(terms.cur);
    };
    grip.addEventListener("pointermove", move);
    grip.addEventListener("pointerup", up);
  };
  grip.ondblclick = () => { root.style.removeProperty("--side-w"); store.set("sideW", null); if (terms.cur) terms.fit(terms.cur); };
}

function setupTermPanel() {
  const main = $("#main"), panel = $("#termpanel"), split = $("#splitter");
  const applyLayout = () => {
    main.classList.toggle("side", store.get("termSide", false));
    const size = store.get(store.get("termSide", false) ? "termW" : "termH", 42);
    panel.style.flex = `0 0 ${size}%`;
    if (terms.cur) requestAnimationFrame(() => terms.fit(terms.cur));
  };
  applyLayout();
  $("#term-layout").onclick = () => { store.set("termSide", !store.get("termSide", false)); applyLayout(); };
  $("#term-max").onclick = () => { main.classList.toggle("term-max"); if (terms.cur) requestAnimationFrame(() => terms.fit(terms.cur)); };
  $("#term-hide").onclick = () => terms.panelShown(false);
  $("#term-show").onclick = () => terms.show(terms.cur || terms.list[terms.list.length - 1]);
  split.onpointerdown = (e) => {
    e.preventDefault();
    split.setPointerCapture(e.pointerId);
    split.classList.add("drag");
    const side = main.classList.contains("side"), r = main.getBoundingClientRect();
    const move = (ev) => {
      const pct = side ? ((r.right - ev.clientX) / r.width) * 100 : ((r.bottom - ev.clientY) / r.height) * 100;
      const v = Math.min(85, Math.max(15, pct));
      panel.style.flex = `0 0 ${v}%`;
      store.set(side ? "termW" : "termH", Math.round(v));
    };
    const up = () => { split.classList.remove("drag"); split.removeEventListener("pointermove", move); split.removeEventListener("pointerup", up); if (terms.cur) terms.fit(terms.cur); };
    split.addEventListener("pointermove", move);
    split.addEventListener("pointerup", up);
  };
}

// ---------- first launch: build the environments ahead of time ----------
async function offerPrepare() {
  let d;
  try { d = await api("GET", "/api/prepare"); } catch { return; }
  if (!d.offer) return;
  const rows = d.envs.map((e) => `<label class="prep-row"><input type="checkbox" value="${esc(e.profile)}" checked>
      <span><b>${esc(e.title)}</b> <span class="muted">· ${esc(e.minutes)}</span><br><span class="small muted">${esc(e.tracks)}</span></span></label>`).join("");
  const yes = await confirmDialog("Build the lab environments now?",
    `<p>An environment is built the first time an exercise needs it, which takes a while. Built now, its exercises start in seconds instead.</p>
     <div class="prep-list">${rows}</div>
     <p class="small">Each is built, saved and then <b>stopped</b>: no VM runs until an exercise needs it, and VMs nothing uses are stopped again.
     It takes disk space (up to ~40 GiB in all) and you can keep working meanwhile. Later: Environments &amp; VMs → Prepare.</p>`,
    "Build selected", false, "Not now");
  const envs = yes ? [...document.querySelectorAll("#modal-body .prep-row input:checked")].map((c) => c.value) : [];
  try {
    await api("POST", "/api/prepare", { envs });
    if (envs.length) { toast("Building in the background — watch it in the status bar or on Environments & VMs", "info", 7000); await refreshState(); }
    else toast("OK — each environment is built when an exercise first needs it (or later from Environments & VMs → Prepare)", "info", 7000);
  } catch (e) { toast(e.message, "bad"); }
}

// ---------- keys ----------
document.addEventListener("keydown", (e) => {
  if (!$("#modal").hidden) return;
  const t = e.target;
  const typing = t.closest?.(".xterm") || ["INPUT", "TEXTAREA", "SELECT"].includes(t.tagName);
  if (typing) return;
  if (e.key === "/") { e.preventDefault(); $("#q").focus(); $("#q").select(); }
  else if ((e.key === "j" || e.key === "k") && S.filtered.length) {
    const i = S.ex ? S.filtered.findIndex((x) => x.id === S.ex.id) : -1;
    const n = S.filtered[Math.min(S.filtered.length - 1, Math.max(0, i + (e.key === "j" ? 1 : -1)))];
    if (n) go("ex/" + n.id);
  } else if (S.ex && /^[1-6]$/.test(e.key)) {
    const tabs = TABS.filter(([k]) => k !== "learn" || S.ex.tutorial);
    const tab = tabs[Number(e.key) - 1];
    if (tab) setTab(tab[0]);
  }
});

// ---------- boot ----------
(async function boot() {
  try {
    S.catalog = await api("GET", "/api/catalog");
  } catch (e) {
    $("#content").replaceChildren(h("div", { class: "page" }, h("div", { class: "banner bad" }, "Can't reach awesome_labs: " + e.message)));
    return;
  }
  S.trackName = Object.fromEntries(S.catalog.tracks.map((t) => [t.id, t.name]));
  for (const e of S.catalog.exercises) S.byId.set(e.id, e);
  S.modRank = new Map();
  for (const e of S.catalog.exercises) { const k = e.track + "/" + e.module; if (!S.modRank.has(k)) S.modRank.set(k, S.modRank.size); }
  // a list toggle for narrow screens
  const menu = h("a", { class: "btn btn-ghost list-btn", href: "#/list", title: "Exercise list" }, "☰ List");
  $(".topbar").insertBefore(menu, $(".top-spacer"));
  setupFilters();
  setupTermPanel();
  setupSidebar();
  await Promise.all([refreshState(), refreshProgress()]);
  connectOps();
  window.addEventListener("hashchange", route);
  route();
  offerPrepare();
  offerResume();
  setInterval(refreshState, 5000);
  window.addEventListener("beforeunload", (e) => { if (terms.list.some((t) => t.term && t.state === "live")) /* shells end with the page; notebooks don't */ { e.preventDefault(); e.returnValue = ""; } });
})();
