package services

import (
	"math"
	"reflect"
	"sort"
	"testing"
)

func TestAssistantDocumentedReceiptExamples(t *testing.T) {
	for _, test := range []struct {
		content   string
		count     int
		total     float64
		positions []int
	}{
		{"489/0178/48", 12, 576, []int{4, 8, 9}},
		{"5/045/343", 3, 1029, []int{5}},
		{"68/单大/811", 4, 3244, []int{6, 8}},
		{"62437/546", 5, 2730, []int{1}},
		{"12345/100", 5, 500, []int{1}},
		{"买12345/1000", 5, 5000, []int{1}},
		{"冠军/12345/100", 5, 500, []int{1}},
		{"冠军12345/100", 5, 500, []int{1}},
		{"第七名/8/200", 1, 200, []int{7}},
		{"1/123/0.10", 3, 0.3, []int{1}},
		{"4444/88", 1, 352, []int{1}},
		{"1/12345/100#6/大/200#7/67890/100", 11, 1200, []int{1, 6, 7}},
		{"2/大小单双12/20#1/大小单双12/20", 12, 240, []int{1, 2}},
		{"1/3/50#2/0/50#3/0/50#4/9/50#5/0/50#6/9/50#7/4/50#8/7/50#9/6/50#0/6/50", 10, 500, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
	} {
		t.Run(test.content, func(t *testing.T) {
			lines, err := ParseAssistantBet(test.content)
			if err != nil || len(lines) != test.count {
				t.Fatalf("count=%d err=%v", len(lines), err)
			}
			var total int64
			seen := map[int]bool{}
			for _, line := range lines {
				total += int64(math.Round(line.Amount * 100))
				seen[line.Position] = true
			}
			positions := []int{}
			for position := range seen {
				positions = append(positions, position)
			}
			sort.Ints(positions)
			if total != int64(math.Round(test.total*100)) || !reflect.DeepEqual(positions, test.positions) {
				t.Fatalf("total=%d positions=%v", total, positions)
			}
		})
	}
}

func TestAssistantTenRankReceiptLabels(t *testing.T) {
	content := "1/1/20#2/4/20#3/3/20#4/7/20#5/9/20#6/5/20#7/5/20#8/5/20#9/2/20#0/8/20"
	want := []string{"冠军[1/20]", "亚军[4/20]", "第三名[3/20]", "第四名[7/20]", "第五名[9/20]", "第六名[5/20]", "第七名[5/20]", "第八名[5/20]", "第九名[2/20]", "第十名[8/20]"}
	lines, err := ParseAssistantBet(content)
	if err != nil || len(lines) != len(want) {
		t.Fatalf("ten ranks: %+v %v", lines, err)
	}
	for index, line := range lines {
		if line.Position != index+1 || line.Amount != 20 || line.Label != want[index] {
			t.Fatalf("rank %d: %+v", index+1, line)
		}
	}
	lines, err = ParseAssistantBet("1/1/80#1/2/80")
	if err != nil || len(lines) != 2 || lines[0].Label != "冠军[1/80]" || lines[1].Label != "冠军[2/80]" {
		t.Fatalf("explicit champion selections were reassigned: %+v %v", lines, err)
	}
}
