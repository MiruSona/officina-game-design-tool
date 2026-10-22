package iter

import (
	"strings"
	"testing"
)

const tpl = `# 반복 {{회}}

## 피드백

| # | 느낀 것 |
| --- | --- |
| 1 | (한 줄) |

## 고친 것

- (고친 것 한 줄)

## 미룬 것

- (미룬 것 한 줄)
`

func TestFileNo(t *testing.T) {
	if n, ok := FileNo("10-2026-10-22.md"); !ok || n != 10 {
		t.Fatalf("10 이어야 한다 : %d %v", n, ok)
	}
	if _, ok := FileNo("틀.md"); ok {
		t.Fatal("번호 없는 이름은 아니다")
	}
}

func TestFillInsertsCarryAboveFeedback(t *testing.T) {
	got := Fill(tpl, map[string]string{"{{회}}": "10", CarryKey: "- 옮긴 것"})
	if !strings.Contains(got, "# 반복 10") {
		t.Fatalf("자리말이 안 바뀌었다 : %s", got)
	}
	carry := strings.Index(got, "## "+SecCarried)
	feed := strings.Index(got, "## "+SecFeedback)
	if carry < 0 || carry > feed || !strings.Contains(got, "- 옮긴 것") {
		t.Fatalf("피드백 위에 절을 끼워야 한다 : %s", got)
	}
}

func TestFillAppendsCarryWithoutFeedback(t *testing.T) {
	got := Fill("# 반복 {{회}}\n\n## 고친 것\n", map[string]string{"{{회}}": "3", CarryKey: "- 없음"})
	if !strings.HasSuffix(strings.TrimSpace(got), "- 없음") || !strings.Contains(got, "## "+SecCarried) {
		t.Fatalf("끝에 붙여야 한다 : %q", got)
	}
}

func TestFillKeepsBraces(t *testing.T) {
	got := Fill("{x} {{회}} {{모름}}\n## 피드백\n", map[string]string{"{{회}}": "1"})
	if !strings.Contains(got, "{x} 1 {{모름}}") {
		t.Fatalf("모르는 자리말·중괄호는 그대로 둬야 한다 : %q", got)
	}
}

func TestHasContent(t *testing.T) {
	if HasContent(Section(tpl, SecFixed), tpl) {
		t.Fatal("틀 그대로면 내용이 없다")
	}
	doc := strings.Replace(tpl, "- (고친 것 한 줄)", "- 체력바 고침", 1)
	if !HasContent(Section(doc, SecFixed), tpl) {
		t.Fatal("고친 줄이 있으면 내용이 있다")
	}
	none := strings.Replace(tpl, "- (미룬 것 한 줄)", "- 없음", 1)
	if !HasContent(Section(none, SecDeferred), tpl) {
		t.Fatal("「- 없음」 은 내용이다")
	}
	if HasContent([]string{"", "| a | b |", "| --- | --- |", "|  |  |"}, "") {
		t.Fatal("표 머리·가름·빈 줄은 내용이 아니다")
	}
}

func TestLeftoversSkipsSections(t *testing.T) {
	got := Leftovers(tpl, tpl)
	if len(got) != 3 {
		t.Fatalf("틀 그대로면 자리표 3줄 : %v", got)
	}
	if got := Leftovers(tpl, tpl, SecFixed, SecDeferred); len(got) != 1 || got[0] != "| 1 | (한 줄) |" {
		t.Fatalf("뺀 절은 안 센다 : %v", got)
	}
}

func TestCarryCopiesListAndIndented(t *testing.T) {
	prev := "## 미룬 것\n\n- 하나\n  - 하나 밑\n* 둘\n1. 셋\n- (미룬 것 한 줄)\n  - 자리표 밑\n글줄\n\n## 넘긴 것\n- 넘김\n"
	got, other := Carry(prev, tpl)
	want := []string{"- 하나", "  - 하나 밑", "* 둘", "1. 셋"}
	if strings.Join(got, "|") != strings.Join(want, "|") || other != 1 {
		t.Fatalf("베낀 줄 = %q, 목록 아닌 줄 %d (1 이어야 한다)", got, other)
	}
	if CarryBlock(nil, 0, "x") != "- 없음" {
		t.Fatal("없으면 「- 없음」")
	}
	if got := CarryBlock(nil, 2, "x"); got != "원문 : x (목록 아닌 줄 2개는 원문 참고)" {
		t.Fatalf("목록 아닌 줄만 있으면 원문 줄로 : %q", got)
	}
}

func TestKeyIgnoresSpacing(t *testing.T) {
	doc := strings.Replace(tpl, "| 1 | (한 줄) |", "|  1  |   (한 줄)|", 1)
	doc = strings.Replace(doc, "- (고친 것 한 줄)", "-   (고친 것  한 줄)", 1)
	if got := Leftovers(doc, tpl); len(got) != 3 {
		t.Fatalf("빈칸만 다른 자리표도 세야 한다 : %v", got)
	}
	if HasContent(Section(doc, SecFixed), tpl) {
		t.Fatal("빈칸만 다른 자리표는 내용이 아니다")
	}
}

func TestLeftoversSubheadingStaysInSection(t *testing.T) {
	doc := strings.Replace(tpl, "- (미룬 것 한 줄)", "### 작은 제목\n- (미룬 것 한 줄)", 1)
	if got := Leftovers(doc, tpl, SecDeferred); len(got) != 2 {
		t.Fatalf("### 아래도 뺀 절이다 : %v", got)
	}
}

func TestFillDropsEmptyTitle(t *testing.T) {
	got := Fill("# 반복 {{회}} — {{한줄}}\n\n> {{한줄}}\n\n## 피드백\n", map[string]string{"{{회}}": "02", "{{한줄}}": "", CarryKey: "- 없음"})
	if strings.Contains(got, " — ") || strings.Contains(got, "> ") || !strings.HasPrefix(got, "# 반복 02\n") {
		t.Fatalf("빈 한 줄은 꼬리·인용 줄째 지운다 : %q", got)
	}
}

func TestFillUsesExistingCarryHeading(t *testing.T) {
	got := Fill("## 지난 반복에서 미룬 것\n\n## 피드백\n", map[string]string{CarryKey: "- 옮김"})
	if strings.Count(got, "## "+SecCarried) != 1 || !strings.Contains(got, "## "+SecCarried+"\n\n- 옮김") {
		t.Fatalf("있는 제목 아래에 넣어야 한다 : %q", got)
	}
}
