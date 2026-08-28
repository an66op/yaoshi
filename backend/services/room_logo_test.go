package services

import "testing"

func TestNormalizeRoomLogoAllowsBuiltInAndUploadedImages(t *testing.T) {
	values := []string{
		"",
		"/images/wangzhe-header-logo.png",
		"/images/room-logos/crown-crystal.webp",
		"/images/room-logos/crown-shield.webp",
		"/images/room-logos/crown-laurel.webp",
		"data:image/png;base64,AAAA",
		"data:image/jpeg;base64,AAAA",
		"data:image/webp;base64,AAAA",
	}
	for _, value := range values {
		got, err := normalizeRoomLogo(value)
		if err != nil || got != value {
			t.Fatalf("normalizeRoomLogo(%q) = %q, %v", value, got, err)
		}
	}
}

func TestNormalizeRoomLogoRejectsUntrustedPaths(t *testing.T) {
	values := []string{
		"https://example.com/logo.png",
		"//example.com/logo.png",
		"/images/room-logos/../secret.webp",
		"/images/room-logos/nested/crown.webp",
		"/images/room-logos/crown-other.webp",
		"data:image/svg+xml;base64,PHN2Zz4=",
	}
	for _, value := range values {
		if _, err := normalizeRoomLogo(value); err == nil {
			t.Fatalf("normalizeRoomLogo accepted unsafe value %q", value)
		}
	}
}
