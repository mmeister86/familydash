// Placement self-check for the 12x24 wall grid. Run: node web/layout.test.mjs
import { readFileSync } from "node:fs";
import assert from "node:assert";

const src = readFileSync(new URL("./static/layout.js", import.meta.url), "utf8");
const { resolveLayout, briefingKind, scenePayloadChanged, EL } = new Function(`${src}; return { resolveLayout, briefingKind, scenePayloadChanged, EL };`)();

const allTypes = ["clock", "shopping", "briefing", "news", "photos", "school", "calendars"];
const allShown = Object.fromEntries(allTypes.map((t) => [t, true]));

// basic placement, rest hidden
{
  const widgets = [
    { type: "clock", x: 0, y: 0, w: 12, h: 4 },
    { type: "news", x: 0, y: 4, w: 6, h: 5 },
  ];
  const got = resolveLayout(widgets, allShown);
  assert.deepStrictEqual(got.placements, [
    { type: "clock", x: 0, y: 0, w: 12, h: 4, narrow: false },
    { type: "news", x: 0, y: 4, w: 6, h: 5, narrow: false },
  ]);
  assert.deepStrictEqual(got.hidden.sort(), ["shopping", "briefing", "photos", "school", "calendars"].sort());
  assert.deepStrictEqual(got.unknown, []);
  assert.strictEqual(EL["clock"], "clock-card");
  assert.strictEqual(EL["photos"], "photo-frame");
}

// narrow tiles (below 6 columns) are flagged for compact CSS
{
  const widgets = [{ type: "news", x: 0, y: 0, w: 4, h: 4 }];
  const got = resolveLayout(widgets, allShown);
  assert.deepStrictEqual(got.placements, [
    { type: "news", x: 0, y: 0, w: 4, h: 4, narrow: true },
  ]);
}

// empty widget: fallback takes the same tiles
{
  const widgets = [
    { type: "briefing", x: 6, y: 4, w: 6, h: 5, fallback: "shopping" },
  ];
  const got = resolveLayout(widgets, { ...allShown, briefing: false });
  assert.deepStrictEqual(got.placements, [{ type: "shopping", x: 6, y: 4, w: 6, h: 5, narrow: false }]);
  assert.ok(got.hidden.includes("briefing"));
}

// empty widget without fallback: area stays empty
{
  const widgets = [{ type: "news", x: 0, y: 4, w: 6, h: 5 }];
  const got = resolveLayout(widgets, { ...allShown, news: false });
  assert.deepStrictEqual(got.placements, []);
  assert.ok(got.hidden.includes("news"));
}

// two empties, same fallback: first wins, second area stays empty
{
  const widgets = [
    { type: "briefing", x: 0, y: 4, w: 6, h: 5, fallback: "shopping" },
    { type: "news", x: 6, y: 4, w: 6, h: 5, fallback: "shopping" },
  ];
  const got = resolveLayout(widgets, { ...allShown, briefing: false, news: false });
  assert.deepStrictEqual(got.placements, [{ type: "shopping", x: 0, y: 4, w: 6, h: 5, narrow: false }]);
}

// unknown types are skipped and reported, empty fallback ignored
{
  const widgets = [
    { type: "toaster", x: 0, y: 0, w: 4, h: 4 },
    { type: "news", x: 0, y: 4, w: 6, h: 5, fallback: "toaster" },
  ];
  const got = resolveLayout(widgets, { ...allShown, news: false });
  assert.deepStrictEqual(got.placements, []);
  assert.deepStrictEqual(got.unknown, ["toaster"]);
}

// empty layout: everything hidden
{
  const got = resolveLayout([], allShown);
  assert.deepStrictEqual(got.placements, []);
  assert.deepStrictEqual(got.hidden.sort(), allTypes.sort());
}

// briefingKind selects the card a layout needs
{
  assert.strictEqual(briefingKind([{ type: "briefing", props: { kind: "today" } }]), "today");
  assert.strictEqual(briefingKind([{ type: "briefing", props: { kind: "tomorrow" } }]), "tomorrow");
  assert.strictEqual(briefingKind([{ type: "briefing" }]), null);
  assert.strictEqual(briefingKind([{ type: "news" }]), null);
  assert.strictEqual(briefingKind([]), null);
  assert.strictEqual(briefingKind(undefined), null);
}

// scenePayloadChanged detects scene, flag, preview-window and revision changes
{
  const base = { scene: { id: "morning", forced: false }, layout: { revision: 3 } };
  assert.strictEqual(scenePayloadChanged(base, { scene: { id: "morning" }, layout: { revision: 3 } }), false);
  assert.strictEqual(scenePayloadChanged(base, { scene: { id: "day" }, layout: { revision: 3 } }), true);
  assert.strictEqual(scenePayloadChanged(base, { scene: { id: "morning", forced: true }, layout: { revision: 3 } }), true);
  assert.strictEqual(scenePayloadChanged(base, { scene: { id: "morning" }, layout: { revision: 4 } }), true);
  assert.strictEqual(scenePayloadChanged(base, { scene: { id: "morning", until: "2026-10-11T20:00:00+02:00" }, layout: { revision: 3 } }), false);
  const preview = { scene: { id: "morning", preview: true, until: "2026-10-11T20:00:00+02:00" }, layout: { revision: 3 } };
  assert.strictEqual(scenePayloadChanged(base, preview), true);
  const repreview = { scene: { id: "morning", preview: true, until: "2026-10-11T20:02:00+02:00" }, layout: { revision: 3 } };
  assert.strictEqual(scenePayloadChanged(preview, repreview), true);
  assert.strictEqual(scenePayloadChanged(preview, preview), false);
}

console.log("layout.test.mjs: all assertions passed");
