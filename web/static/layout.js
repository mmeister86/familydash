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
// Returns {placements: [{type, x, y, w, h}], hidden: [type], unknown: [type]}.
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
    placements.push({ type, x: w.x, y: w.y, w: w.w, h: w.h });
  }
  return { placements, hidden: [...hidden], unknown };
}
