package services

import (
	"backend/data/models/lottery"
	"strings"
	"testing"
)

func TestValidateOfficialDrawsAcceptsCompleteRacingPermutation(t *testing.T) {
	game := lottery.Game{Name: "极速赛车", Category: "赛车"}
	draws := []sourceDraw{{Issue: "34134504", Numbers: []int{4, 8, 1, 9, 2, 10, 7, 6, 5, 3}}}
	if err := validateOfficialDraws(game, draws); err != nil {
		t.Fatalf("valid racing result rejected: %v", err)
	}
}

func TestValidateOfficialDrawsRejectsMalformedRacingBatch(t *testing.T) {
	game := lottery.Game{Name: "极速飞艇", Category: "飞艇"}
	tests := []struct {
		name    string
		numbers []int
		want    string
	}{
		{name: "incomplete", numbers: []int{1, 2, 3, 4, 5, 6, 7, 8, 9}, want: "恰好 10 个号码"},
		{name: "duplicate", numbers: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 9}, want: "号码 9 重复"},
		{name: "out of range", numbers: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 11}, want: "号码 11 超出 1~10"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			draws := []sourceDraw{
				{Issue: "54773822", Numbers: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
				{Issue: "54773823", Numbers: test.numbers},
			}
			err := validateOfficialDraws(game, draws)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateOfficialDraws() error = %v, want message containing %q", err, test.want)
			}
		})
	}
}

func TestValidateOfficialDrawsAppliesToLucky10ButNotSSC(t *testing.T) {
	malformed := []sourceDraw{{Issue: "1", Numbers: []int{9, 9, 9, 9, 9}}}
	if err := validateOfficialDraws(lottery.Game{Name: "澳洲幸运10", Category: "幸运10"}, malformed); err == nil {
		t.Fatal("Lucky 10 malformed result was accepted")
	}
	if err := validateOfficialDraws(lottery.Game{Name: "极速时时彩", Category: "时时彩"}, malformed); err != nil {
		t.Fatalf("SSC result must not use racing validation: %v", err)
	}
}
