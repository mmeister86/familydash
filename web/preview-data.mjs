// Demo payload for the preview harness: mirrors GET /api/dashboard
// (internal/server/server.go) with the content of the redesign mockup.
// All dates are relative to the `now` passed in (today = its local date).

const widget = (id, type, x, y, w, h, props, fallback) => ({
  id, type, x, y, w, h, ...(props ? { props } : {}), ...(fallback ? { fallback } : {}),
});

// Layout scenes copied from internal/scenes/defaults.go (DefaultFeed).
export const demoScenes = {
  morning: [
    widget("clock", "clock", 0, 0, 12, 5, { wide: true, hints: true }),
    widget("school", "school", 0, 5, 6, 11, { mode: "plan-meal", mealDay: "today" }),
    widget("briefing", "briefing", 6, 5, 6, 6, { kind: "today" }, "shopping"),
    widget("news", "news", 6, 11, 6, 5),
    widget("calendars", "calendars", 0, 16, 12, 8),
  ],
  day: [
    widget("clock", "clock", 0, 0, 6, 5, { wide: false, hints: false }),
    widget("shopping", "shopping", 6, 0, 6, 5),
    widget("photos", "photos", 0, 5, 6, 10),
    widget("news", "news", 6, 5, 6, 10),
    widget("calendars", "calendars", 0, 15, 12, 9),
  ],
  afternoon: [
    widget("clock", "clock", 0, 0, 12, 3, { wide: true, hints: false }),
    widget("dayband", "dayband", 0, 3, 12, 4),
    widget("school", "school", 0, 7, 12, 8, { mode: "full", mealDay: "today" }),
    widget("week", "week", 0, 15, 12, 7),
    widget("shopping", "shopping", 0, 22, 12, 2),
  ],
  evening: [
    widget("clock", "clock", 0, 0, 12, 5, { wide: true, hints: false }),
    widget("briefing", "briefing", 0, 5, 6, 6, { kind: "tomorrow" }),
    widget("school", "school", 6, 5, 6, 11, { mode: "plan-meal", mealDay: "tomorrow" }),
    widget("photos", "photos", 0, 11, 6, 5),
    widget("calendars", "calendars", 0, 16, 12, 8),
  ],
};

const SCENE_META = {
  morning: ["Morgen", "layout", "06:45", "09:00"],
  day: ["Tag", "layout", "09:00", "14:00"],
  afternoon: ["Nachmittag", "layout", "14:00", "19:00"],
  evening: ["Abend", "layout", "19:00", "21:30"],
  night: ["Nacht", "kind-night", "21:30", "06:45"],
};

const pad = (n) => String(n).padStart(2, "0");
const ymdOf = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

// Local RFC 3339 timestamp with offset, like Go's time.Time JSON.
function iso(d) {
  const off = -d.getTimezoneOffset();
  const sign = off < 0 ? "-" : "+";
  const abs = Math.abs(off);
  return `${ymdOf(d)}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}${sign}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`;
}

export function demoDashboard(now, opts = {}) {
  const sceneId = opts.scene && (demoScenes[opts.scene] || opts.scene === "night") ? opts.scene : "afternoon";
  // day(n, "HH:MM") = local date n days from today at that time (noon when omitted)
  const day = (n, hm = "12:00") => {
    const [h, m] = hm.split(":").map(Number);
    return new Date(now.getFullYear(), now.getMonth(), now.getDate() + n, h, m, 0);
  };
  const ymd = (n) => ymdOf(day(n));

  // --- calendar: Familie / Dienst / Mama in columns, Hannah as school calendar
  const calendars = [
    { id: 0, name: "Familie", color: "#2563eb", panel: "column", into: -1 },
    { id: 1, name: "Dienst", color: "#7A4BD6", panel: "column", into: -1 },
    { id: 2, name: "Mama", color: "#D43F7A", panel: "column", into: -1 },
    { id: 3, name: "Hannah", color: "#F2B705", panel: "school", into: -1 },
  ];
  const timed = (cal, n, from, to, title, location) => ({
    cal, calendar: calendars[cal].name, color: calendars[cal].color, title,
    ...(location ? { location } : {}),
    start: iso(day(n, from)), end: iso(day(n, to)), allDay: false,
  });
  const allDay = (cal, n, nights, title) => ({
    cal, calendar: calendars[cal].name, color: calendars[cal].color, title,
    start: iso(day(n, "00:00")), end: iso(day(n + nights, "00:00")), allDay: true,
    startDate: ymd(n), endDate: ymd(n + nights),
  });
  const events = [
    timed(0, 0, "07:30", "08:00", "Lukas zur Schule bringen"),
    timed(0, 0, "11:00", "12:00", "Zahnarzt"),
    timed(0, 0, "18:30", "20:30", "Elternabend Klasse 3b", "Grundschule"),
    timed(2, 0, "16:00", "17:30", "Yoga", "Studio am Markt"),
    timed(0, 1, "17:00", "18:30", "Schwimmtraining", "Stadtbad"),
    timed(1, 1, "09:00", "10:00", "Referatsbesprechung"),
    timed(2, 1, "19:00", "22:00", "Mädelsabend"),
    timed(1, 2, "14:00", "15:00", "Rückfahrt Crimmitschau"),
    allDay(0, 3, 1, "Tag der Deutschen Einheit"),
    allDay(0, 5, 5, "Herbstferien"),
    timed(2, 5, "08:00", "09:00", "Kundentermin"),
    timed(0, 6, "17:00", "18:30", "Schwimmtraining", "Stadtbad"),
    timed(3, 0, "15:00", "15:30", "Hort-Ausflug: Abholung am Hort"),
    allDay(3, 1, 1, "Sportsachen mitbringen"),
    allDay(3, 7, 1, "Diktat"),
    allDay(3, 14, 1, "Wandertag Klasse 2"),
    timed(3, 16, "17:00", "18:30", "Elternabend 2b", "Grundschule"),
  ];

  // --- weather: 12 hours from now, 5 days
  const temps = [14, 15, 16, 16, 15, 13, 12, 11, 10, 9, 9, 8];
  const hourStart = day(0, `${pad(now.getHours())}:00`);
  const hourly = temps.map((temp, i) => {
    const t = new Date(hourStart.getTime() + i * 3600e3);
    const raining = t.getHours() >= 16 && t.getHours() <= 19;
    return { time: iso(t), temp, precipProb: raining ? 70 : 10, icon: raining ? "rain" : "partly" };
  });
  // all 24 hours of today: 8 deg at 07:00 up to 16 deg at 15:00, down to 9 deg at 21:00
  const tempAt = (h) => h <= 7 ? 8 : h <= 15 ? 8 + (h - 7) : h <= 21 ? 16 - Math.round(((h - 15) * 7) / 6) : 9;
  const today = Array.from({ length: 24 }, (_, h) => {
    const raining = h >= 16 && h <= 19;
    return {
      time: iso(day(0, `${pad(h)}:00`)), temp: tempAt(h), precipProb: raining ? 60 : 10,
      icon: raining ? "rain" : h >= 7 && h <= 15 ? "partly" : "cloud",
    };
  });
  const dailyRaw = [
    [16, 8, 70, 61, "Leichter Regen", "rain"],
    [13, 7, 40, 3, "Bedeckt", "cloud"],
    [15, 6, 10, 2, "Teilweise bewölkt", "partly"],
    [17, 7, 0, 0, "Sonnig", "sun"],
    [14, 9, 60, 63, "Regen", "rain"],
  ];
  const daily = dailyRaw.map(([max, min, precipProb, code, label, icon], i) => ({
    date: ymd(i), max, min, precipProb, code, label, icon,
    sunrise: iso(day(i, "06:58")), sunset: iso(day(i, "18:42")),
  }));
  const weather = {
    location: "Chemnitz",
    current: { temp: 14, feels: 12, wind: 11, code: 2, label: "Teilweise bewölkt", icon: "partly", isDay: true },
    hourly, today, daily, updatedAt: iso(now),
  };

  // --- shopping
  const items = [
    ["Milch", "2 l"], ["Brot"], ["Hähnchen", "500 g"], ["Äpfel"], ["Hundefutter", "trocken"],
    ["Kaffee"], ["Eier", "10"], ["Käse"], ["Tomaten"], ["Nudeln"],
  ].map(([name, spec]) => ({ name, ...(spec ? { spec } : {}) }));

  // --- school: Lukas (beste.schule shape)
  const lesson = (nr, start, end, subject, room, extra = {}) => ({ nr, start, end, subject, ...(room ? { room } : {}), ...extra });
  const lessons = [
    lesson(1, "07:30", "08:15", "Mathematik", "R12"),
    lesson(2, "08:25", "09:10", "Deutsch", "", { status: "cancelled" }),
    lesson(3, "09:30", "10:15", "Englisch", "R08"),
    lesson(4, "10:25", "11:10", "Mathematik", "R07", { status: "substitution", info: "statt Sport" }),
    lesson(5, "11:30", "12:15", "Biologie", "BIO"),
    lesson(6, "12:25", "13:10", "Geschichte", "R21", { status: "substitution", info: "Vertretung" }),
    lesson(7, "13:40", "14:15", "Informatik", "PC1"),
  ];
  const entry = (n, subject, kind, text) => ({ date: ymd(n), subject, kind, text });
  const school = {
    students: [{
      id: "lukas", name: "Lukas",
      day: { date: ymd(0), lessons, notices: ["7. Stunde endet 14:15 wegen Dienstberatung"] },
      homework: [
        entry(1, "Deutsch", "Hausaufgabe", "Arbeitsblatt Kommasetzung S. 2–3"),
        entry(1, "Mathematik", "Hausaufgabe", "Buch S. 54 Nr. 3a–d"),
        entry(6, "Englisch", "Hausaufgabe", "Vokabeln Unit 2 lernen"),
      ],
      exams: [
        entry(2, "Englisch", "Vokabeltest", "Unit 1"),
        entry(8, "Mathematik", "Klassenarbeit", "Bruchrechnung"),
      ],
    }],
    updatedAt: iso(now),
  };

  // --- Hannah: fixed timetable (timetable.Card); `calendar` pulls her school calendar into the card
  const timetables = [{
    id: "hannah", name: "Hannah", calendar: "Hannah",
    day: {
      date: ymd(0),
      lessons: [
        lesson(1, "08:00", "08:45", "Mathe", "2b"),
        lesson(2, "08:55", "09:40", "Deutsch", "2b"),
        lesson(3, "10:00", "10:45", "Sachunterricht", "2b"),
        lesson(4, "10:55", "11:40", "Sport", "Halle"),
        lesson(5, "11:45", "12:30", "Kunst", "2b"),
      ],
    },
    homework: [], exams: [],
  }];

  // --- lunch for Hannah
  const meals = {
    children: [{
      name: "Hannah", color: "#F2B705",
      days: [
        { date: ymd(0), ordered: [{ name: "Gemüsesuppe", side: "Brötchen" }] },
        { date: ymd(1), ordered: [{ name: "Nudeln mit Tomatensoße", side: "Reibekäse, Obst" }] },
        { date: ymd(2), ordered: [] },
      ],
      updatedAt: iso(now),
    }],
  };

  // --- to-dos from the family app
  const tasks = [
    { id: "t1", title: "Zimmer aufräumen", who: "Lukas", points: 5 },
    { id: "t2", title: "Müll rausbringen", who: "Lukas", points: 3, pending: true },
    { id: "t3", title: "Tisch decken", who: "Hannah", points: 3 },
    { id: "t4", title: "15 Minuten lesen", who: "Hannah", points: 5 },
    { id: "t5", title: "Ranzen packen", who: "Hannah", points: 2, pending: true },
  ];
  const todos = { name: "Aufgaben", source: "familyapp", tasks, pending: 2, updatedAt: iso(now) };

  // --- bins
  const waste = [{ name: "Gelbe Tonne", color: "#E5C100", dates: [ymd(1), ymd(15)] }];

  const [name, kindRaw, since, until] = SCENE_META[sceneId];
  const kind = kindRaw === "kind-night" ? "night" : kindRaw;
  const at = (hm, n = 0) => iso(day(n, hm));
  const scene = {
    id: sceneId, name, kind,
    since: at(since), until: at(until, sceneId === "night" ? 1 : 0),
    schoolDay: true,
    palette: opts.palette ?? (sceneId === "evening" ? "board" : "paper"),
  };

  return {
    version: "preview",
    now: iso(now),
    timezone: "Europe/Berlin",
    days: 7,
    calendar: { calendars, events, updatedAt: iso(now) },
    weather,
    shopping: { name: "Einkauf", items, updatedAt: iso(now) },
    school,
    timetables,
    meals,
    waste,
    todos,
    ...(opts.weatherBg ? { weatherBg: { name: opts.weatherBg, v: 1 } } : {}),
    scene,
    layout: { revision: 0, widgets: demoScenes[sceneId] || [] },
  };
}
