package mdscan

import (
	"strings"
	"testing"
)

const sentenceDoc = `# 문서 제목

## 규칙 절

| 항목 | 값 | 설명 |
| --- | --- | --- |
| 대기 시간 | 3초 | 대기 시간은 3초로 둔다.<br>늘리지 않는다. |
| O | 3 | 짧음 |

본문 첫 문장은 여기서 끝난다. 두 번째 문장은 **굵게** 쓴다! 비율은 1.5 배로 둔다.

- 글머리 줄은 표시를 뗀다.
1. 번호 줄도 [링크 글](http://example.invalid) 처럼 뗀다.

` + "```" + `
코드 안 문장은 안 센다.
` + "```" + `

<!-- 주석 문장도 안 센다. -->
`

func find(ss []Sentence, part string) (Sentence, bool) {
	for _, s := range ss {
		if strings.Contains(s.Text, part) {
			return s, true
		}
	}
	return Sentence{}, false
}

func TestSentencesTableCellsCarryCtx(t *testing.T) {
	ss := Sentences(sentenceDoc)
	s, ok := find(ss, "대기 시간은 3초로")
	if !ok {
		t.Fatalf("표 칸 문장을 못 찾았다 : %+v", ss)
	}
	if s.Line != 7 {
		t.Fatalf("줄 번호 = %d, 7 이어야 한다", s.Line)
	}
	if s.Ctx != "절 : 규칙 절 · 줄 머리 : 대기 시간 · 칸 머리 : 설명" {
		t.Fatalf("Ctx = %q", s.Ctx)
	}
	if _, ok := find(ss, "늘리지 않는다"); !ok {
		t.Fatal("<br> 뒤 문장이 따로 나와야 한다")
	}
	if _, ok := find(ss, "짧음"); ok {
		t.Fatal("낱말 둘 미만 칸은 버려야 한다")
	}
}

func TestSentencesDropsHeadingFenceComment(t *testing.T) {
	ss := Sentences(sentenceDoc)
	for _, bad := range []string{"문서 제목", "코드 안 문장", "주석 문장"} {
		if _, ok := find(ss, bad); ok {
			t.Fatalf("%q 는 문장이 아니어야 한다", bad)
		}
	}
}

func TestSentencesSplitsBodyAndKeepsDecimal(t *testing.T) {
	ss := Sentences(sentenceDoc)
	s, ok := find(ss, "두 번째 문장은 굵게 쓴다!")
	if !ok {
		t.Fatalf("굵게 표시를 떼고 `!` 에서 잘라야 한다 : %+v", ss)
	}
	if s.Line != 10 || s.Ctx != "절 : 규칙 절" {
		t.Fatalf("줄·Ctx = %d %q", s.Line, s.Ctx)
	}
	if _, ok := find(ss, "비율은 1.5 배로 둔다."); !ok {
		t.Fatal("1.5 의 소수점에서 자르면 안 된다")
	}
	if s, ok := find(ss, "번호 줄도 링크 글 처럼"); !ok || s.Line != 13 {
		t.Fatalf("번호 표시·링크를 떼야 한다 : %+v %v", s, ok)
	}
	if s, ok := find(ss, "글머리 줄은"); !ok || strings.HasPrefix(s.Text, "-") {
		t.Fatalf("글머리 표시를 떼야 한다 : %+v", s)
	}
}

func TestSentencesCapsLengthAndStripsControl(t *testing.T) {
	long := strings.Repeat("가나 ", 300) + "끝이다."
	ss := Sentences("본문 \x01줄 " + long + "\n")
	if len(ss) != 1 {
		t.Fatalf("문장 %d개, 1개여야 한다", len(ss))
	}
	if r := []rune(ss[0].Text); len(r) != MaxSentenceRunes+1 || r[len(r)-1] != '…' {
		t.Fatalf("400자에서 잘라야 한다 (지금 %d)", len(r))
	}
	if strings.ContainsRune(ss[0].Text, 0x01) {
		t.Fatal("제어 문자는 빠져야 한다")
	}
}

func TestSentencesFenceWithCommentMarkerDoesNotSwallowRest(t *testing.T) {
	src := "```html\n<!-- 코드 안 주석이다 -->\n```\n\n펜스 뒤 문장은 살아야 한다.\n"
	ss := Sentences(src)
	if len(ss) != 1 || !strings.Contains(ss[0].Text, "펜스 뒤 문장은") {
		t.Fatalf("펜스 안 <!-- 가 주석 시작으로 잡히면 안 된다 : %+v", ss)
	}
}

func TestSentencesPipeLineWithoutDividerSplitsCells(t *testing.T) {
	ss := Sentences("| 대기 시간은 3초로 둔다. | 창문은 아침에 두 번 열린다. |\n")
	if len(ss) != 2 || ss[0].Line != 1 || ss[1].Line != 1 {
		t.Fatalf("구분줄 없는 | 줄은 칸마다 문장이어야 한다 : %+v", ss)
	}
	if strings.Contains(ss[0].Text, "|") {
		t.Fatalf("파이프가 남으면 안 된다 : %q", ss[0].Text)
	}
}

func TestSentencesCRLF(t *testing.T) {
	ss := Sentences("# 제목\r\n\r\n첫째 문장이다.\r\n둘째 문장이다.\r\n")
	if len(ss) != 2 || ss[1].Line != 4 {
		t.Fatalf("CRLF 에서도 줄 번호가 맞아야 한다 : %+v", ss)
	}
}

func TestSentencesSeqAndHead(t *testing.T) {
	src := "# 제목\n\n날짜 2026-10-01 · 정한 사람 사용자 · 상태 임시\n\n## 규칙 절\n\n| 항목 | 값 | 설명 |\n| --- | --- | --- |\n| 대기 | 대기 시간은 3초다. | 창문은 두 번 열린다. |\n\n첫 문장은 여기서 끝난다. 둘째 문장도 같은 줄이다.\n"
	ss := Sentences(src)
	meta, ok := find(ss, "정한 사람")
	if !ok || !meta.Head || meta.Seq != 1 {
		t.Fatalf("첫 절 앞 문장은 Head 여야 한다 : %+v", meta)
	}
	a, _ := find(ss, "대기 시간은 3초다")
	b, _ := find(ss, "창문은 두 번")
	if a.Head || a.Line != b.Line || a.Seq != 1 || b.Seq != 2 {
		t.Fatalf("표 칸은 같은 줄에서 1·2 로 번호가 붙어야 하고 Head 가 아니어야 한다 : %+v %+v", a, b)
	}
	c, _ := find(ss, "첫 문장은")
	d, _ := find(ss, "둘째 문장도")
	if c.Seq != 1 || d.Seq != 2 || c.Line != d.Line {
		t.Fatalf("한 줄의 두 문장도 1·2 여야 한다 : %+v %+v", c, d)
	}
}
