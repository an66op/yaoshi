package services

import "testing"

func TestIsOfficialSourceGroupUsesFixedAllowlist(t *testing.T) {
	for _, group := range []string{"china-welfare", "china-sport", "taiwan-bingo", "taiwan-lottery", "163-highfreq", "163-pc28", "163-bingo", "163-marksix", "sg-ssc-verified"} {
		if !IsOfficialSourceGroup(group) {
			t.Errorf("expected %q to be an official source group", group)
		}
	}
	if IsOfficialSourceGroup("168-marksix") {
		t.Fatal("retired 168 Mark Six group remains callable")
	}
	for _, group := range []string{"", "https://example.com", "../china-welfare", "168-highfreq", "168-bingo", "unknown"} {
		if IsOfficialSourceGroup(group) {
			t.Errorf("unexpected source group accepted: %q", group)
		}
	}
}
