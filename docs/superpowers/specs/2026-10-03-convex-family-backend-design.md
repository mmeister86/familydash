# Convex als gemeinsames Familienbackend

Datum: 2026-10-03

Backlog: TASK-1 – Definiere Convex als gemeinsames Familienbackend

Status: Vom Nutzer am 2026-10-03 bestätigt. Ausführungsmethode: Subagent-driven Development. Die Spezifikation beschreibt das Ziel; Produktimplementierung folgt anhand eigener Abschnittspläne.

## 1. Ziel und bestätigter Rahmen

Das bestehende Convex-Backend der Companion App wird zum gemeinsamen Datenbestand und Steuerungszentrum für Companion App und Wanddashboard. Externe Abrufe und gemeinsame Fachregeln laufen unabhängig davon, ob das Wanddisplay oder der Go-Prozess eingeschaltet ist.

Bestätigte Entscheidungen:

- Die bestehende selbst gehostete Convex-Instanz auf Coolify bleibt bestehen.
- Externe Anbieter werden zunächst ausschließlich gelesen. Es gibt keinen Kalender-, Einkaufslisten- oder Essensbestellungs-Writeback.
- Bestehende Aufgaben, Punkte und Belohnungen bleiben im vorhandenen Convex-Modell.
- Die Migration erfolgt schrittweise. Die bestehende Dashboard-Oberfläche bleibt zunächst erhalten.
- Der gemeinsame Backend-Code gehört zunächst in `mmeister86/familienapp`; Schema, Funktionen und Deployment haben einen eindeutigen Eigentümer.
- Dieser Schritt liefert den Daten- und Verantwortungsvertrag. Produktcode und Live-Konfiguration werden nicht geändert.

Für diesen Entwurf gewählte, bei der Review korrigierbare Defaults:

- Das System bedient weiterhin eine Familie. Es wird keine Mehrmandantenarchitektur eingeführt.
- Go bleibt zunächst als Webserver, Convex-Leseclient und Anbieter lokaler Medien erhalten.
- Das Wanddashboard ist ein lesendes Gerät. Eltern verwalten familieneigene Daten und Integrationen; Kinder sehen die für sie bestimmten Daten.
- Ein persistenter letzter Anzeigestand soll auch nach einem Dashboard-Neustart verfügbar sein.
- Die aktuellen Abrufintervalle werden zunächst übernommen. Optimierungen anhand von Bedarf und Anbietergrenzen folgen separat.

Erfolg bedeutet: Dashboard und App verwenden dieselben gespeicherten Fachinformationen; neue externe Daten und Briefings hängen nach Abschluss der Migration nicht mehr vom Dashboard-Betrieb ab.

## 2. Ausgangslage

Heute startet `cmd/familydash/main.go` die externen Pollingdienste. Go sammelt Kalender, Schule, Bring!, VielfaltMenü, Wetter und Nachrichten, berechnet Kinderansichten und Briefings und hält überwiegend RAM-Caches.

`POST /ingest/child` überträgt vollständige Sieben-Tage-Kinderansichten an Convex. `POST /ingest/briefing` überträgt die Dashboard-Briefings. `GET /todos` liefert Aufgaben aus Convex zurück an Go. Die Snapshot-Verträge sind in [docs/FAMILY_APP.md](../../FAMILY_APP.md) beschrieben.

Convex ist bereits führend für `users`, Aufgaben und Aufgabeninstanzen, das Punkte-Ledger, Belohnungen und Einlösungen. Externe Informationen liegen dagegen überwiegend in `childSnapshots`. Zusätzlich zu den Dashboard-Briefings erzeugt Convex eigene `parentBriefings`; die App verwendet diese eigene Pipeline.

Relevante Einschränkungen:

- Personen und Quellen werden mehrfach über Namen beziehungsweise Vornamen zugeordnet.
- Kalenderereignisse verlieren im bestehenden Ausgabeformat ihre externen Identitäten.
- Ein gemeinsamer Snapshot-Zeitstempel beschreibt den Zustand einzelner Quellen nicht zuverlässig.
- Fehler optionaler Schulabrufe können bisherige Informationen durch leere Teilbereiche ersetzen.
- Der bisherige Snapshot-Ingest ersetzt Dokumente ohne Schutz vor älteren eingehenden Daten.
- Das Dashboard und die App besitzen keinen dauerhaften vollständigen Offline-Datenbestand.

## 3. Zuständigkeiten und Datenhoheit

| Bereich | Führendes System | Verantwortung in Convex | Verantwortung der Clients |
| --- | --- | --- | --- |
| Personen und Quellenzuordnungen | Convex | Identität, Rollen, Namen, Farben und explizite Zuordnungen | Darstellung und erlaubte Verwaltung |
| Aufgaben, Punkte, Belohnungen | Convex | Bestehende Regeln, Zustände und transaktionale Buchungen | Anzeigen und autorisierte Eingaben |
| Google-/ICS-Kalender | Externer Kalender für importierte Felder | Abruf, normalisierte Ereignisse, Zuordnung, aktueller Quellenstand | Anzeigen |
| beste.schule | Externe Schule für importierte Felder | Abruf, Unterricht, Änderungen, Hausaufgaben, Prüfungen | Anzeigen |
| Feste Stundenpläne und Familienregeln | Convex | Gültigkeit, A/B-Wochen, Ferien, Extras und Müllregeln | Elternverwaltung und Darstellung |
| VielfaltMenü und Bring! | Externer Anbieter | Lesende Abfragen, gespeicherte Ergebnisse und Status | Anzeigen |
| Wetter und Nachrichten | Externer Anbieter | Abruf, Normalisierung und begrenzter Datenbestand | Anzeigen |
| Briefings | Convex | Gemeinsame Faktenbasis, Erzeugung, Ergebnisse und Ausführung | Passende Ausgabe anzeigen |
| Szenen und Layout | Convex für gemeinsame Einstellungen; Client für Rendering | Familien-/Geräteeinstellungen | Uhr, Layout, Animation und lokale Medienauswahl |
| Fotos und Hintergründe | Lokaler Dateispeicher zunächst | Bei Bedarf Auswahlregeln und Metadaten | Dateien lokal bereitstellen und anzeigen |
| LAN-interner Kuma-Status | Kuma | Bei Bedarf übermittelter Status und dessen Frische | Lokaler Zugriff über Agent, wenn nötig |

Convex ist die verbindliche gemeinsame Anwendungssicht auf importierte Daten. Ein Import ersetzt nur die Felder, für die sein Anbieter maßgeblich ist. Familieneigene Ergänzungen werden separat gespeichert und durch einen Import nicht überschrieben.

Eine gemeinsame Datenbasis bedeutet unterschiedliche autorisierte Ansichten. Zugangsdaten, Sessions und interne Diagnoseinformationen gehören nicht in Dashboard- oder Kinderantworten.

## 4. Datenvertrag

Die folgenden Begriffe beschreiben fachliche Objekte. Die genaue Aufteilung in Convex-Tabellen und Funktionssignaturen gehört in den anschließenden Implementierungsplan.

### Personen und Quellen

- `users` bleibt die gemeinsame Personenbasis. Es entsteht keine zweite parallele Personentabelle.
- Die bestehende Convex-User-ID ist die interne Identität. Namen und Slugs sind veränderbare Attribute beziehungsweise Kompatibilitätsfelder.
- Eine Integration beschreibt Anbieter, Aktivierung, Abrufintervall und eine serverseitige Credential-Referenz.
- Eine Quellenzuordnung verbindet diese Integration mit externen Schüler-IDs, Essenskonten, Kalendern und internen Personen-IDs.
- Kalender haben eine stabile ID, einen Namen, Farbe, Sichtbarkeit und eine explizite Personen-/Familienzuordnung. Eine Arrayposition ist keine Identität.
- Geheimnisse bleiben zunächst in der serverseitigen Convex-/Coolify-Konfiguration. Eltern dürfen nicht geheime Einstellungen verwalten; eine neue Oberfläche zur Passwortspeicherung ist nicht Teil dieses Umbaus.
- Mehrdeutige Zuordnungen werden als Konfigurationsfehler angezeigt. Ein Vorname darf keine automatische dauerhafte Verknüpfung herstellen.

### Kalenderereignisse

- Identität: stabile Kalender-ID plus ICS-UID und bei Serien eine Wiederholungsinstanz, abgeleitet aus `RECURRENCE-ID` beziehungsweise dem ursprünglichen Serienzeitpunkt.
- Eine Terminverschiebung verändert nicht die Identität der Serieninstanz. Gleiche UIDs in unterschiedlichen Kalendern werden nicht zusammengeworfen.
- Zeitgebundene Ereignisse speichern eindeutig interpretierbare Zeitpunkte und relevante Quellzeitzone. Ganztägige Ereignisse speichern lokale Datumswerte mit exklusivem Enddatum.
- Wiederholungen, Ausnahmen, Absagen und Mehrtagesereignisse werden beim zentralen Import beziehungsweise bei der zentralen Expansion berücksichtigt.
- Fehlt eine brauchbare Quellidentität, wird ein dokumentierter deterministischer Ersatzschlüssel verwendet; mögliche Änderungen dieser Identität werden als eingeschränkte Quellenqualität behandelt.
- Familien- und Elterntermine werden unabhängig von den Sieben-Tage-Kinderansichten gespeichert.
- Der erste Importhorizont beträgt für alle Kalender 42 Tage ab heute in `Europe/Berlin`. Die Darstellung bleibt bei ihren bisherigen Fenstern; der Speicherhorizont ist eine eigene Einstellung.
- Das Importfenster enthält alle Ereignisse, die es überlappen, einschließlich zuvor begonnener Mehrtagesereignisse. Startdatum allein ist kein Auswahlkriterium. Verschobene Serieninstanzen behalten ihre ursprüngliche Identität; der Abgleich berücksichtigt die bisherigen Instanzen auch dann, wenn sie durch die Änderung aus dem Anzeigefenster wandern.
- Eine erfolgreiche Aktualisierung gilt nur für ihr deklariertes Kalender- und Zeitfenster. Außerhalb liegende Daten dürfen nicht aufgrund ihres Fehlens im Abruf gelöscht werden.

### Schule und feste Stundenpläne

- Unterricht, Änderungen, Hausaufgaben und Prüfungen sind getrennt aktualisierbare Bereiche pro Schüler beziehungsweise Quelle.
- Bevorzugt werden externe Objekt-IDs. Fehlen sie, wird je Bereich ein dokumentierter stabiler Schlüssel aus Quellenidentität und fachlicher Identität gebildet.
- Unterricht unterscheidet Grundplan, datumsbezogene Änderungen und berechnete Unterrichtsvorkommen. Tageskarten sind daraus erzeugte Ansichten.
- Bei Hausaufgaben und Prüfungen werden die aktuell vorhandenen Felder und der bisherige Look-ahead erhalten; ein kleineres UI-Fenster darf den gespeicherten Quellenumfang nicht begrenzen.
- Feste Pläne einschließlich A/B-Referenzwoche, Gültigkeit, Ferien und Zusatzunterricht liegen zentral. Manuelle Korrekturen sind explizite Regeln und verändern keine importierten Originalfelder.
- Ein erfolgreicher Grundplanabruf rechtfertigt kein Leeren der Hausaufgaben, wenn deren Abruf fehlgeschlagen ist.

### Mahlzeiten, Einkauf, Wetter und Nachrichten

- Mahlzeiten gehören zu einer festen Person-/Kontozuordnung und einem Liefertag. Menü vorhanden, nicht bestellt, bestellt und unbekannter Abrufzustand bleiben unterscheidbar.
- Ein fehlgeschlagener Essensabruf darf nicht die Aussage „nichts bestellt“ erzeugen.
- Einkaufslisten behalten Anbieterlisten-ID und verfügbare Artikelidentitäten. Sie bleiben reine Lesedaten.
- Wetter wird nach konfiguriertem Standort gespeichert; Nachrichten nach Feed beziehungsweise Nachrichtengruppe. Es werden nur die für die vorhandenen Ansichten benötigten Zeiträume gespeichert.
- Eine erfolgreiche leere Antwort ist fachlich etwas anderes als Fehler, deaktivierte Quelle oder noch nie geladene Quelle.

### Synchronisationszustand

Pro Quelle und unabhängig aktualisierbarem Bereich werden gespeichert:

- letzter Versuch und letzter erfolgreicher Abruf;
- Aktivierung: aktiv oder deaktiviert; Datenfrische: noch nicht geladen, aktuell oder veraltet; Ergebnis des letzten Versuchs: erfolgreich, teilweise erfolgreich oder fehlgeschlagen;
- betroffener Zeitraum beziehungsweise Abrufumfang;
- laufende Ausführungs-ID, Konfigurationsgeneration und Reihenfolge des akzeptierten Abrufs;
- nächste geplante Ausführung, Fehlerklasse und eine bereinigte Fehlermeldung;
- gegebenenfalls Anbieterrevision, Inhaltsfingerabdruck und beobachteter Änderungszeitpunkt.

Abrufversuch, erfolgreicher Abruf und fachliche Änderung sind drei unterschiedliche Zeitpunkte. Ein erfolgreicher unveränderter Abruf verbessert die Frische, ohne unnötig Fachobjekte neu zu schreiben.

Die übernommenen Startintervalle sind: Kalender 5 Minuten, Schule 15 Minuten, VielfaltMenü 30 Minuten, Bring! 2 Minuten, Wetter 15 Minuten und Nachrichten 20 Minuten. Ein Bereich gilt zunächst nach drei ausgebliebenen erfolgreichen Intervallen als veraltet, frühestens nach 5 Minuten. Ein fehlgeschlagener Versuch wird sofort sichtbar, auch wenn die erhaltenen Daten noch innerhalb dieser Frischegrenze liegen. Bestehende individuell konfigurierte Intervalle werden bei der Migration übernommen.

## 5. Zentraler Abruf und Fehlerbehandlung

Der Ablauf ist: fällige Quelle auswählen → Ausführung registrieren → extern abrufen → validieren und normalisieren → Ergebnisse kontrolliert übernehmen → abhängige Ansichten und Briefings aktualisieren.

- Externe HTTP-Aufrufe erfolgen in internen Actions; Datenbankänderungen in internen Mutations.
- Ein zeitgesteuerter Dispatcher prüft fällige Quellen. Eltern dürfen zusätzlich einen begrenzten manuellen Refresh anfordern. Das Öffnen einer Ansicht startet keinen unkontrollierten Anbieterabruf.
- Pro Quelle besitzt höchstens eine Ausführung eine gültige Berechtigung zum Übernehmen ihrer Ergebnisse. Nach einem Absturz begrenzt eine Ablaufzeit die Sperre; eine neue Ausführung erhält eine neue Generation und entzieht einem eventuell noch laufenden alten Abruf die Commit-Berechtigung.
- Ein später zurückkehrender alter Lauf darf keine neueren Ergebnisse übernehmen. Die Prüfung erfolgt beim Datenbank-Commit, nicht ausschließlich vor dem HTTP-Aufruf.
- Zeitüberschreitungen, vorübergehende Netzwerkfehler, `429` und geeignete `5xx` erhalten begrenzte Wiederholungen mit wachsendem Abstand und Zufallsanteil. `Retry-After` wird beachtet.
- Dauerhafte Authentifizierungs- und Parsingfehler werden sichtbar gemacht und verhindern aggressive Wiederholungen. Ein Konfigurationswechsel oder manueller Refresh ermöglicht einen neuen Versuch.
- Actions werden nicht automatisch zuverlässig wiederholt. Die Anwendung führt Ausführungszustand, Wiederholungen und Idempotenz ausdrücklich selbst.
- Auch Pagination, Antwortlimits und vollständig verarbeiteter Abrufumfang gehören zur Prüfung eines erfolgreichen Imports.
- Nur ein vollständig erfolgreicher Bereich darf darin nicht mehr vorhandene Objekte entfernen beziehungsweise als entfernt markieren. Keine Löschung nach Teilabruf, Parserfehler oder abgebrochener Pagination.
- Bei größeren Imports werden neue Ergebnisse zunächst einer Ausführung zugeordnet und erst nach erfolgreichem Abschluss aktiviert. Nutzer sehen keine halb ersetzte Quelle.
- Aktivierter Datenstand, Bereichsrevision, erfolgreicher Abrufzeitpunkt und deklarierter Abrufumfang werden gemeinsam atomar übernommen. Ein frischer Status darf nicht auf einen noch nicht aktivierten Import verweisen.
- Vorherige erfolgreiche Daten bleiben bei einem Fehler erhalten. Beide Clients zeigen Quelle und Alter dieses Datenstands angemessen an.

## 6. Ansichten, Zeitbezug und Briefings

Convex stellt aus dem gemeinsamen Bestand rollenabhängige Ansichten bereit:

- Dashboard: vorhandene Kalender- und Kinderkarten, Wetter, Einkauf, News, Aufgaben, Briefings sowie Quellenstatus und zentrale Anzeigeeinstellungen.
- Eltern-App: Familienübersicht, alle freigegebenen Kinderinformationen, Aufgaben/Freigaben und Integrationsstatus.
- Kinder-App: eigene freigegebene Schul-, Termin-, Essens- und Aufgabendaten sowie bestehende Punkte-/Belohnungsfunktionen.

`childSnapshots` bleibt zunächst als kompatible Sieben-Tage-Projektion bestehen. Es ist keine zweite führende Datenbasis. Der zentrale Projektor wird nach der Migration aller dafür benötigten Quellen einziger Schreiber dieser Projektion.

Eine zentrale Projektion wird aus einem konsistenten aktuellen Bestand berechnet und gespeichert. Erfolgt die Berechnung außerhalb einer einzigen Mutation, prüft ihr Commit alle gelesenen Quellen- und Zuordnungsrevisionen, den fachlichen Zeitbezug und die aktuelle Schreiber-Epoche. Eine ältere Berechnung darf weder einen neueren Quellenstand noch den nächsten Tag wieder überschreiben.

Die Dashboard-Antwort bleibt vorerst kompatibel zu `GET /api/dashboard`. Go übersetzt zentral gelieferte Ansichten in dieses Ausgabeformat und ergänzt ausschließlich lokale Medien-/Geräteinformationen. Neue Backend-Verträge tragen eine Version; bestehende Clients müssen während einer Erweiterung weiter funktionieren.

Datumsbezogene Fachlogik verwendet `Europe/Berlin`. Tageswechsel, Sommerzeit, A/B-Wochen, Unterrichtsende und Essensanzeige werden ausdrücklich berücksichtigt. Fachansichten müssen sich auch ohne neue Anbieterantwort aktualisieren: Ein zentraler minutengesteuerter Tick aktualisiert den fachlichen Zeitbezug beziehungsweise markiert betroffene Projektionen zur Neuberechnung. Eine reine Daten-Subscription ersetzt keinen Zeittrigger.

Briefings verwenden eine gemeinsame konsistent gelesene Faktenbasis. Familien-/Elternansichten und Wandansichten dürfen unterschiedliche Inhalte und Formate erhalten. Der bisherige Gegensatz `briefings` versus `parentBriefings` wird durch ausdrücklich benannte Ausgabearten und Ziel-/Anzeigedatum ersetzt.

- Regeln berechnen Fakten, Zeitpunkte und Empfehlungen; das Sprachmodell formuliert sie.
- Änderungen relevanter Fakten können eine neue Erzeugung anfordern. Der bisherige Mindestabstand von 20 Minuten verhindert unnötige Modellaufrufe.
- Morgens und abends wird das Briefing anhand zentraler Familienzeiten vorbereitet; ein laufendes Wanddisplay ist keine Voraussetzung.
- Ein regelbasiertes Ergebnis ist bei Modellfehlern verfügbar. Ein älteres Ergebnis bleibt als solches erkennbar.
- Briefings speichern Faktenrevision, Datenstand, Ausgabeart, Zieltag, Erzeugungszeit und Zustand. Familieninformationen werden nicht versehentlich über eine Kinderquery freigegeben.
- Vorhandene Aufbewahrung von 14 Tagen für Briefingergebnisse bleibt zunächst bestehen.

## 7. Zugriff, lokale Funktionen und Betrieb

Die eigene PIN-/Session-Authentifizierung der App bleibt für diesen Umbau erhalten. Die Umstellung auf einen anderen Auth-Anbieter ist ein separates Vorhaben.

Go verwendet zunächst eine widerrufbare, ausschließlich lesende Geräteberechtigung für die Dashboard-Ansicht. Der Schlüssel bleibt im Go-Server. Kein Ingest-, Admin- oder Elternschlüssel wird im Browser ausgeliefert. Das vorhandene lokale HTTP-Dashboard bleibt eine lokale Anzeige; ein öffentlich erreichbarer Betrieb erfordert eine separat passende Zugangskontrolle.

Während der Übergangsphasen B/C bleibt daneben die bestehende getrennte Legacy-Ingest-Berechtigung ausschließlich für die bisherigen Push-Endpunkte erhalten. Sie wird beim jeweiligen Wechsel in D/E deaktiviert. Die neue Geräte-Leseberechtigung erlaubt zu keinem Zeitpunkt Ingest. Alte serverseitige Credentials werden nicht mit dem neuen Gerätezugriff zusammengeführt.

Die Geräteberechtigung erlaubt keine Profilverwaltung, Integrationsänderung, Aufgabenfreigabe oder Punktebuchung. Eltern erhalten nicht geheime Quellenkonfiguration, Status und Refresh. Providerzugangsdaten sind auch aus Eltern-UI-Antworten ausgeschlossen.

Fotos und Hintergründe werden zunächst lokal bereitgestellt. Familien- und Szeneneinstellungen können zentral liegen, lokale Dateipfade und Mounts bleiben Gerätekonfiguration. Kuma wird nur dann über Convex abgefragt, wenn es aus dessen Netzwerk erreichbar ist. Andernfalls liefert ein lokaler Agent einen begrenzten Status ausgehend an Convex. Dafür wird kein neuer öffentlicher Unraid-Eingang vorausgesetzt.

Go speichert die letzte vollständig akzeptierte Dashboard-Antwort dauerhaft und atomar. Der Browser erhält bei Backend-Ausfall diesen Stand mit erfolgreichem Empfangszeitpunkt und Quellenalter. Dieser Cache darf keine Änderungen zurück in Convex schreiben. Lokale Uhr und Szenen laufen weiter; veraltete Zukunftsinformationen werden nicht als frisch dargestellt. Ein dauerhafter App-Daten-Offlinecache wird separat geplant; die bestehende PWA-Asset-Zwischenspeicherung ist dafür kein Ersatz.

Vor der produktiven Übernahme werden auf der tatsächlichen Coolify-Instanz geprüft: Backendversion und verfügbare Runtimes, ausgehender Anbieterzugriff, Scheduler, persistenter Speicher, Backup/Restore und Monitoring. Das lokale Compose im Companion-Repository belegt nicht die Produktionskonfiguration. Eine neue Infrastruktur oder Cloudmigration ist nicht vorgesehen.

## 8. Migration und Schreibhoheit

Wir migrieren Quellen einzeln und behalten vorübergehend genau einen Schreiber der bestehenden Gesamt-Kinderprojektion. Dafür ist keine neue dauerhafte Raw-Ingest-Brücke erforderlich.

### Phase A: Gemeinsame Grundlage

Personen-/Quellenzuordnungen, Quellenzustand, Geräte-Lesevertrag, Importidentitäten und Schutz vor alten Ausführungen festlegen. Bestehende Daten vor der Umstellung sichern. Vorhandene Aufgaben-/Punkte-/Belohnungsregeln bleiben erhalten.

### Phase B: Kalender als erste Quelle

Convex importiert die Kalender in einen zunächst nicht aktivierten Vergleichsbestand. Parser- und Identitätsverhalten werden bevorzugt anhand derselben Eingabedaten verglichen; gegebenenfalls ist ein zeitlich begrenzter paralleler Abruf für die Abnahme erlaubt.

Nach Prüfung wird der zentrale Bestand aktiviert und der direkte Kalenderabruf in Go beendet. Go liest fortan die Kalender aus Convex und stellt sie dem vorhandenen Dashboard sowie vorübergehend dem bisherigen Snapshot-Builder zur Verfügung.

Der neue Quellenmodus deaktiviert den alten Poller und die bisherige fachliche Konfigurationsauswertung für diese Quelle, ersetzt deren aktiven Lesebestand durch die zentrale Ansicht und verwirft Ergebnisse bereits gestarteter lokaler Abrufe. Nach dem Wechsel darf der Snapshot-Builder für diese Quelle ausschließlich den zentralen Stand und dessen Zuordnungen verwenden. Erst danach ist die Quellenübernahme abgeschlossen; das spätere Entfernen alter Konfigurationsfelder bleibt Phase F.

### Phase C: Schule, feste Pläne und Mahlzeiten

Dieselben Schritte werden für Schule und Mahlzeiten wiederholt; feste Pläne und Zuordnungen werden zentral übernommen. Go liest migrierte Quellen aus Convex und berechnet vorübergehend weiter die bisherige Gesamt-Kinderprojektion. Noch nicht migrierte Quellen werden weiterhin ausschließlich durch ihre bisherigen Go-Dienste geladen.

Während B/C gilt:

- Convex ist der einzige Schreiber der normalisierten Daten einer übernommenen Quelle.
- Der Legacy-Pusher ist vorübergehend der einzige Schreiber von `childSnapshots`.
- Neue zentrale Quellenadapter schreiben niemals selbst diese Gesamtprojektion.
- Der Legacy-Snapshot ist eine Übergangsansicht mit begrenztem Aktualisierungsverzug; der vollständige Zielzustand ist noch nicht erreicht.
- Versionierte Ansichten liefern die benötigten Zeitfenster und Datenfelder, damit Go keine Details verliert oder erneut beim Anbieter abrufen muss.

### Phase D: Zentrale Kinderprojektion

Sobald Kalender, Schule, feste Pläne und Mahlzeiten zentral verfügbar sind, wird die Convex-Projektion gegen die bisherige Ausgabe geprüft. In einer kontrollierten Umschaltung wird die Schreibhoheit der Kinderprojektion auf den zentralen Projektor gesetzt und `POST /ingest/child` für den bisherigen Pusher deaktiviert.

Die serverseitige Prüfung der Schreibhoheit erfolgt innerhalb der schreibenden Mutation. Bereits unterwegs befindliche alte HTTP-Pushes dürfen nach der Umschaltung keine neuen Projektionen überschreiben. Der zentrale Projektor übernimmt eine aktuelle, vollständig aus dem zentralen Datenbestand berechnete Ansicht; eine fehlende Quelle bleibt ausdrücklich unbekannt beziehungsweise veraltet.

Bei jedem Wechsel der Projektionshoheit, einschließlich Rückschaltung, erhöht Convex eine monotone Schreiber-Epoche (`writerEpoch`). Pushes und Projektionsläufe müssen die gültige Epoche beim Commit nachweisen. Legacy-Clients ohne Epochenangabe bleiben nach dem ersten Wechsel gesperrt; eine notwendige Rückschaltung verwendet neue, an die neue Epoche gebundene Ingest-Credentials. Ein Push aus einer früheren Legacy-Periode wird auch dann abgewiesen, wenn später erneut Legacy der aktive Schreiber ist.

### Phase E: Weitere Quellen und Briefings

Bring!, Wetter und Nachrichten werden nach demselben Quellenverfahren übernommen. Müllregeln und gemeinsamer Briefingkontext werden zentralisiert. Die zentrale Erzeugung übernimmt die Wand-Ausgaben; anschließend wird der alte `POST /ingest/briefing`-Pfad für diese Ausgaben deaktiviert und die Go-Erzeugung entfernt.

### Phase F: Go reduzieren

Nach Abnahme werden obsolete externe Poller, Tokens, Konfigurationsfelder und duplizierte Fachlogik entfernt. Go liefert nur noch zentral gelesene Ansichten, lokalen Cache und Medien beziehungsweise erforderliche LAN-Funktionen.

### Rückschaltung

Eine Rückschaltung stoppt zuerst neue Abrufe beziehungsweise entzieht deren Commit-Berechtigung, setzt die Quellenhoheit mit neuer Generation zurück und aktiviert erst dann den alten Pfad. Eine bereits laufende alte Generation darf nicht mehr schreiben.

Der letzte erfolgreiche zentrale Stand bleibt erhalten. Für die Kinderprojektion ist die Rückschaltung ein gemeinsamer Wechsel ihrer Schreibhoheit; sie darf nicht neben einem weiterhin aktiven zentralen Projektor erfolgen. Während des Übergangs bleiben alte Endpunkte geschützt vorhanden, bis die Abnahme ihre Entfernung erlaubt. Es entstehen keine dauerhaft konkurrierenden Abrufe oder Schreiber.

## 9. Abnahme und spätere Implementierungsabschnitte

Jeder Abschnitt erhält bei Implementierung einen eigenen Backlog-Eintrag mit prüfbarem Ergebnis. Diese Spezifikation ist die gemeinsame Grundlage; sie ist kein Auftrag für eine ungeteilte Komplettmigration.

Wesentliche Prüffälle:

1. App erhält neue externe Daten und Briefings bei ausgeschaltetem Go-Dashboard.
2. Wiederholter gleicher Import erzeugt keine Duplikate; ein verspäteter alter Lauf überschreibt keinen neueren Stand.
3. Vollständige leere Antwort entfernt nur den erfolgreich abgefragten Bereich; Teilfehler und Pagination-Abbruch entfernen nichts.
4. Kalender mit Serienänderung, Absage, ganztägigem Termin und Sommerzeitwechsel ergibt dieselbe fachliche Sicht auf beiden Clients.
5. Gleichnamige Kinder bleiben durch feste Zuordnungen getrennt.
6. Schuldaten bleiben bei ausgefallener Hausaufgabenroute erhalten; unbekanntes Essen wird nicht als unbestellt dargestellt.
7. Mitternacht, Unterrichtsende und A/B-Wechsel aktualisieren die Ansichten ohne zusätzlichen Quellenabruf.
8. Nach Neustart und Backend-Ausfall zeigt das Dashboard den letzten Stand mit Frischehinweis.
9. Kinder und Geräte können weder fremde geschützte Daten noch Integrationsgeheimnisse lesen oder Elternaktionen ausführen.
10. Ein verspäteter Legacy-Push nach Projektorwechsel und auch nach anschließender Rückschaltung wird abgewiesen; eine Rückschaltung aktiviert nur einen Schreiber. Ein älterer zentraler Projektionslauf darf weder neuere Quellenrevisionen noch einen Tageswechsel überschreiben.
11. Bestehende Aufgabenfreigaben, Punktebuchungen und Belohnungseinlösungen behalten ihre Regeln.
12. Backup und Wiederherstellung des gemeinsamen Datenbestands funktionieren auf der bestehenden Instanz.

Die erste Implementierungseinheit umfasst die gemeinsame Grundlage und den Kalender-Pilot. Weitere Quellen bekommen darauf aufbauende eigene Pläne. Eine Dashboard-Neuschreibung, Mehrfamilienbetrieb, externer Writeback und neue Auth-Plattform gehören nicht zum Umfang.

## 10. Referenzen

- Dashboard: `cmd/familydash/main.go`, `internal/calendar/`, `internal/besteschule/`, `internal/familyapp/`, `internal/briefing/`, `internal/server/server.go` und `web/static/app.js`.
- [Companion-Schema](https://github.com/mmeister86/familienapp/blob/main/convex/schema.ts), [HTTP-Verträge](https://github.com/mmeister86/familienapp/blob/main/convex/http.ts), [Snapshot-Ingest](https://github.com/mmeister86/familienapp/blob/main/convex/ingest.ts), [Elternbriefing](https://github.com/mmeister86/familienapp/blob/main/convex/parentBriefing.ts).
- [Convex Actions](https://docs.convex.dev/functions/actions): externe Dienste und Datenzugriff über Queries/Mutations.
- [Convex Scheduled Functions](https://docs.convex.dev/scheduling/scheduled-functions): transaktionales Scheduling und ausdrückliche Wiederholung von Actions.
- [Convex Cron Jobs](https://docs.convex.dev/scheduling/cron-jobs): wiederkehrende Ausführung und UTC-Zeitbezug.
- [Convex Self Hosting](https://docs.convex.dev/self-hosting): bestehendes selbst gehostetes Backend.

Die technische Bestandsaufnahme und Convex-Dokumentation wurden im vorherigen Analyseschritt gelesen, einschließlich Context7-Recherche. Produktive Coolify-Einstellungen wurden nicht inspiziert.
