// Pure matrix helper of the week tile. Run: node tests/week.test.mjs
import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

const src = readFileSync(new URL("../web/static/week.js", import.meta.url), "utf8");
const { weekRows } = new Function(`${src}; return { weekRows };`)();

const now = new Date(2026, 8, 30, 13, 18); // Wednesday
const p = (n) => String(n).padStart(2, "0");
const iso = (day, h, m = 0) => {
  const d = new Date(2026, 8, day, h, m);
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}:00`;
};
const calendars = [
  { id: 0, name: "Familie", color: "#2460D8" },
  { id: 1, name: "Dienst", color: "#B5651D" },
  { id: 2, name: "Mama", color: "#8A3FA0" },
  { id: 3, name: "Feiertage", color: "#999999", into: 0 },
  { id: 4, name: "Hannah", color: "#00AA55", panel: "school" },
];
const timed = (cal, start, end, title, extra = {}) => ({ cal, title, allDay: false, start, end, ...extra });
const allDay = (cal, startDate, endDate, title) => ({ cal, title, allDay: true, startDate, endDate });
const run = (events) => weekRows({ calendars, events, days: 7, now });
const row = (got, date) => got.rows.find((r) => r.date === date);

// columns: school panel and merged calendars are not columns
{
  const got = run([]);
  assert.deepEqual(got.columns, [
    { id: 0, name: "Familie", color: "#2460D8" },
    { id: 1, name: "Dienst", color: "#B5651D" },
    { id: 2, name: "Mama", color: "#8A3FA0" },
  ]);
}
// today always exists, even empty
{
  const got = run([]);
  assert.equal(got.rows.length, 1);
  assert.deepEqual(got.rows[0], {
    date: "2026-09-30", label: "Heute", sub: "Mi 30.9.", isToday: true, free: false,
    cells: [{ allDay: [], timed: [] }, { allDay: [], timed: [] }, { allDay: [], timed: [] }],
  });
}
// labels, empty later days dropped, Sunday free
{
  const got = run([
    timed(1, iso(31, 9), iso(31, 10), "Dienst Do"),
    timed(2, iso(32, 9), iso(32, 10), "Mama Fr"),
    timed(0, iso(34, 9), iso(34, 10), "Sonntag Ding"),
  ]);
  assert.deepEqual(got.rows.map((r) => r.date), ["2026-09-30", "2026-10-01", "2026-10-02", "2026-10-04"]);
  assert.equal(row(got, "2026-10-01").label, "Morgen");
  assert.equal(row(got, "2026-10-01").sub, "Do 1.10.");
  assert.equal(row(got, "2026-10-02").label, "Freitag");
  assert.equal(row(got, "2026-10-02").sub, "2.10.");
  assert.deepEqual(got.rows.map((r) => r.free), [false, false, false, true]);
  assert.equal(row(got, "2026-10-04").label, "Sonntag");
}
// merged all-day holiday lands in Familie; multi-day all-day on 5th and 6th only
{
  const got = run([
    allDay(3, "2026-10-03", "2026-10-04", "Tag der Deutschen Einheit"),
    allDay(0, "2026-10-05", "2026-10-07", "Herbstferien"),
  ]);
  assert.deepEqual(row(got, "2026-10-03").cells[0].allDay, [{ title: "Tag der Deutschen Einheit" }]);
  assert.deepEqual(row(got, "2026-10-05").cells[0].allDay, [{ title: "Herbstferien" }]);
  assert.deepEqual(row(got, "2026-10-06").cells[0].allDay, [{ title: "Herbstferien" }]);
  assert.equal(row(got, "2026-10-07"), undefined);
  assert.equal(row(got, "2026-10-03").free, false); // Saturday
}
// timed entries: past flag, "…" for carried-over, sorting, location
{
  const got = run([
    timed(0, iso(30, 18, 30), iso(30, 20), "Abend", { location: "Halle" }),
    timed(0, iso(30, 7, 30), iso(30, 8), "Früh"),
    timed(0, iso(30, 12), iso(30, 12), "Punkt"),
    timed(1, iso(29, 22), iso(30, 15), "Nachtdienst"),
  ]);
  const [fam, dienst] = row(got, "2026-09-30").cells;
  assert.deepEqual(fam.timed, [
    { time: "07:30", title: "Früh", location: "", past: true },
    { time: "12:00", title: "Punkt", location: "", past: true },
    { time: "18:30", title: "Abend", location: "Halle", past: false },
  ]);
  assert.deepEqual(dienst.timed, [{ time: "…", title: "Nachtdienst", location: "", past: false }]);
}
// school-panel events appear nowhere
{
  const got = run([timed(4, iso(31, 9), iso(31, 10), "Schule")]);
  assert.equal(got.rows.length, 1);
  assert.ok(got.rows[0].cells.every((c) => !c.timed.length && !c.allDay.length));
}
// no columns, no rows
assert.deepEqual(weekRows({ calendars: [], events: [], days: 7, now }), { columns: [], rows: [] });
console.log("week tests passed");
