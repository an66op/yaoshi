package services

import (
	settingsmodel "backend/data/models/settings"
	"encoding/json"
	"testing"
)

func TestNormalizeGameSettingsBackfillsLegacyDisplayFields(t *testing.T) {
	var result map[string]any
	if err := json.Unmarshal(normalizeGameSettings(`{"seal_seconds":45,"show_member_profit":false}`), &result); err != nil {
		t.Fatalf("decode normalized settings: %v", err)
	}
	if result["seal_seconds"] != float64(45) {
		t.Fatalf("stored value was not preserved: %#v", result["seal_seconds"])
	}
	if result["show_member_profit"] != false {
		t.Fatalf("explicit false was not preserved: %#v", result["show_member_profit"])
	}
	if result["room_activity_bots_per_room"] != float64(defaultWorkspaceRobotCount) {
		t.Fatalf("robot default = %#v, want %d", result["room_activity_bots_per_room"], defaultWorkspaceRobotCount)
	}
	for _, key := range []string{"show_member_turnover", "show_member_rebate", "show_orders_tool"} {
		if result[key] != true {
			t.Fatalf("legacy field %s was not enabled: %#v", key, result[key])
		}
	}
	if result["lottery_source_url"] != defaultLotterySourceURL {
		t.Fatalf("legacy source URL = %#v, want %q", result["lottery_source_url"], defaultLotterySourceURL)
	}
}

func TestNormalizeLotterySourceURLAllowsOnlyCredentialFreeHTTPS(t *testing.T) {
	for _, test := range []struct {
		name, value, want string
	}{
		{name: "default", value: "", want: defaultLotterySourceURL},
		{name: "trimmed HTTPS", value: "  HTTPS://draw.example/mobile?q=1  ", want: "https://draw.example/mobile?q=1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeLotterySourceURL(test.value)
			if err != nil || got != test.want {
				t.Fatalf("normalize %q = %q, %v; want %q", test.value, got, err, test.want)
			}
		})
	}
	for _, value := range []string{
		"javascript:alert(1)", "data:text/html,unsafe", "http://draw.example/mobile",
		"//draw.example/mobile", "/mobile", "https://user:secret@draw.example/mobile",
	} {
		if got, err := normalizeLotterySourceURL(value); err == nil {
			t.Fatalf("unsafe source URL %q accepted as %q", value, got)
		}
	}
	var sanitized map[string]any
	if err := json.Unmarshal(normalizeGameSettings(`{"lottery_source_url":"javascript:alert(1)"}`), &sanitized); err != nil {
		t.Fatal(err)
	}
	if sanitized["lottery_source_url"] != defaultLotterySourceURL {
		t.Fatalf("unsafe stored URL leaked through settings read: %#v", sanitized["lottery_source_url"])
	}
}

func TestNormalizedGameSettingsForUpdatePreservesAndValidatesSourceURL(t *testing.T) {
	existing := `{"seal_seconds":30,"lottery_source_url":"https://existing.example/draw"}`
	got, err := normalizedGameSettingsForUpdate(json.RawMessage(`{"seal_seconds":45}`), existing, nil)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := json.Unmarshal(got, &values); err != nil {
		t.Fatal(err)
	}
	if values["lottery_source_url"] != "https://existing.example/draw" || values["seal_seconds"] != float64(45) {
		t.Fatalf("updated settings = %#v", values)
	}
	for _, raw := range []string{
		`{"lottery_source_url":"javascript:alert(1)"}`,
		`{"lottery_source_url":"data:text/html,unsafe"}`,
		`{"lottery_source_url":123}`,
	} {
		if _, err := normalizedGameSettingsForUpdate(json.RawMessage(raw), existing, nil); err == nil {
			t.Fatalf("unsafe nested source setting accepted: %s", raw)
		}
	}
}

func TestLotterySourceURLTopLevelAndGameRoundTripStaySynchronized(t *testing.T) {
	explicit := "https://configured.example/results?room=88001"
	game, err := normalizedGameSettingsForUpdate(
		json.RawMessage(`{"seal_seconds":30,"lottery_source_url":"https://old.example/results"}`),
		`{"lottery_source_url":"https://existing.example/results"}`,
		&explicit,
	)
	if err != nil {
		t.Fatal(err)
	}
	view := toSettingsView(&settingsmodel.SystemConfig{GameSettingsJSON: string(game)})
	var values map[string]any
	if err := json.Unmarshal(view.Game, &values); err != nil {
		t.Fatal(err)
	}
	if view.LotterySourceURL != explicit || values["lottery_source_url"] != explicit {
		t.Fatalf("source URL drifted: top-level=%q game=%#v", view.LotterySourceURL, values["lottery_source_url"])
	}
}
