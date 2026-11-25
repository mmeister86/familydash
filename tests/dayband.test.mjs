// Pure lane/weather helpers of the day band. Run: node tests/dayband.test.mjs
import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const src = readFileSync(new URL("../web/static/dayband.js", import.meta.url), "utf8");
const { daybandLanes, daybandWeather, daybandRain, daybandNow, daybandRoom } = new Function(
  `${src}; return { daybandLanes, daybandWeather, daybandRain, daybandNow, daybandRoom };`,
)();

const now = new Date(2026, 8, 30, 13, 18);
const p = (n) => String(n).padStart(2, "0");
const iso = (h, m = 0, off = 0) => {
  const d = new Date(2026, 8, 30 + off, h, m);
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}:00`;
};
const TODAY = "2026-09-30", TOMORROW = "2026-10-01";
const les = (start, end, status) => ({ start, end, status });
const kid = (o = {}) => ({ name: "Lukas", color: "#2460D8", day: { date: TODAY, lessons: [] }, events: [], ...o });
const lanes = (o = {}) => daybandLanes({ kids: [], calendars: [], events: [], now, ...o });
const ev = (cal, from, to, title, extra = {}) => ({ cal, title, allDay: false, start: from, end: to, ...extra });

// school bar: first to last lesson that is not cancelled
{
  const k = kid({ day: { date: TODAY, lessons: [les("07:30", "08:15"), les("08:25", "09:10", "cancelled"), les("13:40", "14:15")] } });
  const got = lanes({ kids: [k] });
  assert.equal(got.length, 1);
  assert.equal(got[0].name, "Lukas");
  assert.equal(got[0].color, "#2460D8");
  assert.equal(got[0].rows, 1);
  assert.deepEqual(got[0].segments, [{ start: 7.5, end: 14.25, label: "Schule bis 14:15", kind: "school", row: 0, side: "in" }]);
}
{
  const k = kid({ day: { date: TODAY, lessons: [les("07:30", "08:15"), les("08:25", "09:10", "cancelled")] } });
  const s = lanes({ kids: [k] })[0].segments;
  assert.equal(s[0].end, 8.25);
  assert.equal(s[0].label, "Schule bis 08:15");
}
// no school: all cancelled, noSchool, holiday notice, other day; the kid then has no lane
{
  const l = [les("07:30", "08:15"), les("08:25", "09:10")];
  const cancelled = l.map((x) => ({ ...x, status: "cancelled" }));
  for (const day of [
    { date: TODAY, lessons: cancelled },
    { date: TODAY, lessons: l, noSchool: true },
    { date: TODAY, lessons: l, notices: ["Herbstferien"] },
    { date: TODAY, lessons: l, notices: ["Schulfrei am Brückentag"] },
    { date: TOMORROW, lessons: l },
    { date: TODAY },
  ]) assert.deepEqual(lanes({ kids: [kid({ day })] }), [], JSON.stringify(day));
}
// zero-length kid event shows as 15 minutes, in the kid's lane
{
  const k = kid({ events: [ev(2, iso(15), iso(15), "Abholung")] });
  const got = lanes({ kids: [k] });
  assert.equal(got.length, 1);
  assert.deepEqual(got[0].segments, [{ start: 15, end: 15.25, label: "Abholung", kind: "event", row: 0, side: "right" }]);
}
// school bar and kid events share the lane; kid events ignore all-day entries
{
  const k = kid({
    day: { date: TODAY, lessons: [les("08:00", "12:30")] },
    events: [ev(2, iso(12, 30), iso(15), "Hort"), ev(2, iso(0), iso(0, 0, 1), "Diktat", { allDay: true })],
  });
  const s = lanes({ kids: [k] })[0].segments;
  assert.deepEqual(s.map((x) => [x.kind, x.label, x.row]), [["school", "Schule bis 12:30", 0], ["event", "Hort", 0]]);
}

// calendar lanes
const cals = [
  { id: 0, name: "Familie", color: "#14211B", panel: "column", into: -1 },
  { id: 1, name: "Feiertage", color: "#C4372C", panel: "column", into: 0 },
  { id: 2, name: "Hannah", color: "#F2B705", panel: "school", into: -1 },
  { id: 3, name: "Mama", color: "#D43F7A", panel: "column", into: -1 },
];
{
  const events = [
    ev(0, iso(11), iso(12), "Zahnarzt"),
    ev(0, iso(18, 30), iso(20, 30), "Elternabend 3b"),
    ev(1, iso(10), iso(11), "Umzug"),
    ev(0, iso(0), iso(0, 0, 1), "Geburtstag", { allDay: true }),
    ev(0, iso(9, 0, 1), iso(10, 0, 1), "Morgen"),
    ev(2, iso(16), iso(17), "Hort"),
  ];
  const got = lanes({ calendars: cals, events });
  assert.equal(got.length, 1);
  assert.equal(got[0].name, "Familie");
  assert.equal(got[0].color, "#14211B");
  assert.deepEqual(got[0].segments.map((s) => [s.label, s.start, s.end, s.kind]), [
    ["Umzug", 10, 11, "event"], ["Zahnarzt", 11, 12, "event"], ["Elternabend 3b", 18.5, 20.5, "event"],
  ]);
}
// clipping to the 6..22 window
{
  const one = (a, b) => lanes({ calendars: cals, events: [ev(0, a, b, "X")] })[0]?.segments.map((s) => [s.start, s.end]);
  assert.deepEqual(one(iso(5), iso(7)), [[6, 7]]);
  assert.deepEqual(one(iso(21), iso(23, 30)), [[21, 22]]);
  assert.equal(one(iso(4), iso(5, 30)), undefined);
  assert.equal(one(iso(22, 15), iso(23)), undefined);
  assert.deepEqual(one(iso(20, 0, -1), iso(8)), [[6, 8]]);
}
// overlaps go to a second row
{
  const events = [ev(0, iso(10), iso(12), "A"), ev(0, iso(11), iso(13), "B"), ev(0, iso(11, 30), iso(12, 30), "C")];
  const l = lanes({ calendars: cals, events })[0];
  assert.equal(l.rows, 2);
  assert.deepEqual(l.segments.map((s) => s.row), [0, 1, 1]);
  const flat = lanes({ calendars: cals, events: events.slice(0, 1) })[0];
  assert.equal(flat.rows, 1);
}
// label side
{
  const l = lanes({ calendars: cals, events: [ev(0, iso(11), iso(12), "Zahnarzt"), ev(0, iso(18, 30), iso(20, 30), "Elternabend 3b")] })[0];
  assert.deepEqual(l.segments.map((s) => s.side), ["right", "left"]);
}
// lane order: kids in input order, then calendars in calendar order
{
  const kids = [kid({ name: "B", day: { date: TODAY, lessons: [les("08:00", "09:00")] } }), kid({ name: "A", day: { date: TODAY, lessons: [les("08:00", "09:00")] } })];
  const events = [ev(3, iso(16), iso(17), "Yoga"), ev(0, iso(11), iso(12), "Zahnarzt")];
  assert.deepEqual(lanes({ kids, calendars: cals, events }).map((l) => l.name), ["B", "A", "Familie", "Mama"]);
}

// weather slots
const H24 = Array.from({ length: 24 }, (_, h) => ({ time: iso(h), temp: 10 + h, precipProb: 0, icon: h % 2 ? "rain" : "cloud" }));
{
  const w = daybandWeather(H24, now);
  assert.deepEqual(w.map((s) => s.hour), [7, 9, 11, 13, 15, 17, 19, 21]);
  assert.deepEqual(w.map((s) => s.past), [true, true, true, true, false, false, false, false]);
  assert.deepEqual(w[0], { hour: 7, temp: 17, icon: "rain", past: true });
  assert.deepEqual(daybandWeather(H24.filter((e) => new Date(e.time).getHours() !== 9), now).map((s) => s.hour), [7, 11, 13, 15, 17, 19, 21]);
  assert.deepEqual(daybandWeather(undefined, now), []);
}
// rain bars
{
  const rain = (hs) => H24.map((e, h) => ({ ...e, precipProb: hs.includes(h) ? 50 : 49 }));
  assert.deepEqual(daybandRain(rain([16, 17, 18, 19])), [{ start: 16, end: 20 }]);
  assert.deepEqual(daybandRain(rain([4, 5, 6, 7])), [{ start: 6, end: 8 }]);
  assert.deepEqual(daybandRain(rain([8, 9, 14])), [{ start: 8, end: 10 }, { start: 14, end: 15 }]);
  assert.deepEqual(daybandRain(rain([2, 3])), []);
  assert.deepEqual(daybandRain(undefined), []);
}
// now line
{
  assert.ok(Math.abs(daybandNow(now) - (13.3 - 6) / 16) < 0.001);
  assert.equal(daybandNow(new Date(2026, 8, 30, 5, 59)), null);
  assert.equal(daybandNow(new Date(2026, 8, 30, 22, 0)), null);
  assert.equal(daybandNow(new Date(2026, 8, 30, 6, 0)), 0);
}

// labels claim space: colliding labels move to a second row
{
  const l = lanes({ calendars: cals, events: [ev(0, iso(7, 30), iso(8), "Lukas zur Schule bringen"), ev(0, iso(11), iso(12), "Zahnarzt")] })[0];
  assert.equal(l.rows, 2);
  assert.deepEqual(l.segments.map((s) => [s.row, s.side]), [[0, "right"], [1, "right"]]);
}
// short events far apart stay on one row
{
  const l = lanes({ calendars: cals, events: [ev(0, iso(8), iso(8, 30), "A"), ev(0, iso(15), iso(15, 30), "B")] })[0];
  assert.equal(l.rows, 1);
  assert.deepEqual(l.segments.map((s) => s.row), [0, 0]);
}
// three colliding segments: the third lands on row 1 with the label right, nothing throws
{
  const l = lanes({ calendars: cals, events: [ev(0, iso(10), iso(10, 30), "Zahnarzt"), ev(0, iso(10, 15), iso(10, 45), "Friseur"), ev(0, iso(10, 20), iso(10, 50), "Sport")] })[0];
  assert.equal(l.rows, 2);
  assert.deepEqual(l.segments.map((s) => s.row), [0, 1, 1]);
  assert.equal(l.segments[2].side, "right");
}
// label space stays inside 6..22 and never overlaps a neighbour in its row
{
  const l = lanes({ calendars: cals, events: [ev(0, iso(5), iso(6, 30), "Frühstück bei Oma"), ev(0, iso(7, 30), iso(8), "Lukas zur Schule bringen"), ev(0, iso(11), iso(12), "Zahnarzt"), ev(0, iso(21, 30), iso(23), "Spätschicht im Krankenhaus")] })[0];
  for (const s of l.segments) {
    const room = daybandRoom(l.segments, s);
    if (s.side === "in") assert.equal(room, null);
    else if (s.side === "right") assert.ok(room >= 0 && s.end + room <= 22);
    else assert.ok(room >= 0 && s.start - room >= 6);
  }
}
console.log("dayband.test.mjs ok");
