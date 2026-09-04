package services

import "testing"

func TestDecodeStoredRebateConfigFailsClosed(t *testing.T) {
	missing, err := decodeStoredRebateConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if missing.Enabled || missing.RatePercent != 0 || missing.SettleMode != "daily" {
		t.Fatalf("missing config did not fail closed: %+v", missing)
	}

	valid, err := decodeStoredRebateConfig(`{"enabled":true,"rate_percent":0.5,"min_turnover":100,"settle_mode":"daily","auto_credit":false}`)
	if err != nil {
		t.Fatal(err)
	}
	if !valid.Enabled || valid.RatePercent != 0.5 || valid.MinTurnover != 100 {
		t.Fatalf("valid config changed: %+v", valid)
	}

	for name, raw := range map[string]string{
		"malformed":     `{`,
		"negative rate": `{"enabled":true,"rate_percent":-1,"min_turnover":0,"settle_mode":"daily"}`,
		"large rate":    `{"enabled":true,"rate_percent":101,"min_turnover":0,"settle_mode":"daily"}`,
		"negative min":  `{"enabled":true,"rate_percent":1,"min_turnover":-1,"settle_mode":"daily"}`,
		"unknown mode":  `{"enabled":true,"rate_percent":1,"min_turnover":0,"settle_mode":"hourly"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeStoredRebateConfig(raw); err == nil {
				t.Fatal("unsafe rebate config accepted")
			}
		})
	}
}

func TestNormalizeRebateSettingsForUpdateCanonicalizesAndRejects(t *testing.T) {
	encoded, err := normalizeRebateSettingsForUpdate(nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := decodeStoredRebateConfig(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.RatePercent != 0 {
		t.Fatalf("empty update did not store disabled configuration: %+v", cfg)
	}
	if _, err := normalizeRebateSettingsForUpdate([]byte(`{"enabled":true,"rate_percent":-0.1,"settle_mode":"daily"}`)); err == nil {
		t.Fatal("negative update was accepted")
	}
}
