package exporters

import "testing"

func TestOTLPTracesURL(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "http://localhost:4318/v1/traces", false},
		{"http://jaeger:4318", "http://jaeger:4318/v1/traces", false},
		{"http://jaeger:4318/", "http://jaeger:4318/v1/traces", false},
		{"jaeger:4318", "http://jaeger:4318/v1/traces", false},
		{"https://otel.example.com/custom/path", "https://otel.example.com/custom/path", false},
		// Legacy Jaeger collector URLs keep working.
		{"http://localhost:14268/api/traces", "http://localhost:4318/v1/traces", false},
		{"http://jaeger:14268/api/traces", "http://jaeger:4318/v1/traces", false},
		{"http://", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := OTLPTracesURL(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("OTLPTracesURL(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("OTLPTracesURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
