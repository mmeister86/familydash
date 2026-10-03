---
id: TASK-2
title: Plane die Convex-Grundlage und den Kalender-Pilot
status: In Progress
assignee: []
created_date: '2026-10-03 15:48'
updated_date: '2026-10-03 16:14'
labels: []
dependencies: []
priority: high
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Erstelle den konkreten Implementierungsplan fuer Phase A/B des bestaetigten Familienbackend-Vertrags. Definiere repositoryuebergreifende Schnittstellen, Kalenderimport, sichere Quellenuebernahme und Tests fuer die anschliessende Umsetzung mit Subagent-driven Development.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 Plan benennt konkrete Dateien, Schnittstellen und voneinander abhaengige Umsetzungseinheiten in beiden Repositories
- [x] #2 Tests decken Serien, Zeitzonen, Teilfehler, alte Laeufe, Berechtigungen und Quellenwechsel ab
- [x] #3 Plan ist gegen die bestaetigte Spezifikation und den aktuellen Repositoryzustand geprueft
- [ ] #4 Nutzer hat den schriftlichen Implementierungsplan geprueft; Subagent-driven Development bleibt als Ausfuehrungsmethode festgelegt
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Companion lokal lesen und vorhandene Schnittstellen pruefen
2. Aktuelle Dokumentation fuer Kalenderparser und Convex abrufen
3. Konkrete Umsetzungseinheiten mit Tests und Schnittstellen definieren
4. Plan selbst pruefen und zur Nutzerreview vorlegen
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Implementierungsplan mit sieben abhaengigen Deliverables erstellt. Zwei spezialisierte Subagenten haben Companion- und Go-Schnittstellen read-only inventarisiert. Selbstreview gegen Spezifikation: Vertragsfelder vereinheitlicht, unveraenderte Imports ohne Event-Neuschreiben ergaenzt, referenzierte Datengenerationen bei Cleanup geschuetzt, lokale HTTPS-Smoke-Tests konkretisiert. Aktuelle Convex-Dokumentation via Context7 und Primaerquellen geprueft. Keine Produktdependencies installiert, keine Produkttests ausgefuehrt, kein Produktionszugriff. Schriftliche Nutzerreview des Plans steht aus; Ausfuehrungsmethode Subagent-driven Development ist bereits gewaehlt.
<!-- SECTION:NOTES:END -->
