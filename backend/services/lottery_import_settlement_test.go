package services

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestSettleImportedDrawCandidatesSkipsDisplayOnlyHistory(t *testing.T) {
	draws := make([]sourceDraw, 0, 500)
	for index := 0; index < 500; index++ {
		draws = append(draws, sourceDraw{Issue: fmt.Sprintf("history-%03d", index)})
	}
	called := make([]string, 0)
	settleImportedDrawCandidates(context.Background(), draws, nil, nil, func(issue string) {
		called = append(called, issue)
	})
	if len(called) != 0 {
		t.Fatalf("display-only history triggered per-period settlement: %v", called)
	}
}

func TestSettleImportedDrawCandidatesSettlesUnionOnceInDrawOrder(t *testing.T) {
	draws := []sourceDraw{
		{Issue: "history-a"},
		{Issue: "pending-b"},
		{Issue: "unfinished-c"},
		{Issue: "unfinished-c"},
		{Issue: "history-d"},
	}
	called := make([]string, 0)
	settleImportedDrawCandidates(
		context.Background(),
		draws,
		[]string{"pending-b", "candidate-without-draw"},
		[]string{"unfinished-c", "pending-b"},
		func(issue string) { called = append(called, issue) },
	)
	if want := []string{"pending-b", "unfinished-c"}; !reflect.DeepEqual(called, want) {
		t.Fatalf("settlement calls = %v, want %v", called, want)
	}
}

func TestSettleImportedDrawCandidatesStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	draws := []sourceDraw{{Issue: "first"}, {Issue: "second"}}
	called := make([]string, 0, 1)
	settleImportedDrawCandidates(ctx, draws, []string{"first", "second"}, nil, func(issue string) {
		called = append(called, issue)
		cancel()
	})
	if want := []string{"first"}; !reflect.DeepEqual(called, want) {
		t.Fatalf("settlement continued after cancellation: got %v want %v", called, want)
	}
}
