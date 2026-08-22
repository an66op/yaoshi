package config

import "testing"

func TestOriginAllowed(t *testing.T) {
	allowed := []string{"http://localhost:5173", "https://example.com"}
	for _, origin := range []string{"http://localhost:5173", "https://example.com", ""} {
		if !OriginAllowed(origin, allowed, "release") {
			t.Fatalf("expected %q to be allowed", origin)
		}
	}
	for _, origin := range []string{"https://evil.example", "http://localhost:5173/path", "ftp://localhost:5173"} {
		if OriginAllowed(origin, allowed, "release") {
			t.Fatalf("expected %q to be denied", origin)
		}
	}
}

func TestOriginAllowedDevelopmentFallback(t *testing.T) {
	if !OriginAllowed("http://127.0.0.1:5173", nil, "debug") {
		t.Fatal("local development origin should be allowed without explicit config")
	}
	if OriginAllowed("https://evil.example", nil, "debug") {
		t.Fatal("non-local origin must not be allowed by the development fallback")
	}
}
