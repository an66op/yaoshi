package services

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestBingoSSC1UsesOnlyCrossValidatedOrderForFirstFiveDigits(t *testing.T) {
	if !bingoGameRequiresOrderedSource("bingo-ssc-1") {
		t.Fatal("bingo-ssc-1 must never derive its five digits from the sorted 168 set")
	}
	orderedNumbers := []int{10, 39, 59, 2, 47, 1, 3, 4, 5, 6, 7, 8, 9, 11, 12, 13, 14, 15, 16, 80}
	sorted168Numbers := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 39, 47, 59, 80}
	drawAt := time.Date(2026, 9, 2, 1, 5, 0, 0, time.UTC)
	authoritative := sourceDraw{
		Issue: "115049561", Numbers: sorted168Numbers, DrawAt: drawAt,
		NextIssue: "115049562", NextDrawAt: drawAt.Add(5 * time.Minute),
		BingoSourceTail: 80, HasBingoSourceTail: true,
	}

	// The 168 set alone is structurally valid, but it has no draw-order proof
	// and therefore cannot reach the SSC1 conversion or settlement pipeline.
	if draws, err := transform168BingoDraws("bingo-ssc-1", []sourceDraw{authoritative}, bingoSSCNumbers(0)); !errors.Is(err, err168BingoOrderMismatch) || draws != nil {
		t.Fatalf("unverified sorted 168 set reached SSC1: draws=%+v err=%v", draws, err)
	}

	verified, err := crossValidate168BingoOrder([]sourceDraw{authoritative}, []sourceDraw{{
		Issue: authoritative.Issue, Numbers: orderedNumbers, DrawAt: drawAt,
	}})
	if err != nil || len(verified) != 1 || !verified[0].BingoOrderVerified ||
		!reflect.DeepEqual(verified[0].Numbers, orderedNumbers) {
		t.Fatalf("matching dual sources did not preserve verified order: %+v err=%v", verified, err)
	}
	draws, err := transform168BingoDraws("bingo-ssc-1", verified, bingoSSCNumbers(0))
	wantDigits := []int{0, 9, 9, 2, 7}
	if err != nil || len(draws) != 1 || !reflect.DeepEqual(draws[0].Numbers, wantDigits) ||
		draws[0].SourceRevision != bingoOrderedSourceRevision || draws[0].ConversionRevision != bingoSSC1ConversionVersion {
		t.Fatalf("verified first-five residues: draws=%+v want=%v err=%v", draws, wantDigits, err)
	}
	if draws[0].Issue != authoritative.Issue || draws[0].NextIssue != authoritative.NextIssue ||
		draws[0].DrawAt != authoritative.DrawAt || draws[0].NextDrawAt != authoritative.NextDrawAt {
		t.Fatalf("conversion changed authoritative issue metadata: %+v", draws[0])
	}
}

func TestBingoSSC1RejectsIncompleteDualSourceEvidence(t *testing.T) {
	ordered := []int{10, 39, 59, 2, 47, 1, 3, 4, 5, 6, 7, 8, 9, 11, 12, 13, 14, 15, 16, 80}
	authoritative := sourceDraw{
		Issue: "115049561", Numbers: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 39, 47, 59, 80},
		BingoSourceTail: 80, HasBingoSourceTail: true,
	}
	for _, test := range []struct {
		name          string
		authoritative sourceDraw
		ordered       []sourceDraw
	}{
		{name: "missing same issue", authoritative: authoritative, ordered: []sourceDraw{{Issue: "115049560", Numbers: ordered}}},
		{name: "different set", authoritative: authoritative, ordered: []sourceDraw{{Issue: authoritative.Issue, Numbers: append(append([]int(nil), ordered[:19]...), 79)}}},
		{name: "missing 168 tail proof", authoritative: func() sourceDraw { row := authoritative; row.HasBingoSourceTail = false; return row }(), ordered: []sourceDraw{{Issue: authoritative.Issue, Numbers: ordered}}},
		{name: "tail mismatch", authoritative: func() sourceDraw { row := authoritative; row.BingoSourceTail = 16; return row }(), ordered: []sourceDraw{{Issue: authoritative.Issue, Numbers: ordered}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			verified, err := crossValidate168BingoOrder([]sourceDraw{test.authoritative}, test.ordered)
			if !errors.Is(err, err168BingoOrderMismatch) || verified != nil {
				t.Fatalf("incomplete evidence was accepted: %+v err=%v", verified, err)
			}
		})
	}
}
