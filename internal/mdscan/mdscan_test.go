package mdscan

import "testing"

const pillarDoc = `# 컨셉

## 기둥 3~4개

| 기둥 | 느낌 |
| --- | --- |
| 하나 | … |
| 둘 | … |
| 셋 | … |

## 안 할 것

| 무엇 | 왜 |
| --- | --- |
| 겨루기 | 이번엔 아니다 |
`

func TestPillarRowsCountsFirstTableInSection(t *testing.T) {
	n, ok := PillarRows(pillarDoc)
	if !ok {
		t.Fatal("기둥 표를 못 찾았다")
	}
	if n != 3 {
		t.Fatalf("기둥 = %d, 3 이어야 한다", n)
	}
}

func TestPillarRowsCases(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want int
		ok   bool
	}{
		{"절이 없다", "# 컨셉\n\n글만 있다\n", 0, false},
		{"절은 있는데 표가 없다", "## 기둥\n\n글로만 적었다\n", 0, false},
		{"다음 제목 뒤의 표는 안 센다", "## 기둥\n\n## 다른 절\n\n| a |\n| --- |\n| 1 |\n", 0, false},
		{"머리줄만 있는 표는 0줄", "## 기둥\n\n| 기둥 | 느낌 |\n| --- | --- |\n", 0, true},
		{"넷도 센다", "## 기둥\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n| 1 | 2 |\n| 1 | 2 |\n| 1 | 2 |\n", 4, true},
	}
	for _, c := range cases {
		n, ok := PillarRows(c.src)
		if ok != c.ok || n != c.want {
			t.Fatalf("%s : (%d, %v) 인데 (%d, %v) 여야 한다", c.name, n, ok, c.want, c.ok)
		}
	}
}

func TestWontRows(t *testing.T) {
	src := `# 기능 목록

| 칸 | 기능 |
| --- | --- |
| Must | 고르기 |
| **Won't** | 겨루기 |
| Won't | 소리 |
`
	if got := WontRows(src); got != 2 {
		t.Fatalf("Won't = %d, 2 여야 한다", got)
	}
	if got := WontRows("# 기능 목록\n\n| 칸 |\n| --- |\n| Must |\n"); got != 0 {
		t.Fatalf("Won't 없음 = %d, 0 이어야 한다", got)
	}
	if got := WontRows("표가 아예 없다"); got != 0 {
		t.Fatalf("표 없음 = %d, 0 이어야 한다", got)
	}
}

func TestLinesCountsBlankLines(t *testing.T) {
	if got := Lines("한 줄\n\n세 줄\n"); got != 3 {
		t.Fatalf("줄 수 = %d, 3 이어야 한다", got)
	}
	if got := Lines("끝에 줄바꿈이 없다"); got != 1 {
		t.Fatalf("줄 수 = %d, 1 이어야 한다", got)
	}
	if got := Lines(""); got != 0 {
		t.Fatalf("빈 글 = %d, 0 이어야 한다", got)
	}
}

func TestTablesSkipsBrokenTable(t *testing.T) {
	src := "| 구분줄이 없다 |\n| 그냥 줄 |\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n"
	tables := Tables(src)
	if len(tables) != 1 {
		t.Fatalf("표 = %d개, 1개여야 한다", len(tables))
	}
	if len(tables[0].Rows) != 1 {
		t.Fatalf("본문 줄 = %d, 1 이어야 한다", len(tables[0].Rows))
	}
}
