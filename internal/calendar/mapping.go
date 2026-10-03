package calendar

import "time"

// Exact person/mapping resolution for central (Convex) snapshots.
//
// A central snapshot carries explicit person bindings (kind + external id)
// and every calendar lists the persons it belongs to. All views resolve
// people and calendars through these stable ids only: a rename or a new
// array order never changes a mapping, and a missing binding never falls
// back to name guessing. In local mode no bindings exist and every lookup
// reports "not found" so the legacy name behavior stays untouched.

// PersonForSource resolves one exact stable binding in central mode: the
// person bound to (kind, externalID), e.g. ("timetable", "plan-01") or
// ("besteschule", "<student id>"). Anything else — local snapshots,
// unknown kinds/ids, name lookalikes — returns false.
func PersonForSource(s Snapshot, kind, externalID string) (PersonBinding, bool) {
	if !s.Central || kind == "" || externalID == "" {
		return PersonBinding{}, false
	}
	for _, b := range s.Bindings {
		if b.Kind == kind && b.ExternalID == externalID {
			return b, true
		}
	}
	return PersonBinding{}, false
}

// PersonByID returns the central person record, if present.
func PersonByID(s Snapshot, personID string) (Person, bool) {
	if !s.Central || personID == "" {
		return Person{}, false
	}
	for _, p := range s.People {
		if p.ID == personID {
			return p, true
		}
	}
	return Person{}, false
}

// CalendarsForPerson returns every calendar assigned to the person, in
// snapshot (display) order. A person with several school calendars sees
// events from all of them; renames and reorders cannot change the set
// because membership is by stable person id, not by name or index.
func CalendarsForPerson(s Snapshot, personID string) []CalendarInfo {
	if !s.Central || personID == "" {
		return nil
	}
	var out []CalendarInfo
	for _, c := range s.Calendars {
		for _, pid := range c.PersonIDs {
			if pid == personID {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// AssignedCalendarState reports the calendar contribution of one person's
// assigned calendars of one panel to a derived timestamp: the oldest real
// success time, and the stable ids of every required calendar that never
// loaded. Disabled calendars are not required. A missing entry means the
// derived data is explicitly unknown — callers must surface a zero
// timestamp, never "now".
func AssignedCalendarState(s Snapshot, personID, panel string) (oldest time.Time, missing []string) {
	for _, c := range CalendarsForPerson(s, personID) {
		if c.Panel != panel {
			continue
		}
		if c.Status.Freshness == "disabled" {
			continue
		}
		if c.Status.LastSuccessAt.IsZero() {
			missing = append(missing, c.CalendarID)
			continue
		}
		if oldest.IsZero() || c.Status.LastSuccessAt.Before(oldest) {
			oldest = c.Status.LastSuccessAt
		}
	}
	return oldest, missing
}
