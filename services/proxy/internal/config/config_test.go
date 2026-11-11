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
