# Convex-Grundlage und Kalender-Pilot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. The user has selected this execution method.

**Goal:** Kalender zentral in der bestehenden Companion-Convex-Anwendung laden und speichern; das Go-Dashboard verwendet im ausdrücklich gewählten Convex-Modus ausschließlich diese Daten und behält sie bei Ausfällen dauerhaft.

**Architecture:** Kalenderquellen besitzen stabile IDs, explizite Personenbindungen und einen generationsgeschützten Importzustand. Ein zentraler Dispatcher plant interne Actions; geprüfte Ergebnisse werden als neue Generation atomar veröffentlicht. Ein geschützter HTTP-Lesevertrag verbindet Convex mit einer austauschbaren Go-Kalenderquelle; der bestehende Kinder-Snapshot-Pusher bleibt zunächst alleiniger Schreiber seiner Projektion.

**Tech Stack:** Bestehendes Convex/TypeScript/Vitest und Go 1.24.7; vorgeschlagen `node-ical@0.26.1`, `convex-test@0.0.60` und `@edge-runtime/vm` als Testumgebung. Kein Frameworkwechsel.

**Spec:** [Convex als gemeinsames Familienbackend](../specs/2026-10-03-convex-family-backend-design.md).

**Backlog:** TASK-2 – Plane die Convex-Grundlage und den Kalender-Pilot. Umsetzungsaufgaben werden beim Start als separate Backlog-Aufgaben angelegt; keine Aufgabe wird ohne ausdrückliche Nutzerbestätigung als Done markiert.

## Global Constraints

- Die bestehende selbst gehostete Convex-Instanz auf Coolify bleibt bestehen.
- Externe Anbieter werden zunächst ausschließlich gelesen. Es gibt keinen Kalender-, Einkaufslisten- oder Essensbestellungs-Writeback.
- Bestehende Aufgaben, Punkte und Belohnungen bleiben im vorhandenen Convex-Modell.
- `users` bleibt die gemeinsame Personenbasis. Es entsteht keine zweite parallele Personentabelle.
- Das System bedient weiterhin eine Familie. Es wird keine Mehrmandantenarchitektur eingeführt.
- Datumsbezogene Fachlogik verwendet `Europe/Berlin`.
- Der erste Importhorizont beträgt für alle Kalender 42 Tage ab heute in `Europe/Berlin`.
- Geheimnisse bleiben zunächst in der serverseitigen Convex-/Coolify-Konfiguration.
- Kein Ingest-, Admin- oder Elternschlüssel wird im Browser ausgeliefert.
- `childSnapshots` bleibt zunächst als kompatible Sieben-Tage-Projektion bestehen.
- Vorherige erfolgreiche Daten bleiben bei einem Fehler erhalten.
- Produktcode, Bezeichner und Kommentare sind Englisch; sichtbare Texte sind Deutsch; TypeScript strict ohne neue `any`-Typen; pnpm ausschließlich für die Companion.
- Der erste Abschnitt umfasst Grundlage und Kalender. Andere Anbieter, die zentrale Kinderprojektion und gemeinsame Briefing-Erzeugung folgen in eigenen Plänen.

## Review Focus

1. Ein verspäteter Abruf nach Lease-Ablauf, Konfigurationswechsel oder Rückschaltung darf weder Daten noch Fehler-/Retryzustand überschreiben: Task 3.
2. Ein korrekt leerer Kalender darf alte Ereignisse entfernen; ein abgebrochener, fehlerhafter oder zu großer Abruf darf das nicht: Tasks 2–5.
3. Serienausnahmen, verschobene Termine und über Mitternacht laufende Ereignisse behalten Identität und korrekten Zeitbezug: Task 2.
4. Umbenennung oder neue Reihenfolge der Kalender darf keine Kinderzuordnung oder Spaltenzuordnung verändern: Tasks 1, 4 und 6.
5. Ein erfolgreicher Backend-GET mit alten Quelldaten sowie ein Neustart dürfen die Daten nicht scheinbar frischer machen oder andere Backend-Caches wiederverwenden: Task 5.

## Arbeitsbereiche und Verifikation

Gelesene Ausgangsstände:

- Dashboard: `/Users/matthias/Documents/Code/familydash`, Dokumentationscommit `f7fc396`.
- Companion: `/Users/matthias/Documents/Code/familienapp`, `main` bei `461dd10090653c98e81d138bf58a560e7938ac0d`.
- Beide Ausgangsstände ohne Produktänderungen. Companion wurde für die Bestandsaufnahme lokal geklont.

Bei Ausführung je Repository einen isolierten Arbeitsbereich über den Worktree-Skill erstellen, Branch `feat/convex-calendar-pilot`. Bereits verwaltete Worktrees werden weiterverwendet. Die beiden Repositorypfade in jedem Subagentenauftrag ausdrücklich nennen; Änderungen nicht zwischen Repositories verwechseln.

Go fehlt derzeit auf PATH. Eine lokale Go-Toolchain passend zu `go.mod` einrichten und `go version` prüfen, bevor Go-Tests starten. Node auf dem Host ist 26.10.0; für reale lokale Convex-Node-Actions zusätzlich eine unterstützte Node-Version bereitstellen. Self-hosted verwendet die Node-Version seines Backend-Builds; eine neue Cloud-Node-Konfiguration ist keine Lösung für Coolify.

Die neuen Bibliotheken werden erst nach Planreview installiert. `node-ical@0.26.1` benötigt Node >=20; neuere 0.27.x verlangen >=22, weshalb dieser Plan nicht ungeprüft die neueste Version verwendet. Registrymetadaten und Typen des Tags `0.26.1` wurden gelesen. Context7 hat trotz drei Namensvarianten keinen passenden node-ical-Eintrag geliefert; dafür wurden aktuelle Primärquellen verwendet. Die separate Bibliothek ICAL.js wurde untersucht, aber für diesen Plan nicht gewählt.

Für Tests eine lokale anonyme Convex-Entwicklungsinstanz verwenden. Im isolierten Companion-Arbeitsbereich alle geerbten produktiven `CONVEX_*`-Zielvariablen für Entwicklungsbefehle entfernen und keine bestehende `.env.local` mit Produktionsziel kopieren. Über `pnpm exec convex deployment select local` das lokale Ziel auswählen; vor jedem Sync die lokale Zielkonfiguration prüfen. `pnpm exec convex dev --once` ist eine schreibende Entwicklungsoperation und wird ausschließlich lokal ausgeführt. Produktionsdeployment und produktive Kalenderumschaltung sind ein abschließender separater Schritt nach reviewbarem Ergebnis.

Die bisherigen Companion-Anweisungen in `.docs/PLAN.md` verbieten direkte Anbieterabrufe. Diese Regel wird mit diesem bestätigten Vertrag für Kalender ausdrücklich ersetzt und in Task 1 dokumentiert. Bestehende Sicherheits- und Verifikationsregeln bleiben erhalten.

## Gemeinsame Schnittstellenentscheidungen

### Quellen, Zuordnungen und Import

Neue Tabellen in der Companion:

- `calendarSources`: stabile `sourceKey`, Name, Farbe, `panel: column|school`, Reihenfolge, optionales stabiles Spaltenziel, Personen-IDs, `urlEnvKey`, `enabled`, `mode: shadow|convex|local`, Intervall, Konfigurationsgeneration, nächster Abruf, laufender Import/Lease, veröffentlichter Import und `publishedDataImportId` für dessen Ereignisbestand.
- `calendarImports`: Quelle, monotoner Laufzähler, Konfigurationsgeneration, Zustand `running|staging|ready|error|superseded`, Versuch/Lease, Erfolgszeit, Abdeckung, Batchfortschritt und bereinigter Fehler. Keine URLs oder Tokens.
- `calendarEvents`: Quellen-ID, Import-ID, stabiler Ereignisschlüssel und normalisierte Ereignisfelder. Alte Generationen werden begrenzt und in Batches bereinigt.
- `personSourceBindings`: `userId`, `kind: besteschule|timetable`, `externalId`; eindeutige Zuordnung pro `(kind, externalId)`. Damit können die noch lokalen Kinderquellen feste Personen-IDs verwenden.
- `familyBackendSettings`: Singleton `key=calendars`, Konfigurationsrevision, explizites `configured`, zentraler Berlin-Tag. `configured=false` beziehungsweise fehlender Datensatz bedeutet noch nicht eingerichtet; eine bewusst leere Konfiguration hat `configured=true`.

Indizes: Quellen `by_sourceKey`, `by_enabled_mode_nextAttempt`; Imports `by_source_sequence`, `by_state_lease`; Ereignisse `by_source_import_key`, `by_import_start`; Bindings `by_kind_externalId`, `by_user`; Einstellungen `by_key`. Quellenmaximalzahl 20; User-/Quellenlisten sind entsprechend begrenzt. Wachsender Ereignisbestand wird ausschließlich über Indizes und begrenzte Abfragen gelesen.

`CalendarWindow` ist `{fromDate:string,toDate:string,fromMs:number,toMs:number}`, Ende exklusiv. `NormalizedCalendarEvent` enthält `{key:string,uid:string,identityQuality:"provider"|"fallback",recurrenceId?:string,title:string,location?:string,startMs:number,endMs:number,allDay:boolean,timezone:string,startDate?:string,endDate?:string}`. Bei Einzelterminen ist der Schlüssel `UID + single`; bei Serien `UID + ursprüngliche Instanzkennung`, mit DATE und DATE-TIME unterscheidbar. Quellen-ID ergänzt die globale Identität. Ausgabeschlüssel verwenden eine eindeutige strukturierte Kodierung, keine kollisionsanfällige Trennzeichenverkettung.

`CalendarSourceInput` ist `{sourceKey:string,name:string,color:string,panel:"column"|"school",order:number,intoCalendarId?:Id<"calendarSources">,personIds:Id<"users">[],urlEnvKey:string,enabled:boolean,intervalMs:number}`. Modusänderungen laufen ausschließlich über `activate`; `save` darf keine Importzustände übernehmen.

`contentFingerprint` ist SHA-256 über Fenster und nach Schlüssel sortierte normalisierte Ereignisse; Abrufzeit und Versuchstatus gehören nicht hinein. Ein unveränderter erfolgreicher Abruf erzeugt neue Erfolgsmetadaten, verwendet aber denselben `publishedDataImportId` und schreibt keine Ereignisse erneut.

Neue Funktionstypen in `convex/lib/calendarTypes.ts` und passende Validatoren in `convex/lib/calendarValidators.ts`; keine Node-Imports in diesen beiden Dateien.

### HTTP-Lesevertrag v1

`GET /dashboard/calendars` mit ausschließlich `Authorization: Bearer <CALENDAR_DASHBOARD_TOKEN>`. Neue Companion-Env `CALENDAR_DASHBOARD_TOKEN`, Go-Env `FAMILY_APP_CALENDAR_TOKEN`; beide bleiben serverseitig. Bisherige `DASHBOARD_TOKEN`/`INGEST_TOKEN` autorisieren diesen neuen Endpunkt nicht. Der neue Token autorisiert keine bestehenden Ingest-Endpunkte oder Elternfunktionen.

Antwort:

```typescript
type CalendarFeedV1 = {
  version: 1;
  scope: "family-calendars";
  timezone: "Europe/Berlin";
  configurationRevision: number;
  generatedAt: number;
  window: CalendarWindow;
  people: Array<{id: string; slug: string; name: string; role: "parent" | "child"}>;
  bindings: Array<{personId: string; kind: "besteschule" | "timetable"; externalId: string}>;
  calendars: Array<{
    id: string; sourceKey: string; name: string; color: string;
    panel: "column" | "school"; order: number; intoCalendarId?: string;
    personIds: string[];
    lastAttemptAt?: number; lastSuccessAt?: number;
    freshness: "neverLoaded" | "fresh" | "stale" | "disabled";
    lastAttemptStatus?: "success" | "error";
    error?: string;
    coverage?: CalendarWindow;
  }>;
  events: Array<NormalizedCalendarEvent & {calendarId: string}>;
};
```

Die Ausgabe enthält nur aktive `mode=convex`-Kalender sowie deren letzten veröffentlichten Ereignisstand. `mode=shadow` ist nur über Elternfunktionen einsehbar; `local` wird nicht zentral gepollt. Ein noch nicht konfigurierter Feed liefert 503, kein scheinbar erfolgreiches leeres Dokument. Eine explizit leere eingerichtete Auswahl liefert die vollständige v1-Antwort mit leeren Arrays.

Der Feed enthält keine Zugangsdaten, Env-Namen, PIN-Felder, Sessiontokens oder Rohantworten. Beim Ereignislesen Überlappung mit dem angefragten Fenster prüfen. Erfolgszeit und Abdeckung gehören zur tatsächlich veröffentlichten Generation; `generatedAt` ist keine Quellenfrische.

### Grenzen und Defaults

- Kalenderintervall 300000 ms; individuell konfigurierbar 60000–86400000 ms.
- Dispatcher jede Minute; maximal 20 Quellen; eine aktuelle Commit-Berechtigung pro Quelle.
- Lease 120000 ms; HTTP-Timeout 20000 ms; maximal drei kurzfristige Versuche pro Zyklus.
- Retrybasis 30000 ms, anschließend 60000 ms, mit maximal 20 % zusätzlichem Zufallsanteil; `Retry-After` darf den Mindestabstand erhöhen. Nach Ausschöpfen frühestens zum nächsten regulären Intervall erneut versuchen. Auth-/Parserfehler warten auf reguläres Intervall, Konfigurationsänderung oder begrenzten manuellen Refresh.
- Manueller Refresh frühestens 60000 ms nach dem letzten gestarteten Versuch; laufender gültiger Import wird nicht doppelt gestartet.
- Frischegrenze `max(3 * intervalMs, 300000)` ab `lastSuccessAt`.
- ICS höchstens 32 MiB vollständig gelesen; pro Quelle höchstens 2000 normalisierte Vorkommen. Überschreitung ist ein Fehler, keine erfolgreiche Trunkierung.
- Stagingbatches höchstens 100 Ereignisse und 256 KiB serialisierte Argumente; gesamte HTTP-Feedantwort höchstens 4 MiB. Zu große Gesamtantwort liefert 503; der Go-Cache bleibt erhalten.
- Nicht migrierte Quellen und Legacy-Ingest bleiben aktiv. Der Projektorwechsel mit `writerEpoch` aus Spec Phase D wird hier noch nicht implementiert.

## Task 1: Gemeinsamer Kalendervertrag und explizite Quellenverwaltung

**Files – Companion:**
- Modify: `convex/schema.ts`, `package.json`, `pnpm-lock.yaml`, `convex/_generated/api.d.ts`, `.docs/PLAN.md`.
- Create: `convex/calendarSources.ts`, `convex/lib/calendarTypes.ts`, `convex/lib/calendarValidators.ts`, `convex/lib/calendarPolicy.ts`, `vitest.config.ts`, `tests/convex/setup.ts`, `tests/convex/calendar-sources.test.ts`, `tests/unit/calendar-policy.test.ts`.

**Interfaces:**
- Produces `CalendarWindow`, `NormalizedCalendarEvent`, `CalendarFeedV1`, `CalendarSourceInput` and corresponding validators.
- Pure `calendarWindow(now:number): CalendarWindow`, `calendarFreshness(lastSuccessAt:number|undefined, intervalMs:number, now:number): "neverLoaded"|"fresh"|"stale"`, `occurrenceKey(uid:string, recurrenceId:string|undefined): string`.
- Public `calendarSources.list({token})`, `calendarSources.save({token,source:CalendarSourceInput}) -> sourceId`, `calendarSources.bindPerson({token,userId,kind,externalId}) -> bindingId`, `calendarSources.activate({token,sourceId,mode}) -> {configurationRevision}`, all guarded by `requireParent`.
- `save` resolves stable `sourceKey` by index and preserves source ID on rename/reorder. `activate(convex)` requires a successful current-generation import covering the current 42-day window. Config/mode changes revoke in-flight commit permission. New sources default disabled/shadow.

- [ ] Write failing policy and DB tests: `windowHas42BerlinDaysAcrossDST`, `freshnessDoesNotUseAttemptTime`, `sameKeyDifferentCalendarsRemainDistinct`, `renameKeepsSourceId`, `rejectDuplicateBinding`, `childCannotConfigure`, `activationNeedsCurrentCompleteImport`. Assertions: 42 calendar days despite 23/25-hour day; stale at 900000 ms for default interval; parent-only; fixed IDs survive rename; old config invalidated.
- [ ] Install proposed dependencies as part of this deliverable: `pnpm add node-ical@0.26.1`; `pnpm add -D convex-test@0.0.60 @edge-runtime/vm`. Existing Vitest is reused. Configure separate Node unit-test and edge-runtime DB-test projects, preserving all existing `tests/unit` tests.
- [ ] Run `pnpm exec vitest run tests/unit/calendar-policy.test.ts tests/convex/calendar-sources.test.ts`; confirm failures exercise missing behavior, not missing test infrastructure.
- [ ] Implement tables, validators, policy and guarded configuration functions. Build test harness with `convexTest(schema, modules)`, glob relative to `tests/convex/setup.ts`, and synthetic parent/child sessions inserted through `t.run`. Do not use JWT identity mocks for the app's token-argument auth.
- [ ] Generate Convex types against the isolated local target, rerun the two tests and `pnpm typecheck`; expect passing tests and no TypeScript diagnostics. Document the replacement of the old dashboard-only-fetch rule.
- [ ] Commit `feat: define shared calendar sources and contracts`; submit to separate spec and quality reviewers before continuing.

## Task 2: ICS-Normalisierung mit stabiler Serienidentität

**Files – Companion:**
- Create: `convex/lib/ics.ts`, `tests/unit/ics.test.ts`, `tests/fixtures/calendars/*.ics`.

**Interfaces:**
- Consumes Task 1 types, keys and time-window helpers.
- Produces `normalizeIcs(text:string, window:CalendarWindow, defaultTimezone:string): NormalizedCalendarEvent[]` in Node-only helper `convex/lib/ics.ts` with `"use node"`.

- [ ] Write failing fixture tests `berlinWeeklyAcrossFallDST`, `springDST`, `movedExceptionKeepsOriginalIdentity`, `cancelledMasterAndException`, `exdateAndRdate`, `allDayExclusiveEnd`, `ongoingBeforeWindow`, `duplicateUidDifferentSources`, `floatingTimeUsesBerlin`, `windowsTzid`, `malformedIsNotEmpty`, `tooManyOccurrencesIsNotPartialSuccess`. Assert a 17:00 Berlin weekly lesson remains 17:00 after 2026-10-25; an override at 18:30 keeps the 17:00 original instance key; an all-day event ending 2026-10-04 occupies only dates before that day.
- [ ] Run `pnpm exec vitest run tests/unit/ics.test.ts`; expect behavioral failures before implementing the helper.
- [ ] Normalize via `node-ical` parsing and typed recurrence expansion, supplemented where needed for RDATE and detached/moved exceptions. Iterate only the bounded window, include ongoing events and detached exceptions moved into the window, deduplicate dual-key recurrence metadata, and filter cancelled instances. Validate full VCALENDAR boundaries and every relevant event; permissive parser output alone is not proof of a successful complete feed.
- [ ] Preserve DATE values as date strings and exclusive ends; use source timezone/default Berlin for floating times, not process timezone. Identify absent UID using SHA-256 over a structured normalized tuple of original DTSTART, DTEND/DURATION, title and recurrence definition, excluding DTSTAMP/LAST-MODIFIED. Prefix with `fallback:` and mark limited identity quality; a change to these identity fields may change the key. Do not depend on parser-generated random IDs. Normalization must reject unsupported or ambiguous recurrence shapes without replacing last-good data.
- [ ] Test hosts with different TZ settings using `TZ=UTC pnpm exec vitest run tests/unit/ics.test.ts` and `TZ=America/New_York pnpm exec vitest run tests/unit/ics.test.ts`; expect identical normalized instants and DATE values. No network requests.
- [ ] Commit `feat: normalize ICS calendars with stable occurrences`; spec and quality review.

## Task 3: Generationsgeschützte Imports und atomare Veröffentlichung

**Files – Companion:**
- Create: `convex/calendarSync.ts`, `tests/convex/calendar-sync.test.ts`.

**Interfaces:**
- Consumes Task 1 source/import/events schema and normalized event validator.
- Internal `claim({sourceId}) -> {runId,sourceId,configGeneration,window,urlEnvKey,previousFingerprint?:string}|null`.
- Internal `stage({runId,configGeneration,batchIndex,events}) -> {accepted:boolean}`.
- Internal `publish({runId,configGeneration,batchCount,eventCount,contentFingerprint}) -> {accepted:boolean}`.
- Internal `publishUnchanged({runId,configGeneration,contentFingerprint}) -> {accepted:boolean}` compares the current published fingerprint/window, refreshes success metadata and preserves the existing data pointer without staging events.
- Internal `fail({runId,configGeneration,errorClass,retryAfterMs?}) -> {accepted:boolean}`.
- Internal `cleanup({sourceId}) -> {deleted:number,remaining:boolean}`.
- Publish compares active run/config generation, verifies all expected batches and unique keys, and switches published import plus lastSuccessAt/coverage together. Repeated identical batches do not duplicate records; older calls fail closed without changing current state.

- [ ] Write failing DB tests `leasePreventsDuplicateClaim`, `expiredClaimCannotPublishOrFail`, `configChangeRevokesRun`, `localConvexLocalCannotReviveOldRun`, `stagedDataInvisibleBeforePublish`, `missingBatchCannotPublish`, `identicalBatchIsIdempotent`, `emptyCompleteImportReplacesWindow`, `failedImportKeepsPublishedGeneration`, `retainedOutsideCoverage`, `cleanupCannotDeleteActiveGeneration`, `unchangedSuccessReusesEventsAndAdvancesFreshness`, `changedWindowCannotUseUnchangedPublish`.
- [ ] Run `pnpm exec vitest run tests/convex/calendar-sync.test.ts`; confirm intended missing protocol behavior.
- [ ] Implement lease/monotone sequence and claim checks within mutations; all timestamps come from mutation time. Staging is isolated by run ID. No success pointer is updated before full validation. Out-of-window retained data belongs to its previous recorded coverage, never becomes falsely current; current feed reads only covered/overlapping active data.
- [ ] Implement unchanged publication, retry policy and bounded cleanup. A failure from a superseded run cannot write an error or reschedule the current source. Cleanup retains all data generations referenced by the active and immediately preceding successful revision, including reused older data; it deletes older/abandoned staging records in bounded batches. Import metadata retains the latest 20 attempts plus referenced data-generation records per source. Cleanup is explicit generation retention, never an inference that an event outside import coverage was removed by its provider.
- [ ] Rerun DB tests and `pnpm typecheck`; inspect mocked DB result after interleaved claim/publish/fail sequences. Expect no partial visibility or stale overwrite.
- [ ] Commit `feat: publish calendar imports atomically`; spec and quality review.

## Task 4: Zentraler Fetch, Dispatcher und geschützter Kalenderfeed

**Files – Companion:**
- Create: `convex/calendarFetch.ts`, `convex/lib/calendarFetch.ts`, `convex/calendar.ts`, `tests/unit/calendar-fetch.test.ts`, `tests/convex/calendar-http.test.ts`, `tests/convex/calendar-dispatch.test.ts`, `tests/fixtures/calendar-feed-v1.json`.
- Modify: `convex/calendarSync.ts`, `convex/calendarSources.ts`, `convex/crons.ts`, `convex/http.ts`, `.env.example`, `convex/_generated/api.d.ts`.

**Interfaces:**
- Consumes Tasks 1–3; `calendarFetch.fetchCalendar({sourceId})` internal Node action claims, resolves secret env, calls transport helper, stages and publishes or reports a classified failure.
- Pure/transport helper `fetchAndNormalizeCalendar(url:string,window:CalendarWindow,fetchImpl:typeof fetch): Promise<NormalizedCalendarEvent[]>` in Node-only module; no Convex DB imports.
- `calendarSync.dispatch({})` internal mutation schedules due sources through `ctx.scheduler.runAfter`; it also advances the stored Berlin day. `calendarSources.requestRefresh({token,sourceId}) -> {started:boolean,reason:"running"|"throttled"|null}` is parent-only and schedules the same action.
- `calendar.getDashboardFeed({})` internal query returns `CalendarFeedV1`; `calendar.forUser({token})` returns only calendars permitted for the authenticated user. Children get only explicitly assigned personal calendars; parents get configured active calendars. Family calendars are parent-visible initially unless explicitly assigned to a child.
- HTTP route consumes new calendar-only env token, returns 401 on failed auth, 503 on not configured/oversize, and sets `Cache-Control: no-store`.

- [ ] Write failing transport tests for successful fixture, 401/429/503, timeout, too-large body, redirect handling and error redaction; assert failed parse does not call publish. DB/HTTP tests assert no token/old dashboard token/ingest token returns 401; calendar token succeeds but cannot ingest; child query returns no unassigned events; response never contains envKey/URLs/PINs; shadow data excluded; intentionally empty differs from not configured.
- [ ] Write `dispatchSchedulesOnlyDueEnabledCentralSources`, `manualRefreshIsThrottled`, `midnightAdvancesWindowWithoutProviderSuccess`, `importedCoverageDoesNotAdvanceWithoutFetch`. DB tests inspect scheduled functions without running Node libraries inside the edge-runtime mock. Node transport tests use real parser plus synthetic fetch; a real local backend smoke test covers the thin action wrapper.
- [ ] Run `pnpm exec vitest run tests/unit/calendar-fetch.test.ts tests/convex/calendar-http.test.ts tests/convex/calendar-dispatch.test.ts`; confirm failures before implementation.
- [ ] Implement fetch with timeout, complete stream size accounting and structured failures; never log raw URLs or unredacted exception strings. Resolve `webcal://` to HTTPS. Only server-configured HTTPS feed addresses are accepted; no user-controlled URL is exposed through read APIs. Normalize an entire successful response before staging, compute its fingerprint and select changed or unchanged publication without trusting provider ETags alone.
- [ ] Implement minute dispatcher, manual refresh, read projection and HTTP route. Read only published imports and report their actual per-source age. Sort calendars by order then sourceKey, preserving stable IDs and validated direct `intoCalendarId`; reject cycles/chained column targets in source configuration.
- [ ] Run tests, `pnpm typecheck`, `pnpm lint`, then create a synthetic local HTTPS ICS server with a locally trusted test CA and run one real local-backend import; production TLS validation remains enabled. Confirm the same fixture is returned via authenticated HTTP while no dashboard process is running. Save the synthetic v1 contract fixture for Task 5 and verify it against the return validator.
- [ ] Commit `feat: schedule central calendar sync and serve protected feeds`; spec and quality review.

## Task 5: Go-Quellenauswahl und persistenter Kalendercache

**Files – Dashboard:**
- Create: `internal/calendar/source.go`, `internal/familyapp/calendars.go`, `internal/familyapp/calendarcache.go`, `internal/familyapp/calendars_test.go`, `internal/familyapp/calendarcache_test.go`, `testdata/calendar-feed-v1.json`.
- Modify: `internal/config/config.go`, `internal/config/config_test.go`, `internal/familyapp/client.go`, `internal/calendar/calendar.go`, `internal/calendar/ics.go`, `internal/server/server.go`, `cmd/familydash/main.go`.

**Interfaces:**
- `calendar.Source` is `{Snapshot() calendar.Snapshot; Refresh(context.Context); Run(context.Context,time.Duration)}`; existing local Service and new companion CalendarService implement it.
- New `familyapp.NewCalendarService(client *Client, token string, cachePath string, loc *time.Location) *CalendarService`.
- `familyapp.ParseCalendarFeed(data []byte, loc *time.Location) (calendar.Snapshot,error)` validates v1 before conversion. `CalendarService` accepts only a fully validated response and atomically replaces its last-good snapshot.
- Add `calendar.Person {ID,Slug,Name,Role string}`, `calendar.PersonBinding {PersonID,Kind,ExternalID string}` and `calendar.SourceStatus {LastAttemptAt,LastSuccessAt time.Time; Freshness,LastAttemptStatus,Error string}`. Snapshot adds `Central bool`, `People []Person`, `Bindings []PersonBinding`, `ConfigurationRevision int64`, `WindowStart,WindowEnd time.Time`; CalendarInfo adds `CalendarID string`, `PersonIDs []string`, `Status SourceStatus`; Event adds `CalendarID,Key,UID,IdentityQuality string`. Use additive lower-camel JSON names. Existing fields retain their meanings; `UpdatedAt` in central mode is the oldest successful active source timestamp and stays zero if any required source is never loaded.
- Go config: `CALENDAR_SOURCE=local|convex`, default `local`; `FAMILY_APP_CALENDAR_TOKEN`; `CALENDAR_CACHE_FILE`, default `/data/calendar-cache.json`. Existing `FAMILY_APP_SITE_URL` supplies endpoint base. Convex poll interval 1 minute; provider interval stays centrally configured.
- Shared main factory `newCalendarSource(cfg *config.Config) calendar.Source` is used in main, briefing preview and familyapp preview. Convex selection never starts local ICS polling, including when credentials/HTTP/validation fail. Invalid chosen Convex config yields a visible unavailable-source state rather than automatic fallback.

- [ ] Write failing Go tests `TestConvexCalendarsWithoutLocalURLs`, `TestConvexNoLocalFallback`, `TestRejectMissingVersionOrArrays`, `TestRejectBrokenReferences`, `TestCompleteEmptyReplacesEvents`, `TestProviderSuccessNotGETTime`, `TestOversizeDetected`, `TestCacheRestart`, `TestWrongBackendCacheRejected`, `TestInvalidResponsePreservesCache`, `TestCacheWriteFailureKeepsRAM`. Use httptest servers and synthetic v1 fixture copied exactly from Task 4.
- [ ] Run `go test ./internal/familyapp ./internal/config ./internal/calendar`; failures must exercise absent new behavior, not missing Go setup.
- [ ] Implement Source interface and shared factory without `calendar -> familyapp` imports: familyapp already depends on calendar. Update server/newBriefing/newPusher parameter types to Source. Preserve `/api/dashboard` fields and existing local behavior.
- [ ] Add wire types with presence-aware validation: `{}` is invalid; arrays must be explicitly present; version/scope/timezone/dates/references/uniqueness/panel/column links are checked. Do not reject permitted additive JSON fields. Enforce 4 MiB by reading one extra byte rather than silently truncating. Existing todo/client behavior remains tested.
- [ ] Persist versioned snapshot envelope atomically using temp file, sync and rename with owner-only permissions. Bind it to normalized backend site URL and nonsecret scope; never store tokens/ICS URLs. On start validate cache as strictly as a response. Keep cache per-source success times; absence of a source success is not replaced by now. Cache persistence failure reports warning but does not discard accepted RAM data.
- [ ] Convert stable calendar IDs into display-only numeric id/cal/into indexes after sorting; carry stable IDs, persons, bindings and per-source status additively. Service and persisted snapshot retain the complete received 42-day coverage; each existing UI/child consumer applies its own display window. Convex mode uses the feed's family timezone for calendar-day logic; local mode retains its existing timezone behavior.
- [ ] Run targeted tests, `go vet ./...`, `go test ./...` and `go test -race ./internal/calendar ./internal/familyapp`; expect clean results. Verify all preview factories select the same mode using tests against a local fake endpoint.
- [ ] Commit `feat: read Convex calendars with persistent last-good cache`; spec and quality review.

## Task 6: Feste Personen- und Kalenderzuordnung in den bisherigen Ansichten

**Files – Dashboard:**
- Modify: `internal/familyapp/children.go`, `internal/familyapp/familyapp_test.go`, `internal/briefing/facts.go`, `internal/briefing/briefing_test.go`, `internal/timetable/timetable.go`, `internal/timetable/timetable_test.go`, `internal/timetable/stundenplan.json`, `internal/server/server.go`, `web/static/index.html`, `web/static/app.js`, `web/static/style.css`.
- Create: `internal/calendar/mapping.go`, `internal/calendar/mapping_test.go`, `web/static/calendar-mapping.js`, `tests/calendar-mapping.test.mjs`.

**Interfaces:**
- Consumes Task 5 Snapshot with explicit bindings and CalendarInfo stable IDs/personIds/status.
- `calendar.PersonForSource(s Snapshot,kind string,externalID string) (PersonBinding,bool)` resolves only exact stable bindings in central mode.
- Existing timetable Child JSON gets optional `id`, propagated to cards; built-in children receive stable non-name IDs. Go local mode keeps old names as compatibility behavior; central mode requires `timetable` bindings to those IDs and `besteschule` bindings to existing Student.ID.
- Existing BuildChildren signature remains usable; in central mode numeric extra-calendar ENV and name matching do not override explicit bindings. Unmapped children keep their school data without guessed calendars and expose a configuration warning. Never merge same-name source children.
- `web/static/calendar-mapping.js` exposes `globalThis.FamilyCalendarMapping.personForSource(bindings,kind,externalId)` and `calendarIdsForPerson(calendars,personId)` as pure helpers. Load it as a deferred script before app.js; Node built-in tests load the same script and inspect the exported global. No frontend framework/build step is introduced.

- [ ] Write failing tests `TestCalendarReorderKeepsChildMapping`, `TestSameNamesDoNotMergeCentralBindings`, `TestAllAssignedCalendarsIncluded`, `TestMissingSourceNeverBecomesFresh`, `TestStableTimetableBinding`, `TestBriefingOverlapUsesCalendarIdentity`. Node tests assert renamed/reordered calendars and same-name children resolve by IDs; missing binding yields no guessed match.
- [ ] Run `go test ./internal/familyapp ./internal/calendar ./internal/briefing ./internal/timetable` and `node --test tests/calendar-mapping.test.mjs`; expect missing behavior failures.
- [ ] Implement exact binding resolution across Pusher and UI school extras. Preserve legacy `/ingest/child` payload/slugs while deriving them from bound users in central mode. Add current user/person ID to school/timetable view objects when bindings exist; avoid mutating cached provider snapshots when enriching server output.
- [ ] Compute a child's calendar contribution to `sourceUpdatedAt` from the assigned calendars' real success times. Required never-loaded calendars produce explicitly stale/unknown state and a zero source timestamp, not now; do not transmit newly incomplete child projections until required central calendar sources have first valid data. Existing successful snapshots in Convex remain intact.
- [ ] Display concise German status for unavailable/stale calendar sources in the existing footer. Numeric indexes remain layout references only. Briefing overlap compares stable calendar identity while using names solely in text. Fixed-plan rules and other providers remain local.
- [ ] Run targeted tests and full Go suite. Verify the unchanged wall layout in the product-native preview at the current portrait size using synthetic data, including stale status and two same-name sources. No redesign.
- [ ] Commit `feat: preserve explicit calendar person mappings across views`; spec and quality review.

## Task 7: Gemeinsame Abnahme und dokumentierter Quellenwechsel

**Files – both repositories:**
- Modify Companion: `README.md`, `.docs/FAMILY_APP.md`, `.docs/PLAN.md`.
- Modify Dashboard: `README.md`, `docs/FAMILY_APP.md`, `.env.example`, `deploy/unraid/my-familydash.xml`, `docs/ROADMAP.md`.
- Create Dashboard: `docs/CONVEX_CALENDAR_ROLLOUT.md`.

**Interfaces:**
- Uses all preceding tasks and their shared synthetic contract fixture.
- Documents exact configuration, shadow comparison, source activation, Go local→convex selection, last-good behavior and rollback. Credentials are placeholders described by env names, never real values.

- [ ] Perform cross-repository smoke test against isolated local backend: central import with Go stopped; start Go in convex mode with no local ICS URLs; validate `/api/dashboard` and child Pusher calendar selection; stop backend; restart Go; assert same cache retained with original source age and visible outage.
- [ ] Repeat with successful empty ICS, HTTP error, moved recurrence, Berlin date rollover, renamed/reordered calendars, invalid backend JSON and explicit rollback. Assert no local provider request occurs in convex mode and a superseded central action cannot commit after mode/generation changes.
- [ ] Run Companion `pnpm test`, `pnpm typecheck`, `pnpm lint`, `pnpm build`, local-only `pnpm exec convex dev --once`; Dashboard `go vet ./...`, `go test ./...`, focused race tests and Node mapping tests. Existing task/points/rewards tests remain included. Report any baseline failure separately; never claim a skipped deployment check passed.
- [ ] Write rollout document: save existing config/backup; populate secret envs outside Git; create disabled/shadow sources and explicit bindings; import and compare; activate central source generation; select Go convex mode and verify every preview/path; remove direct calendar access only after acceptance. Rollback first revokes central commit permission via mode/generation change, then explicitly selects local Go mode. No automatic fallback.
- [ ] Document phase boundary: legacy child/briefing push remains active, other sources remain local, full backend independence for school and briefings comes in later plans. The cache introduced here covers calendars; full dashboard-response cache follows the full dashboard read contract.
- [ ] Commit `docs: document Convex calendar pilot rollout`; run whole-branch independent review of both repos. Fix findings and rerun only affected checks before handing over.
- [ ] Present reviewable changes and test evidence before a production deployment/merge step. Backlog tasks remain In Progress until explicit user confirmation to mark Done. No automatic next-provider migration.

## Execution handoff

Execution method is already selected: Subagent-driven Development. Each task gets a fresh implementer, then separate spec and quality review. Implementation tasks are sequential because their schemas, contracts and generated types depend on one another; parallel reading during this planning step does not authorize concurrent conflicting edits.

The next action after written-plan review is to invoke the Subagent-driven Development and worktree skills, prepare isolated toolchains/local backend, create the implementation Backlog entries and dispatch Task 1. No need to ask the execution-method question again.

## Documentation used

- Context7 Convex backend and convex-test results, plus [official testing documentation](https://docs.convex.dev/testing/convex-test).
- [Convex runtimes](https://docs.convex.dev/functions/runtimes), [CLI](https://docs.convex.dev/cli/overview), [local deployments](https://docs.convex.dev/cli/local-deployments).
- [node-ical official source at 0.26.1](https://github.com/jens-maus/node-ical/tree/0.26.1), its published npm metadata and bundled TypeScript declarations.
- Current repository files and two read-only specialist inventories. Tests were not run during plan creation; product dependencies were not installed and production was not contacted.
