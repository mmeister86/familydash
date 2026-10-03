# Roadmap

## Next

- [ ] Refine the beste.schule parser against real data (`-besteschule-dump`)
- [ ] Tap-to-check Bring! items on a touch display (Bring! batch update endpoint)
- [x] Family app (React PWA + Convex): push each child's week + briefings, to-dos from the app (`FAMILY_APP_*`)
- [x] Remove the legacy to-do source (code, env vars, the CLI and its Rust build stage) once the family app has run for a while
- [ ] Calendar legend / per-person filter
- [x] Time-of-day scenes (`SCENE_*`) with focus zone, night = dimmed clock only
- [x] Photo slideshow (`PHOTOS_DIR`)
- [x] One layout per scene (sketches of 2026-10-01), weather pictures, news card
- [x] AI card: morning briefing (instead of Bring!) and evening outlook above the photos (Gemini, `GEMINI_*`)
- [x] Footer lists every Uptime Kuma monitor on its own („🟢 Unraid / 🟢 Internet / …")
- [ ] Morning checklist per child (focus widget `checklist`), "Sportbeutel" from the timetable
- [ ] Countdown to the next appointment, pushed into every scene
- [ ] Scenes: treat public holidays / school vacations as weekend
- [ ] Hardware dimming at night via `ddcutil` on the Pi

## Voice control (later)

### Hardware

No Raspberry Pi model has a built-in microphone. Options:

- USB conference speakerphone (mic + speaker, echo cancellation, plug & play) – simplest
- ReSpeaker-style mic array HAT/USB board – better far-field pickup, more tinkering

### Architecture

```
Pi: mic → wake word (openWakeWord, on-device) → record until silence
    → POST audio to Unraid STT
Unraid: Parakeet container → text
    → POST /api/command (familydash) → intent → action
       "Milch auf die Einkaufsliste" → Bring! item
       "Was steht morgen an?"        → TTS answer (Piper) back to the Pi
```

- **STT:** `parakeet-tdt-0.6b-v3` is multilingual (25 European languages incl. German, automatic language detection).
  There's a community FastAPI server with an **OpenAI-compatible** `/v1/audio/transcriptions` endpoint and a CPU Docker profile:
  https://github.com/groxaxo/parakeet-tdt-0.6b-v3-fastapi-openai
- **Run it on the Xeon CPU.** The RX 580 in the Unraid box won't help – Parakeet/NeMo tooling targets CUDA, and current ROCm no longer supports Polaris GPUs. For short voice commands the CPU is plenty.
- Keep the STT endpoint generic (OpenAI-compatible) so Parakeet can be swapped for faster-whisper etc.
- Wake word and VAD run on the Pi so no audio leaves the room until the wake word fires.

### familydash side (to build)

- `POST /api/command` `{ "text": "…" }` → simple rule-based intents first (list add, "what's today/tomorrow", weather). No LLM needed for v1.
- Frontend: small listening/answer overlay, pushed via Server-Sent Events.
