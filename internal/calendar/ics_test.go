package calendar

import (
	"strings"
	"testing"
	"time"
)

var berlin, _ = time.LoadLocation("Europe/Berlin")

func d(y int, m time.Month, day, h, min int) time.Time {
	return time.Date(y, m, day, h, min, 0, 0, berlin)
}

func wrap(events ...string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Google Inc//Google Calendar 70.9054//EN\r\n" +
		strings.Join(events, "") + "END:VCALENDAR\r\n")
}

func vevent(lines ...string) string {
	return "BEGIN:VEVENT\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VEVENT\r\n"
}

func starts(evs []Event) []string {
	var out []string
	for _, e := range evs {
		if e.AllDay {
			out = append(out, e.StartDate+" "+e.Title)
		} else {
			out = append(out, e.Start.In(berlin).Format("2006-01-02 15:04")+" "+e.Title)
		}
	}
	return out
}

func assertStarts(t *testing.T, got []Event, want ...string) {
	t.Helper()
	g := starts(got)
	if strings.Join(g, "\n") != strings.Join(want, "\n") {
		t.Fatalf("occurrences differ\n got: %q\nwant: %q", g, want)
	}
}

func TestSingleEventsTimezonesAndText(t *testing.T) {
	ics := wrap(
		vevent("UID:a", "DTSTART;TZID=Europe/Berlin:20261001T090000", "DTEND;TZID=Europe/Berlin:20261001T100000",
			`SUMMARY:Elternabend\, Klasse 3b`, "LOCATION:Grundschule"),
		vevent("UID:b", "DTSTART:20261001T160000Z", "DTEND:20261001T170000Z", "SUMMARY:UTC-Termin"),
		vevent("UID:c", "DTSTART;VALUE=DATE:20261002", "DTEND;VALUE=DATE:20261003", "SUMMARY:Brückentag"),
		// folded line
		vevent("UID:d", "DTSTART;TZID=Europe/Berlin:20261003T080000", "DURATION:PT30M", "SUMMARY:Sehr langer Ti", " tel"),
		// outside window
		vevent("UID:e", "DTSTART;TZID=Europe/Berlin:20261020T080000", "SUMMARY:Später"),
		// cancelled
		vevent("UID:f", "DTSTART;TZID=Europe/Berlin:20261001T120000", "STATUS:CANCELLED", "SUMMARY:Abgesagt"),
	)
	evs, err := Expand(ics, "Familie", "#fff", d(2026, 10, 1, 0, 0), d(2026, 10, 8, 0, 0), berlin)
	if err != nil {
		t.Fatal(err)
	}
	assertStarts(t, evs,
		"2026-10-01 09:00 Elternabend, Klasse 3b",
		"2026-10-01 18:00 UTC-Termin",
		"2026-10-02 Brückentag",
		"2026-10-03 08:00 Sehr langer Titel",
	)
	if evs[0].Location != "Grundschule" || evs[3].End.Sub(evs[3].Start) != 30*time.Minute {
		t.Fatalf("unexpected details: %+v", evs)
	}
	if evs[2].EndDate != "2026-10-03" {
		t.Fatalf("all-day end: %s", evs[2].EndDate)
	}
}

func TestMultiDayAllDayOverlapsWindow(t *testing.T) {
	ics := wrap(vevent("UID:x", "DTSTART;VALUE=DATE:20260928", "DTEND;VALUE=DATE:20261004", "SUMMARY:Herbstferien"))
	evs, _ := Expand(ics, "F", "", d(2026, 10, 1, 0, 0), d(2026, 10, 8, 0, 0), berlin)
	assertStarts(t, evs, "2026-09-28 Herbstferien")
}

func TestWeeklyWithExdateOverrideAndDST(t *testing.T) {
	ics := wrap(
		vevent("UID:swim", "DTSTART;TZID=Europe/Berlin:20260908T170000", "DTEND;TZID=Europe/Berlin:20260908T180000",
			"RRULE:FREQ=WEEKLY;BYDAY=TU,TH", "EXDATE;TZID=Europe/Berlin:20261015T170000", "SUMMARY:Schwimmen"),
		// moved instance: Tue 20.10. 17:00 -> 18:30
		vevent("UID:swim", "RECURRENCE-ID;TZID=Europe/Berlin:20261020T170000",
			"DTSTART;TZID=Europe/Berlin:20261020T183000", "DTEND;TZID=Europe/Berlin:20261020T193000", "SUMMARY:Schwimmen (verschoben)"),
		// cancelled instance: Thu 29.10.
		vevent("UID:swim", "RECURRENCE-ID;TZID=Europe/Berlin:20261029T170000",
			"DTSTART;TZID=Europe/Berlin:20261029T170000", "STATUS:CANCELLED", "SUMMARY:Schwimmen"),
	)
	evs, _ := Expand(ics, "F", "", d(2026, 10, 13, 0, 0), d(2026, 11, 1, 0, 0), berlin)
	// DST ends 25.10.2026 – wall clock must stay 17:00.
	assertStarts(t, evs,
		"2026-10-13 17:00 Schwimmen",
		"2026-10-20 18:30 Schwimmen (verschoben)",
		"2026-10-22 17:00 Schwimmen",
		"2026-10-27 17:00 Schwimmen",
	)
}

func TestMonthlyYearlyCountUntil(t *testing.T) {
	ics := wrap(
		vevent("UID:m1", "DTSTART;TZID=Europe/Berlin:20260113T190000", "RRULE:FREQ=MONTHLY;BYDAY=2TU", "SUMMARY:Stammtisch"),
		vevent("UID:m2", "DTSTART;TZID=Europe/Berlin:20260130T150000", "RRULE:FREQ=MONTHLY;BYDAY=-1FR", "SUMMARY:Letzter Freitag"),
		vevent("UID:m3", "DTSTART;TZID=Europe/Berlin:20260131T080000", "RRULE:FREQ=MONTHLY", "SUMMARY:Am 31."),
		vevent("UID:y1", "DTSTART;VALUE=DATE:20150317", "RRULE:FREQ=YEARLY", "SUMMARY:Geburtstag"),
		vevent("UID:c1", "DTSTART;TZID=Europe/Berlin:20260101T070000", "RRULE:FREQ=DAILY;COUNT=3", "SUMMARY:Nur 3x"),
		vevent("UID:u1", "DTSTART;TZID=Europe/Berlin:20260301T070000", "RRULE:FREQ=WEEKLY;INTERVAL=2;UNTIL=20260330T000000Z", "SUMMARY:Bis Ende März"),
		vevent("UID:p1", "DTSTART;TZID=Europe/Berlin:20260102T100000", "RRULE:FREQ=MONTHLY;BYDAY=MO,TU,WE,TH,FR;BYSETPOS=-1", "SUMMARY:Letzter Werktag"),
	)
	evs, _ := Expand(ics, "F", "", d(2026, 1, 1, 0, 0), d(2026, 4, 1, 0, 0), berlin)
	assertStarts(t, evs,
		"2026-01-01 07:00 Nur 3x",
		"2026-01-02 07:00 Nur 3x",
		"2026-01-03 07:00 Nur 3x",
		"2026-01-13 19:00 Stammtisch",
		"2026-01-30 10:00 Letzter Werktag",
		"2026-01-30 15:00 Letzter Freitag",
		"2026-01-31 08:00 Am 31.",
		"2026-02-10 19:00 Stammtisch",
		"2026-02-27 10:00 Letzter Werktag",
		"2026-02-27 15:00 Letzter Freitag",
		"2026-03-01 07:00 Bis Ende März",
		"2026-03-10 19:00 Stammtisch",
		"2026-03-15 07:00 Bis Ende März",
		"2026-03-17 Geburtstag",
		"2026-03-27 15:00 Letzter Freitag",
		"2026-03-29 07:00 Bis Ende März",
		"2026-03-31 08:00 Am 31.",
		"2026-03-31 10:00 Letzter Werktag",
	)
}

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{"PT1H30M": 90 * time.Minute, "P1D": 24 * time.Hour, "P1W": 168 * time.Hour, "-PT15M": -15 * time.Minute, "P1DT2H": 26 * time.Hour}
	for in, want := range cases {
		got, err := parseDuration(in)
		if err != nil || got != want {
			t.Errorf("%s: got %v %v, want %v", in, got, err, want)
		}
	}
}
