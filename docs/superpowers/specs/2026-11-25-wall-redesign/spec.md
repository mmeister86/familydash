# Wall redesign: Familienplaner

The wall display moves from a dark card grid organised by data source to a
family planner organised by people and time. The visual reference is
`mockup.html` in this folder (open it in a browser at 1080x1920; add
`?scene=evening` for the dark palette). `day.png` and `evening.png` are
renders of it. The mockup is binding for look and proportions; this text is
binding for behaviour and data.

## Constraints that do not change

- Go standard library only, no Node build step, frontend is plain
  HTML/CSS/JS embedded in the binary. Fonts ship inside `web/static/fonts/`.
- Portrait kiosk on a Raspberry Pi: animate only `opacity` and `transform`.
- Tiles are still placed on the shared 12x24 grid by the scenes feed.
- The night scene keeps its current look.
- UI strings are German, code and comments are English.
- A widget with nothing to show hands its tile to its fallback, as today.

## Look

- **Type:** Sofia Sans (OFL) in three widths, self-hosted variable woff2
  files from `fonts/` in this folder. Font-face names: `Sofia` (normal,
  running text), `SofiaC` (condensed, dense rows), `SofiaX` (extra condensed,
  clock, name plates, tile titles). No uppercase-via-CSS labels with letter
  spacing; headings are sentence case. Numbers are tabular.
- **Palettes:** two, switched with `data-palette` on `<html>`.
  The existing custom-property names stay; only their values change.

  | Property | `paper` (default) | `board` |
  |---|---|---|
  | `--bg` | `#E6ECE5` | `#101D17` |
  | `--panel` | `#FAFCF8` | `#192A22` |
  | `--text` | `#14211B` | `#E8EDE4` |
  | `--muted` | `#55645B` | `#9BAA9F` |
  | `--faint` | `#93A097` | `#62746A` |
  | `--line` | `#CDD7CE` | `#2A3F34` |
  | `--danger` | `#C4372C` | `#FF7B6B` |
  | `--warn` | `#8F5B00` | `#F2B705` |
  | `--rain` | `#2F7FD1` | `#6FB2F2` |
  | `--sun` | `#E0A000` | `#E0A000` |
  | `--good` | `#2E9E6B` | `#46C28E` |
  | `--notice` | `#FFE9A8` | `#4A3B0C` |
  | `--notice-ink` | `#4A3600` | `#FFE39A` |

  `--accent` equals `--text` (emphasis is weight, not a blue accent).
  `--panel-2` is `color-mix(in srgb, var(--text) 6%, var(--panel))`.
- **Plates:** tiles are flat plates: `--panel` background, no border, no
  shadow, radius 6px. No gradients as decoration.
- **Colour belongs to people and calendars.** A child's card carries a name
  plate filled with the child's colour; the same colour fills the child's
  bars in the day band. Text on a colour fill is chosen for contrast
  (dark ink on light fills, white on dark fills).
- **Status is a word, not a badge.** "entfällt", "Vertretung",
  "wartet auf Bestätigung", "überfällig" are coloured text in the row.

## Palette per scene

A layout scene may carry `palette: "paper" | "board"` in the scenes feed.
Without it the wall picks `board` when the scene's current start is at or
after 18:00 or before 06:00 local time, otherwise `paper`. Built-in default
scenes: morning, day, afternoon = `paper`; evening = `board`.

## Clock tile

Extra-condensed clock, weekday in heavy weight above the date in light
weight, current temperature and condition, then a one-line rain outlook
("ab 16 Uhr Regen", "Regen bis 15 Uhr") when rain is likely (probability at
least 50 %). Waste collection is a sentence with a small coloured bin mark:
"Gelbe Tonne heute Abend rausstellen" (collected tomorrow),
"Gelbe Tonne wird heute abgeholt" (collected today, until 12:00). The
five-day forecast stays but hides when the tile is too short for it. A
weather photo (`WEATHER_BG_DIR`), when present, still fills the tile behind
light text; without a photo the tile is the plain sheet, with no gradients.

## Children's cards

Name plate (`SofiaX`, child's colour) in the top-left corner, the day label
on the right. Timetable rows: time, subject, room; past lessons faint, the
running or next lesson heavy. Sub-headings in sentence case: "Arbeiten",
"Hausaufgaben", "Aufgaben" (was "To-dos"), "Termine", "Essen". Points show
as "5 ★" at the right edge of a task row.

With the central calendar feed, a child's colour is the person's colour from
the family app (`people[].color`) when the feed carries one; otherwise the
colour of their school calendar or lunch account, as today.

## Day band (new widget `dayband`, minimum 8x4 tiles)

One time axis from 06:00 to 22:00 for the weather and everyone's day.

- Axis labels every two hours; a vertical grid line at each.
- Weather lane: icon and temperature every two hours (07, 09 … 21), past
  slots faint; a thin blue bar under the hours with rain probability of at
  least 50 %.
- One lane per child with school today: a bar from the first to the last
  lesson that is not cancelled, labelled "Schule bis HH:MM". Timed entries
  of the child's school calendar today are further bars in the same lane.
- One lane per calendar column (calendars merged into a column share its
  lane) with today's timed events. All-day events do not appear.
- Lanes with nothing today are left out. Bars are clipped to the window;
  events entirely outside it are left out. An event shorter than 15 minutes
  shows as 15 minutes.
- A bar's label sits inside the bar when it fits, otherwise right of it,
  otherwise left of it.
- Two bars that overlap in one lane go on two rows of that lane.
- A vertical "now" line with a round cap marks the current time; it is
  absent outside the window.
- The tile is empty (falls back) only when there are neither lanes nor
  weather.

## Week (new widget `week`, minimum 6x5 tiles)

A matrix: rows are days, columns are the calendar columns (same column rules
as the existing calendars tile). Today is always shown and tinted; later
days appear only when some column has an entry. Day cell: "Heute" /
"Morgen" / weekday name plus the short date. Sundays have a red day label.
All-day entries are a line without a time; timed entries are time plus
title, location in small type below; past entries of today are faint.

The existing `calendars` tile stays available and only takes the new look.

## Shopping

The list is flowing text (name heavy, amount soft), not chips. In a tile at
most two grid rows tall it becomes a single strip: title and count on the
left, items flowing to the right. Minimum size drops to 4x2.

## Built-in afternoon scene

clock 12x3 (wide) at y=0, dayband 12x4 at y=3, school 12x8 (mode full) at
y=7, week 12x7 at y=15, shopping 12x2 at y=22; palette `paper`.

## Data additions

- Weather payload gets `today`: every hour of the current local day, next
  to the existing `hourly` (next 12 hours), which keeps its meaning.
- Calendar feed people may carry `color` (`#RRGGBB`).

## Preview harness

`node web/preview.mjs` serves the static frontend with demo data relative
to the current date, so any scene and palette can be opened and
screenshotted without a configured server.

## Not in scope

Public-holiday detection for the week tile (only Sundays are red), touch
interaction, changes to the night scene, changes to the AI briefing content.
