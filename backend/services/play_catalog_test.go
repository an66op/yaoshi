package services

import "testing"

func TestInferPlay(t *testing.T) {
	code, name := InferPlay("", "", 1, "7")
	if code != "ball_1_5" || name != "1-5球号" {
		t.Fatalf("digit infer got %s/%s", code, name)
	}
	code, name = InferPlay("", "", 1, "大")
	if code != "two_sided" {
		t.Fatalf("side infer got %s", code)
	}
	code, _ = InferPlay("", "", 6, "小")
	if code != "sum" {
		t.Fatalf("sum infer got %s", code)
	}
	code, _ = InferPlay("", "", 1, "龙")
	if code != "dragon_tiger" {
		t.Fatalf("dragon infer got %s", code)
	}
}
