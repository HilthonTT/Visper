package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		env          map[string]string
		wantListen   string
		wantBackends int
		wantErr      bool
	}{
		{"defaults", nil, nil, ":5004", 1, false},
		{"env", nil, map[string]string{"PROXY_LISTEN_ADDR": ":8080", "PROXY_BACKENDS": "http://a:1, http://b:2"}, ":8080", 2, false},
		{"flag beats env", []string{"-listen", ":9090"}, map[string]string{"PROXY_LISTEN_ADDR": ":8080"}, ":9090", 1, false},
		{"bad duration env", nil, map[string]string{"PROXY_READ_TIMEOUT": "soon"}, "", 0, true},
		{"bad backend", nil, map[string]string{"PROXY_BACKENDS": "localhost:5005"}, "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(tt.args, envMap(tt.env))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if err != nil {
				return
			}
			if cfg.LB.ListenAddr != tt.wantListen || len(cfg.LB.Backends) != tt.wantBackends {
				t.Fatalf("got listen=%q backends=%v", cfg.LB.ListenAddr, cfg.LB.Backends)
			}
		})
	}
}

func TestLoadFileDurationsAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lb.json")
	data := `{"health_check_interval":"5s","backends":[{"url":"http://api:5005"}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if time.Duration(cfg.HealthCheckInterval) != 5*time.Second || cfg.Backends[0].Weight != 1 ||
		cfg.MaxFailCount != DefaultMaxFailCount || cfg.Strategy != DefaultStrategy || cfg.ListenAddr != DefaultListenAddr {
		t.Fatalf("unexpected config %+v", cfg)
	}
}

func writeFile(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lb.json")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Regression: settings the file omits used to be reset to the built-in
// defaults instead of keeping their env/flag values.
func TestFileOverlaysEnvAndFlags(t *testing.T) {
	path := writeFile(t, `{"strategy":"ip_hash"}`)
	env := map[string]string{
		"PROXY_CONFIG":                path,
		"PROXY_LISTEN_ADDR":           ":7000",
		"PROXY_BACKENDS":              "http://a:1,http://b:1",
		"PROXY_HEALTH_CHECK_INTERVAL": "5s",
	}
	app, err := Load([]string{"-max-fail-count", "9"}, envMap(env))
	if err != nil {
		t.Fatal(err)
	}
	lb := app.LB
	if lb.Strategy != "ip_hash" || lb.ListenAddr != ":7000" || len(lb.Backends) != 2 ||
		time.Duration(lb.HealthCheckInterval) != 5*time.Second || lb.MaxFailCount != 9 {
		t.Fatalf("unexpected config %+v", lb)
	}

	// A reload re-reads the file over the same env/flag base.
	if err := os.WriteFile(path, []byte(`{"backends":[{"url":"http://c:1","weight":2}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	re, err := app.LoadLB()
	if err != nil {
		t.Fatal(err)
	}
	if re.Strategy != DefaultStrategy || re.ListenAddr != ":7000" || len(re.Backends) != 1 || re.Backends[0].Weight != 2 ||
		time.Duration(re.HealthCheckInterval) != 5*time.Second {
		t.Fatalf("unexpected reloaded config %+v", re)
	}
	if len(app.LB.Backends) != 2 {
		t.Fatal("reload mutated the running config")
	}
}

func TestLoadFileRejects(t *testing.T) {
	tests := map[string]string{
		"unknown field":   `{"backends":[{"url":"http://a:1"}],"helth_check_path":"/x"}`,
		"duplicate url":   `{"backends":[{"url":"http://a:1"},{"url":"http://a:1"}]}`,
		"empty backends":  `{"backends":[]}`,
		"bad duration":    `{"health_check_interval":"soon","backends":[{"url":"http://a:1"}]}`,
		"trailing object": `{"backends":[{"url":"http://a:1"}]} {}`,
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadFile(writeFile(t, data)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
