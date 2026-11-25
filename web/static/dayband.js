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

// Label inside the bar when it fits, else right of it, else left of it.
// prevEnd/nextStart (neighbours in the same row) keep an outside label off
// the next bar: a right label that would run into it goes left when that is free.
function daybandSide(label, start, end, prevEnd = -Infinity, nextStart = Infinity) {
  const need = label.length * DAYBAND_CHAR_W;
  if (need <= end - start) return "in";
  if (end + need <= DAYBAND_TO && end + need <= nextStart) return "right";
  if (start - need >= DAYBAND_FROM && start - need >= prevEnd) return "left";
  return end + need <= DAYBAND_TO ? "right" : "left";
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

// Sorts by start and puts overlapping segments on a second row.
function daybandLayout(name, color, segs) {
  segs.sort((a, b) => a.start - b.start || a.end - b.end);
  const rowEnd = [-Infinity, -Infinity];
  let rows = 1;
  const placed = segs.map((s) => {
    const row = rowEnd[0] <= s.start ? 0 : 1;
    rowEnd[row] = Math.max(rowEnd[row], s.end);
    if (row) rows = 2;
    return { ...s, row };
  });
  const segments = placed.map((s, i) => {
    const mine = placed.filter((o) => o.row === s.row);
    const k = mine.indexOf(s);
    return {
      start: s.start, end: s.end, label: s.label, kind: s.kind, row: s.row,
      side: daybandSide(s.label, s.start, s.end, mine[k - 1]?.end, mine[k + 1]?.start),
    };
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
