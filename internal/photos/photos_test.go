package photos

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanAndServe(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.jpg", "jpeg")
	write(t, dir, "Sommer/b.PNG", "png")
	write(t, dir, "._a.jpg", "appledouble")  // macOS SMB junk
	write(t, dir, "@eaDir/c.jpg", "thumb")   // Synology junk
	write(t, dir, ".hidden/d.jpg", "hidden") // hidden folder
	write(t, dir, "IMG_0001.HEIC", "heic")   // browser can't show it
	write(t, dir, "notes.txt", "text")
	write(t, dir, "empty.jpg", "")
	secret := filepath.Join(t.TempDir(), "secret.jpg")
	os.WriteFile(secret, []byte("secret"), 0o644)
	os.Symlink(secret, filepath.Join(dir, "link.jpg"))

	s := NewService(dir, 30*time.Second, true)
	s.Scan()
	snap := s.Snapshot()
	var got []string
	for _, it := range snap.Items {
		got = append(got, it.Path)
	}
	want := []string{"Sommer/b.PNG", "a.jpg"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("items = %v, want %v", got, want)
	}
	if snap.Interval != 30 || snap.Error != "" {
		t.Errorf("snapshot = %+v", snap)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /photos/{path...}", s)
	get := func(p string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		return rec
	}
	if rec := get("/photos/Sommer/b.PNG"); rec.Code != 200 || rec.Body.String() != "png" || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("serve: %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	for _, p := range []string{"/photos/notes.txt", "/photos/link.jpg", "/photos/../secret.jpg", "/photos/._a.jpg"} {
		if rec := get(p); rec.Code == 200 {
			t.Errorf("%s: served, want 404", p)
		}
	}
}

func TestMissingFolder(t *testing.T) {
	s := NewService(filepath.Join(t.TempDir(), "pictures"), time.Minute, false)
	s.Scan()
	snap := s.Snapshot()
	if len(snap.Items) != 0 || snap.Error == "" {
		t.Errorf("missing folder: %+v", snap)
	}
	// folder appears later → next scan picks it up
	write(t, s.Dir, "x.jpg", "x")
	s.Scan()
	if snap := s.Snapshot(); len(snap.Items) != 1 || snap.Error != "" {
		t.Errorf("after mkdir: %+v", snap)
	}
}
