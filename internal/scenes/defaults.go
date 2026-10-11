package scenes

func sp(s string) *string { return &s }

func bp(b bool) *bool { return &b }

// DefaultFeed is the built-in wall program: the same five scenes the Convex
// seed publishes (ids, times and 12x24 layouts). The wall runs on it while
// the feed never loaded. Pinned valid by TestDefaultFeedValid.
func DefaultFeed() Feed {
	return Feed{
		Version:  1,
		Revision: 0,
		Grid:     Grid{Cols: 12, Rows: 24},
		Scenes: []Scene{
			{
				ID: "morning", Name: "Morgen", Kind: "layout",
				Start: Start{SchoolDay: sp("06:45"), Weekend: sp("07:30")},
				Widgets: []Widget{
					{ID: "clock", Type: "clock", X: 0, Y: 0, W: 12, H: 5, Props: &WidgetProps{Wide: bp(true), Hints: bp(true)}},
					{ID: "school", Type: "school", X: 0, Y: 5, W: 6, H: 11, Props: &WidgetProps{Mode: "plan-meal", MealDay: "today"}},
					{ID: "briefing", Type: "briefing", X: 6, Y: 5, W: 6, H: 6, Props: &WidgetProps{Kind: "today"}, Fallback: "shopping"},
					{ID: "news", Type: "news", X: 6, Y: 11, W: 6, H: 5},
					{ID: "calendars", Type: "calendars", X: 0, Y: 16, W: 12, H: 8},
				},
			},
			{
				ID: "day", Name: "Tag", Kind: "layout",
				Start: Start{SchoolDay: sp("09:00"), Weekend: sp("10:00")},
				Widgets: []Widget{
					{ID: "clock", Type: "clock", X: 0, Y: 0, W: 6, H: 5, Props: &WidgetProps{Wide: bp(false), Hints: bp(false)}},
					{ID: "shopping", Type: "shopping", X: 6, Y: 0, W: 6, H: 5},
					{ID: "photos", Type: "photos", X: 0, Y: 5, W: 6, H: 10},
					{ID: "news", Type: "news", X: 6, Y: 5, W: 6, H: 10},
					{ID: "calendars", Type: "calendars", X: 0, Y: 15, W: 12, H: 9},
				},
			},
			{
				ID: "afternoon", Name: "Nachmittag", Kind: "layout",
				Start: Start{SchoolDay: sp("14:00"), Weekend: sp("14:00")},
				Widgets: []Widget{
					{ID: "clock", Type: "clock", X: 0, Y: 0, W: 6, H: 5, Props: &WidgetProps{Wide: bp(false), Hints: bp(false)}},
					{ID: "shopping", Type: "shopping", X: 6, Y: 0, W: 6, H: 5},
					{ID: "school", Type: "school", X: 0, Y: 5, W: 12, H: 11, Props: &WidgetProps{Mode: "full", MealDay: "today"}},
					{ID: "calendars", Type: "calendars", X: 0, Y: 16, W: 12, H: 8},
				},
			},
			{
				ID: "evening", Name: "Abend", Kind: "layout",
				Start: Start{SchoolDay: sp("19:00"), Weekend: sp("19:00")},
				Widgets: []Widget{
					{ID: "clock", Type: "clock", X: 0, Y: 0, W: 12, H: 5, Props: &WidgetProps{Wide: bp(true), Hints: bp(false)}},
					{ID: "briefing", Type: "briefing", X: 0, Y: 5, W: 6, H: 6, Props: &WidgetProps{Kind: "tomorrow"}},
					{ID: "school", Type: "school", X: 6, Y: 5, W: 6, H: 11, Props: &WidgetProps{Mode: "plan-meal", MealDay: "tomorrow"}},
					{ID: "photos", Type: "photos", X: 0, Y: 11, W: 6, H: 5},
					{ID: "calendars", Type: "calendars", X: 0, Y: 16, W: 12, H: 8},
				},
			},
			{
				ID: "night", Name: "Nacht", Kind: "night",
				Start: Start{SchoolDay: sp("21:30"), Weekend: sp("21:30")},
			},
		},
	}
}
