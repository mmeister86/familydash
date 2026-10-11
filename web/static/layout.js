// Pure tile placement for the 12x24 wall grid: no DOM, no side effects.
// Tested via web/layout.test.mjs (`pnpm test`); loaded before app.js.

// Widget type → section element id. One element per type exists, so each
// type is placed at most once.
const EL = {
  clock: "clock-card",
  shopping: "shopping",
  briefing: "briefing",
  news: "news",
  photos: "photo-frame",
  school: "school",
  calendars: "calendars",
};

// resolveLayout places feed widgets onto the grid.
// widgets: [{type, x, y, w, h, fallback?}], has: {type: bool} content flags.
// An empty widget hands its tiles to its fallback (when that one has
// content and isn't placed yet); otherwise the area stays empty. Unknown
// types are skipped and reported.
// Returns {placements: [{type, x, y, w, h, narrow}], hidden: [type], unknown: [type]}.
// narrow flags tiles below 6 columns for compact CSS (data-narrow).
function resolveLayout(widgets, has) {
  const placements = [];
  const hidden = new Set(Object.keys(EL));
  const unknown = [];
  const placed = new Set();
  for (const w of widgets || []) {
    if (EL[w.type] === undefined) {
      unknown.push(w.type);
      continue;
    }
    let type = w.type;
    if (has[type] === false) {
      if (!w.fallback || EL[w.fallback] === undefined || has[w.fallback] === false || placed.has(w.fallback)) continue;
      type = w.fallback;
    }
    if (placed.has(type)) continue;
    placed.add(type);
    hidden.delete(type);
    placements.push({ type, x: w.x, y: w.y, w: w.w, h: w.h, narrow: w.w < 6 });
  }
  return { placements, hidden: [...hidden], unknown };
}

// briefingKind returns the briefing kind a layout needs ("today"/
// "tomorrow") or null when it shows no briefing widget.
function briefingKind(widgets) {
  const w = (widgets || []).find((w) => w.type === "briefing");
  return w?.props?.kind || null;
}

// scenePayloadChanged reports whether a light /api/scene payload differs
// from the dashboard state: scene, force/preview flags, preview window
// (re-previewing the same scene bumps until, not revision) or revision.
function scenePayloadChanged(cur, payload) {
  const a = cur?.scene, b = payload?.scene;
  if (!b) return false;
  return a?.id !== b.id
    || !!a?.forced !== !!b.forced
    || !!a?.preview !== !!b.preview
    || (b.preview && a?.until !== b.until)
    || cur?.layout?.revision !== payload?.layout?.revision;
}
