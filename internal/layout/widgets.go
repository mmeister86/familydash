// Package layout mirrors the wall-dashboard widget catalog from the family
// app (convex/lib/dashboardWidgets.ts): the known widget types and their
// minimum tile sizes on the scene grid.
package layout

// GridCols and GridRows fix the portrait scene grid both repos share.
const (
	GridCols = 12
	GridRows = 24
)

// WidgetType is a tile kind the wall frontend can render.
type WidgetType string

// Known widget types, mirroring the Convex catalog.
const (
	WidgetClock     WidgetType = "clock"
	WidgetShopping  WidgetType = "shopping"
	WidgetBriefing  WidgetType = "briefing"
	WidgetNews      WidgetType = "news"
	WidgetPhotos    WidgetType = "photos"
	WidgetSchool    WidgetType = "school"
	WidgetCalendars WidgetType = "calendars"
)

// WidgetDef pins one catalog entry: the type and its minimum tile size.
type WidgetDef struct {
	Type       WidgetType
	MinW, MinH int
}

// Catalog lists every widget type with its minimum size.
var Catalog = []WidgetDef{
	{Type: WidgetClock, MinW: 4, MinH: 3},
	{Type: WidgetShopping, MinW: 4, MinH: 4},
	{Type: WidgetBriefing, MinW: 4, MinH: 4},
	{Type: WidgetNews, MinW: 4, MinH: 3},
	{Type: WidgetPhotos, MinW: 4, MinH: 4},
	{Type: WidgetSchool, MinW: 6, MinH: 4},
	{Type: WidgetCalendars, MinW: 6, MinH: 4},
}

// Lookup returns the catalog entry for a type string.
func Lookup(widgetType string) (WidgetDef, bool) {
	for _, def := range Catalog {
		if string(def.Type) == widgetType {
			return def, true
		}
	}
	return WidgetDef{}, false
}
