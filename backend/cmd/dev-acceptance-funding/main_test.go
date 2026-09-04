package main

import (
	"io"
	"strings"
	"testing"
)

func TestParseCommandOptionsRequiresEveryExplicitFlag(t *testing.T) {
	valid := []string{
		"--reset-request-id", "dev-reset-20260905T010203Z-42-7",
		"--login-scope", "agent:2",
		"--username", "acceptance_member",
		"--amount-cents", "250000",
		"--confirm-dev-acceptance-funding",
	}
	options, err := parseCommandOptions(valid)
	if err != nil {
		t.Fatal(err)
	}
	if !options.Confirmed || options.Input.ResetRequestID != "dev-reset-20260905T010203Z-42-7" || options.Input.LoginScope != "agent:2" || options.Input.Username != "acceptance_member" || options.Input.AmountCents != 250000 {
		t.Fatalf("unexpected options: %+v", options)
	}

	for index := 0; index < len(valid); index++ {
		if strings.HasPrefix(valid[index], "--") {
			end := index + 1
			if index+1 < len(valid) && !strings.HasPrefix(valid[index+1], "--") {
				end++
			}
			args := append([]string{}, valid[:index]...)
			args = append(args, valid[end:]...)
			if _, err := parseCommandOptions(args); err == nil || !strings.Contains(err.Error(), "必须显式提供") {
				t.Fatalf("missing %s was accepted: %v", valid[index], err)
			}
		}
	}
}

func TestParseCommandOptionsRejectsFalseConfirmationAndExtraArguments(t *testing.T) {
	base := []string{
		"--reset-request-id=dev-reset-20260905T010203Z-42-7",
		"--login-scope=agent:2",
		"--username=acceptance_member",
		"--amount-cents=250000",
		"--confirm-dev-acceptance-funding=false",
	}
	if _, err := parseCommandOptions(base); err == nil || !strings.Contains(err.Error(), "明确启用") {
		t.Fatalf("false confirmation was accepted: %v", err)
	}
	withArgument := append(append([]string{}, base[:len(base)-1]...), "--confirm-dev-acceptance-funding", "unexpected")
	if _, err := parseCommandOptions(withArgument); err == nil || !strings.Contains(err.Error(), "位置参数") {
		t.Fatalf("positional argument was accepted: %v", err)
	}
}

func TestRunRejectsInvalidAmountBeforeLoadingConfigurationOrOpeningDatabase(t *testing.T) {
	args := []string{
		"--reset-request-id=dev-reset-20260905T010203Z-42-7",
		"--login-scope=agent:2",
		"--username=acceptance_member",
		"--amount-cents=10000000001",
		"--confirm-dev-acceptance-funding",
	}
	if err := run(args, io.Discard); err == nil || !strings.Contains(err.Error(), "amount-cents") {
		t.Fatalf("unsafe amount reached configuration/database setup: %v", err)
	}
}
