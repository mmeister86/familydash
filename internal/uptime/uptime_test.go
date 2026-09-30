package uptime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Real answers of the "dashboard" status page (Uptime Kuma on Unraid).
const pageJSON = `{"config":{"slug":"dashboard","title":"Dashboard","description":null,"icon":"/icon.svg","theme":"auto","published":true,"showTags":false,"customCSS":"body {\n  \n}\n","footerText":null,"showPoweredBy":false,"googleAnalyticsId":null,"showCertificateExpiry":false},"incident":null,"publicGroupList":[{"id":1,"name":"Dienste","weight":1,"monitorList":[{"id":1,"name":"Unraid","sendUrl":0,"type":"http"}]}],"maintenanceList":[]}`
const beatsJSON = `{"heartbeatList":{"1":[{"status":1,"time":"2026-09-30 20:10:16.566","msg":"","ping":100},{"status":1,"time":"2026-09-30 20:15:16.689","msg":"","ping":94}]},"uptimeList":{"1_24":1}}`

func TestParsePageURL(t *testing.T) {
	for in, want := range map[string][2]string{
		"http://192.168.188.127:3001/status/dashboard":    {"http://192.168.188.127:3001", "dashboard"},
		"http://192.168.188.127:3001/status/dashboard/":   {"http://192.168.188.127:3001", "dashboard"},
		"https://kuma.example.com/sub/status/familie?x=1": {"https://kuma.example.com/sub", "familie"},
	} {
		base, slug, err := ParsePageURL(in)
		if err != nil || base != want[0] || slug != want[1] {
			t.Errorf("%s → %q %q %v, want %q %q", in, base, slug, err, want[0], want[1])
		}
	}
	for _, bad := range []string{"", "192.168.188.127:3001", "http://192.168.188.127:3001", "http://x/status/", "http://x/status/a/b"} {
		if _, _, err := ParsePageURL(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestFetchRealAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status-page/dashboard":
			w.Write([]byte(pageJSON))
		case "/api/status-page/heartbeat/dashboard":
			w.Write([]byte(beatsJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	s := NewService(srv.URL, "dashboard", "", 14)
	s.Refresh(context.Background())
	st := s.Snapshot()
	if st == nil || st.Error != "" {
		t.Fatalf("no status: %+v", st)
	}
	if st.Title != "Dashboard" || len(st.Monitors) != 1 {
		t.Fatalf("status: %+v", st)
	}
	m := st.Monitors[0]
	if m.Name != "Unraid" || m.State != StateUp || m.Ping != 94 || m.Uptime24 != 1 || m.Since != nil || m.CertDays != nil {
		t.Errorf("monitor: %+v", m)
	}
}

func TestDownSinceAndOrder(t *testing.T) {
	var page pageResponse
	json.Unmarshal([]byte(`{"config":{"title":"X"},"publicGroupList":[
		{"monitorList":[{"id":2,"name":"Jellyfin"},{"id":1,"name":"Internet"}]},
		{"monitorList":[{"id":3,"name":"Neu"},{"id":2,"name":"Jellyfin"}]}]}`), &page)
	beats := &heartbeatResponse{
		HeartbeatList: map[string][]heartbeat{
			"1": {{Status: 1, Time: "2026-09-30 20:00:00.000"}, {Status: 1, Time: "2026-09-30 20:01:00.000"}},
			"2": {
				{Status: 1, Time: "2026-09-30 19:58:00.000"},
				{Status: 0, Time: "2026-09-30 19:59:00.123", Msg: "timeout"},
				{Status: 0, Time: "2026-09-30 20:00:00.000"},
				{Status: 0, Time: "2026-09-30 20:01:00.000", Msg: " connect ECONNREFUSED "},
			},
		},
	}
	st := build(&page, beats, time.Now())
	if len(st.Monitors) != 3 {
		t.Fatalf("want 3 monitors (duplicate dropped), got %+v", st.Monitors)
	}
	if st.Monitors[0].Name != "Jellyfin" || st.Monitors[1].Name != "Internet" {
		t.Errorf("status page order lost: %+v", st.Monitors)
	}
	j := st.Monitors[0]
	want := time.Date(2026, 9, 30, 19, 59, 0, 123e6, time.UTC)
	if j.State != StateDown || j.Since == nil || !j.Since.Equal(want) || j.Msg != "connect ECONNREFUSED" {
		t.Errorf("jellyfin: %+v since %v", j, j.Since)
	}
	if st.Monitors[2].State != StateUnknown {
		t.Errorf("monitor without heartbeats: %+v", st.Monitors[2])
	}
}

func TestParseTimeISO(t *testing.T) {
	for _, v := range []string{"2026-09-30 20:15:16.689", "2026-09-30T20:15:16.689Z", "2026-09-30T22:15:16.689+02:00", "2026-09-30 20:15:16"} {
		tm, ok := parseTime(v)
		if !ok || tm.UTC().Hour() != 20 || tm.Minute() != 15 {
			t.Errorf("%s → %v %v", v, tm, ok)
		}
	}
}

const metrics = `# HELP monitor_cert_days_remaining The number of days remaining until the certificate expires
# TYPE monitor_cert_days_remaining gauge
monitor_cert_days_remaining{monitor_name="Mail",monitor_type="http",monitor_url="https://mail.example.com",monitor_hostname="null",monitor_port="null"} 9
monitor_cert_days_remaining{monitor_name="Say \"hi\"",monitor_type="http",monitor_url="https://x",monitor_hostname="null",monitor_port="null"} 61
monitor_cert_is_valid{monitor_name="Mail",monitor_type="http",monitor_url="https://mail.example.com",monitor_hostname="null",monitor_port="null"} 1
monitor_status{monitor_name="Unraid",monitor_type="http",monitor_url="http://192.168.188.127",monitor_hostname="null",monitor_port="null"} 1
`

func TestParseCerts(t *testing.T) {
	got, err := parseCerts(strings.NewReader(metrics))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["Mail"] != 9 || got[`Say "hi"`] != 61 {
		t.Errorf("certs: %v", got)
	}
}

func TestCertsWithAPIKey(t *testing.T) {
	page := strings.Replace(pageJSON, `"name":"Unraid"`, `"name":"Mail"`, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status-page/dashboard":
			w.Write([]byte(page))
		case "/api/status-page/heartbeat/dashboard":
			w.Write([]byte(beatsJSON))
		case "/metrics":
			if user, pass, ok := r.BasicAuth(); !ok || user != "" || pass != "secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(metrics))
		}
	}))
	defer srv.Close()

	s := NewService(srv.URL, "dashboard", "secret", 14)
	s.Refresh(context.Background())
	st := s.Snapshot()
	if st.Error != "" || st.Monitors[0].CertDays == nil || *st.Monitors[0].CertDays != 9 || st.CertWarn != 14 {
		t.Fatalf("status: %+v", st)
	}

	// wrong key: status still works, only the certificate is missing
	s = NewService(srv.URL, "dashboard", "wrong", 14)
	s.Refresh(context.Background())
	if st := s.Snapshot(); st.Error != "" || st.Monitors[0].CertDays != nil || st.Monitors[0].State != StateUp {
		t.Fatalf("status with wrong key: %+v", st)
	}
}

func TestKeepsLastGoodOnError(t *testing.T) {
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if strings.Contains(r.URL.Path, "heartbeat") {
			w.Write([]byte(beatsJSON))
		} else {
			w.Write([]byte(pageJSON))
		}
	}))
	defer srv.Close()

	s := NewService(srv.URL, "dashboard", "", 14)
	if s.Snapshot() != nil {
		t.Fatal("snapshot before the first refresh")
	}
	s.Refresh(context.Background())
	fail = true
	s.Refresh(context.Background())
	st := s.Snapshot()
	if st.Error == "" || len(st.Monitors) != 1 {
		t.Fatalf("want stale data with error, got %+v", st)
	}
}
