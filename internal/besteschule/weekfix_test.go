package besteschule

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Friday slot 3 alternates BIO/PH, but beste.schule marks both as "B".
const weekFixFixture = `{
 "students": {"data": [{"id": 11, "forename": "Lukas"}]},
 "timetable": {"data": {
   "valid_from": "2026-08-17", "valid_to": "2027-07-09",
   "weeks": [{"nr": 40, "year": 2026, "types": ["A"]}, {"nr": 41, "year": 2026, "types": ["B"]}],
   "lessons": [
     {"weekday": 5, "nr": 1, "subject": {"name": "Englisch", "local_id": "EN"}, "time": {"from": "08:00", "to": "08:45"}},
     {"weekday": 5, "nr": 3, "weeks": ["B"], "subject": {"name": "Biologie", "local_id": "BIO"}, "group": {"local_id": "BioPh6s2"}, "time": {"from": "09:50", "to": "10:35"}},
     {"weekday": 5, "nr": 3, "weeks": ["B"], "subject": {"name": "Physik", "local_id": "PH"}, "group": {"local_id": "PhBio61"}, "time": {"from": "09:50", "to": "10:35"}}
   ]
 }}
}`

func TestWeekFix(t *testing.T) {
	var raw Raw
	if err := json.Unmarshal([]byte(weekFixFixture), &raw); err != nil {
		t.Fatal(err)
	}
	friA := time.Date(2026, 10, 2, 7, 0, 0, 0, berlin) // KW 40 = A
	friB := time.Date(2026, 10, 9, 7, 0, 0, 0, berlin) // KW 41 = B

	// without fix: A week has a gap, B week shows both
	if d := Build(&raw, friA, berlin, nil).Students[0].Day; strings.Contains(lessons(d), "Physik") {
		t.Fatalf("unfixed A week should miss PH:\n%s", lessons(d))
	}

	fixes, err := ParseWeekFixes("Fr:PH=A")
	if err != nil {
		t.Fatal(err)
	}
	a := lessons(Build(&raw, friA, berlin, nil, fixes...).Students[0].Day)
	if !strings.Contains(a, "Physik") || strings.Contains(a, "Biologie") {
		t.Fatalf("A week:\n%s", a)
	}
	b := lessons(Build(&raw, friB, berlin, nil, fixes...).Students[0].Day)
	if strings.Contains(b, "Physik") || !strings.Contains(b, "Biologie") {
		t.Fatalf("B week:\n%s", b)
	}

	// matching by group name and full subject name works too
	for _, s := range []string{"freitag:phbio61=a", "Fr:Physik=A"} {
		f, err := ParseWeekFixes(s)
		if err != nil {
			t.Fatal(err)
		}
		if a := lessons(Build(&raw, friA, berlin, nil, f...).Students[0].Day); !strings.Contains(a, "Physik") {
			t.Fatalf("%s: A week:\n%s", s, a)
		}
	}
}

func TestParseWeekFixes(t *testing.T) {
	f, err := ParseWeekFixes(" Fr:PH=A , Mo:KU=b ")
	if err != nil || len(f) != 2 || f[0].Weekday != time.Friday || f[1].Week != "B" {
		t.Fatalf("%+v %v", f, err)
	}
	for _, bad := range []string{"PH=A", "Fr:PH", "Xy:PH=A", "Fr:=A"} {
		if _, err := ParseWeekFixes(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
	if f, err := ParseWeekFixes(""); err != nil || f != nil {
		t.Fatalf("empty: %+v %v", f, err)
	}
}
