// Pure lane/weather helpers of the day band tile: no DOM, no side effects.
// Tested via tests/dayband.test.mjs (`pnpm test`); loaded before app.js.

const DAYBAND_FROM = 6;
const DAYBAND_TO = 22;
const DAYBAND_MIN_LEN = 0.25; // an entry shorter than 15 minutes shows as 15 minutes
const DAYBAND_CHAR_W = 0.17;  // label width estimate in hours per character

function daybandHM(hm) {
  const [h, m] = String(hm).split(":").map(Number);
  return h + (m || 0) / 60;
}

function daybandYmd(d) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

// Decimal hours of a timestamp relative to the midnight of `now`'s day.
function daybandHours(iso, midnight) {
  return (new Date(iso).getTime() - midnight.getTime()) / 3600000;
}

// Clips [start, end] to the window; null when nothing is left.
function daybandClip(start, end) {
  end = Math.max(end, start + DAYBAND_MIN_LEN);
  const s = Math.max(start, DAYBAND_FROM), e = Math.min(end, DAYBAND_TO);
  return e > s ? { start: s, end: e } : null;
}

// Hours an outside label of this width claims next to a bar.
function daybandNeed(label) {
  return label.length * DAYBAND_CHAR_W;
}

// Space a segment occupies with its label on `side`, within the window.
function daybandExtent(seg, side) {
  const need = daybandNeed(seg.label);
  if (side === "right") return { start: seg.start, end: Math.min(seg.end + need, DAYBAND_TO) };
  if (side === "left") return { start: Math.max(seg.start - need, DAYBAND_FROM), end: seg.end };
  return { start: seg.start, end: seg.end };
}

// Label sides a segment may take, preferred first: inside the bar, right of
// it, left of it, each only when the label fits the window on that side.
function daybandSides(seg) {
  const need = daybandNeed(seg.label);
  const sides = [];
  if (need <= seg.end - seg.start) sides.push("in");
  if (seg.end + need <= DAYBAND_TO) sides.push("right");
  if (seg.start - need >= DAYBAND_FROM) sides.push("left");
  return sides;
}

// Hours available to an outside label up to the next occupied extent of the
// same row (or the window edge); null for a label inside its bar.
function daybandRoom(segments, seg) {
  if (seg.side === "in") return null;
  const others = segments.filter((o) => o !== seg && o.row === seg.row).map((o) => daybandExtent(o, o.side));
  if (seg.side === "right") {
    const next = Math.min(DAYBAND_TO, ...others.filter((e) => e.start >= seg.end).map((e) => e.start));
    return Math.max(0, next - seg.end);
  }
  const prev = Math.max(DAYBAND_FROM, ...others.filter((e) => e.end <= seg.start).map((e) => e.end));
  return Math.max(0, seg.start - prev);
}

// Timed events of `events` that touch today's window, as segments.
function daybandEventSegments(events, midnight) {
  const out = [];
  for (const e of events || []) {
    if (e.allDay || !e.start) continue;
    const s = daybandHours(e.start, midnight);
    const clip = daybandClip(s, e.end ? daybandHours(e.end, midnight) : s);
    if (clip) out.push({ ...clip, label: e.title || "", kind: "event" });
  }
  return out;
}

// Same rule as onHoliday in app.js.
function daybandNoSchool(day) {
  return !!day.noSchool || (day.notices || []).some((n) => /ferien|schulfrei/i.test(n));
}

function daybandSchoolSegment(day, today) {
  if (!day || day.date !== today || daybandNoSchool(day)) return null;
  const lessons = (day.lessons || []).filter((l) => l.status !== "cancelled" && l.start && l.end);
  if (!lessons.length) return null;
  const first = lessons[0], last = lessons[lessons.length - 1];
  const clip = daybandClip(daybandHM(first.start), daybandHM(last.end));
  return clip ? { ...clip, label: `Schule bis ${last.end}`, kind: "school" } : null;
}

// Sorts by start and places every segment with its label. A segment claims
// its bar plus the label space; it takes row 0 when that is free, else row 1,
// else row 1 anyway with the label to the right (truncated by CSS).
function daybandLayout(name, color, segs) {
  segs.sort((a, b) => a.start - b.start || a.end - b.end);
  const taken = [[], []];
  const free = (row, e) => taken[row].every((t) => e.end <= t.start || t.end <= e.start);
  let rows = 1;
  const segments = segs.map((s) => {
    let sides = daybandSides(s);
    if (!sides.length) sides = [s.start - DAYBAND_FROM > DAYBAND_TO - s.end ? "left" : "right"];
    let row = 1, side = "right";
    search: for (const r of [0, 1]) {
      for (const sd of sides) {
        if (free(r, daybandExtent(s, sd))) { row = r; side = sd; break search; }
      }
    }
    taken[row].push(daybandExtent(s, side));
    if (row) rows = 2;
    return { start: s.start, end: s.end, label: s.label, kind: s.kind, row, side };
  });
  return { name, color, rows, segments };
}

// kids: [{name, color, day, events}]; calendars/events: the calendar payload.
// One lane per kid with school or school-calendar entries today, then one per
// calendar column (merged calendars share it) with timed events today.
function daybandLanes({ kids, calendars, events, now }) {
  const midnight = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const today = daybandYmd(now);
  const lanes = [];
  for (const k of kids || []) {
    const segs = daybandEventSegments(k.events, midnight);
    const school = daybandSchoolSegment(k.day, today);
    if (school) segs.push(school);
    if (segs.length) lanes.push(daybandLayout(k.name, k.color, segs));
  }
  const all = calendars || [];
  for (const c of all.filter((c) => c.panel !== "school" && !(c.into >= 0))) {
    const ids = new Set([c.id, ...all.filter((m) => m.into === c.id).map((m) => m.id)]);
    const segs = daybandEventSegments((events || []).filter((e) => ids.has(e.cal)), midnight);
    if (segs.length) lanes.push(daybandLayout(c.name, c.color, segs));
  }
  return lanes;
}

// hours: weather.today entries. Icon and temperature at 07, 09 ... 21.
function daybandWeather(hours, now) {
  const byHour = new Map();
  for (const h of hours || []) byHour.set(new Date(h.time).getHours(), h);
  const nowH = now.getHours() + now.getMinutes() / 60;
  const out = [];
  for (let hour = DAYBAND_FROM + 1; hour < DAYBAND_TO; hour += 2) {
    const h = byHour.get(hour);
    if (h) out.push({ hour, temp: h.temp, icon: h.icon, past: hour <= nowH });
  }
  return out;
}

// Runs of hours with rain probability >= 50 %, clipped to the window.
function daybandRain(hours) {
  const rainy = (hours || []).filter((h) => h.precipProb >= 50).map((h) => new Date(h.time).getHours()).sort((a, b) => a - b);
  const runs = [];
  for (const h of rainy) {
    const last = runs[runs.length - 1];
    if (last && last.end === h) last.end = h + 1;
    else if (!last || last.end < h) runs.push({ start: h, end: h + 1 });
  }
  return runs
    .map((r) => ({ start: Math.max(r.start, DAYBAND_FROM), end: Math.min(r.end, DAYBAND_TO) }))
    .filter((r) => r.end > r.start);
}

// Position of "now" as a fraction of the window; null outside it.
function daybandNow(now) {
  const h = now.getHours() + now.getMinutes() / 60;
  return h < DAYBAND_FROM || h >= DAYBAND_TO ? null : (h - DAYBAND_FROM) / (DAYBAND_TO - DAYBAND_FROM);
}
