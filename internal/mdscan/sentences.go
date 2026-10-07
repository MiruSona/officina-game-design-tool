package mdscan

import (
	"regexp"
	"strings"
	"unicode"
)

// MaxSentenceRunes 는 문장 하나의 글자 상한이다. 넘으면 앞만 남기고 「…」 를 붙인다.
// 판정기에 보내는 입력 길이를 여기서 묶는다.
const MaxSentenceRunes = 400

// Sentence 는 문서에서 뗀 주장 한 토막이다. Line 은 원문 줄 번호(1부터), Ctx 는 「어느 절 · 어느 줄 머리」다.
type Sentence struct {
	Line int
	Text string
	Ctx  string
}

var (
	linkRe    = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	brRe      = regexp.MustCompile(`(?i)<br\s*/?>`)
	tagRe     = regexp.MustCompile(`</?[A-Za-z][^>]*>`)
	edgeUnder = regexp.MustCompile(`(^|\s)_+|_+(\s|$)`)
	listRe    = regexp.MustCompile(`^(\d+[.)]|[-*+])\s+`)
	spaceRe   = regexp.MustCompile(`\s+`)
)

// Sentences 는 문서 한 장을 문장으로 나눈다. 제목·펜스·주석은 버리고, 표 칸은 칸 하나가 문장 하나다.
// 살균(꾸밈 떼기 · 제어 문자 제거 · 길이 상한)은 **이 함수 한 곳**에서만 한다.
func Sentences(src string) []Sentence {
	lines := splitLines(src)
	out := []Sentence{}
	section := ""
	fence := ""
	comment := false
	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		t := strings.TrimSpace(ln)
		// 펜스 안이 먼저다 — 코드 안의 `<!--` 를 주석 시작으로 보면 문서 나머지가 통째로 빠진다.
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
			continue
		}
		if comment {
			if strings.Contains(t, "-->") {
				comment = false
			}
			continue
		}
		if strings.HasPrefix(t, "<!--") {
			comment = !strings.Contains(t, "-->")
			continue
		}
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = t[:3]
			continue
		}
		if t == "" {
			continue
		}
		if isAnyHeading(ln) {
			section = headingText(t)
			continue
		}
		if isTableLine(ln) {
			tbl, next := readTable(lines, i)
			if tbl != nil {
				// 머리줄 i · 구분줄 i+1 · 첫 본문 i+2 (0부터) → 사람 줄 번호는 i+3 이다.
				out = append(out, tableSentences(tbl, i+3, section)...)
				i = next - 1
				continue
			}
			// 구분줄 없는 `|` 줄은 표가 아니지만 칸은 칸이다 — 칸마다 따로 자른다.
			for _, c := range cells(ln) {
				out = append(out, split(clean(c), i+1, sectionCtx(section))...)
			}
			continue
		}
		out = append(out, bodySentences(ln, i+1, sectionCtx(section))...)
	}
	return out
}

// tableSentences 는 표 본문 줄의 칸을 문장으로 만든다. 첫 칸은 문장이 아니라 줄 머리(Ctx)다.
func tableSentences(t *Table, firstBodyLine int, section string) []Sentence {
	out := []Sentence{}
	for r, row := range t.Rows {
		line := firstBodyLine + r
		if len(row) == 1 {
			out = append(out, split(clean(row[0]), line, sectionCtx(section))...)
			continue
		}
		rowHead := clean(row[0])
		for c := 1; c < len(row); c++ {
			colHead := ""
			if c < len(t.Head) {
				colHead = clean(t.Head[c])
			}
			ctx := joinCtx(sectionCtx(section), "줄 머리 : "+rowHead, "칸 머리 : "+colHead)
			out = append(out, split(clean(row[c]), line, ctx)...)
		}
	}
	return out
}

// bodySentences 는 본문 한 줄을 문장으로 나눈다. 글머리·번호 표시는 뗀다.
func bodySentences(ln string, line int, ctx string) []Sentence {
	t := strings.TrimSpace(ln)
	t = strings.TrimLeft(t, "> ")
	t = listRe.ReplaceAllString(t, "")
	return split(clean(t), line, ctx)
}

// split 은 살균된 글을 문장 끝(`다.` `요.` `음.` `함.` `됨.` `?` `!` + 공백·줄끝)에서 자른다.
// 「1.5」 같은 소수점은 뒤가 공백이 아니라 안 잘린다.
func split(text string, line int, ctx string) []Sentence {
	out := []Sentence{}
	runes := []rune(text)
	start := 0
	for i := 0; i < len(runes); i++ {
		if !endsSentence(runes, i) {
			continue
		}
		out = appendSentence(out, string(runes[start:i+1]), line, ctx)
		start = i + 1
	}
	return appendSentence(out, string(runes[start:]), line, ctx)
}

// endsSentence 는 i 번 글자가 문장 끝인지 본다.
func endsSentence(runes []rune, i int) bool {
	atEnd := i+1 >= len(runes) || unicode.IsSpace(runes[i+1])
	if !atEnd {
		return false
	}
	switch runes[i] {
	case '?', '!':
		return true
	case '.':
		if i == 0 {
			return false
		}
		return strings.ContainsRune("다요음함됨", runes[i-1])
	}
	return false
}

// appendSentence 는 토막이 문장 꼴이면 더한다. 낱말 둘 미만은 짝이 안 되니 버린다.
func appendSentence(out []Sentence, raw string, line int, ctx string) []Sentence {
	text := strings.TrimSpace(raw)
	if wordCount(text) < 2 {
		return out
	}
	if r := []rune(text); len(r) > MaxSentenceRunes {
		text = string(r[:MaxSentenceRunes]) + "…"
	}
	return append(out, Sentence{Line: line, Text: text, Ctx: ctx})
}

// wordCount 는 두 글자 넘는 낱말 수를 어림한다. 내용어 판정의 정본은 conflict 쪽이다.
func wordCount(text string) int {
	n := 0
	for _, f := range strings.Fields(text) {
		if len([]rune(strings.Trim(f, ".,:;·()「」\"'`"))) >= 2 {
			n++
		}
	}
	return n
}

// clean 은 꾸밈(굵게 · 코드 · 밑줄 · 링크 · HTML)과 제어 문자를 떼고 공백을 하나로 접는다.
func clean(s string) string {
	t := brRe.ReplaceAllString(s, " ")
	t = linkRe.ReplaceAllString(t, "$1")
	t = tagRe.ReplaceAllString(t, " ")
	t = strings.NewReplacer("**", "", "~~", "", "`", "", "*", "").Replace(t)
	t = edgeUnder.ReplaceAllString(t, "$1$2")
	t = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, t)
	return strings.TrimSpace(spaceRe.ReplaceAllString(t, " "))
}

// headingText 는 제목 줄에서 `#` 과 꾸밈을 뗀 글이다.
func headingText(t string) string {
	return clean(strings.TrimSpace(strings.TrimLeft(t, "#")))
}

// sectionCtx 는 절 제목을 Ctx 꼴로 만든다. 제목이 없으면 빈 글이다.
func sectionCtx(section string) string {
	if section == "" {
		return ""
	}
	return "절 : " + section
}

// joinCtx 는 Ctx 조각을 「 · 」 로 잇는다. 빈 조각과 값 없는 조각은 뺀다.
func joinCtx(parts ...string) string {
	kept := []string{}
	for _, p := range parts {
		if p == "" || strings.HasSuffix(p, ": ") {
			continue
		}
		kept = append(kept, p)
	}
	return strings.Join(kept, " · ")
}
