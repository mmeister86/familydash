# Wall Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the wall display the "Familienplaner" look (Sofia Sans, paper/board palettes, name plates) and two new tiles, the day band and the week matrix.

**Architecture:** The Go server stays the data aggregator and gains three small data additions (scene palette, full-day weather hours, person colour). All new frontend logic that can be pure lives in small script files next to `layout.js`, tested with plain `node` assert scripts; `app.js` only renders. A node preview harness serves demo data so every visual task can be screenshotted.

**Tech Stack:** Go 1.24 standard library, plain HTML/CSS/JS (no build step), node for test scripts, headless Chromium for screenshots.

**Spec:** `docs/superpowers/specs/2026-11-25-wall-redesign/spec.md` (visual reference: `mockup.html`, `day.png`, `evening.png` in the same folder)

## Global Constraints

- Go standard library only; no new Go or npm dependencies; no build step for the frontend.
- Frontend files are classic scripts loaded with `<script defer>` in `index.html` (no ES modules); pure helpers are plain top-level `function` declarations so node tests can load them with `new Function(src + "; return {...}")`, exactly like `web/layout.test.mjs` does.
- UI strings German, code/comments English. No CSS `text-transform: uppercase` and no letter-spaced label style anywhere outside the night scene.
- Existing CSS custom-property names stay (`--bg`, `--panel`, `--panel-2`, `--line`, `--text`, `--muted`, `--faint`, `--accent`, `--warn`, `--danger`, `--sun`, `--rain`, `--good`); values per palette come from the spec table. New: `--notice`, `--notice-ink`.
- Root font size stays `clamp(13px, 1.62vmin, 24px)` (17.5px at 1080 wide). Sizes in the mockup are px at 1080 wide: convert with rem = px / 17.5, rounded to 0.05rem.
- Animate only `opacity` and `transform`. The night scene (`body.scene-night`, `.night-clock`, `.n-*`) keeps its current rules and colours untouched.
- Every commit leaves `go vet ./... && go test ./...` and `pnpm test` green. Conventional Commits.
- Screenshot command for visual checks (after `node web/preview.mjs --port 8099 &`): `chromium --headless --no-sandbox --hide-scrollbars --window-size=1080,1920 --virtual-time-budget=4000 --screenshot=<abs path>.png "http://127.0.0.1:8099/?scene=<id>"`. Save screenshots outside the repo (under `/tmp`), look at them with the Read tool, kill the preview server afterwards.

## Review Focus

- A calendar or person colour that is very light or very dark: text on name plates and bars must stay readable (Task 6 `inkOn` tests).
- Events that start before 06:00, end after 22:00 or span midnight: bars are clipped, never negative width or outside the track (Task 7 tests).
- A day with no lessons, only cancelled lessons, or holidays: no school bar, no crash (Task 7 tests).
- Several overlapping events in one calendar: two rows, nothing lost silently beyond row two (Task 7 tests).
- A feed from an older family app without `palette` or `people[].color`: wall still parses it and picks sensible defaults (Task 3 and Task 4 tests).

---

### Task 1: Preview harness with demo data

**Files:**
- Create: `web/preview.mjs`, `web/preview-data.mjs`, `tests/preview.test.mjs`
- Modify: `package.json` (test script)

**Interfaces:**
- Produces: `demoDashboard(now: Date, opts: {scene?: string, palette?: string}) -> object` and `demoScenes: Record<string, widget[]>` exported from `web/preview-data.mjs`. CLI: `node web/preview.mjs [--port 8099] [--scene <id>] [--palette paper|board]`. Later tasks extend the demo data and use the server for screenshots.

- [ ] **Step 1: Write the failing test** `tests/preview.test.mjs` (node `assert` script, prints `preview.test.mjs: all assertions passed`):

```js
import assert from "node:assert";
import { demoDashboard, demoScenes } from "../web/preview-data.mjs";
const now = new Date(2026, 8, 30, 13, 18); // Wed 30 Sep 2026, local
const d = demoDashboard(now, { scene: "afternoon" });
assert.strictEqual(d.scene.id, "afternoon");
assert.strictEqual(d.scene.kind, "layout");
assert.deepStrictEqual(d.layout.widgets, demoScenes.afternoon);
assert.strictEqual(d.days, 7);
assert.ok(d.calendar.calendars.length >= 3 && d.calendar.events.length >= 6);
assert.ok(d.calendar.events.every((e) => e.allDay ? e.startDate && e.endDate : e.start && e.end));
assert.ok(d.calendar.events.some((e) => !e.allDay && e.start.startsWith("2026-09-30")));
assert.strictEqual(d.weather.daily.length, 5);
assert.strictEqual(d.weather.daily[0].date, "2026-09-30");
assert.ok(d.weather.hourly.length === 12);
assert.ok(d.shopping.items.length === 10);
assert.strictEqual(d.school.students[0].day.date, "2026-09-30");
assert.ok(d.school.students[0].day.lessons.length === 7);
assert.ok(d.meals.children.length >= 1 && d.todos.tasks.length >= 3 && d.waste.length >= 1);
assert.strictEqual(demoDashboard(now, { scene: "night" }).scene.kind, "night");
assert.deepStrictEqual(Object.keys(demoScenes).sort(), ["afternoon", "day", "evening", "morning"]);
console.log("preview.test.mjs: all assertions passed");
```

- [ ] **Step 2: Run** `node tests/preview.test.mjs` — Expected: FAIL, module not found.

- [ ] **Step 3: Implement `web/preview-data.mjs`.** The payload mirrors the JSON of `GET /api/dashboard`: read the `dashboard` struct in `internal/server/server.go` and the structs it references (calendar.Snapshot, weather.Weather, bring.List, dashboardSchool, vielfalt.Meals, todo.List, waste.Pickup, scene.Current, sceneLayout) plus how `web/static/app.js` reads each field, and produce exactly those field names. Content is the mockup's: calendars Familie/Dienst/Mama (panel `column`) and a school calendar "Hannah" (panel `school`); the events, lessons (student "Lukas", one cancelled, two substitutions, a notice), exams, homework, to-dos for Lukas and Hannah (one `pending`, with `points`), lunch for Hannah, ten shopping items and a "Gelbe Tonne" collected tomorrow, all dated relative to `now` (today = `now`'s local date). `demoScenes` holds the widget lists of the four layout scenes copied from `internal/scenes/defaults.go` (`DefaultFeed`). `scene` is `{id, name, kind, since, until, schoolDay}`; `version` is `"preview"`. `opts.palette`, when given, is set as `scene.palette`.

- [ ] **Step 4: Implement `web/preview.mjs`** with `node:http` only: serves files from `web/static/` with correct content types (html, css, js, woff2, svg), answers `GET /api/dashboard` and `GET /api/scene` (the latter `{scene, layout}`) from `demoDashboard(new Date(), {scene, palette})` where `scene` comes from the request's `?scene=` query, else the `--scene` flag, else `afternoon`; everything else 404. Prints the URL on start.

- [ ] **Step 5: Run** `node tests/preview.test.mjs` — Expected: PASS.

- [ ] **Step 6: Set the test script** in `package.json` to `node web/layout.test.mjs && node tests/calendar-mapping.test.mjs && node tests/preview.test.mjs`; run `pnpm test` — Expected: all three pass.

- [ ] **Step 7: Visual check.** Start the server, take a screenshot of `?scene=afternoon` with the command from Global Constraints, Read it: the current (old) design must render with every tile filled (clock, weather, shopping chips, two child cards, three calendar columns) and no "Setze …" hints. Fix the demo data until it does.

- [ ] **Step 8: Commit** `feat: add preview harness with demo data`.

---

### Task 2: Theme foundation (fonts, palettes, plates, typography)

**Files:**
- Create: `web/static/fonts/sofia-sans.woff2`, `sofia-sans-condensed.woff2`, `sofia-sans-extra-condensed.woff2`, `web/static/fonts/OFL.txt` (copy all four from `docs/superpowers/specs/2026-11-25-wall-redesign/fonts/`)
- Modify: `web/static/style.css`, `web/static/app.js` (`applyScene` only), `web/static/index.html` (favicon colour only)
- Test: `tests/theme.test.mjs`, `package.json`

**Interfaces:**
- Consumes: preview harness (Task 1); `scene.palette` in the payload (set by `--palette` now, by the server from Task 3).
- Produces: `html[data-palette="paper"|"board"]`; font families `Sofia`, `SofiaC`, `SofiaX`; classes `.plate`-equivalent styling on `.panel`; palette tokens. Later tasks style against these tokens only.

- [ ] **Step 1: Write the failing test** `tests/theme.test.mjs`: reads `web/static/style.css` as text and asserts: it contains three `@font-face` blocks with `url(fonts/sofia-sans.woff2)`, `url(fonts/sofia-sans-condensed.woff2)`, `url(fonts/sofia-sans-extra-condensed.woff2)`; the three font files exist under `web/static/fonts/` and are larger than 20 000 bytes; the css contains `html[data-palette="board"]`; every paper value and every board value from the spec table appears (case-insensitive) in the css; outside the part of the file between the comment markers `/* night:start */` and `/* night:end */` there is no `text-transform: uppercase` and no `letter-spacing` larger than `0.03em`; the old values `#0E1116`, `#161B22` and `#4F8EF7` no longer appear. Add it to the `package.json` test chain.

- [ ] **Step 2: Run** `node tests/theme.test.mjs` — Expected: FAIL (no font-face).

- [ ] **Step 3: Rework `style.css`.** Wrap the existing night-scene rules in `/* night:start */ … /* night:end */` and leave them as they are, except that their font family becomes `SofiaX` for `.n-time` only. Then: add the `@font-face` rules (`font-weight: 100 900`, `font-display: block`); set the paper tokens on `:root` and the board tokens on `html[data-palette="board"]`; `body` uses `Sofia`, tabular numbers; `.panel`, `.calendars`, `.photo-frame`: `--panel` background, no border, `border-radius: 6px`; `--radius: 6px`. Replace every uppercase/letter-spaced label style (`h2`, `.sub-h`, `.cday-h`, `.tag`, `.meal .when`, `.bi .who`) with sentence-case styles from the mockup: sub-headings `Sofia` 700 at 1.1rem in `--muted`; tile titles (`.head .name`, `h2`) `SofiaX` 800 at 1.7rem; dense rows (`.lesson`, `.entry`, `.ev`, `.meal`, `.todo`, `.headline`, `.bi`, `.chips`) in `SofiaC`. `.tag` becomes plain coloured bold text without background. Past rows use `--faint` colour instead of `opacity`. Remove the `.panel.briefing.ai` gradient (plain plate; the `✦` stays in `--text`), the `.chip` pill background (all-day entries are plain `--muted` text lines) and every other decorative gradient except the weather-photo scrims (`.wx-bg::after`, `.n-bg::after`). Keep all layout/grid rules, `[data-narrow]` rules and class names. Leave the clock card's own rules (`.clock-card`, `.cc-*`, `.time`, `.date`, `.wx-*`, `.bin`, including the `--wx-grad` gradients) to Task 5: here they only pick up the new tokens and fonts.

- [ ] **Step 4: Apply the palette in `app.js`:** in `applyScene`, set `document.documentElement.dataset.palette = sc.palette === "board" ? "board" : "paper"`. Change the favicon fill in `index.html` from `%234F8EF7` to `%2314211B`.

- [ ] **Step 5: Run** `pnpm test` — Expected: PASS.

- [ ] **Step 6: Visual check.** Screenshot `?scene=afternoon` with the preview started once with `--palette paper` and once with `--palette board`; also `?scene=morning`, `?scene=day`, `?scene=evening`. Compare with `day.png`/`evening.png` for colour, type and plate feel (layout still the old one). Every text must be readable in both palettes; nothing uppercase; no leftover blue.

- [ ] **Step 7: Commit** `feat: Sofia Sans, paper and board palettes, flat plates`.

---

### Task 3: Scene palette on the server

**Files:**
- Modify: `internal/scenes/feed.go`, `internal/scenes/defaults.go`, `internal/scene/scene.go`, `internal/server/scenes.go`, `testdata/dashboard-scenes-v1.json`, `docs/FAMILY_APP.md`, `web/preview-data.mjs`
- Test: `internal/scenes/feed_test.go`, `internal/scenes/defaults_test.go`, `internal/scene/scene_test.go`, `internal/server/scenes_test.go`

**Interfaces:**
- Produces: `scenes.Scene.Palette string` (`json:"palette,omitempty"`, `""`, `"paper"` or `"board"`); `scene.DefaultPalette(since time.Time) string`; `scene.Current.Palette string` (`json:"palette,omitempty"`), always `"paper"` or `"board"` for a layout scene in `/api/dashboard` and `/api/scene`, empty for night.

- [ ] **Step 1: Write failing tests.**
  - `feed_test.go`: a feed whose layout scene has `"palette":"board"` parses to `Scene.Palette == "board"`; a scene without the field parses to `""`; `"palette":"neon"` fails with an error containing `unknown palette "neon"`; a night scene with a palette parses and its `Palette` is `""`.
  - `defaults_test.go`: `DefaultFeed()` palettes are morning/day/afternoon `"paper"`, evening `"board"`, night `""`.
  - `scene_test.go`: `DefaultPalette` returns `"board"` for 18:00, 21:29, 00:30, 05:59 and `"paper"` for 06:00, 09:00, 17:59 (times built in `Europe/Berlin`).
  - `scenes_test.go`: the resolved payload carries the feed scene's explicit palette; a feed scene without palette starting 19:00 resolves to `"board"`, one starting 09:00 to `"paper"`; a preview payload carries the previewed scene's palette by the same rule.
- [ ] **Step 2: Run** `go test ./internal/scenes/ ./internal/scene/ ./internal/server/` — Expected: FAIL (unknown field / undefined).
- [ ] **Step 3: Implement.** `sceneJSON` gains `Palette string`; `validateFeed` rejects values other than `""`, `"paper"`, `"board"` with `scenesErr("scene %q has unknown palette %q", id, palette)` and drops it for night scenes. `DefaultPalette` uses the hour of `since` in its own location: `>= 18 || < 6` gives `"board"`. Every place in `internal/server/scenes.go` (and the schedule/provider code it calls) that builds a `scene.Current` for a layout scene sets `Palette` to the scene's explicit palette, else `DefaultPalette(current.Since)`. The built-in `SCENE_*` schedule path (no feed) uses `DefaultFeed`'s palettes by scene id.
- [ ] **Step 4:** Add `"palette"` to the layout scenes in `testdata/dashboard-scenes-v1.json` exactly as in `DefaultFeed` (leave one layout scene without the field if the fixture has more than the defaults; otherwise all four carry it). Document the optional field in `docs/FAMILY_APP.md` next to the scenes feed description. In `web/preview-data.mjs`, set `scene.palette` from `opts.palette`, else `"board"` for `evening` and `"paper"` otherwise.
- [ ] **Step 5: Run** `go vet ./... && go test ./... && pnpm test` — Expected: PASS.
- [ ] **Step 6: Commit** `feat: palette per scene in the scenes feed`.

---

### Task 4: Person colour and full-day weather hours

**Files:**
- Modify: `internal/calendar/calendar.go` (`Person`), `internal/familyapp/calendars.go`, `internal/weather/weather.go`, `testdata/calendar-feed-v1.json`, `docs/FAMILY_APP.md`, `web/preview-data.mjs`, `tests/preview.test.mjs`
- Test: `internal/familyapp/calendars_test.go`, `internal/weather/weather_test.go`

**Interfaces:**
- Produces: `calendar.Person.Color string` (`json:"color,omitempty"`), reaching the frontend as `d.calendar.people[i].color`; `weather.Weather.Today []Hour` (`json:"today"`), reaching the frontend as `d.weather.today` (`[{time, temp, precipProb, icon}]`, 24 entries on a normal day).

- [ ] **Step 1: Write failing tests.**
  - `calendars_test.go`: a feed person with `"color":"#16a34a"` yields `Person.Color == "#16a34a"`; a person without `color` yields `""` and the feed is accepted; `"color":"green"` is rejected with an error containing `people[0]: color`.
  - `weather_test.go`: with a fake hourly response covering today 00:00–23:00 and tomorrow, and `now` = today 13:20 local, `Today` has 24 entries from 00:00 to 23:00 of today (temp, precipProb, icon filled) while `Hourly` still has the 12 hours starting 13:00. On the last Sunday of October (25 hours) `Today` has 25 entries.
- [ ] **Step 2: Run** `go test ./internal/familyapp/ ./internal/weather/` — Expected: FAIL.
- [ ] **Step 3: Implement.** Colour pattern `^#[0-9A-Fa-f]{6}$`. `Today` is filled from the same hourly arrays as `Hourly`, selecting entries whose local date equals `now`'s local date; `Hourly` logic is unchanged.
- [ ] **Step 4:** Add a `color` to each person in `testdata/calendar-feed-v1.json`; document both additions in `docs/FAMILY_APP.md` / `README.md` where the payloads are described. In `web/preview-data.mjs` add `weather.today` (24 hourly entries for `now`'s date: temperatures rising from 8° at 07:00 to 16° at 15:00 and falling to 9° at 21:00, `precipProb` 60 for 16:00–19:00 and 10 otherwise, icons `cloud`/`partly`/`rain` to match) and assert in `tests/preview.test.mjs` that `d.weather.today.length === 24`.
- [ ] **Step 5: Run** `go vet ./... && go test ./... && pnpm test` — Expected: PASS.
- [ ] **Step 6: Commit** `feat: person colour in the calendar feed, full-day weather hours`.

---

### Task 5: Clock tile

**Files:**
- Create: `web/static/wallfmt.js`, `tests/wallfmt.test.mjs`
- Modify: `web/static/index.html`, `web/static/app.js` (`tickClock`, `renderWeather`, `renderWaste`), `web/static/style.css`, `package.json`

**Interfaces:**
- Consumes: `d.weather.today` (Task 4), tokens and fonts (Task 2).
- Produces (top-level functions in `wallfmt.js`, loaded before `app.js`):
  - `rainOutlook(hours, now) -> string` — `hours`: `[{time: ISO string, precipProb: number}]`.
  - `wasteSentence(name, state, moved) -> string` — `state`: `"today" | "tomorrow"`.

- [ ] **Step 1: Write the failing test** `tests/wallfmt.test.mjs` (load `wallfmt.js` with `new Function` like `web/layout.test.mjs`). With `H = (probs) => probs.map((p, i) => ({ time: new Date(2026, 8, 30, i).toISOString(), precipProb: p }))` and `now = new Date(2026, 8, 30, 13, 18)`:
  - all 24 hours at 10 → `""`
  - hours 16–19 at 60, rest 10 → `"ab 16 Uhr Regen"`
  - hours 12–14 at 70, rest 10 → `"Regen bis 15 Uhr"`
  - hours 12–23 at 70 → `"Regen bis in die Nacht"`
  - rain only at hours 8–10 (already past) → `""`
  - exactly 50 counts as rain; 49 does not
  - `rainOutlook([], now)` and `rainOutlook(undefined, now)` → `""`
  - `wasteSentence("Gelbe Tonne", "tomorrow", false)` → `"Gelbe Tonne heute Abend rausstellen"`
  - `wasteSentence("Gelbe Tonne", "today", false)` → `"Gelbe Tonne wird heute abgeholt"`
  - `wasteSentence("Gelbe Tonne", "tomorrow", true)` → `"Gelbe Tonne heute Abend rausstellen (verlegt)"`
- [ ] **Step 2: Run** `node tests/wallfmt.test.mjs` — Expected: FAIL.
- [ ] **Step 3: Implement `wallfmt.js`.** `rainOutlook` looks only at hours of `now`'s local date from `now`'s hour onward; threshold `>= 50`. Add the script tag before `app.js` and the test to the `package.json` chain.
- [ ] **Step 4: Rework the clock tile markup and render code.** `index.html`: inside `.cc-clock` the date becomes two elements, `#weekday` and `#date`; the waste container moves below the weather line (still `#waste`). `tickClock` writes `fmt.weekday` to `#weekday` and `fmt.dayMonth` to `#date` (night clock unchanged, still the long date). `renderWeather`: the "now" block is icon, temperature in heavy weight, then `c.label` and, when non-empty, `rainOutlook(w.today || w.hourly, new Date())` joined as `"<label>, <outlook>"`; the `gefühlt … · Wind …` line is dropped; the five-day row and the hints stay. `renderWaste`: each bin renders as `<span class="bin bin--<state>" style="--c:<color>"><i></i><b><name></b> <rest of sentence></span>` using `wasteSentence` (name bold, rest soft); keep the only-touch-DOM-on-change guard; the pulse animation and `BIN_SVG` go away.
- [ ] **Step 5: Style it** after the mockup header: `.time` in `SofiaX` 800, `line-height: 0.74`, sized to the tile with container units (`.clock-card { container-type: size; }`, `.time { font-size: min(92cqh, 46cqw) }` in the compact layout and `min(125cqh, 30cqw)` under `.board.wide-top`); `#weekday` `Sofia` 800 and `#date` `Sofia` 300 at the same size (3.55rem wide layout, 2rem compact); the wide layout is the mockup's two columns (clock left, date/weather/waste right, bottom-aligned). Without a photo the tile has no background and no padding box (it sits on the sheet): remove the `--wx-grad` gradients and the `[data-wx]` light-text rules, and scope light text plus scrim to `.clock-card.wx-photo`. Hide `.wx-days` with `@container (max-height: 17rem)`. `.bin i` is the small bin mark from the mockup (`.bins i`).
- [ ] **Step 6: Run** `pnpm test` — Expected: PASS. **Visual check:** screenshots of `?scene=afternoon` (compact clock 6x5), `?scene=morning` (wide 12x5, hints visible) in both palettes: clock is the dominant element, nothing overflows the tile, date and weather line readable, waste sentence shows.
- [ ] **Step 7: Commit** `feat: clock tile with condensed clock, rain outlook and waste sentence`.

---

### Task 6: Children's cards

**Files:**
- Modify: `web/static/wallfmt.js`, `tests/wallfmt.test.mjs`, `web/static/app.js` (`lessonRow`, `studentCard`, `kidPlan`, `schoolCalendarCard`, `mealCard`, `todoRow`, `renderSchoolRow` → `extras`), `web/static/style.css`, `web/preview-data.mjs`

**Interfaces:**
- Consumes: `d.calendar.people[].color` (Task 4).
- Produces: `inkOn(hex) -> "#FFFFFF" | "#14211B"` in `wallfmt.js`; CSS class `.nameplate` with `--c` (fill) and `--on` (text); used again by Tasks 7 and 8.

- [ ] **Step 1: Add failing tests** to `tests/wallfmt.test.mjs`: `inkOn("#2460D8")`, `inkOn("#D43F7A")`, `inkOn("#7A4BD6")`, `inkOn("#14211B")`, `inkOn("#e11d48")`, `inkOn("#16a34a")` → `"#FFFFFF"`; `inkOn("#F2B705")`, `inkOn("#FFE9A8")`, `inkOn("#E8EDE4")`, `inkOn("#fff")` → `"#14211B"`; `inkOn("")`, `inkOn(undefined)`, `inkOn("var(--accent)")` → `"#FFFFFF"`.
- [ ] **Step 2: Run** `node tests/wallfmt.test.mjs` — Expected: FAIL.
- [ ] **Step 3: Implement `inkOn`:** parse `#RGB`/`#RRGGBB`, WCAG relative luminance, dark ink when luminance > 0.4; anything unparsable gives white.
- [ ] **Step 4: Person colour.** In `renderSchoolRow`'s `extras`, when `d.calendar?.central` and the child resolves to a person id (the same id `schoolCalsFor` derives: `personId`, else the binding lookup), and `d.calendar.people` has that person with a `color`, that colour wins over calendar and meal colour. A child without any colour uses `var(--text)`.
- [ ] **Step 5: Markup.** Card heads (`studentCard`, `kidPlan`, `schoolCalendarCard`, `mealCard`) become `<div class="kid-head"><span class="nameplate" style="--c:<color>;--on:<inkOn(color)>"><name></span><span class="meta">…</span></div>`. The name is rendered as given (no JS uppercasing). `lessonRow`: drop the `.nr` column; status words replace `.tag` badges: cancelled → `<em class="st off">entfällt</em>` (`entfiel` when the lesson is past), substitution → `<em class="st sub">Vertretung</em>`; `l.tag` → `<em class="st">…</em>`; the next upcoming lesson of today (first lesson that is neither past nor cancelled) gets class `next`. Sub-heading "To-dos" becomes "Aufgaben". `todoRow`: points as `<span class="pts">5 ★</span>` in a right column; pending → the title followed by `, wartet auf Bestätigung` in soft text and a dashed box; due words (`überfällig`, `heute fällig`, `morgen fällig`) as `<em class="st off">`/`<em class="st sub">`. Exam and homework rows lose the `·` separator (subject bold, rest plain/soft). In `web/preview-data.mjs`, give the demo `calendar` `central: true`, `people` with colours and the bindings needed so Lukas resolves to a person colour, if that is possible without breaking the demo; otherwise leave the demo in local mode and say so in the report.
- [ ] **Step 6: Style** after the mockup's `.kid`, `.nameplate`, `.lessons`, `.rows`, `.todo`, `.notice`: name plate `SofiaX` 800, 2.3rem, flush to the card's top-left corner with only the bottom-right corner rounded, card padding moved to the body; `.notice` uses `--notice`/`--notice-ink`; `.st.off` in `--danger`, `.st.sub` in `--warn`; `.lesson.next` 800 weight in `--text`. `.kid-stack` cards (morning/evening) use the same name plates.
- [ ] **Step 7: Run** `pnpm test` — Expected: PASS. **Visual check:** `?scene=afternoon` (side-by-side cards) and `?scene=morning` (stacked) in both palettes against `day.png`: plates, status words, no badges, nothing clipped mid-row.
- [ ] **Step 8: Commit** `feat: name plates and status words on the children's cards`.

---

### Task 7: Day band tile

**Files:**
- Create: `web/static/dayband.js`, `tests/dayband.test.mjs`
- Modify: `internal/layout/widgets.go`, `internal/layout/widgets_test.go`, `web/static/layout.js`, `web/layout.test.mjs`, `web/static/index.html`, `web/static/app.js`, `web/static/style.css`, `web/preview-data.mjs`, `package.json`

**Interfaces:**
- Consumes: `d.weather.today` (Task 4), `inkOn` (Task 6), the per-child data `renderSchoolRow` already assembles.
- Produces: widget type `dayband` (`layout.WidgetDayband`, `MinW: 8, MinH: 4`, appended to the catalog after `calendars`; `EL.dayband = "dayband"`); functions in `dayband.js`:
  - `DAYBAND_FROM = 6`, `DAYBAND_TO = 22`
  - `daybandLanes({kids, calendars, events, now}) -> [{name, color, rows, segments: [{start, end, label, kind, row, side}]}]`
    - `kids`: `[{name, color, day: {date, noSchool, notices, lessons: [{start: "HH:MM", end: "HH:MM", status}]}, events: [calendar event]}]`
    - `calendars`: the calendar list of the payload (`id`, `name`, `color`, `panel`, `into`); `events`: the event list (`cal`, `allDay`, `start`, `end`, `title`)
    - `start`/`end`: decimal hours within 6..22; `kind`: `"school" | "event"`; `row`: 0 or 1; `rows`: 1 or 2; `side`: `"in" | "right" | "left"`
  - `daybandWeather(hours, now) -> [{hour, temp, icon, past}]`
  - `daybandRain(hours) -> [{start, end}]`
  - `daybandNow(now) -> number | null` (fraction 0..1 of the window)

- [ ] **Step 1: Write the failing test** `tests/dayband.test.mjs`. `now = new Date(2026, 8, 30, 13, 18)`; `iso(h, m = 0, dayOffset = 0)` builds local ISO strings for that date. Cases:
  - kid with lessons 07:30–08:15, 08:25–09:10 (status `cancelled`), 13:40–14:15 and `day.date` today → one lane, one segment `{start: 7.5, end: 14.25, label: "Schule bis 14:15", kind: "school", row: 0}`.
  - last lesson cancelled (07:30–08:15 ok, 08:25–09:10 cancelled) → end 8.25, label `"Schule bis 08:15"`.
  - all lessons cancelled, `day.noSchool: true`, a notice matching `/ferien/i`, or `day.date` tomorrow → no school segment; a kid without any segment produces no lane.
  - kid event 15:00–15:00 (zero length) titled "Abholung" → segment `{start: 15, end: 15.25, label: "Abholung", kind: "event"}` in the kid's lane.
  - calendars Familie (id 0, column), Feiertage (id 1, `into: 0`), Hannah (id 2, panel `school`), Mama (id 3, column, no events today); events: Familie 11:00–12:00 "Zahnarzt", Familie 18:30–20:30 "Elternabend 3b", Feiertage 10:00–11:00 "Umzug", Familie all-day today, Familie tomorrow 09:00–10:00, Hannah-calendar 16:00–17:00 → exactly one calendar lane named "Familie" with three segments (Umzug, Zahnarzt, Elternabend 3b, sorted by start); no lane for Mama; the school-panel calendar makes no lane of its own.
  - clipping: event 05:00–07:00 → `{start: 6, end: 7}`; 21:00–23:30 → `{start: 21, end: 22}`; 04:00–05:30 and 22:15–23:00 → dropped; event yesterday 20:00 to today 08:00 → `{start: 6, end: 8}`.
  - overlap: 10:00–12:00 and 11:00–13:00 in one lane → rows 0 and 1, lane `rows: 2`; a third overlapping 11:30–12:30 → also row 1.
  - `side`: label "Schule bis 14:15" on 7.5–14.25 → `"in"`; "Zahnarzt" on 11–12 → `"right"`; "Elternabend 3b" on 18.5–20.5 → `"left"` (rule: `need = label.length * 0.17`; `in` if `need <= end - start`, else `right` if `end + need <= 22`, else `left`).
  - lane order: kids in input order, then calendar lanes in calendar order.
  - `daybandWeather(H24, now)` (24 hourly entries) → 8 slots for hours 7, 9, …, 21; `past` true for 7, 9, 11, 13 and false from 15; missing hours are skipped; `daybandWeather(undefined, now)` → `[]`.
  - `daybandRain`: probs ≥ 50 at hours 16, 17, 18, 19 → `[{start: 16, end: 20}]`; rain at 4–7 → `[{start: 6, end: 8}]`; two separate runs give two entries.
  - `daybandNow(now)` ≈ `(13.3 - 6) / 16` (±0.001); 05:59 and 22:00 → `null`.
- [ ] **Step 2: Run** `node tests/dayband.test.mjs` — Expected: FAIL.
- [ ] **Step 3: Implement `dayband.js`** as pure top-level functions (no DOM). Holiday detection reuses the rule of `onHoliday` in `app.js` (`noSchool` or a notice matching `/ferien|schulfrei/i`).
- [ ] **Step 4: Catalog and layout.** Add `WidgetDayband` to `internal/layout/widgets.go` and the expected list in `widgets_test.go` (`{Type: WidgetDayband, MinW: 8, MinH: 4}` after calendars). Add `dayband: "dayband"` to `EL` in `layout.js` and extend `web/layout.test.mjs` (`allTypes`, hidden lists, one placement case for `dayband`). Add `<section class="panel dayband" id="dayband"></section>` to `index.html` after the clock card, and the script tag before `app.js`.
- [ ] **Step 5: Render.** `renderSchoolRow` already resolves each child's colour and school-calendar events; factor that resolution into a function both it and the new `renderDayband(d)` call (so the school row's behaviour is unchanged), and build `kids` from it. `renderDayband` returns whether there is anything to show (`lanes.length > 0 || weather slots > 0`) and `render()` passes it as `has.dayband`. DOM as in the mockup's `.band`: axis row, grid lines, weather lane with `.wxslot`/`.rainbar`, one `.who` + `.track` per lane, bars positioned with `--s`/`--e` custom properties, `--c` and `--on` (via `inkOn`) per lane, `.nowline` positioned from `daybandNow` (absent when `null`). A lane with `rows: 2` is twice as tall.
- [ ] **Step 6: Style** after the mockup's `.band` rules, with lane heights in rem and the track width fluid; lanes share the tile height evenly but never exceed 2.6rem per row; overflow hidden.
- [ ] **Step 7: Demo.** In `web/preview-data.mjs` change `demoScenes.afternoon` to: clock `{x:0,y:0,w:12,h:3, props:{wide:true,hints:false}}`, dayband `{x:0,y:3,w:12,h:4}`, school `{x:0,y:7,w:12,h:8, props:{mode:"full",mealDay:"today"}}`, calendars `{x:0,y:15,w:12,h:9}` (ids = types) and add `node tests/dayband.test.mjs` to the test chain.
- [ ] **Step 8: Run** `go vet ./... && go test ./... && pnpm test` — Expected: PASS. **Visual check:** `?scene=afternoon` in both palettes against the mockup band: lanes for Lukas, Hannah, Familie, Mama; now line at the right spot; labels not cut off; clock tile still fine at 12x3.
- [ ] **Step 9: Commit** `feat: day band tile`.

---

### Task 8: Week tile

**Files:**
- Create: `web/static/week.js`, `tests/week.test.mjs`
- Modify: `internal/layout/widgets.go`, `internal/layout/widgets_test.go`, `web/static/layout.js`, `web/layout.test.mjs`, `web/static/index.html`, `web/static/app.js`, `web/static/style.css`, `web/preview-data.mjs`, `package.json`

**Interfaces:**
- Produces: widget type `week` (`layout.WidgetWeek`, `MinW: 6, MinH: 5`, appended after `dayband`; `EL.week = "week"`); in `week.js`:
  `weekRows({calendars, events, days, now}) -> {columns: [{id, name, color}], rows: [{date: "YYYY-MM-DD", label, sub, isToday, free, cells: [{allDay: [{title}], timed: [{time, title, location, past}]}]}]}` — `cells[i]` belongs to `columns[i]`.

- [ ] **Step 1: Write the failing test** `tests/week.test.mjs`, `now = new Date(2026, 8, 30, 13, 18)` (a Wednesday), `days: 7`, calendars Familie (id 0), Dienst (id 1), Mama (id 2), Feiertage (id 3, `into: 0`), Hannah (id 4, panel `school`):
  - columns are Familie, Dienst, Mama (in that order, with their colours).
  - today's row exists even with no events: `label: "Heute"`, `sub: "Mi 30.9."`, `isToday: true`, three cells with empty arrays.
  - tomorrow with an event: `label: "Morgen"`, `sub: "Do 1.10."`; a later day: `label: "Freitag"`, `sub: "2.10."`.
  - days after today without any event in any column are not in `rows`.
  - Sunday 4 Oct with an event: `free: true`; all other rows `free: false`.
  - a Feiertage all-day event on 3 Oct ("Tag der Deutschen Einheit", `startDate: "2026-10-03"`, `endDate: "2026-10-04"`) lands in the Familie cell's `allDay`; a multi-day all-day event 5–7 Oct appears on the 5th and the 6th.
  - timed event today 07:30–08:00 → `{time: "07:30", past: true}`; today 18:30–20:00 → `past: false`; a zero-length event today at 12:00 → `past: true`; an event that started yesterday 22:00 and ends today 15:00 → `time: "…"`, `past: false`.
  - timed entries in a cell are sorted by start; `location` is passed through, `""` when missing.
  - events of the school-panel calendar appear nowhere.
  - `weekRows({calendars: [], events: [], days: 7, now})` → `{columns: [], rows: []}`.
- [ ] **Step 2: Run** `node tests/week.test.mjs` — Expected: FAIL.
- [ ] **Step 3: Implement `week.js`** (pure). Column rule mirrors `renderCalendars` in `app.js`: `panel !== "school"` and not merged (`!(into >= 0)`); merged calendars feed their target column. Day membership mirrors `eventsOn` in `app.js`.
- [ ] **Step 4: Catalog and layout** as in Task 7 Step 4 for `week` (`<section class="panel week" id="week"></section>` before the calendars section; script tag; `allTypes` and expectations in `web/layout.test.mjs`).
- [ ] **Step 5: Render** `renderWeek(d)` in `app.js` from `weekRows({calendars, events, days: d.days || 7, now: new Date()})`: header row with a colour swatch and the column name in `SofiaX`; per row a day cell and one cell per column; classes `is-today`, `free`, `done` (past) as in the mockup's `.week`. Returns `columns.length > 0` for `has.week`. The grid's column template is set inline: `178px`-equivalent in rem (10.2rem) for the day cell, then `repeat(<n>, minmax(0, 1fr))`.
- [ ] **Step 6: Style** after the mockup's `.week`, `.ev`, `.allday`, `.day` rules; the tile clips at the bottom with the existing soft fade (`.fade` mask).
- [ ] **Step 7: Demo.** In `demoScenes.afternoon` replace the calendars widget with week `{x:0,y:15,w:12,h:7}` and add shopping `{x:0,y:22,w:12,h:2}` (the shopping tile will look cramped until Task 9); add `node tests/week.test.mjs` to the test chain.
- [ ] **Step 8: Run** `go vet ./... && go test ./... && pnpm test` — Expected: PASS. **Visual check:** `?scene=afternoon` in both palettes against the mockup's week: rows aligned across columns, today tinted, all-day lines, no clipped text; `?scene=day` still shows the old calendars tile correctly.
- [ ] **Step 9: Commit** `feat: week matrix tile`.

---

### Task 9: Shopping strip, built-in afternoon scene, docs

**Files:**
- Modify: `internal/layout/widgets.go`, `internal/layout/widgets_test.go`, `web/static/layout.js`, `web/layout.test.mjs`, `web/static/app.js` (`applyScene`, `renderShopping`), `web/static/style.css`, `internal/scenes/defaults.go`, `internal/scenes/defaults_test.go`, `testdata/dashboard-scenes-v1.json`, `web/preview-data.mjs`, `README.md`, `docs/ROADMAP.md`, `docs/FAMILY_APP.md`, `docs/screenshot.png`

**Interfaces:**
- Produces: shopping minimum `MinW: 4, MinH: 2`; `resolveLayout` placements gain `flat: boolean` (`h <= 2`); `[data-flat]` on a flat tile; `DefaultFeed()` afternoon scene = clock `{0,0,12,3}` props `wide:true, hints:false`; dayband `{0,3,12,4}`; school `{0,7,12,8}` props `mode:"full", mealDay:"today"`; week `{0,15,12,7}`; shopping `{0,22,12,2}`; widget ids equal their types. The family app mirrors exactly these values.

- [ ] **Step 1: Write failing tests.** `widgets_test.go`: shopping is `MinW: 4, MinH: 2`. `web/layout.test.mjs`: every expected placement gains `flat` (`true` for a widget with `h: 2`, `false` for `h: 3`). `defaults_test.go`: the afternoon scene has exactly the five widgets above with those rectangles and props, and `DefaultFeed()` still validates.
- [ ] **Step 2: Run** `go test ./internal/layout/ ./internal/scenes/ && node web/layout.test.mjs` — Expected: FAIL.
- [ ] **Step 3: Implement** catalog, `flat` in `resolveLayout`, `el.toggleAttribute("data-flat", p.flat)` in `applyScene`, the new `DefaultFeed` afternoon scene; update the afternoon scene in `testdata/dashboard-scenes-v1.json` to match if the fixture contains it; make `demoScenes.afternoon` equal the new default.
- [ ] **Step 4: Shopping markup and style.** `renderShopping`: title is the list name in `SofiaX` with the count in soft weight; items are an inline flow (`<ul class="shop-items">`, name 600, amount soft), no chips. `[data-flat]` lays title and items out in one row as in the mockup's `.shop`, items clipped to the tile height; the "+ n weitere" line and the hint text hide in the flat layout.
- [ ] **Step 5: Run** `go vet ./... && go test ./... && pnpm test` — Expected: PASS. **Visual check:** `?scene=afternoon` in both palettes side by side with `day.png` / `evening.png`: same structure top to bottom (clock, day band, children, week, shopping strip). Also check `?scene=morning`, `?scene=day`, `?scene=evening`, `?scene=night` for leftovers of the old look or broken tiles, and fix what is broken.
- [ ] **Step 6: Docs.** Replace `docs/screenshot.png` with the paper afternoon screenshot. Update `README.md` (layout sketch for the afternoon scene, a short "Preview" section with the `node web/preview.mjs` command, the new tiles `dayband` and `week`, palettes, Sofia Sans/OFL note), `docs/ROADMAP.md` (tick/add the redesign items) and the widget catalog and default-scene description in `docs/FAMILY_APP.md`.
- [ ] **Step 7: Commit** `feat: shopping strip, redesigned afternoon scene, docs`.
