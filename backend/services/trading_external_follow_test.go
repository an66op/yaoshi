package services

import (
	"backend/data/models/user"
	"math"
	"testing"
)

func TestNormalizeUserExternalFollowKeepsNonSecretPreparationData(t *testing.T) {
	result, err := normalizeUserExternalFollow(UpdateUserExternalFollowInput{
		TargetPlatform: "  示例平台  ",
		TargetAccount:  "member-88001",
		EndpointLabel:  "华东线路 A",
		SingleLimit:    12.345,
		DailyLimit:     100,
		Remark:         "  等待平台提供正式连接器  ",
	})
	if err != nil {
		t.Fatalf("normalize external follow: %v", err)
	}
	if result.targetPlatform != "示例平台" || result.targetAccount != "member-88001" || result.endpointLabel != "华东线路 A" {
		t.Fatalf("unexpected identifiers: %#v", result)
	}
	if result.singleLimitCents != 1235 || result.dailyLimitCents != 10000 {
		t.Fatalf("unexpected cents: single=%d daily=%d", result.singleLimitCents, result.dailyLimitCents)
	}
	if result.remark != "等待平台提供正式连接器" {
		t.Fatalf("remark = %q", result.remark)
	}
}

func TestNormalizeUserExternalFollowRejectsUnsafeOrImpossibleValues(t *testing.T) {
	tests := []struct {
		name  string
		input UpdateUserExternalFollowInput
	}{
		{name: "daily below single", input: UpdateUserExternalFollowInput{SingleLimit: 20, DailyLimit: 10}},
		{name: "negative", input: UpdateUserExternalFollowInput{SingleLimit: -1}},
		{name: "too large", input: UpdateUserExternalFollowInput{DailyLimit: 100_000_000.01}},
		{name: "control character", input: UpdateUserExternalFollowInput{EndpointLabel: "endpoint\nsecret"}},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			if _, err := normalizeUserExternalFollow(item.input); err == nil {
				t.Fatal("invalid external-follow preparation was accepted")
			}
		})
	}
}

func TestExternalFollowStatusCannotClaimConnectivity(t *testing.T) {
	result := externalFollowConfig(user.User{FlyTargetPlatform: "示例平台", FlyTargetAccount: "member-1"})
	if result.Status != "not_connected" || result.Capability != "configuration_only" {
		t.Fatalf("external status = %#v", result)
	}
}

func TestTradingUpdateRejectsNonFiniteRatesBeforeDatabaseWork(t *testing.T) {
	service := &TradingAdminService{}
	for _, rate := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := service.Update(1, UpdateUserTradingInput{FlyMode: "custom", FlyRate: rate, RebateMode: "inherit"})
		if err == nil {
			t.Fatalf("non-finite fly rate %v was accepted", rate)
		}
	}
}
