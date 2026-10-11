package scenes

import (
	"fmt"
	"sort"

	"familydash/internal/scene"
)

// Schedule converts validated feed scenes into a wall schedule, skipping
// null starts. Feed times are pre-validated, so a bad clock is unreachable
// in practice and surfaces as an error.
func (f Feed) Schedule() (scene.Schedule, error) {
	var school, weekend []scene.Start
	for _, feedScene := range f.Scenes {
		if feedScene.Start.SchoolDay != nil {
			minute, err := scene.ParseClock(*feedScene.Start.SchoolDay)
			if err != nil {
				return scene.Schedule{}, fmt.Errorf("scene %q: %w", feedScene.ID, err)
			}
			school = append(school, scene.Start{ID: feedScene.ID, Name: feedScene.Name, Kind: feedScene.Kind, Minute: minute})
		}
		if feedScene.Start.Weekend != nil {
			minute, err := scene.ParseClock(*feedScene.Start.Weekend)
			if err != nil {
				return scene.Schedule{}, fmt.Errorf("scene %q: %w", feedScene.ID, err)
			}
			weekend = append(weekend, scene.Start{ID: feedScene.ID, Name: feedScene.Name, Kind: feedScene.Kind, Minute: minute})
		}
	}
	byMinute := func(starts []scene.Start) {
		sort.SliceStable(starts, func(i, j int) bool { return starts[i].Minute < starts[j].Minute })
	}
	byMinute(school)
	byMinute(weekend)
	return scene.Schedule{SchoolDay: school, Weekend: weekend}, nil
}
