"use strict";

const POLL_MS = 60_000;
const NIGHT = { from: 22, to: 6 }; // dim the display between these hours

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
  $("time").textContent = fmt.time.format(now);
  $("date").textContent = fmt.longDate.format(now);
  const h = now.getHours();
  document.body.classList.toggle("night", NIGHT.from > NIGHT.to ? h >= NIGHT.from || h < NIGHT.to : h >= NIGHT.from && h < NIGHT.to);
}

// ------------------------------------------------------------------ weather

function renderWeather(w) {
  const el = $("weather");
  if (!w || !w.current || w.daily == null) {
    el.hidden = !w;
    if (w) el.innerHTML = `<h2>Wetter</h2><p class="hint">Noch keine Wetterdaten.</p>`;
    return;
  }
  el.hidden = false;
  const c = w.current;
  const hours = (w.hourly || []).filter((_, i) => i % 2 === 0).slice(0, 6);
  const days = (w.daily || []).slice(0, 5);
  const today = ymd(new Date());

  el.innerHTML = `
    <h2>${esc(w.location || "Wetter")}</h2>
    <div class="wx-now">
      ${icon(c.icon)}
      <div>
        <div class="wx-temp">${round(c.temp)}°</div>
        <div class="wx-label">${esc(c.label)}</div>
        <div class="wx-meta">gefühlt ${round(c.feels)}° · Wind ${round(c.wind)} km/h</div>
      </div>
    </div>
    <div class="wx-hours">
      ${hours.map((h) => `
        <div>
          <div class="h">${fmt.hour.format(new Date(h.time))}</div>
          ${icon(h.icon)}
          <div class="t">${round(h.temp)}°</div>
          <div class="p">${h.precipProb >= 20 ? h.precipProb + " %" : ""}</div>
        </div>`).join("")}
    </div>
    <div class="wx-days">
      ${days.map((d) => `
        <div class="wx-day">
          <span>${d.date === today ? "Heute" : fmt.wdShort.format(new Date(d.date + "T12:00"))}</span>
          ${icon(d.icon)}
          <span class="p">${d.precipProb >= 20 ? d.precipProb + " %" : ""}</span>
          <span class="range">${round(d.max)}°<span class="min">${round(d.min)}°</span></span>
        </div>`).join("")}
    </div>
    ${days[0]?.sunrise ? `<div class="wx-sun">Sonnenaufgang ${fmt.time.format(new Date(days[0].sunrise))} · Sonnenuntergang ${fmt.time.format(new Date(days[0].sunset))}</div>` : ""}`;
}

// ------------------------------------------------------------------ calendar

function eventsForDay(events, day) {
  const dayStart = day, dayEnd = addDays(day, 1), key = ymd(day);
  const allDay = [], timed = [];
  for (const e of events) {
    if (e.allDay) {
      if (e.startDate <= key && key < e.endDate) allDay.push(e);
      continue;
    }
    const s = new Date(e.start), en = new Date(e.end);
    const overlaps = s < dayEnd && (en > dayStart || (+s === +en && s >= dayStart));
    if (overlaps) timed.push({ ...e, s, en });
  }
  return { allDay, timed };
}

function dayLabel(i, day) {
  if (i === 0) return "Heute";
  if (i === 1) return "Morgen";
  return fmt.weekday.format(day);
}

function renderAgenda(data) {
  const el = $("agenda");
  const events = data.calendar?.events || [];
  const days = data.days || 7;
  const now = new Date();
  const today = startOfDay(now);

  if (!events.length && !Object.keys(data.calendar?.errors || {}).length && data.calendar?.updatedAt?.startsWith("0001")) {
    el.innerHTML = `<div class="panel"><h2>Kalender</h2><p class="hint">Kein Kalender konfiguriert. Setze <code>CALENDAR_1_URL</code> auf die „Privatadresse im iCal-Format“ aus den Google-Kalender-Einstellungen.</p></div>`;
    return;
  }

  let html = "";
  for (let i = 0; i < days; i++) {
    const day = addDays(today, i);
    const { allDay, timed } = eventsForDay(events, day);
    if (i >= 2 && !allDay.length && !timed.length) continue; // hide empty days after tomorrow

    html += `<div class="day${i === 0 ? " today" : ""}">
      <div class="day-head"><span class="day-name">${dayLabel(i, day)}</span><span class="day-date">${fmt.dayMonth.format(day)}</span></div>`;
    if (allDay.length) {
      html += `<div class="allday">${allDay.map((e) => `<span class="chip" style="--c:${esc(e.color)}">${esc(e.title)}</span>`).join("")}</div>`;
    }
    for (const e of timed) {
      const startsToday = e.s >= day;
      const endsToday = e.en <= addDays(day, 1);
      const past = +e.s === +e.en ? e.s < now : e.en <= now;
      const running = e.s <= now && now < e.en;
      const from = startsToday ? fmt.time.format(e.s) : "…";
      const to = +e.s === +e.en ? "" : endsToday ? fmt.time.format(e.en) : "…";
      html += `<div class="ev${past ? " past" : ""}${running ? " now" : ""}" style="--c:${esc(e.color)}">
        <span class="bar"></span>
        <span class="when"><b>${from}</b>${to}</span>
        <span><div class="what">${esc(e.title)}</div>
          ${e.location ? `<div class="where">${esc(e.location)}</div>` : ""}
        </span>
      </div>`;
    }
    if (!allDay.length && !timed.length) html += `<div class="day-empty">Nichts geplant</div>`;
    html += `</div>`;
  }
  el.innerHTML = html;
}

// ------------------------------------------------------------------ school (beste.schule)

const MAX_HOMEWORK = 5;
const MAX_EXAMS = 4;
const MAX_SHOPPING = 40;

function relDay(dateStr) {
  const today = startOfDay(new Date());
  const d = new Date(dateStr + "T00:00");
  const diff = Math.round((d - today) / 86_400_000);
  if (diff === 0) return "Heute";
  if (diff === 1) return "Morgen";
  if (diff > 1 && diff < 7) return fmt.weekday.format(d);
  return `${fmt.wdShort.format(d)} ${fmt.dayMonthShort.format(d)}`;
}

function shortDay(dateStr) {
  const d = new Date(dateStr + "T00:00");
  const diff = Math.round((d - startOfDay(new Date())) / 86_400_000);
  if (diff === 0) return "heute";
  if (diff === 1) return "morgen";
  return `${fmt.wdShort.format(d)} ${fmt.dayMonthShort.format(d)}`;
}

function lessonRow(l, isToday) {
  const now = new Date();
  const at = (hm) => { const [h, m] = hm.split(":").map(Number); const d = new Date(); d.setHours(h, m, 0, 0); return d; };
  const past = isToday && l.end && at(l.end) <= now;
  const running = isToday && l.start && l.end && at(l.start) <= now && now < at(l.end);
  const cls = ["lesson", l.status || "", past ? "past" : "", running ? "now" : ""].join(" ");
  const badge = l.status === "cancelled" ? `<span class="tag danger">entfällt</span>` : l.status === "substitution" ? `<span class="tag warn">Vertretung</span>` : "";
  return `<div class="${cls}">
    <span class="nr">${l.nr || ""}</span>
    <span class="t">${esc(l.start || "")}</span>
    <span class="subj"><span class="s">${esc(l.subject)}</span>${badge}${l.info ? `<span class="info">${esc(l.info)}</span>` : ""}</span>
    <span class="room">${esc(l.room || "")}</span>
  </div>`;
}

function renderSchool(school) {
  if (!school) return "";
  if (!school.students?.length) {
    return `<div class="panel"><h2>Schule</h2><p class="hint">${school.error ? esc(school.error) : "Noch keine Daten von beste.schule."}</p></div>`;
  }
  return school.students.map((st) => {
    const day = st.day || {};
    const isToday = day.date === ymd(new Date());
    const lessons = day.lessons || [];
    const exams = (st.exams || []).slice(0, MAX_EXAMS);
    const hw = (st.homework || []).slice(0, MAX_HOMEWORK);
    return `<div class="panel school">
      <div class="list-head">
        <span class="dot" style="--c:var(--accent)"></span>
        <span class="name">${esc(st.name || "Schule")}</span>
        <span class="count">${day.noSchool ? "" : esc(relDay(day.date))}</span>
      </div>
      ${(day.notices || []).map((n) => `<div class="notice">${esc(n)}</div>`).join("")}
      ${day.noSchool ? `<div class="list-empty">Keine Schule in Sicht 🎉</div>` : lessons.map((l) => lessonRow(l, isToday)).join("")}
      ${exams.length ? `<div class="sub-h">Arbeiten</div>${exams.map((e) => `
        <div class="entry exam"><span class="when">${esc(shortDay(e.date))}</span>
          <span class="what"><b>${esc(e.subject || e.kind || "Arbeit")}</b> ${esc(e.kind && e.subject ? e.kind : "")}${e.text ? ` · <span class="muted">${esc(e.text)}</span>` : ""}</span></div>`).join("")}` : ""}
      ${hw.length ? `<div class="sub-h">Hausaufgaben</div>${hw.map((e) => `
        <div class="entry"><span class="when">${esc(shortDay(e.date))}</span>
          <span class="what"><b>${esc(e.subject || "")}</b>${e.text ? ` <span class="muted">${esc(e.text)}</span>` : ""}</span></div>`).join("")}` : ""}
    </div>`;
  }).join("");
}

// ------------------------------------------------------------------ shopping (Bring!)

function renderShopping(list) {
  if (!list) return "";
  const items = list.items || [];
  const shown = items.slice(0, MAX_SHOPPING);
  return `<div class="panel shopping">
    <div class="list-head"><span class="dot" style="--c:#46C28E"></span><span class="name">${esc(list.name || "Einkauf")}</span><span class="count">${items.length || ""}</span></div>
    ${list.error && !items.length ? `<p class="hint">${esc(list.error)}</p>` : shown.length
      ? `<div class="chips">${shown.map((it) => `<span class="item">${esc(it.name)}${it.spec ? `<small>${esc(it.spec)}</small>` : ""}</span>`).join("")}</div>`
      : `<div class="list-empty">Nichts zu kaufen ✓</div>`}
    ${items.length > shown.length ? `<div class="more">+ ${items.length - shown.length} weitere</div>` : ""}
  </div>`;
}

function renderRight(d) {
  const html = renderSchool(d.school) + renderShopping(d.shopping);
  $("side").innerHTML = html || `<div class="panel"><h2>Schule & Einkauf</h2><p class="hint">Setze <code>BESTESCHULE_TOKEN</code> und/oder <code>BRING_EMAIL</code>/<code>BRING_PASSWORD</code>.</p></div>`;
}

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
    renderWeather(d.weather);
    renderAgenda(d);
    renderRight(d);
  }
  renderStatus();
}

tickClock();
setInterval(tickClock, 1000);
refresh();
setInterval(refresh, POLL_MS);
// Re-render every minute on the minute so "jetzt"/past markers stay correct.
setTimeout(() => { render(); setInterval(render, 60_000); }, (60 - new Date().getSeconds()) * 1000);
