package config

import (
	"testing"
)

func TestGetPostgresConnectionStringQuotesValues(t *testing.T) {
	cfg := &Config{Postgres: PostgresConfig{
		Host:     "db",
		Port:     "5432",
		User:     "app",
		Password: `p a'ss\word`,
		DbName:   "visper",
		SSLMode:  "disable",
	}}

	got := cfg.GetPostgresConnectionString()
	want := `host='db' port='5432' user='app' password='p a\'ss\\word' dbname='visper' sslmode='disable'`
	if got != want {
		t.Fatalf("GetPostgresConnectionString()\n got: %s\nwant: %s", got, want)
	}
}

func TestLoadConfigEnvOverride(t *testing.T) {
	t.Setenv(EnvPrefix+"_POSTGRES_PASSWORD", "from-env")
	t.Setenv(EnvPrefix+"_REDIS_HOST", "redis.internal")

	v, err := LoadConfig("config-docker", "yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	cfg, err := ParseConfig(v)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	if cfg.Postgres.Password != "from-env" {
		t.Errorf("Postgres.Password = %q, want %q", cfg.Postgres.Password, "from-env")
	}
	if cfg.Redis.Host != "redis.internal" {
		t.Errorf("Redis.Host = %q, want %q", cfg.Redis.Host, "redis.internal")
	}
	// Untouched keys keep the file value.
	if cfg.Postgres.DbName != "visper_db" {
		t.Errorf("Postgres.DbName = %q, want %q", cfg.Postgres.DbName, "visper_db")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidate(t *testing.T) {
	valid := func() Config {
		return Config{
			Server:   ServerConfig{InternalPort: "1", ExternalPort: "1", Domain: "d"},
			Postgres: PostgresConfig{Host: "h", Port: "1", DbName: "db"},
			Redis:    RedisConfig{Host: "h", Port: "1"},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"valid", func(*Config) {}, false},
		{"missing domain", func(c *Config) { c.Server.Domain = "" }, true},
		{"missing postgres host", func(c *Config) { c.Postgres.Host = "" }, true},
		{"missing redis port", func(c *Config) { c.Redis.Port = "" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid()
			tt.mutate(&cfg)
			if err := cfg.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
