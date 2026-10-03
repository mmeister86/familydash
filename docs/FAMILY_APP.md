# Family app interface

Contract between familydash (wall, Unraid) and the family app's Convex backend (Coolify).
All requests go **from familydash to Convex** (HTTP actions on the "site" URL, e.g.
`https://familybackend-http.matthias.lol`). Unraid never has to be reachable from the internet.

| | Direction | Auth (`Authorization: Bearer …`) | familydash env | Convex env |
|---|---|---|---|---|
| `POST /ingest/child` | wall → app | ingest token | `FAMILY_APP_INGEST_TOKEN` | `INGEST_TOKEN` |
| `POST /ingest/briefing` | wall → app | ingest token | `FAMILY_APP_INGEST_TOKEN` | `INGEST_TOKEN` |
| `GET /todos?days=2` | app → wall | dashboard token | `FAMILY_APP_DASHBOARD_TOKEN` | `DASHBOARD_TOKEN` |
| `GET /dashboard/calendars` | app → wall | calendar token | `FAMILY_APP_CALENDAR_TOKEN` | `CALENDAR_DASHBOARD_TOKEN` |

The calendar token authorizes only `GET /dashboard/calendars` (never ingest or parent functions); the ingest/dashboard
tokens never authorize it. Calendar rollout, comparison and rollback: [docs/CONVEX_CALENDAR_ROLLOUT.md](CONVEX_CALENDAR_ROLLOUT.md).

Answers: `2xx` = ok (body ignored for POSTs), `400` invalid payload (message in the body), `401` wrong token.
Optional fields are **omitted, never `null`** (fits `v.optional(...)`). Dates are `YYYY-MM-DD` in Europe/Berlin,
timestamps are milliseconds.

## `POST /ingest/child`

One document per child; the app **replaces** it (upsert by `childSlug`). Sent when anything changed and at least
every `FAMILY_APP_HEARTBEAT` (default 15 min). Children are matched by first name: `Lukas Meister` → `lukas`,
umlauts spelled out (`ü` → `ue`). The slug must exist as a user in the app.

```ts
const change = v.object({
  type: v.union(v.literal("cancelled"), v.literal("substitution"), v.literal("roomChange"), v.literal("other")),
  note: v.optional(v.string()),
});

const lesson = v.object({
  period: v.number(),              // 0 = outside the numbered lessons (afternoon club)
  start: v.optional(v.string()),   // "07:30"
  end: v.optional(v.string()),
  subject: v.string(),
  room: v.optional(v.string()),
  teacher: v.optional(v.string()),
  tag: v.optional(v.string()),     // e.g. "GTA"
  change: v.optional(change),
});

const event = v.object({
  title: v.string(),
  start: v.string(),               // RFC 3339 ("2026-10-02T16:00:00+02:00"), or "YYYY-MM-DD" when allDay
  end: v.optional(v.string()),     // RFC 3339, or exclusive "YYYY-MM-DD" when allDay
  allDay: v.boolean(),
  calendar: v.optional(v.string()),
  location: v.optional(v.string()),
});

const meal = v.object({            // only on delivery days
  ordered: v.boolean(),            // false = nothing ordered
  title: v.optional(v.string()),   // "Nudeln / Suppe"
  description: v.optional(v.string()), // side dishes
});

const childDay = v.object({
  date: v.string(),
  notices: v.optional(v.array(v.string())),  // e.g. "Herbstferien bis 24.10."
  timetable: v.array(lesson),      // [] on weekends/holidays
  events: v.array(event),
  meal: v.optional(meal),
});

export const childSnapshotPayload = v.object({
  childSlug: v.string(),
  days: v.array(childDay),         // today … today+6, always 7
  homework: v.array(v.object({ subject: v.string(), text: v.string(), dueDate: v.string() })),
  exams: v.array(v.object({ subject: v.string(), date: v.string(), text: v.optional(v.string()) })),
  sourceUpdatedAt: v.number(),     // oldest fetch time of the sources behind it
});
```

Appointments: the child's school calendar (`CALENDAR_n_PANEL=school` with the child's name, or the fixed
timetable's `calendar`) plus `FAMILY_APP_<SLUG>_CALENDARS`. In `CALENDAR_SOURCE=convex` mode the same appointments
resolve through explicit central person bindings instead of names (see below).

## `POST /ingest/briefing`

Upsert by `kind` + `date`. Sent whenever the card changes (rule-based first, the AI version a moment later).

```ts
export const briefingPayload = v.object({
  kind: v.union(v.literal("morning"), v.literal("evening")),
  date: v.string(),                // the day it is about: today (morning) / tomorrow (evening)
  text: v.string(),                // Markdown, ready to render
  headline: v.optional(v.string()),
  items: v.array(v.object({
    section: v.union(v.literal("tonight"), v.literal("day")),
    icon: v.string(),              // schule, frei, ausfall, test, hausaufgabe, termin, fahrt, essen, brotbox,
                                   // muell, regen, kalt, warm, schnee, sport, todo, geburtstag, hinweis
    who: v.optional(v.string()),   // child's first name, calendar name or "Familie"
    color: v.optional(v.string()),
    text: v.string(),
  })),
  ai: v.boolean(),                 // false = rule-based fallback
  generatedAt: v.number(),
});
```

## `GET /todos?days=2`

Read-only. `days` = 1…7, the window starts today. familydash asks for 2 (today + tomorrow for the evening outlook).

```jsonc
{
  "date": "2026-10-02",                       // today, Europe/Berlin
  "people": [
    { "slug": "lukas", "name": "Lukas", "role": "child", "color": "#16a34a", "points": 120 }
    // … all four, parents included (points 0)
  ],
  "tasks": [
    {
      "id": "k17…",                           // task instance id
      "title": "Zimmer aufräumen",
      "assignee": "lukas",                    // omitted = whole family
      "date": "2026-10-02",                   // omitted = undated ("anytime")
      "status": "open",                       // open | pending | done | missed
      "points": 10,                           // omitted/0 = no points
      "recurring": true                       // false/omitted = one-off
    }
  ]
}
```

Which tasks to return: every instance with `date` in the window, **plus** open/pending one-offs with
`date < today` (overdue), plus instances done today. Undated ("anytime") tasks should be included
while open or pending: the wall shows them every day (no due tag) until they're done; done undated tasks
are ignored.

What the wall does with it: today's tasks of a child go into that child's card (⭐ points, ⏳ for `pending`);
one-offs get a due tag ("heute fällig" / "überfällig"); `missed` is ignored. The briefing gets every person's
open tasks, the number of `pending` ones (parents should confirm) and the children's points.

## `GET /dashboard/calendars`

Read-only central calendar feed (`CalendarFeedV1`, `version: 1`, `scope: "family-calendars"`, `timezone: "Europe/Berlin"`).
Used only with `CALENDAR_SOURCE=convex`; `Cache-Control: no-store`. Answers `401` on a wrong token, `503` while
unconfigured (no scheinbar erfolgreiches empty document — an explicitly empty setup returns full v1 with empty arrays)
or when the response would exceed 4 MiB (the Go cache is kept then).

```jsonc
{
  "version": 1,
  "scope": "family-calendars",
  "timezone": "Europe/Berlin",
  "configurationRevision": 7,       // bumped on every source/mode change; revokes old commit permission
  "generatedAt": 1728123456789,     // feed build time — NOT source freshness
  "window": { "fromDate": "2026-10-03", "toDate": "2026-11-14", "fromMs": 0, "toMs": 0 }, // 42 Berlin days, end exclusive
  "people": [{ "id": "…", "slug": "lukas", "name": "Lukas", "role": "child" }],
  "bindings": [{ "personId": "…", "kind": "besteschule|timetable", "externalId": "…" }],
  "calendars": [{
    "id": "…", "sourceKey": "family", "name": "Familie", "color": "#4F8EF7",
    "panel": "column|school", "order": 0, "intoCalendarId": "…", // optional
    "personIds": ["…"],                 // [] = family stand; all-child = personal stand
    "lastAttemptAt": 0, "lastSuccessAt": 0,
    "freshness": "neverLoaded|fresh|stale|disabled",
    "lastAttemptStatus": "success|error", "error": "…", "coverage": { /* window */ },
  }],
  "events": [{ "calendarId": "…", "key": "…", "uid": "…", "identityQuality": "provider|fallback", /* … */ }],
}
```

Only `mode: "convex"` calendars with their last published stand are included; `shadow` stands and secrets
(env names, URLs, PINs, tokens, raw responses) never appear. Go validates strictly (presence-aware: `{}` is
invalid, arrays must be present), converts stable ids to display indexes after sorting by `order`/`sourceKey`,
and atomically replaces its persistent last-good snapshot only on full success. A byte-identical fixture of this
contract lives in `testdata/calendar-feed-v1.json` (mirrors the Companion's `tests/fixtures/calendar-feed-v1.json`).
