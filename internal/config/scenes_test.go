package config

import "testing"

func TestScenesFeedEnv(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.FamilyAppScenesToken != "" {
		t.Errorf("default FAMILY_APP_SCENES_TOKEN = %q, want empty", c.FamilyAppScenesToken)
	}
	if c.ScenesCacheFile != "/data/scenes-cache.json" {
		t.Errorf("default SCENES_CACHE_FILE = %q", c.ScenesCacheFile)
	}

	t.Setenv("FAMILY_APP_SCENES_TOKEN", "scenes-secret")
	t.Setenv("SCENES_CACHE_FILE", "/tmp/test-scenes-cache.json")
	if c, err = Load(); err != nil {
		t.Fatal(err)
	} else if c.FamilyAppScenesToken != "scenes-secret" ||
		c.ScenesCacheFile != "/tmp/test-scenes-cache.json" {
		t.Errorf("scenes env: token=%q cache=%q", c.FamilyAppScenesToken, c.ScenesCacheFile)
	}
}
