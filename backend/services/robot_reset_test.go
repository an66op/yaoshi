package services

import (
	apperrors "backend/errors"
	"math"
	"testing"
)

func TestDefaultWorkspaceRobotCountIsTen(t *testing.T) {
	if defaultWorkspaceRobotCount != 10 {
		t.Fatalf("defaultWorkspaceRobotCount = %d, want 10", defaultWorkspaceRobotCount)
	}
}

func TestRandomRobotResetPlansUseAliasLibraryAndBalanceRange(t *testing.T) {
	config, err := normalizeRobotResetInput(ResetWorkspaceRobotsInput{
		RequestID: "reset-request-001", Mode: "random", BalanceMin: 1000, BalanceMax: 2000,
	})
	if err != nil {
		t.Fatalf("normalizeRobotResetInput() error = %v", err)
	}
	first, err := buildRobotResetPlans(config, 41, defaultWorkspaceRobotCount)
	if err != nil {
		t.Fatalf("buildRobotResetPlans() error = %v", err)
	}
	second, err := buildRobotResetPlans(config, 41, defaultWorkspaceRobotCount)
	if err != nil {
		t.Fatalf("second buildRobotResetPlans() error = %v", err)
	}
	aliases := make(map[string]struct{}, len(roomActivityAliases))
	for _, alias := range roomActivityAliases {
		aliases[alias] = struct{}{}
	}
	seen := make(map[string]struct{}, len(first))
	for index, plan := range first {
		if plan != second[index] {
			t.Fatalf("same idempotency request produced different plan at %d: %#v != %#v", index, plan, second[index])
		}
		if _, ok := aliases[plan.nickname]; !ok {
			t.Fatalf("nickname %q is not from the existing robot alias library", plan.nickname)
		}
		if _, duplicated := seen[plan.nickname]; duplicated {
			t.Fatalf("nickname %q was generated twice", plan.nickname)
		}
		seen[plan.nickname] = struct{}{}
		if plan.balanceCents < 100_000 || plan.balanceCents > 200_000 {
			t.Fatalf("balance %d is outside requested range", plan.balanceCents)
		}
	}
}

func TestCustomRobotResetPlansAppendStableSequence(t *testing.T) {
	config, err := normalizeRobotResetInput(ResetWorkspaceRobotsInput{
		RequestID: "reset-request-002", Mode: "custom", NicknamePrefix: "幸运用户", Balance: 8888.88,
	})
	if err != nil {
		t.Fatalf("normalizeRobotResetInput() error = %v", err)
	}
	plans, err := buildRobotResetPlans(config, 8, 10)
	if err != nil {
		t.Fatalf("buildRobotResetPlans() error = %v", err)
	}
	if plans[0].nickname != "幸运用户01" || plans[9].nickname != "幸运用户10" {
		t.Fatalf("unexpected custom nicknames: first=%q last=%q", plans[0].nickname, plans[9].nickname)
	}
	for _, plan := range plans {
		if plan.balanceCents != 888_888 {
			t.Fatalf("balance cents = %d, want 888888", plan.balanceCents)
		}
	}
}

func TestRobotResetValidationRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name  string
		input ResetWorkspaceRobotsInput
		code  string
	}{
		{name: "short request id", input: ResetWorkspaceRobotsInput{RequestID: "short", Mode: "custom", NicknamePrefix: "机器人", Balance: 1}, code: "INVALID_REQUEST_ID"},
		{name: "unknown mode", input: ResetWorkspaceRobotsInput{RequestID: "reset-request-003", Mode: "other"}, code: "INVALID_RESET_MODE"},
		{name: "empty prefix", input: ResetWorkspaceRobotsInput{RequestID: "reset-request-004", Mode: "custom", Balance: 1}, code: "INVALID_NICKNAME_PREFIX"},
		{name: "negative balance", input: ResetWorkspaceRobotsInput{RequestID: "reset-request-005", Mode: "custom", NicknamePrefix: "机器人", Balance: -1}, code: "INVALID_BALANCE"},
		{name: "non finite balance", input: ResetWorkspaceRobotsInput{RequestID: "reset-request-006", Mode: "random", BalanceMin: math.NaN(), BalanceMax: 1}, code: "INVALID_BALANCE"},
		{name: "reversed range", input: ResetWorkspaceRobotsInput{RequestID: "reset-request-007", Mode: "random", BalanceMin: 2, BalanceMax: 1}, code: "INVALID_BALANCE_RANGE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeRobotResetInput(test.input)
			if err == nil || apperrors.GetErrorCode(err) != test.code {
				t.Fatalf("error = %v (%s), want code %s", err, apperrors.GetErrorCode(err), test.code)
			}
		})
	}
}

func TestRobotResetReferenceSeparatesPayloadReuse(t *testing.T) {
	first, err := normalizeRobotResetInput(ResetWorkspaceRobotsInput{
		WorkspaceID: 3, RequestID: "reset-request-008", Mode: "custom", NicknamePrefix: "甲", Balance: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := normalizeRobotResetInput(ResetWorkspaceRobotsInput{
		WorkspaceID: 999, RequestID: "reset-request-008", Mode: "custom", NicknamePrefix: "乙", Balance: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstPrefix, firstReference := robotResetReferences(3, first)
	secondPrefix, secondReference := robotResetReferences(3, second)
	if firstPrefix != secondPrefix {
		t.Fatalf("same request id must share conflict prefix: %q != %q", firstPrefix, secondPrefix)
	}
	if firstReference == secondReference {
		t.Fatalf("different payloads must not share reference: %q", firstReference)
	}
	otherPrefix, _ := robotResetReferences(4, first)
	if otherPrefix == firstPrefix {
		t.Fatalf("workspace must be part of idempotency scope: %q", firstPrefix)
	}
}

func TestRobotResetPayloadWorkspaceIsNotPartOfRoomScopedParameters(t *testing.T) {
	first, err := normalizeRobotResetInput(ResetWorkspaceRobotsInput{
		WorkspaceID: 3, RequestID: "reset-request-009", Mode: "custom", NicknamePrefix: "房间机器人", Balance: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := normalizeRobotResetInput(ResetWorkspaceRobotsInput{
		WorkspaceID: 999, RequestID: "reset-request-009", Mode: "custom", NicknamePrefix: "房间机器人", Balance: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("payload workspace selector leaked into reset parameters: %#v != %#v", first, second)
	}
	firstPrefix, _ := robotResetReferences(3, first)
	secondPrefix, _ := robotResetReferences(999, second)
	if firstPrefix == secondPrefix {
		t.Fatal("authenticated workspace argument must remain part of the server-side idempotency scope")
	}
}
