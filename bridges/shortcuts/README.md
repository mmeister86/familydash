# Apple-Erinnerungen per iOS-Kurzbefehl pushen

Fallback, falls kein Mac dauerhaft läuft. Nachteil: iOS-Automationen laufen
nur zu festen Zeiten (oder z. B. beim Öffnen einer App), nicht bei jeder Änderung.

## Kurzbefehl „Dashboard: Einkauf“

1. **Erinnerungen suchen** → Filter: *Liste* ist *Einkauf*, *Ist erledigt* ist *falsch*
2. **Text kombinieren** → Erinnerungen, Trennzeichen *Neue Zeile*
3. **Inhalte von URL abrufen**
   - URL: `http://192.168.188.127:8080/api/reminders`
   - Methode: `POST`
   - Header: `Authorization` = `Bearer <REMINDERS_TOKEN>`
   - Anfragetext: **JSON**
     - `source` (Text) = `iphone`
     - `list` (Text) = `Einkauf`
     - `text` (Text) = *Kombinierter Text*

Pro Liste einen Block 1–3 (oder den Kurzbefehl duplizieren).

## Automation

Kurzbefehle → Automation → *Tageszeit* (z. B. stündlich 7–21 Uhr, je eine
Automation) oder *App → Erinnerungen → Wird geschlossen* → Kurzbefehl
ausführen, **„Sofort ausführen“** aktivieren.

> Der Kurzbefehl erreicht die Unraid-IP nur im Heimnetz (oder per VPN/Tailscale).
