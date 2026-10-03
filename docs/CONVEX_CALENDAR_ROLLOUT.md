# Convex calendar pilot — rollout & rollback

How the wall dashboard stops polling Google/iCloud/Outlook iCal feeds itself
and reads calendars from the family app's Convex backend instead
(`CALENDAR_SOURCE=convex`). Companion side: `calendarSources`,
`calendarImports`, `calendarEvents`, `personSourceBindings`,
`familyBackendSettings` plus `GET /dashboard/calendars`
(`CalendarFeedV1`, `convex/calendar.ts`, route in `convex/http.ts`).

Status: pilot accepted on synthetic/contract verification only (see
*Acceptance evidence*). No production deployment or merge has happened;
backlog tasks stay In Progress until explicit confirmation.

## Phase boundary (what this does NOT change)

- The legacy child/briefing push (`POST /ingest/child`,
  `POST /ingest/briefing`) stays active and remains the only writer of
  `childSnapshots`. The pusher feeds it from the central calendar stand
  once calendars are activated, but the push itself is untouched.
- Every other source (beste.schule, timetable, VielfaltMenü, Bring!,
  weather, news) stays local to Go. Central school/briefing independence
  comes in later plans.
- The persistent cache introduced here covers **calendars only**
  (`CALENDAR_CACHE_FILE`, default `/data/calendar-cache.json`). A full
  dashboard-response cache follows the full dashboard read contract.
- No automatic next-provider migration: each source is switched
  explicitly, by hand, after comparison.

## 0. Save the existing config / backup

1. Export the current Go calendar env (`CALENDAR_*`) and the Unraid
   template / `.env` to a dated backup outside Git.
2. Back up the Convex production database (Coolify volume snapshot or
   `npx convex export`) before touching sources; verify the restore path
   on the existing instance before the productive switchover.
3. No secret ever lands in Git: tokens and iCal URLs live only in
   Convex env / Coolify config and in the Unraid container env.
   Below, env **names** are placeholders — never paste real values
   into docs, chat logs or commits.

## 1. Secrets (names only, set outside Git)

| Where | Name | Purpose |
|---|---|---|
| Convex env | `CALENDAR_DASHBOARD_TOKEN` | Only credential for `GET /dashboard/calendars`. Authorizes nothing else (no ingest, no parent functions) |
| Convex env | `<urlEnvKey>` per source, e.g. `FAMILY_CALENDAR_URL`, `SCHOOL_CALENDAR_URL` | Server-side HTTPS feed addresses. Never exposed through any read API |
| Go env | `FAMILY_APP_CALENDAR_TOKEN` | Must equal `CALENDAR_DASHBOARD_TOKEN`. Stays in the Go server, never in the browser |
| Go env | `FAMILY_APP_SITE_URL` | Existing Convex site URL (HTTP actions, port 3211) |
| Go env (legacy, keep) | `DASHBOARD_TOKEN` / `INGEST_TOKEN`, `FAMILY_APP_INGEST_TOKEN`, `FAMILY_APP_DASHBOARD_TOKEN` | Unchanged; the calendar token authorizes neither old endpoint |

```sh
# Convex (production Coolify backend) — values only on the command line,
# never in a file:
npx convex env set CALENDAR_DASHBOARD_TOKEN '<generated>'
npx convex env set FAMILY_CALENDAR_URL '<secret-ical-url>'
npx convex env set SCHOOL_CALENDAR_URL '<secret-ical-url>'
```

## 2. Disabled/shadow sources + explicit bindings

Create every calendar disabled first (`enabled: false`, default mode
`shadow`). Shadow sources import on their interval but are only visible
through parent functions — never in the device feed. New sources must
always start disabled/shadow; `save` never takes over import state.

```ts
// parent session, token = parent auth token (never a device token)
await calendarSources.save({ token, source: {
  sourceKey: "family", name: "Familie", color: "#4F8EF7",
  panel: "column", order: 0, personIds: [],   // [] = family stand
  urlEnvKey: "FAMILY_CALENDAR_URL", enabled: false, intervalMs: 300000,
}});
await calendarSources.bindPerson({ token, userId, kind: "besteschule", externalId: "<student-id>" });
await calendarSources.bindPerson({ token, userId, kind: "timetable", externalId: "<plan-child-id>" });
```

- `sourceKey` is stable: renames/reorders keep the source id and every
  binding. Array positions are layout only.
- One binding per `(kind, externalId)`; ambiguous names never auto-link.
  A missing binding means *unmapped* (visible configuration warning),
  never a name guess.
- `personIds: []` on a `column` calendar = family stand (parent-visible
  until explicitly assigned to a child). A calendar whose every
  `personIds` entry is a child is that child's personal stand.
- Calendars bound to a deleted person are hidden from everyone
  (fail-closed); re-bind or the stand stays invisible.

## 3. Import + compare (shadow)

1. Enable the shadow source (still `mode: "shadow"`). The dispatcher
   (every minute) claims, fetches (20 s timeout, ≤ 32 MiB, ≤ 2000
   occurrences), stages in ≤ 100-event / ≤ 256 KiB batches and publishes
   atomically with a new configuration generation.
2. Compare against the live wall for at least a few days, ideally across
   a DST boundary: same 17:00 Berlin lesson before/after the change,
   moved/cancelled single instances keep their identity, all-day events
   end exclusively, multi-day events overlapping the window start are
   present. A limited parallel local poll for comparison is allowed
   during acceptance only.
3. Rules that must hold (all covered by tests, see *Acceptance evidence*):
   a correct empty feed clears its window; an aborted, failed or
   oversized fetch never clears last-good data; a late run from before a
   config change or rollback cannot commit (generation-guarded);
   `Retry-After` and backoff apply, auth/parser errors wait for the
   regular interval.

`requestRefresh` (parent-only, ≥ 60 s after the last attempt) triggers
one manual import without doubling a running one.

## 4. Activate central generation

Only after the shadow stand matches the wall:

```ts
await calendarSources.activate({ token, sourceId, mode: "convex" });
```

`activate("convex")` requires a successful current-generation import
covering the current 42-day Berlin window, and revokes in-flight commit
permission of older runs. From here the source is in the device feed.
Repeat per calendar; each source switches individually.

## 5. Go local → convex selection

```sh
CALENDAR_SOURCE=convex
FAMILY_APP_SITE_URL=https://familybackend-http.matthias.lol
FAMILY_APP_CALENDAR_TOKEN='<same-as-CALENDAR_DASHBOARD_TOKEN>'
# optional: CALENDAR_CACHE_FILE=/data/calendar-cache.json (default)
```

- Default stays `local`. `CALENDAR_SOURCE` accepts only `local|convex`;
  anything else fails startup loudly.
- Convex selection **never starts local ICS polling** — not on missing
  credentials, HTTP errors or invalid feed JSON either. A broken convex
  config is a visible *unavailable-source* state (footer status +
  `errors`), never a silent fallback.
- Only fully validated v1 responses replace the last-good snapshot
  (RAM + atomic disk cache, owner-only permissions, bound to backend
  URL + scope; tokens/URLs are never persisted). Go polls every minute;
  provider intervals stay centrally configured.
- `UpdatedAt` in convex mode is the oldest successful active source
  timestamp and stays zero while any required source was never loaded;
  required never-loaded calendars keep the child projection
  explicitly stale/unknown instead of showing `now`.

Verify every preview/path with the same factory (`newCalendarSource`
is shared by server, briefing, pusher and previews):

1. `-familyapp-preview`: children resolve calendars by stable id
   (rename/reorder-proof, same-name children never merge).
2. Wall `/api/dashboard`: footer shows concise German status for
   unavailable/stale sources; unmapped children keep school data
   without guessed calendars.
3. Child snapshots: `sourceUpdatedAt` per child comes from the assigned
   calendars' real success times; incomplete central projections are
   not transmitted until required sources have first valid data.
4. Stop the backend, restart Go: the same cache is served with the
   original source age and a visible outage — the restart must not
   make data look fresher and must not reuse other backend caches.
5. `?scene=` previews and the briefing paths use the same mode
   (covered by factory tests against a local fake endpoint).

## 6. Remove direct calendar access — only after acceptance

Only when the central stand has proven itself in production:

1. Clear the `CALENDAR_n_URL` values (Unraid template + `.env`).
2. Keep the `CALENDAR_n_*` parsing code until Phase F of the backend
   plan; rollback needs the old path present but unconfigured.
3. Never delete the local ICS code while `CALENDAR_SOURCE=local`
   remains the documented rollback target.

## Rollback (no automatic fallback)

Order matters — first revoke central commit permission, then select
the local Go mode. A running old generation must never write again:

1. `activate({ sourceId, mode: "shadow" })` (or `"local"`) — a config
   change issues a new generation, so in-flight central runs lose
   commit permission and fail closed without touching current state.
2. Only then set `CALENDAR_SOURCE=local` on Go and restore the
   `CALENDAR_n_URL` values from the §0 backup.
3. The last successful central stand stays stored in Convex; Go serves
   its own last-good local data. Nothing is merged automatically.
4. For the child projection the same rule applies later: a projection
   handover always moves a single writer epoch; an old push after the
   switch (or after a switch-back) is rejected.

## Acceptance evidence (synthetic/contract, no live backend in this env)

A live cross-repo smoke test was impossible here: no Docker daemon
exists in this environment, and `pnpm exec convex dev --once` fails
because the host runs Node 26 while local `use node` actions require
Node 20/22/24. No live check is claimed; instead:

- CompanionFixture ≡ GoFixture: `tests/fixtures/calendar-feed-v1.json`
  and `testdata/calendar-feed-v1.json` are byte-identical (`cmp`).
- Go httptest stand-in for the Companion HTTP contract: convex-mode
  import with no local ICS URLs, no local fallback on failure,
  invalid-JSON preservation, oversize rejection, cache restart and
  wrong-backend-cache rejection
  (`internal/familyapp`, `internal/calendar`, `cmd/familydash` suites,
  incl. `-race`).
- Generation/rollback semantics: lease, expiry, config-change revocation,
  invisible staging, idempotent batches, empty-replaces-window,
  failure-keeps-generation, unchanged-reuses-events
  (`tests/convex/calendar-sync.test.ts`, `calendar-dispatch.test.ts`).
- Transport edge cases: 401/429/503, timeout, oversize body, redirects,
  redaction, moved recurrence, Berlin DST rollover, rename/reorder
  (`tests/unit/calendar-fetch.test.ts`, `tests/unit/ics.test.ts`,
  `tests/convex/calendar-http.test.ts`).
- Wall mapping/render: renamed/reordered calendars and same-name
  children resolve by id; central mode with a missing mapping helper
  attaches no calendar (treated as unmapped, regression test in
  `tests/calendar-mapping.test.mjs`); local name matching unchanged.

## Known limitations (ledgered, not fixed here)

- `internal/calendar/calendar.go`: central-only JSON fields
  (`calendarId`, `personIds`, `status`, `central`, `people`,
  `bindings`, `configurationRevision`, `windowStart/End`, event
  `key`/`uid`/`identityQuality`) intentionally carry **no** `omitempty`.
  Adding it would *remove* keys from the local-mode `/api/dashboard`
  wire output instead of keeping it unchanged, and no JSON golden test
  exists to pin the wire shape — so the one-liner was skipped. The wall
  tolerates both shapes (`calIdOf`/`evCalIdOf` fall back to numeric
  indexes; missing `bindings` means unmapped).
- Full `pnpm exec convex dev --once` deployment sync did not pass in
  this environment (Node 26 vs required 20/22/24 for `use node`
  actions). Unit, DB (edge-runtime), typecheck, lint and build all pass;
  the deployment sync must be re-run where a supported Node exists
  before any production switch.
