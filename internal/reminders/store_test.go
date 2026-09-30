package reminders

import (
	"encoding/json"
	"testing"
	"time"
)

func TestApplyFormatsAndPersistence(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	s, err := NewStore(dir, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	// 1) full snapshot from the Mac bridge, items as objects
	var full Push
	json.Unmarshal([]byte(`{"source":"mac","lists":[{"name":"Familie","items":[
		{"title":"Zahnarzt anrufen","priority":1},
		{"title":"Müll raus","due":"2026-09-30T18:00:00+02:00"},
		{"title":"  "}]}]}`), &full)
	if err := s.Apply(full, now); err != nil {
		t.Fatal(err)
	}

	// 2) single list from an iOS Shortcut: bare strings + newline text
	var sc Push
	json.Unmarshal([]byte(`{"source":"iphone","list":"Einkauf","items":["Milch"],"text":"Brot\n\nButter\n"}`), &sc)
	if err := s.Apply(sc, now); err != nil {
		t.Fatal(err)
	}

	// reload from disk
	s2, err := NewStore(dir, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	snap := s2.Snapshot(now.Add(time.Hour))
	if snap.Stale || len(snap.Lists) != 2 {
		t.Fatalf("snapshot: %+v", snap)
	}
	ein, fam := snap.Lists[0], snap.Lists[1]
	if ein.Name != "Einkauf" || len(ein.Items) != 3 || ein.Items[0].Title != "Brot" {
		t.Errorf("Einkauf: %+v", ein)
	}
	if len(fam.Items) != 2 || fam.Items[0].Title != "Müll raus" {
		t.Errorf("Familie (dated first): %+v", fam)
	}
	if !s2.Snapshot(now.Add(3 * time.Hour)).Stale {
		t.Error("expected stale after 3h")
	}
	if err := s2.Apply(Push{}, now); err == nil {
		t.Error("empty push should fail")
	}
}
