# familydash

A lightweight family wall dashboard: **Google Calendar**, **weather**, the **school timetable** from beste.schule and the **Bring! shopping list** and the **school lunch** ordered at VielfaltMenü on one screen.
Runs as a single static Go binary in an ~8 MB container on Unraid; a Raspberry Pi only runs a browser in kiosk mode.

<img src="docs/screenshot.png" alt="Wall display (portrait)" width="420">

- Go standard library only — no dependencies, no database, no Node build step
- Frontend is plain HTML/CSS/JS, embedded in the binary
- Reloads itself on the wall display when you deploy a new image
- Keeps the last good data per source, so a flaky feed never blanks the screen

```
 Google Calendar ──(iCal secret URL, 5 min)──┐
 Open-Meteo      ──(15 min)──────────────────┤
 beste.schule    ──(API token, 15 min)───────┼──▶ familydash (Unraid, :8080) ◀── Chromium kiosk (Raspberry Pi)
 Bring!          ──(login, 2 min)────────────┤
 VielfaltMenü    ──(login per child, 30 min)─┘
```

## Quick start (Unraid)

1. Every push to `main` builds `ghcr.io/mmeister86/familydash` (amd64 + arm64) via GitHub Actions.
2. **Either** copy `deploy/unraid/my-familydash.xml` to `/boot/config/plugins/dockerMan/templates-user/` and add the container via *Docker → Add Container*,
   **or** use the Compose Manager plugin with `docker-compose.yml` + `.env` (see `.env.example`).
3. Open `http://<unraid-ip>:8080`.

## Configuration

All settings are environment variables.

| Variable | Default | |
|---|---|---|
| `TZ` | `Europe/Berlin` | Display time zone |
| `CALENDAR_n_URL` | – | iCal URL, `n` = 1…20. `webcal://` is accepted |
| `CALENDAR_n_NAME` | `Kalender n` | |
| `CALENDAR_n_COLOR` | palette | Hex color for bars/chips |
| `CALENDAR_n_PANEL` | `column` | `column` = own column in the calendar row, `school` = card next to beste.schule (next 3 weeks) |
| `CALENDAR_n_COLUMN` | – | Number `m` of another calendar: show this one **inside calendar m's column** instead of its own (entries keep their own color), e.g. public holidays in the family column |
| `CALENDAR_DAYS` | `7` | Days shown (today + n-1) |
| `CALENDAR_REFRESH` | `5m` | Go duration |
| `WEATHER_LAT` / `WEATHER_LON` | – | Weather is off until set |
| `WEATHER_NAME` | – | Label above the weather panel |
| `WEATHER_REFRESH` | `15m` | |
| `BESTESCHULE_TOKEN` | – | Personal Access Token; school panel is off until set |
| `BESTESCHULE_STUDENTS` | all | Comma-separated first names or ids to show |
| `BESTESCHULE_REFRESH` | `15m` | |
| `BESTESCHULE_URL` | `https://beste.schule/api` | |
| `WASTE_n_NAME` | – | Bin name, `n` = 1…10, e.g. `Gelbe Tonne`; waste chips are off until set |
| `WASTE_n_DAY` | – | Collection weekday: `Mo` … `So` (also `mittwochs`, `Wed`) |
| `WASTE_n_WEEKS` | every week | `gerade` / `ungerade` ISO calendar week, or `jede` |
| `WASTE_n_COLOR` | grey, yellow, blue … | Dot color |
| `WASTE_HOLIDAY_SHIFT` | `on` | Move a collection one day later per weekday public holiday (Saxony) earlier in the same week; `off` to disable |
| `TIMETABLE_FILE` | built-in | Path to a JSON timetable (see *Fixed timetable*); `off` disables it |
| `BRING_EMAIL` / `BRING_PASSWORD` | – | Bring! account; shopping panel is off until set |
| `BRING_LIST` | account default | Name of the list to show, e.g. `Zuhause` |
| `BRING_LOCALE` | `de-DE` | Language for catalog item names |
| `BRING_REFRESH` | `2m` | |
| `VIELFALT_n_USER` / `VIELFALT_n_PASSWORD` | – | VielfaltMenü login (Kundennummer + password) of child `n` = 1…6; lunch cards are off until set |
| `VIELFALT_n_NAME` | name from the portal | Card title, e.g. `Lukas` |
| `VIELFALT_n_COLOR` | palette | Dot color of the card |
| `VIELFALT_REFRESH` | `30m` | |
| `LISTEN_ADDR` | `:8080` | |

Values may be wrapped in quotes (`KEY="value"`) – they are stripped, since `docker --env-file` would otherwise keep them.

### Layout

Built for a **portrait** wall display (landscape works too):

```
┌──────────────┬──────────────┐
│ clock+weather│ Bring! list  │
├──────────────┼──────────────┤
│ beste.schule │ school cal.  │   one card per child / per CALENDAR_n_PANEL=school
├──────────────┼──────────────┤
│ lunch child 1│ lunch child 2│   one card per VIELFALT_n_*
├────────┬─────┴──┬───────────┤
│ cal 1  │ cal 2  │ cal 3 …   │   one column per CALENDAR_n_PANEL=column
└────────┴────────┴───────────┘
```

For a child without beste.schule, keep their school dates in a Google calendar and set `CALENDAR_n_PANEL=school`.

**One card per child:** a `CALENDAR_n_PANEL=school` calendar and a `VIELFALT_n_*` lunch account whose name matches a child's card
(beste.schule or fixed timetable) move into that card, ordered *school → Termine → Essen*. Names match case-insensitively, and a
single first name also matches a full name (`Lukas` ↔ `Meister Lukas`). Anything without a matching child keeps a card of its own.

### Waste collection

For districts that only publish a printable plan ("mittwochs gerade Kalenderwoche"): one `WASTE_n_*` set per bin.
The clock card shows a chip for each bin collected **tomorrow** (put it out tonight) – nothing otherwise.
Holiday shifts follow the common rule (Saxon public holidays, +1 day per holiday on or before the collection day in that week) –
check it against your district's announcements around holidays.

### Fixed timetable (school without beste.schule)

`internal/timetable/stundenplan.json` holds a weekly plan that is compiled into the image and shows up as the same card as a beste.schule child:
lesson times, subjects per weekday (`""` = free period, `{"A": "Werken", "B": "Kunst"}` = alternating weekly, counted from `weekA`), extras such as afternoon clubs (`"tag": "GTA"`) and holidays (`noSchool`).
It switches to the next school day after the last lesson, like beste.schule. With `"calendar": "<name>"` the upcoming entries of the
`CALENDAR_n_PANEL=school` calendar with that name appear at the bottom of the card instead of in a card of their own.
Edit the file and push, or mount your own and point `TIMETABLE_FILE` at it.

To show two calendars in one column, point the second one at the first: `CALENDAR_5_COLUMN=1` puts calendar 5 into calendar 1's column.
Good for a holiday feed next to the family calendar, e.g. `https://www.feiertage-deutschland.de/kalender-download/ics/feiertage-deutschland.ics`.

### Google Calendar

Google Calendar (web) → ⚙ Settings → pick the calendar → **Integrate calendar** → *Secret address in iCal format*.
Paste that into `CALENDAR_1_URL`. One variable per calendar (family, school, work…).
No OAuth, no Google Cloud project. Treat the URL like a password.

The parser handles what Google/iCloud/Outlook export in practice: time zones, all-day and multi-day events,
`RRULE` (daily/weekly/monthly/yearly, `INTERVAL`, `COUNT`, `UNTIL`, `BYDAY` incl. `2TU`/`-1FR`, `BYMONTHDAY`, `BYMONTH`, `BYSETPOS`),
`EXDATE`, moved/cancelled single instances (`RECURRENCE-ID`) and DST changes. See `internal/calendar/ics_test.go`.

Note: Google refreshes the secret iCal feed itself only every few minutes to hours; that delay is on Google's side.

### beste.schule

beste.schule → user menu (top right) → **API** → create a *Personal Access Token* → `BESTESCHULE_TOKEN`.
Works with a parent account; each child gets its own card.

Per child the dashboard shows:

- **Timetable** for today while school is running, afterwards for the next school day (weekends, holidays and `no_school_dates` are skipped)
- **Cancellations and substitutions** from the substitution plan inline (struck through / "Vertretung", incl. room changes) plus day notices
- **Exams** (Klassenarbeit, Leistungskontrolle, Test …) for the next 3 weeks and **homework** for the next 2 weeks from the class journal

The beste.schule API returns nested, loosely structured data, so the parser is deliberately tolerant
(modelled on the Home Assistant integration [RF1705/beste-schule](https://github.com/RF1705/beste-schule)).
If something looks wrong for your school, inspect what the API returns:

```sh
docker exec familydash /familydash -besteschule-preview   # what the dashboard would show
docker exec familydash /familydash -besteschule-dump      # raw API responses (contains personal data!)
```

### Bring!

Set `BRING_EMAIL`, `BRING_PASSWORD` and optionally `BRING_LIST`. This needs a classic e-mail/password login –
if you sign in to Bring! with Apple/Google only, the login will fail.

Bring! has **no public API** – this uses the endpoints of the Bring! apps as documented by the community
library [bring-api](https://github.com/miaucl/bring-api) (also used by Home Assistant). It may break when Bring! changes something;
only the shopping panel is affected then.

### VielfaltMenü (school lunch)

Set `VIELFALT_1_USER` (Kundennummer), `VIELFALT_1_PASSWORD` and `VIELFALT_1_NAME`, then the same with `_2_` for the next child.
Each child has its own portal account. Per child the card shows the next two delivery days (today until 14:00, then from tomorrow on):
the ordered dish (sides after `|` in small print) or **"Noch nichts bestellt!"** in orange when a day with menus has no order yet.

There is **no public API** – this logs in like the parent portal (`POST bestellung.vielfaltmenue.com/frontend/login` → token)
and reads the week plan HTML (`GET ibs.vielfaltmenue.com/…/Mealplan/Weekplan?year=…&week=…`); a menu counts as ordered when its
button has `data-quantity-ordered` ≥ 1. It may break when the portal changes; only the lunch cards are affected then. Check the logins with:

```sh
docker exec familydash /familydash -vielfalt-preview
```

### Weather

[Open-Meteo](https://open-meteo.com) — free, no API key, uses DWD's ICON model for Germany.

## Raspberry Pi (display)

Raspberry Pi OS (Desktop). Add to `~/.config/labwc/autostart`:

```sh
chromium --kiosk --noerrdialogs --disable-infobars --incognito --check-for-update-interval=31536000 http://<unraid-ip>:8080 &
```

Disable screen blanking via `sudo raspi-config` → *Display Options* → *Screen Blanking*.
Set the Pi's time zone to `Europe/Berlin` – the frontend groups days by the browser's local time.
The page dims itself 22:00–06:00 (`NIGHT` in `web/static/app.js`). Portrait and landscape both work.

## API

| | |
|---|---|
| `GET /api/dashboard` | Everything the frontend needs (JSON) |
| `GET /healthz` | Liveness |

## Development

```sh
go test ./...
cp .env.example .env   # fill in
set -a; . ./.env; set +a; go run ./cmd/familydash
```

Frontend: edit `web/static/*`, restart the binary (files are embedded).

## Roadmap

See [docs/ROADMAP.md](docs/ROADMAP.md) – incl. voice control with Parakeet.
