package services

import "testing"

func TestRoomLogoForDisplayDefaultsOnlyEmptyRoomIdentities(t *testing.T) {
	const classicLogo = "/images/wangzhe-header-logo.png"
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "new or legacy room without a logo", want: classicLogo},
		{name: "blank legacy value", value: " \t\n", want: classicLogo},
		{name: "explicit classic preset", value: classicLogo, want: classicLogo},
		{name: "another chosen preset", value: "/images/room-logos/crown-shield.webp", want: "/images/room-logos/crown-shield.webp"},
		{name: "uploaded PNG", value: "data:image/png;base64,AAAA", want: "data:image/png;base64,AAAA"},
		{name: "uploaded JPEG", value: "data:image/jpeg;base64,AAAA", want: "data:image/jpeg;base64,AAAA"},
		{name: "uploaded WebP", value: "data:image/webp;base64,AAAA", want: "data:image/webp;base64,AAAA"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := roomLogoForDisplay(test.value); got != test.want {
				t.Fatalf("roomLogoForDisplay(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestRoomLogoDefaultDoesNotChangeEmptyStoredValue(t *testing.T) {
	for _, value := range []string{"", " \t\n"} {
		stored, err := normalizeRoomLogo(value)
		if err != nil || stored != "" {
			t.Fatalf("clearing a custom logo must keep the stored default marker empty: %q, %v", stored, err)
		}
		if shown := roomLogoForDisplay(stored); shown != defaultRoomLogo {
			t.Fatalf("empty stored logo displays %q, want %q", shown, defaultRoomLogo)
		}
	}
}

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
