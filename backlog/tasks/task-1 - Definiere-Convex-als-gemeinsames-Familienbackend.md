---
id: TASK-1
title: Definiere Convex als gemeinsames Familienbackend
status: In Progress
assignee: []
created_date: '2026-10-03 15:09'
updated_date: '2026-10-03 15:48'
labels: []
dependencies: []
priority: high
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Erstelle den Architektur-, Daten- und Verantwortungsvertrag fuer familydash und familienapp. Bestehendes Convex auf Coolify weiterverwenden; externe Quellen zunaechst nur lesen. Diese Aufgabe umfasst die schriftliche Spezifikation und deren Review, keine Produktimplementierung.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Datenhoheit, Personenidentitaeten und gemeinsame Fachobjekte sind eindeutig beschrieben
- [x] #2 Synchronisation, Teilfehler, Loeschungen, Zeitbezug und Berechtigungen sind definiert
- [x] #3 Migration pro Quelle mit genau einem Schreiber und Rueckschaltung ist beschrieben
- [x] #4 Die schriftliche Spezifikation ist durch den Nutzer geprueft und bestaetigt
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Bestaetigte Architekturentscheidungen konsolidieren
2. Daten- und Verantwortungsvertrag schriftlich ausarbeiten
3. Konsistenz und Migrationsrisiken unabhaengig pruefen
4. Spezifikation dem Nutzer zur Review vorlegen
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Architekturspezifikation erstellt: docs/superpowers/specs/2026-10-03-convex-family-backend-design.md
Unabhaengige Architekturreview abgeschlossen; sechs Praezisierungen eingearbeitet (writerEpoch auch nach Rollback, Projektionsrevisionen/Zeitbezug, getrennte Read/Ingest-Credentials, lokale in-flight Fetches, ueberlappende Kalenderereignisse, atomare Daten-/Statusaktivierung).
Bestehendes Convex Coolify und lesende externe Integrationen bestaetigt. Keine Produktimplementierung und keine Live-Konfigurationsaenderung.
AC 4 und Status Done bleiben offen bis zur ausdruecklichen Nutzerreview und Bestaetigung.

Nutzer hat am 2026-10-03 den schriftlichen Vertrag bestaetigt und Subagent-driven Development fuer die Umsetzung gewaehlt. Status bleibt In Progress, da keine ausdrueckliche Anweisung zum Markieren als Done vorliegt.
<!-- SECTION:NOTES:END -->
