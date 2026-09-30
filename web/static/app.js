"use strict";

const POLL_MS = 60_000;

const $ = (id) => document.getElementById(id);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

const fmt = {
  time: new Intl.DateTimeFormat("de-DE", { hour: "2-digit", minute: "2-digit" }),
  longDate: new Intl.DateTimeFormat("de-DE", { weekday: "long", day: "numeric", month: "long" }),
  weekday: new Intl.DateTimeFormat("de-DE", { weekday: "long" }),
  wdShort: new Intl.DateTimeFormat("de-DE", { weekday: "short" }),
  dayMonth: new Intl.DateTimeFormat("de-DE", { day: "numeric", month: "long" }),
  dayMonthShort: new Intl.DateTimeFormat("de-DE", { day: "2-digit", month: "2-digit" }),
  hour: new Intl.DateTimeFormat("de-DE", { hour: "2-digit" }),
};

let state = { data: null, version: null, lastOk: 0, error: null };

// ------------------------------------------------------------------ helpers

const startOfDay = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate());
const addDays = (d, n) => new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
const ymd = (d) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
const round = (n) => Math.round(n);

// ------------------------------------------------------------------ icons

const ICONS = (() => {
  const sun = `<g class="sun" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round"><circle cx="24" cy="24" r="8"/><path d="M24 6v4M24 38v4M6 24h4M38 24h4M11.3 11.3l2.8 2.8M33.9 33.9l2.8 2.8M11.3 36.7l2.8-2.8M33.9 14.1l2.8-2.8"/></g>`;
  const smallSun = `<g class="sun" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round"><circle cx="18" cy="17" r="6"/><path d="M18 4v3M5 17h3M8.8 7.8l2.1 2.1M27.2 7.8l-2.1 2.1"/></g>`;
  const moon = `<path fill="none" stroke="currentColor" stroke-width="3" stroke-linejoin="round" d="M30 8a15 15 0 1 0 10 24A13 13 0 0 1 30 8z"/>`;
  const smallMoon = `<path fill="none" stroke="currentColor" stroke-width="3" stroke-linejoin="round" d="M20 6a10 10 0 1 0 8 15 9 9 0 0 1-8-15z"/>`;
  const cloud = (y = 0) => `<path transform="translate(0 ${y})" fill="var(--panel)" stroke="currentColor" stroke-width="3" stroke-linejoin="round" d="M14 36h20a8 8 0 0 0 .8-16 11 11 0 0 0-21.3 2.6A6.8 6.8 0 0 0 14 36z"/>`;
  const drops = `<g class="drop" stroke="currentColor" stroke-width="3" stroke-linecap="round"><path d="M17 38l-2 5M25 38l-2 5M33 38l-2 5"/></g>`;
  const drizzle = `<g class="drop" stroke="currentColor" stroke-width="3" stroke-linecap="round"><path d="M18 39v2M26 39v2M34 39v2"/></g>`;
  const flakes = `<g class="drop" fill="currentColor"><circle cx="16" cy="41" r="2"/><circle cx="24" cy="44" r="2"/><circle cx="32" cy="41" r="2"/></g>`;
  const bolt = `<path class="sun" fill="currentColor" d="M25 32l-5 9h5l-2 7 8-10h-5l3-6z"/>`;
  const fog = `<g stroke="currentColor" stroke-width="3" stroke-linecap="round"><path d="M10 20h28M6 27h36M10 34h28"/></g>`;
  const svg = (inner) => `<svg class="icon" viewBox="0 0 48 48" aria-hidden="true">${inner}</svg>`;
  return {
    sun: svg(sun),
    moon: svg(moon),
    partly: svg(smallSun + cloud(2)),
    "partly-night": svg(smallMoon + cloud(2)),
    cloud: svg(cloud()),
    fog: svg(fog),
    drizzle: svg(cloud(-4) + drizzle),
    rain: svg(cloud(-4) + drops),
    sleet: svg(cloud(-4) + drizzle + flakes),
    snow: svg(cloud(-4) + flakes),
    thunder: svg(cloud(-6) + bolt),
  };
})();
const icon = (key) => ICONS[key] || ICONS.cloud;

// ------------------------------------------------------------------ clock

function tickClock() {
  const now = new Date();
  const t = fmt.time.format(now), d = fmt.longDate.format(now);
  $("time").textContent = t;
  $("date").textContent = d;
  $("night-time").textContent = t;
  $("night-date").textContent = d;
}

// ------------------------------------------------------------------ weather

function renderWeather(w) {
  const el = $("weather");
  if (!w) { el.innerHTML = ""; return; }
  if (!w.current || !w.daily) { el.innerHTML = `<p class="hint">Noch keine Wetterdaten.</p>`; return; }
  const c = w.current;
  const today = ymd(new Date());
  el.innerHTML = `
    <div class="wx-now">
      ${icon(c.icon)}
      <div>
        <div class="wx-temp">${round(c.temp)}°</div>
        <div class="wx-label">${esc(c.label)}</div>
        <div class="wx-meta">gefühlt ${round(c.feels)}° · Wind ${round(c.wind)} km/h</div>
      </div>
    </div>
    <div class="wx-days">
      ${w.daily.slice(0, 5).map((d) => `
        <div>
          <div class="d">${d.date === today ? "Heute" : fmt.wdShort.format(new Date(d.date + "T12:00"))}</div>
          ${icon(d.icon)}
          <div class="r">${round(d.max)}°<span>${round(d.min)}°</span></div>
          <div class="p">${d.precipProb >= 20 ? d.precipProb + " %" : ""}</div>
        </div>`).join("")}
    </div>`;
}

// ------------------------------------------------------------------ date helpers

const dayDiff = (dateStr) => Math.round((new Date(dateStr + "T00:00") - startOfDay(new Date())) / 86_400_000);

function relDay(dateStr) {
  const diff = dayDiff(dateStr), d = new Date(dateStr + "T00:00");
  if (diff === 0) return "Heute";
  if (diff === 1) return "Morgen";
  if (diff > 1 && diff < 7) return fmt.weekday.format(d);
  return `${fmt.wdShort.format(d)} ${fmt.dayMonthShort.format(d)}`;
}

function shortDay(dateStr) {
  const diff = dayDiff(dateStr), d = new Date(dateStr + "T00:00");
  if (diff === 0) return "heute";
  if (diff === 1) return "morgen";
  return `${fmt.wdShort.format(d)} ${fmt.dayMonthShort.format(d)}`;
}

// events of one calendar on one local day, split into all-day and timed
function eventsOn(events, day) {
  const dayStart = day, dayEnd = addDays(day, 1), key = ymd(day);
  const allDay = [], timed = [];
  for (const e of events) {
    if (e.allDay) {
      if (e.startDate <= key && key < e.endDate) allDay.push(e);
      continue;
    }
    const s = new Date(e.start), en = new Date(e.end);
    if (s < dayEnd && (en > dayStart || (+s === +en && s >= dayStart))) timed.push({ ...e, s, en });
  }
  return { allDay, timed };
}

// ------------------------------------------------------------------ waste collection

// Bin icons next to the clock: outline = collected tomorrow (put it out
// tonight), filled + pulsing = collected today (until WASTE_TODAY_UNTIL).
const WASTE_TODAY_UNTIL = 12; // hour; afterwards today's bins are gone
const BIN_SVG = `<svg viewBox="0 0 24 24" aria-hidden="true">
  <path class="bin-body" d="M5.5 8h13l-1.1 12a1.8 1.8 0 0 1-1.8 1.6H8.4a1.8 1.8 0 0 1-1.8-1.6z"/>
  <rect class="bin-lid" x="3.5" y="5" width="17" height="2" rx="1"/>
  <path class="bin-handle" d="M9.5 5V3.5h5V5"/></svg>`;

function renderWaste(list) {
  const now = new Date();
  const today = ymd(now), tomorrow = ymd(addDays(now, 1));
  const bins = [];
  for (const b of list || []) {
    const dates = b.dates || [];
    if (dates.includes(today) && now.getHours() < WASTE_TODAY_UNTIL) bins.push({ b, day: today, state: "today" });
    else if (dates.includes(tomorrow)) bins.push({ b, day: tomorrow, state: "tomorrow" });
  }
  const html = bins.map(({ b, day, state }) => {
    const moved = (b.shifted || []).includes(day) ? " (verlegt)" : "";
    const label = `${b.name} ${state === "today" ? "heute" : "morgen"}${moved}`;
    return `<span class="bin bin--${state}" style="--c:${esc(b.color)}" title="${esc(label)}">${BIN_SVG}</span>`;
  }).join("");
  // render() runs every minute – only touch the DOM on change, so the pulse doesn't restart
  const el = $("waste");
  if (el.dataset.html !== html) { el.innerHTML = html; el.dataset.html = html; }
}

// ------------------------------------------------------------------ shopping (Bring!)

const MAX_SHOPPING = 40;

function renderShopping(list) {
  const el = $("shopping");
  if (!list) {
    el.innerHTML = `<h2>Einkauf</h2><p class="hint">Setze <code>BRING_EMAIL</code> und <code>BRING_PASSWORD</code>.</p>`;
    return;
  }
  const items = list.items || [];
  const shown = items.slice(0, MAX_SHOPPING);
  el.className = "panel shopping fade";
  el.innerHTML = `
    <div class="head"><span class="dot" style="--c:#46C28E"></span><span class="name">${esc(list.name || "Einkauf")}</span><span class="meta">${items.length || ""}</span></div>
    ${list.error && !items.length ? `<p class="hint">${esc(list.error)}</p>` : shown.length
      ? `<div class="chips">${shown.map((it) => `<span class="item">${esc(it.name)}${it.spec ? `<small>${esc(it.spec)}</small>` : ""}</span>`).join("")}</div>`
      : `<div class="empty">Nichts zu kaufen ✓</div>`}
    ${items.length > shown.length ? `<div class="more">+ ${items.length - shown.length} weitere</div>` : ""}`;
}

// ------------------------------------------------------------------ school row

const MAX_HOMEWORK = 3;
const MAX_EXAMS = 3;
const MAX_SCHOOL_EVENTS = 8;
const MAX_CARD_EVENTS = 4; // school calendar entries under a fixed timetable

function lessonRow(l, isToday) {
  const now = new Date();
  const at = (hm) => { const [h, m] = hm.split(":").map(Number); const d = new Date(); d.setHours(h, m, 0, 0); return d; };
  const past = isToday && l.end && at(l.end) <= now;
  const running = isToday && l.start && l.end && at(l.start) <= now && now < at(l.end);
  const cls = ["lesson", l.status || "", past ? "past" : "", running ? "now" : ""].join(" ");
  const badge = (l.status === "cancelled" ? `<span class="tag danger">entfällt</span>` : l.status === "substitution" ? `<span class="tag warn">Vertretung</span>` : "")
    + (l.tag ? `<span class="tag">${esc(l.tag)}</span>` : "");
  return `<div class="${cls}">
    <span class="nr">${l.nr || ""}</span>
    <span class="t">${esc(l.start || "")}</span>
    <span class="subj"><span class="s">${esc(l.subject)}</span>${badge}${l.info ? `<span class="info">${esc(l.info)}</span>` : ""}</span>
    <span class="room">${esc(l.room || "")}</span>
  </div>`;
}

// beste.schule or fixed timetable: one card per child, top to bottom
// school (lessons, exams, homework) → Termine → Essen.
// opts.color tints the dot, opts.cal/opts.events add the child's school calendar,
// opts.meal the child's lunch (VielfaltMenü). The school part absorbs any
// overflow so Termine and Essen always stay visible at the bottom.
function studentCard(st, opts = {}) {
  const day = st.day || {};
  const isToday = day.date === ymd(new Date());
  const exams = (st.exams || []).slice(0, MAX_EXAMS);
  const hw = (st.homework || []).slice(0, MAX_HOMEWORK);
  return `<div class="panel kid">
    <div class="head"><span class="dot" style="--c:${opts.color ? esc(opts.color) : "var(--accent)"}"></span><span class="name">${esc(st.name || "Schule")}</span>
      <span class="meta">${day.noSchool ? "" : esc(relDay(day.date))}</span></div>
    <div class="kid-main">
    ${(day.notices || []).map((n) => `<div class="notice">${esc(n)}</div>`).join("")}
    ${day.noSchool ? `<div class="empty">Keine Schule in Sicht 🎉</div>` : (day.lessons || []).map((l) => lessonRow(l, isToday)).join("")}
    ${exams.length ? `<div class="sub-h">Arbeiten</div>${exams.map((e) => `
      <div class="entry exam"><span class="when">${esc(shortDay(e.date))}</span>
        <span class="what"><b>${esc(e.subject || e.kind || "Arbeit")}</b> ${esc(e.kind && e.subject ? e.kind : "")}${e.text ? ` · <span class="muted">${esc(e.text)}</span>` : ""}</span></div>`).join("")}` : ""}
    ${hw.length ? `<div class="sub-h">Hausaufgaben</div>${hw.map((e) => `
      <div class="entry"><span class="when">${esc(shortDay(e.date))}</span>
        <span class="what"><b>${esc(e.subject || "")}</b>${e.text ? ` <span class="muted">${esc(e.text)}</span>` : ""}</span></div>`).join("")}` : ""}
    </div>
    ${opts.cal ? `<div class="kid-sec"><div class="sub-h">Termine</div>${calendarRows(opts.events || [], MAX_CARD_EVENTS) || `<div class="empty">Nichts eingetragen</div>`}</div>` : ""}
    ${opts.meal ? `<div class="kid-sec"><div class="sub-h">Essen</div>${mealRows(opts.meal)}</div>` : ""}
  </div>`;
}

// "Lukas" matches "Lukas", "lukas" and "Meister Lukas"; two multi-word names must be equal
function sameKid(a, b) {
  const wa = String(a || "").trim().toLowerCase().split(/\s+/).filter(Boolean);
  const wb = String(b || "").trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (!wa.length || !wb.length) return false;
  if (wa.join(" ") === wb.join(" ")) return true;
  const [short, long] = wa.length <= wb.length ? [wa, wb] : [wb, wa];
  return short.length === 1 && long.includes(short[0]);
}

// upcoming entries of a school calendar as rows
function calendarRows(events, max) {
  const now = new Date(), today = startOfDay(now);
  const upcoming = events
    .filter((e) => (e.allDay ? e.endDate > ymd(today) : new Date(e.end) > now))
    .slice(0, max);
  return upcoming.map((e) => {
    const date = e.allDay ? (e.startDate < ymd(today) ? ymd(today) : e.startDate) : ymd(new Date(e.start));
    const diff = dayDiff(date);
    const when = shortDay(date) + (e.allDay ? "" : " " + fmt.time.format(new Date(e.start)));
    const cls = diff === 0 ? "today" : diff === 1 ? "soon" : "";
    return `<div class="entry wide ${cls}"><span class="when">${esc(when)}</span>
      <span class="what">${esc(e.title)}${e.location ? ` <span class="loc">· ${esc(e.location)}</span>` : ""}</span></div>`;
  }).join("");
}

// a calendar with CALENDAR_n_PANEL=school: upcoming entries as a list
function schoolCalendarCard(cal, events) {
  const rows = calendarRows(events, MAX_SCHOOL_EVENTS);
  return `<div class="panel fade" style="--c:${esc(cal.color)}">
    <div class="head"><span class="dot"></span><span class="name">${esc(cal.name)}</span><span class="meta">nächste 3 Wochen</span></div>
    ${rows || `<div class="empty">Nichts eingetragen</div>`}
  </div>`;
}

// One card per child. A school calendar (CALENDAR_n_PANEL=school) and a lunch
// account (VIELFALT_n_*) with the child's name move into that child's card;
// a timetable's "calendar" field picks the calendar explicitly.
// Returns the lunch accounts that found no card.
function renderSchoolRow(d) {
  const cals = d.calendar?.calendars || [];
  const events = d.calendar?.events || [];
  const meals = d.meals?.children || [];
  const usedCals = new Set(), usedMeals = new Set();

  const extras = (name, calName) => {
    const opts = {};
    const cal = cals.find((c) => c.panel === "school" && !usedCals.has(c.id) &&
      (calName ? c.name.toLowerCase() === calName.toLowerCase() : sameKid(c.name, name)));
    if (cal) {
      usedCals.add(cal.id);
      Object.assign(opts, { color: cal.color, cal, events: events.filter((e) => e.cal === cal.id) });
    }
    const mi = meals.findIndex((m, i) => !usedMeals.has(i) && sameKid(m.name, name));
    if (mi >= 0) {
      usedMeals.add(mi);
      opts.meal = meals[mi];
      opts.color ||= meals[mi].color;
    }
    return opts;
  };

  let html = "";
  if (d.school) {
    html += d.school.students?.length
      ? d.school.students.map((st) => studentCard(st, extras(st.name))).join("")
      : `<div class="panel"><h2>Schule</h2><p class="hint">${esc(d.school.error || "Noch keine Daten von beste.schule.")}</p></div>`;
  }
  for (const t of d.timetables || []) html += studentCard(t, extras(t.name, t.calendar));
  for (const cal of cals.filter((c) => c.panel === "school" && !usedCals.has(c.id))) {
    html += schoolCalendarCard(cal, events.filter((e) => e.cal === cal.id));
  }
  $("school").innerHTML = html;
  $("board").classList.toggle("no-school", !html);
  return meals.filter((_, i) => !usedMeals.has(i));
}

// ------------------------------------------------------------------ school lunch (VielfaltMenü)

const MEAL_DAYS = 2;     // delivery days shown per child
const LUNCH_OVER = 14;   // from this hour on, today's lunch is done → start with the next day

function mealRow(day) {
  const isToday = day.date === ymd(new Date());
  const ordered = day.ordered || [];
  let what;
  if (ordered.length) {
    what = ordered.map((m) => `<span class="dish">${esc(m.name)}</span>${m.side ? `<span class="side">${esc(m.side)}</span>` : ""}`).join("");
  } else if (isToday) {
    what = `<span class="dish none">nichts bestellt</span>`;
  } else {
    what = `<span class="dish">Noch nichts bestellt!</span>`;
  }
  const cls = ["meal", isToday ? "today" : "", !ordered.length && !isToday ? "missing" : ""].join(" ");
  return `<div class="${cls}"><span class="when">${esc(relDay(day.date))}</span><span class="what">${what}</span></div>`;
}

function mealRows(ch) {
  const now = new Date(), today = ymd(now);
  const days = (ch.days || [])
    .filter((d) => d.date > today || (d.date === today && now.getHours() < LUNCH_OVER))
    .slice(0, MEAL_DAYS);
  return days.length
    ? days.map(mealRow).join("")
    : ch.error ? `<p class="hint">${esc(ch.error)}</p>` : `<div class="empty">Kein Essen in Sicht</div>`;
}

// lunch of a child without a school card: a card of its own
function mealCard(ch) {
  return `<div class="panel" style="--c:${esc(ch.color || "var(--accent)")}">
    <div class="head"><span class="dot"></span><span class="name">${esc(ch.name)}</span><span class="meta">Mittagessen</span></div>
    ${mealRows(ch)}
  </div>`;
}

function renderMeals(kids) {
  $("meals").innerHTML = kids.map(mealCard).join("");
  $("board").classList.toggle("no-meals", !kids.length);
}

// ------------------------------------------------------------------ calendar columns

// a column shows one calendar plus any calendars merged into it via
// CALENDAR_n_COLUMN; entries of merged calendars keep their own color
function calendarColumn(cal, events, days, merged = []) {
  const tint = (e) => (e.cal === cal.id ? "" : ` style="--c:${esc(e.color)}"`);
  const now = new Date(), today = startOfDay(now);
  let html = "";
  for (let i = 0; i < days; i++) {
    const day = addDays(today, i);
    const { allDay, timed } = eventsOn(events, day);
    if (i > 0 && !allDay.length && !timed.length) continue;
    const label = i === 0 ? "Heute" : i === 1 ? "Morgen" : fmt.weekday.format(day);
    html += `<div class="cday${i === 0 ? " today" : ""}">
      <div class="cday-h">${label}<span>${fmt.dayMonthShort.format(day)}</span></div>`;
    if (allDay.length) html += `<div class="allday">${allDay.map((e) => `<span class="chip"${tint(e)}>${esc(e.title)}</span>`).join("")}</div>`;
    for (const e of timed) {
      const past = +e.s === +e.en ? e.s < now : e.en <= now;
      const running = e.s <= now && now < e.en;
      const when = e.s >= day ? fmt.time.format(e.s) : "…";
      html += `<div class="ev${past ? " past" : ""}${running ? " now" : ""}${e.cal === cal.id ? "" : " other"}"${tint(e)}>
        <span class="when">${when}</span>
        <span><div class="what">${esc(e.title)}</div>${e.location ? `<div class="where">${esc(e.location)}</div>` : ""}</span>
      </div>`;
    }
    if (i === 0 && !allDay.length && !timed.length) html += `<div class="empty">Nichts geplant</div>`;
    html += `</div>`;
  }
  return `<div class="calcol" style="--c:${esc(cal.color)}">
    <div class="head"><span class="dot"></span><span class="name">${esc(cal.name)}</span>${merged.map((m) =>
      `<span class="also" style="--c:${esc(m.color)}"><span class="dot"></span>${esc(m.name)}</span>`).join("")}</div>
    ${html}
  </div>`;
}

function renderCalendars(d) {
  const all = d.calendar?.calendars || [];
  const cols = all.filter((c) => c.panel !== "school" && !(c.into >= 0));
  const events = d.calendar?.events || [];
  $("calendars").innerHTML = cols.length
    ? cols.map((c) => {
        const merged = all.filter((m) => m.into === c.id);
        const ids = new Set([c.id, ...merged.map((m) => m.id)]);
        return calendarColumn(c, events.filter((e) => ids.has(e.cal)), d.days || 7, merged);
      }).join("")
    : `<div class="calcol"><h2>Kalender</h2><p class="hint">Setze <code>CALENDAR_1_URL</code> auf die „Privatadresse im iCal-Format“ aus Google Kalender.</p></div>`;
}

// ------------------------------------------------------------------ scenes (time of day)

// The server decides the scene from SCENE_* (school days vs. weekend) and
// sends it with every poll. ?scene=evening in the URL overrides it – handy
// for testing on the laptop. Each scene lists the widgets of the focus zone
// (between clock and school row); the board's rows resize via CSS classes.
const SCENES = {
  morning:   { label: "Morgen",     focus: (sc) => (sc.schoolDay ? ["hints", "checklist", "todos"] : ["hints", "todos", "photos"]) },
  day:       { label: "Tag",        focus: () => ["todos", "photos"] },
  afternoon: { label: "Nachmittag", focus: () => ["todos", "photos"] },
  evening:   { label: "Abend",      focus: () => ["tomorrow", "todos", "photos"] },
  night:     { label: "Nacht",      focus: () => [] },
};

const urlScene = new URLSearchParams(location.search).get("scene");

function currentScene(d) {
  const sc = { name: "day", schoolDay: true, ...(d?.scene || {}) };
  if (urlScene && SCENES[urlScene]) Object.assign(sc, { name: urlScene, forced: true });
  if (!SCENES[sc.name]) sc.name = "day";
  return sc;
}

function applyScene(sc) {
  const board = $("board");
  for (const n of Object.keys(SCENES)) {
    board.classList.toggle("scene-" + n, n === sc.name);
    document.body.classList.toggle("scene-" + n, n === sc.name);
  }
}

// focus widgets: each returns HTML, or "" when it has nothing to say
const FOCUS_WIDGETS = {
  hints: (d) => hintsCard("Heute", weatherHints(d.weather, ymd(new Date()), 7, 16)),
  checklist: () => "", // TODO: morning checklist per child (CHECKLIST_n_*), + "Sportbeutel" from the timetable
  tomorrow: (d) => tomorrowCard(d),
  todos: (d) => todosCard(d.todos),
};

function renderFocus(d, sc) {
  const wanted = SCENES[sc.name].focus(sc);
  const shown = wanted.filter((w) => FOCUS_WIDGETS[w]).map((w) => FOCUS_WIDGETS[w](d)).filter(Boolean);
  const cards = shown.join("");
  // two cards need the full width – the photo only joins a single card
  const photos = wanted.includes("photos") && slideshow.has() && shown.length < 2;
  $("focus-cards").innerHTML = cards;
  $("focus").classList.toggle("no-photo", !photos);
  $("board").classList.toggle("no-focus", !cards && !photos);
  photos ? slideshow.start() : slideshow.stop();
}

// ------------------------------------------------------------------ weather hints

// plain-language hints for the time kids are out (hourly forecast when it
// covers the window, otherwise the daily forecast)
function weatherHints(w, dateStr, fromH, toH) {
  if (!w?.daily) return [];
  const hours = (w.hourly || []).filter((h) => {
    const t = new Date(h.time);
    return ymd(t) === dateStr && t.getHours() >= fromH && t.getHours() < toH;
  });
  const day = w.daily.find((x) => x.date === dateStr);
  if (!hours.length && !day) return [];
  const rain = hours.length ? Math.max(...hours.map((h) => h.precipProb)) : day.precipProb;
  const tmin = hours.length ? Math.min(...hours.map((h) => h.temp)) : day.min;
  const tmax = hours.length ? Math.max(...hours.map((h) => h.temp)) : day.max;
  const icons = hours.length ? hours.map((h) => h.icon) : [day.icon];

  const out = [];
  if (icons.some((i) => i === "snow" || i === "sleet")) out.push(["⛄", "Winterstiefel an", "Schnee möglich"]);
  if (rain >= 50) out.push(["☔", "Regenjacke mitnehmen", `bis ${rain} % Regen`]);
  else if (rain >= 30) out.push(["🌂", "Schirm einpacken", `${rain} % Regen`]);
  if (tmin <= 2) out.push(["🧤", "Mütze & Handschuhe", `nur ${round(tmin)}°`]);
  else if (tmin <= 10) out.push(["🧥", "Warme Jacke", `${round(tmin)}°`]);
  if (tmax >= 26) out.push(["🧴", "Sonnencreme & Trinkflasche", `bis ${round(tmax)}°`]);
  if (!out.length) out.push(["👍", "Wetter passt", `${round(tmin)}–${round(tmax)}°`]);
  return out;
}

const hintRows = (hints) => hints.map(([ic, what, why]) =>
  `<div class="hint-row"><span class="ic">${ic}</span><span class="what">${esc(what)}</span><span class="why">${esc(why)}</span></div>`).join("");

function hintsCard(title, hints) {
  if (!hints.length) return "";
  return `<div class="panel focus-card">
    <div class="head"><span class="dot" style="--c:var(--sun)"></span><span class="name">${esc(title)}</span><span class="meta">Wetter</span></div>
    ${hintRows(hints)}
  </div>`;
}

// ------------------------------------------------------------------ evening: "Morgen" card

const MAX_TOMORROW = 5;

function tomorrowCard(d) {
  const tomorrow = addDays(new Date(), 1), key = ymd(tomorrow);
  const bins = (d.waste || []).filter((b) => (b.dates || []).includes(key));
  const { allDay, timed } = eventsOn(d.calendar?.events || [], tomorrow);
  timed.sort((a, b) => a.s - b.s);
  const hints = weatherHints(d.weather, key, 7, 16).filter(([ic]) => ic !== "👍");

  const rows = [
    ...bins.map((b) => `<div class="tm-row bin-row" style="--c:${esc(b.color)}"><span class="dot"></span><b>${esc(b.name)}</b> rausstellen</div>`),
    ...(allDay.length ? [`<div class="allday">${allDay.map((e) => `<span class="chip" style="--c:${esc(e.color)}">${esc(e.title)}</span>`).join("")}</div>`] : []),
    ...timed.slice(0, MAX_TOMORROW).map((e) => `<div class="ev" style="--c:${esc(e.color)}">
        <span class="when">${e.s >= tomorrow ? fmt.time.format(e.s) : "…"}</span>
        <span class="what"><span class="cdot"></span>${esc(e.title)}</span></div>`),
    ...(timed.length > MAX_TOMORROW ? [`<div class="more">+ ${timed.length - MAX_TOMORROW} weitere</div>`] : []),
    hintRows(hints),
  ].filter(Boolean);

  return `<div class="panel focus-card fade">
    <div class="head"><span class="dot" style="--c:var(--accent)"></span><span class="name">Morgen</span>
      <span class="meta">${esc(fmt.wdShort.format(tomorrow) + " " + fmt.dayMonthShort.format(tomorrow))}</span></div>
    ${rows.length ? rows.join("") : `<div class="empty">Nichts Besonderes – ruhiger Tag 🙂</div>`}
  </div>`;
}

// ------------------------------------------------------------------ to-dos (Things 3)

// Today's to-dos of one Things area (THINGS_AREA). Open ones in Things' order
// ("This Evening" last, with a moon), then what was ticked off today, struck
// through. The card disappears when there's nothing open and nothing done.
const MAX_TODOS = 8;
const TODO_DONE_SHOWN = 3; // ticked-off tasks listed below the open ones

const CHECK_SVG = `<svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="10" cy="10" r="8.2"/><path class="tick" d="M6.2 10.4l2.5 2.5 5.1-5.6"/></svg>`;

function todoRow(t) {
  const diff = t.deadline ? dayDiff(t.deadline) : null;
  const due = diff === null ? "" : diff < 0 ? `<span class="tag danger">überfällig</span>`
    : diff === 0 ? `<span class="tag danger">heute fällig</span>`
    : diff === 1 ? `<span class="tag warn">morgen fällig</span>` : "";
  const meta = [
    t.project ? esc(t.project) : "",
    t.checklist ? `${t.checklist.done}/${t.checklist.total} erledigt` : "",
  ].filter(Boolean).join(" · ");
  return `<div class="todo${t.done ? " done" : ""}">
    <span class="box">${CHECK_SVG}</span>
    <span class="what"><span class="t">${esc(t.title)}</span>${t.done ? "" : due}${t.evening && !t.done ? `<span class="eve" title="Heute Abend">🌙</span>` : ""}${meta ? `<span class="sub">${meta}</span>` : ""}</span>
  </div>`;
}

function todosCard(list) {
  if (!list) return "";
  const tasks = list.tasks || [];
  const open = tasks.filter((t) => !t.done), done = tasks.filter((t) => t.done);
  if (!open.length && !done.length) return list.error ? `<div class="panel focus-card todos"><div class="head"><span class="dot" style="--c:var(--good)"></span><span class="name">To-dos</span></div><p class="hint">${esc(list.error)}</p></div>` : "";
  const shownOpen = open.slice(0, MAX_TODOS);
  const shownDone = done.slice(0, Math.max(0, Math.min(TODO_DONE_SHOWN, MAX_TODOS - shownOpen.length)));
  const hidden = open.length - shownOpen.length;
  return `<div class="panel focus-card todos fade">
    <div class="head"><span class="dot" style="--c:var(--good)"></span><span class="name">To-dos</span>
      <span class="meta">${open.length ? `${open.length} offen` : "alles erledigt 🎉"}</span></div>
    ${shownOpen.map(todoRow).join("")}
    ${hidden > 0 ? `<div class="more">+ ${hidden} weitere</div>` : ""}
    ${shownDone.map(todoRow).join("")}
  </div>`;
}

// ------------------------------------------------------------------ photo slideshow

// Two stacked <img>s: the next photo loads into the hidden one and fades in
// once it's decoded, so there's never a half-loaded frame on the wall.
const slideshow = {
  list: [], order: [], pos: 0, key: "", interval: 45, shuffle: true, timer: null, front: 0,

  set(snap) {
    const items = snap?.items || [];
    const key = items.map((i) => i.path + "@" + i.v).join("|");
    const interval = Math.max(5, snap?.interval || 45);
    if (key === this.key && interval === this.interval) return;
    const restart = this.timer && (interval !== this.interval || !items.length);
    Object.assign(this, { list: items, key, interval, shuffle: snap?.shuffle !== false, order: [], pos: 0 });
    if (restart) { this.stop(); if (items.length) this.start(); }
  },
  has() { return this.list.length > 0; },

  start() {
    if (this.timer || !this.list.length) return;
    this.next();
    this.timer = setInterval(() => this.next(), this.interval * 1000);
  },
  stop() {
    clearInterval(this.timer);
    this.timer = null;
  },

  pick() {
    if (this.pos >= this.order.length) {
      const last = this.order[this.order.length - 1];
      this.order = this.list.map((_, i) => i);
      if (this.shuffle) {
        for (let i = this.order.length - 1; i > 0; i--) {
          const j = Math.floor(Math.random() * (i + 1));
          [this.order[i], this.order[j]] = [this.order[j], this.order[i]];
        }
        if (this.order.length > 1 && this.order[0] === last) this.order.push(this.order.shift()); // no repeat across rounds
      }
      this.pos = 0;
    }
    return this.list[this.order[this.pos++]];
  },

  next() {
    const p = this.pick();
    if (!p) return;
    const imgs = $("photo-frame").querySelectorAll("img");
    const back = imgs[1 - this.front];
    const url = "photos/" + p.path.split("/").map(encodeURIComponent).join("/") + "?v=" + p.v;
    back.onload = () => {
      back.classList.add("show");
      imgs[this.front].classList.remove("show");
      this.front = 1 - this.front;
    };
    back.onerror = () => console.warn("photo failed", p.path);
    back.src = url;
  },
};

// ------------------------------------------------------------------ status

function renderStatus() {
  const d = state.data;
  const parts = [];
  if (state.error) parts.push(`<span class="off">● Server nicht erreichbar</span>`);
  if (d) {
    for (const [name, err] of Object.entries(d.calendar?.errors || {})) parts.push(`<span class="err">${esc(name)}: ${esc(err)}</span>`);
    if (d.weather?.error) parts.push(`<span class="err">Wetter: ${esc(d.weather.error)}</span>`);
    if (d.school?.error && d.school.students?.length) parts.push(`<span class="err">Schule: ${esc(d.school.error)}</span>`);
    if (d.shopping?.error && d.shopping.items?.length) parts.push(`<span class="err">Bring!: ${esc(d.shopping.error)}</span>`);
    for (const c of d.meals?.children || []) if (c.error && c.days?.length) parts.push(`<span class="err">Essen ${esc(c.name)}: ${esc(c.error)}</span>`);
    if (d.todos?.error && d.todos.tasks?.length) parts.push(`<span class="err">Things: ${esc(d.todos.error)}</span>`);
  }
  if (d) {
    const sc = currentScene(d);
    if (sc.forced) parts.push(`<span class="err">Szene fest: ${esc(SCENES[sc.name].label)}</span>`);
  }
  if (state.lastOk) parts.push(`<span>aktualisiert ${fmt.time.format(new Date(state.lastOk))}</span>`);
  $("status").innerHTML = parts.join("");
}

// ------------------------------------------------------------------ loop

async function refresh() {
  try {
    const res = await fetch("api/dashboard", { cache: "no-store" });
    if (!res.ok) throw new Error("HTTP " + res.status);
    const data = await res.json();
    if (state.version && data.version !== state.version) {
      location.reload(); // new container version → load new frontend
      return;
    }
    state = { data, version: data.version, lastOk: Date.now(), error: null };
  } catch (e) {
    state.error = e;
  }
  render();
}

function render() {
  const d = state.data;
  if (d) {
    const sc = currentScene(d);
    applyScene(sc);
    slideshow.set(d.photos);
    renderFocus(d, sc);
    renderWeather(d.weather);
    renderWaste(d.waste);
    renderShopping(d.shopping);
    renderMeals(renderSchoolRow(d));
    renderCalendars(d);
  }
  renderStatus();
}

tickClock();
setInterval(tickClock, 1000);
refresh();
setInterval(refresh, POLL_MS);
// Re-render every minute on the minute so "jetzt"/past markers stay correct.
setTimeout(() => { render(); setInterval(render, 60_000); }, (60 - new Date().getSeconds()) * 1000);
