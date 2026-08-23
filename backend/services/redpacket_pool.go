package services

import (
	"backend/data/models/activity"
	apperrors "backend/errors"
	"encoding/json"
	"math/rand"
)

type redPacketConfig struct {
	Pool      float64 `json:"pool"`
	MinAmount float64 `json:"min_amount"`
	MaxAmount float64 `json:"max_amount"`
}

func parseRedPacketConfig(raw string) redPacketConfig {
	cfg := redPacketConfig{Pool: 88, MinAmount: 1, MaxAmount: 8.8}
	_ = json.Unmarshal([]byte(defaultJSON(raw, "{}")), &cfg)
	if cfg.Pool <= 0 {
		cfg.Pool = 88
	}
	if cfg.MinAmount <= 0 {
		cfg.MinAmount = 1
	}
	if cfg.MaxAmount <= 0 {
		cfg.MaxAmount = cfg.Pool / 10
		if cfg.MaxAmount < cfg.MinAmount {
			cfg.MaxAmount = cfg.MinAmount
		}
	}
	return cfg
}

func ensureActivityPool(row *activity.Activity) {
	if row.Type != "redpacket" {
		return
	}
	if row.PoolTotalCents > 0 && row.PoolRemainingCents > 0 {
		return
	}
	cfg := parseRedPacketConfig(row.ConfigJSON)
	total := int64(cfg.Pool * 100)
	if total <= 0 {
		total = 8800
	}
	row.PoolTotalCents = total
	if row.PoolRemainingCents <= 0 {
		row.PoolRemainingCents = total
	}
}

func drawRedPacketReward(remaining int64, cfg redPacketConfig) (int64, error) {
	if remaining <= 0 {
		return 0, errPoolEmpty
	}
	minCents := int64(cfg.MinAmount * 100)
	maxCents := int64(cfg.MaxAmount * 100)
	if minCents < 100 {
		minCents = 100
	}
	if maxCents < minCents {
		maxCents = minCents
	}
	if maxCents > remaining {
		maxCents = remaining
	}
	if minCents > maxCents {
		minCents = maxCents
	}
	if minCents == maxCents {
		return minCents, nil
	}
	span := int(maxCents - minCents + 1)
	reward := minCents + int64(rand.Intn(span))
	if reward > remaining {
		reward = remaining
	}
	return reward, nil
}

var errPoolEmpty = apperrors.NewBusinessError("RED_PACKET_EMPTY", "红包奖池已领完")
