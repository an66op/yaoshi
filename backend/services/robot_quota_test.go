package services

import "testing"

func TestValidateWorkspaceRobotQuota(t *testing.T) {
	for _, quota := range []int{0, 1, MaxWorkspaceRobotQuota} {
		if err := validateWorkspaceRobotQuota(quota); err != nil {
			t.Fatalf("quota %d should be valid: %v", quota, err)
		}
	}
	for _, quota := range []int{-1, MaxWorkspaceRobotQuota + 1} {
		if err := validateWorkspaceRobotQuota(quota); err == nil {
			t.Fatalf("quota %d should be rejected", quota)
		}
	}
}
