package services

import "testing"

func TestOperatingProfitAccountingIdentity(t *testing.T) {
	tests := []struct {
		name                                     string
		turnover, payout, rebate, welfare, share int64
		wantGross, wantPlatform                  int64
	}{
		{name: "profitable room", turnover: 10_000, payout: 6_000, rebate: 200, welfare: 100, share: 900, wantGross: 4_000, wantPlatform: 2_800},
		{name: "player wins", turnover: 10_000, payout: 12_000, rebate: 100, welfare: 0, share: 0, wantGross: -2_000, wantPlatform: -2_100},
		{name: "no turnover but welfare cost", turnover: 0, payout: 0, rebate: 0, welfare: 500, share: 0, wantGross: 0, wantPlatform: -500},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gross, platform := operatingProfitCents(test.turnover, test.payout, test.rebate, test.welfare, test.share)
			if gross != test.wantGross || platform != test.wantPlatform {
				t.Fatalf("gross/platform = %d/%d, want %d/%d", gross, platform, test.wantGross, test.wantPlatform)
			}
			if platform != test.turnover-test.payout-test.rebate-test.welfare-test.share {
				t.Fatal("platform profit no longer matches the ledger conservation identity")
			}
		})
	}
}
