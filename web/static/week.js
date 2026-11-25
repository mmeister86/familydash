// Pure matrix helper of the week tile: no DOM, no side effects.
// Tested via tests/week.test.mjs (`pnpm test`); loaded before app.js.

const WEEK_DAYS = ["Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"];
const WEEK_DAYS_SHORT = ["So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"];

function weekYmd(d) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function weekHM(d) {
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

// Rows are days, columns are the calendar columns (same rule as the
// calendars tile: not a school-panel calendar, not merged into another).
// Today always has a row, later days only when some column has an entry.
function weekRows({ calendars, events, days, now }) {
  const columns = (calendars || []).filter((c) => c.panel !== "school" && !(c.into >= 0));
  if (!columns.length) return { columns: [], rows: [] };
  // calendar id -> column index; merged calendars feed their target column
  const colOf = new Map();
  columns.forEach((c, i) => colOf.set(c.id, i));
  for (const c of calendars) if (c.into >= 0 && colOf.has(c.into)) colOf.set(c.id, colOf.get(c.into));

  const rows = [];
  for (let i = 0; i < days; i++) {
    const day = new Date(now.getFullYear(), now.getMonth(), now.getDate() + i);
    const next = new Date(now.getFullYear(), now.getMonth(), now.getDate() + i + 1);
    const key = weekYmd(day);
    const cells = columns.map(() => ({ allDay: [], timed: [] }));
    const starts = columns.map(() => []);
    for (const e of events || []) {
      const col = colOf.get(e.cal);
      if (col === undefined) continue;
      if (e.allDay) {
        if (e.startDate <= key && key < e.endDate) cells[col].allDay.push({ title: e.title });
        continue;
      }
      const s = new Date(e.start), en = new Date(e.end);
      if (!(s < next && (en > day || (+s === +en && s >= day)))) continue;
      const past = +s === +en ? s < now : en <= now;
      starts[col].push({ s, entry: { time: s >= day ? weekHM(s) : "…", title: e.title, location: e.location || "", past } });
    }
    starts.forEach((list, c) => { cells[c].timed = list.sort((a, b) => a.s - b.s).map((x) => x.entry); });
    const has = cells.some((c) => c.allDay.length || c.timed.length);
    if (i > 0 && !has) continue;
    const dow = day.getDay();
    const short = `${day.getDate()}.${day.getMonth() + 1}.`;
    rows.push({
      date: key,
      label: i === 0 ? "Heute" : i === 1 ? "Morgen" : WEEK_DAYS[dow],
      sub: i <= 1 ? `${WEEK_DAYS_SHORT[dow]} ${short}` : short,
      isToday: i === 0,
      free: dow === 0,
      cells,
    });
  }
  return { columns: columns.map(({ id, name, color }) => ({ id, name, color })), rows };
}
