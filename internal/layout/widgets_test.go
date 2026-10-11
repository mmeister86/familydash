package layout

import "testing"

func TestCatalog(t *testing.T) {
	if GridCols != 12 || GridRows != 24 {
		t.Fatalf("grid = %dx%d, want 12x24", GridCols, GridRows)
	}
	want := []WidgetDef{
		{Type: WidgetClock, MinW: 4, MinH: 3},
		{Type: WidgetShopping, MinW: 4, MinH: 4},
		{Type: WidgetBriefing, MinW: 4, MinH: 4},
		{Type: WidgetNews, MinW: 4, MinH: 3},
		{Type: WidgetPhotos, MinW: 4, MinH: 4},
		{Type: WidgetSchool, MinW: 6, MinH: 4},
		{Type: WidgetCalendars, MinW: 6, MinH: 4},
	}
	if len(Catalog) != len(want) {
		t.Fatalf("catalog = %d entries, want %d", len(Catalog), len(want))
	}
	for i := range want {
		if Catalog[i] != want[i] {
			t.Errorf("catalog[%d] = %+v, want %+v", i, Catalog[i], want[i])
		}
	}
}

func TestLookup(t *testing.T) {
	def, ok := Lookup("school")
	if !ok || def.MinW != 6 || def.MinH != 4 {
		t.Errorf("Lookup(school) = %+v,%v, want 6x4,true", def, ok)
	}
	if _, ok := Lookup("toaster"); ok {
		t.Error("Lookup(toaster) should fail")
	}
}
