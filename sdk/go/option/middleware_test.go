package option

import (
	"strings"
	"testing"
)

func TestRedactSensitiveHeaders(t *testing.T) {
	dump := "POST /api/v1/rooms HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"X-User-Token: secret-token-value\r\n" +
		"cookie: visper_user_id=abc\r\n" +
		"Authorization: Bearer xyz\r\n" +
		"Content-Type: application/json\r\n\r\n{}"
	got := redactSensitiveHeaders(dump)
	for _, secret := range []string{"secret-token-value", "visper_user_id=abc", "Bearer xyz"} {
		if strings.Contains(got, secret) {
			t.Errorf("dump still contains %q:\n%s", secret, got)
		}
	}
	if !strings.Contains(got, "X-User-Token: [REDACTED]\r\n") || !strings.Contains(got, "Content-Type: application/json") {
		t.Errorf("unexpected redaction:\n%s", got)
	}
}
