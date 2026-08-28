package services

import "testing"

func TestIsOfficialSourceGroupUsesFixedAllowlist(t *testing.T) {
	for _, group := range []string{"china-welfare", "china-sport", "taiwan-bingo", "taiwan-lottery", "168-highfreq", "168-marksix", "168-bingo"} {
		if !IsOfficialSourceGroup(group) {
			t.Errorf("expected %q to be an official source group", group)
		}
	}
	for _, group := range []string{"", "https://example.com", "../china-welfare", "unknown"} {
		if IsOfficialSourceGroup(group) {
			t.Errorf("unexpected source group accepted: %q", group)
		}
	}
}
