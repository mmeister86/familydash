# familydash

A lightweight family wall dashboard: **Google Calendar**, **Apple Reminders** and **weather** on one screen.
Runs as a single static Go binary in an ~8 MB container on Unraid; a Raspberry Pi only runs a browser in kiosk mode.

![Wall display](docs/screenshot.png)

- Go standard library only — no dependencies, no database, no Node build step
- Frontend is plain HTML/CSS/JS, embedded in the binary
- Reloads itself on the wall display when you deploy a new image
- Keeps the last good data per source, so a flaky feed never blanks the screen

```
 Google Calendar ──(iCal secret URL, pull 5 min)──┐
 Open-Meteo      ──(HTTPS, pull 15 min)───────────┤
                                                  ▼
 Mac (EventKit bridge) ──POST /api/reminders──▶ familydash (Unraid, :8080) ◀── Chromium kiosk (Raspberry Pi)
 iPhone Shortcut ────────POST /api/reminders──┘      /data/reminders.json
```

## Quick start (Unraid)

1. Push this repo to GitHub → the workflow builds `ghcr.io/<you>/familydash` (amd64 + arm64).
   Replace `OWNER` in `docker-compose.yml` and `deploy/unraid/my-familydash.xml`.
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
| `CALENDAR_DAYS` | `7` | Days shown (today + n-1) |
| `CALENDAR_REFRESH` | `5m` | Go duration |
| `WEATHER_LAT` / `WEATHER_LON` | – | Weather is off until set |
| `WEATHER_NAME` | – | Label above the weather panel |
| `WEATHER_REFRESH` | `15m` | |
| `REMINDERS_TOKEN` | – | Bearer token for `POST /api/reminders`; push is disabled when empty |
| `REMINDERS_STALE_AFTER` | `2h` | Show a hint in the status bar if no push arrived for this long |
| `DATA_DIR` | `/data` | Stores `reminders.json` |
| `LISTEN_ADDR` | `:8080` | |

### Google Calendar

Google Calendar (web) → ⚙ Settings → pick the calendar → **Integrate calendar** → *Secret address in iCal format*.
Paste that into `CALENDAR_1_URL`. One variable per calendar (family, school, work…).
No OAuth, no Google Cloud project. Treat the URL like a password.

The parser handles what Google/iCloud/Outlook export in practice: time zones, all-day and multi-day events,
`RRULE` (daily/weekly/monthly/yearly, `INTERVAL`, `COUNT`, `UNTIL`, `BYDAY` incl. `2TU`/`-1FR`, `BYMONTHDAY`, `BYMONTH`, `BYSETPOS`),
`EXDATE`, moved/cancelled single instances (`RECURRENCE-ID`) and DST changes. See `internal/calendar/ics_test.go`.

Note: Google refreshes the secret iCal feed itself only every few minutes to hours; that delay is on Google's side.

### Apple Reminders

Apple has no server API for iCloud Reminders (they left CalDAV with the iOS 13 upgrade), so the dashboard can't pull them.
Something that *can* read them pushes a snapshot instead:

- **Mac bridge (recommended)** – `bridges/reminders-mac/`: a small Swift/EventKit tool that pushes on every change (debounced) and every 5 min.
  Needs a Mac that is usually on and signed in to the family iCloud.
  ```sh
  cd bridges/reminders-mac && ./build.sh
  DASH_URL=http://<unraid-ip>:8080/api/reminders DASH_TOKEN=<token> ./familydash-reminders   # first run: grant access
  # then install the LaunchAgent: see comment in de.matthiasmeister.familydash.reminders.plist
  ```
- **iOS Shortcut** – `bridges/shortcuts/README.md`: no Mac needed, but only runs on time/app triggers.

Push API:

```http
POST /api/reminders
Authorization: Bearer <REMINDERS_TOKEN>
Content-Type: application/json

{"source":"mac","lists":[{"name":"Einkauf","color":"#34C759","items":[{"title":"Milch"},{"title":"Arzt","due":"2026-10-01T09:00:00+02:00","priority":1}]}]}
```

`lists` replaces everything from that `source`. For a single list you can also send
`{"source":"iphone","list":"Einkauf","items":["Milch","Brot"]}` or `{"list":"Einkauf","text":"Milch\nBrot"}`.

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
| `POST /api/reminders` | Reminders push, see above |
| `GET /healthz` | Liveness |

## Development

```sh
go test ./...
cp .env.example .env   # fill in
set -a; . ./.env; set +a; DATA_DIR=./data go run ./cmd/familydash
```

Frontend: edit `web/static/*`, restart the binary (files are embedded).

## Roadmap

See [docs/ROADMAP.md](docs/ROADMAP.md) – incl. voice control with Parakeet.
