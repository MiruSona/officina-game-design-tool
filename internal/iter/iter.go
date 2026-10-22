// Package iter 는 마지막 단계 뒤 반복 기록 파일(md)을 다룬다.
// 틀 채우기 · 절 뽑기 · 「내용이 있나」 가르기 · 남은 자리표 세기 · 지난 「미룬 것」 베끼기.
package iter

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// done 이 보는 절 이름. 게임 틀도 이 이름을 지켜야 검사가 된다.
const (
	SecFixed    = "고친 것"
	SecDeferred = "미룬 것"
	SecHanded   = "넘긴 것"
	// SecCarried 는 지난 반복에서 옮겨 온 줄이 들어가는 절이다.
	SecCarried = "지난 반복에서 미룬 것"
	// SecFeedback 은 틀에 {{지난미룬것}} 이 없을 때 그 위에 절을 끼우는 기준 절이다.
	SecFeedback = "피드백"
)

// CarryKey 는 지난 「미룬 것」 이 들어갈 자리말이다.
const CarryKey = "{{지난미룬것}}"

var (
	fileNo   = regexp.MustCompile(`^(\d+)-`)
	numbered = regexp.MustCompile(`^\d+\.\s`)
	noneLine = regexp.MustCompile(`^[-*]\s+없음\s*$`)
)

// FileNo 는 기록 파일 이름 앞 번호를 읽는다 (`10-2026-10-22.md` → 10).
func FileNo(name string) (int, bool) {
	m := fileNo.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// Fill 은 틀의 자리말을 글자 그대로 바꾼다. text/template 을 안 쓰는 것은 사람이 쓴 틀의 `{` 가 안 깨지게 하려는 것이다.
// 틀에 {{지난미룬것}} 이 없으면 그 절 제목 아래에 넣고, 제목도 없으면 `## 피드백` 바로 위에 절을 끼우고, 그것도 없으면 끝에 붙인다.
// 빈 값인 자리말은 앞 ` — ` 째 지우고, 지운 뒤 비거나 `>` 만 남은 줄은 뺀다.
func Fill(tpl string, vals map[string]string) string {
	src := normalize(tpl)
	if !strings.Contains(src, CarryKey) {
		src = insertCarry(src)
	}
	pairs := []string{}
	for k, v := range vals {
		if v == "" {
			src = dropEmpty(src, k)
			continue
		}
		pairs = append(pairs, k, v)
	}
	return strings.NewReplacer(pairs...).Replace(src)
}

// dropEmpty 는 빈 값 자리말을 지운다. 자리말만 있던 줄(인용 줄 포함)은 줄째 뺀다.
func dropEmpty(src, key string) string {
	out := []string{}
	dropped := false
	for _, ln := range strings.Split(src, "\n") {
		// 뺀 줄 뒤 빈 줄은 앞도 빈 줄이면 같이 빼 빈 줄이 겹치지 않게 한다.
		if dropped && strings.TrimSpace(ln) == "" && len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
			dropped = false
			continue
		}
		dropped = false
		if !strings.Contains(ln, key) {
			out = append(out, ln)
			continue
		}
		ln = strings.ReplaceAll(strings.ReplaceAll(ln, " — "+key, ""), key, "")
		if t := strings.TrimSpace(ln); t == "" || t == ">" {
			dropped = true
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// insertCarry 는 「지난 반복에서 미룬 것」 자리말을 넣는다.
func insertCarry(src string) string {
	block := "## " + SecCarried + "\n\n" + CarryKey + "\n\n"
	lines := strings.Split(src, "\n")
	// 제목은 있는데 자리말만 없으면 그 제목 바로 아래에 넣는다 (절을 또 끼우지 않는다).
	for i, ln := range lines {
		if level, name := heading(ln); level > 0 && name == SecCarried {
			out := append([]string{}, lines[:i+1]...)
			out = append(out, "", CarryKey)
			return strings.Join(append(out, lines[i+1:]...), "\n")
		}
	}
	for i, ln := range lines {
		if level, name := heading(ln); level == 2 && name == SecFeedback {
			head := strings.Join(lines[:i], "\n")
			if head != "" {
				head += "\n"
			}
			return head + block + strings.Join(lines[i:], "\n")
		}
	}
	if !strings.HasSuffix(src, "\n") {
		src += "\n"
	}
	return src + "\n" + strings.TrimSuffix(block, "\n")
}

// Section 은 그 이름의 절 안 줄들을 돌려준다. 제목 줄은 빼고, 같은 깊이나 더 얕은 다음 제목 앞까지다. 없으면 nil.
func Section(src, name string) []string {
	lines := strings.Split(normalize(src), "\n")
	for i, ln := range lines {
		level, got := heading(ln)
		if level == 0 || got != name {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if l, _ := heading(lines[j]); l > 0 && l <= level {
				return lines[i+1 : j]
			}
		}
		return lines[i+1:]
	}
	return nil
}

// HasContent 는 절에 사람이 쓴 줄이 하나라도 있는지 본다.
// 빈 줄 · 제목 · 표 머리 · 표 가름 · 칸이 다 빈 표 줄 · 틀에 똑같이 있는 줄은 내용이 아니다. 「- 없음」 은 내용이다.
func HasContent(lines []string, tpl string) bool {
	known := lineSet(tpl)
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if noneLine.MatchString(t) {
			return true
		}
		if t == "" || isHeading(t) || known[lineKey(t)] || isTableFrame(lines, i) {
			continue
		}
		return true
	}
	return false
}

// Leftovers 는 틀의 목록 줄·표 데이터 줄이 문서에 그대로 남은 것을 모은다. skip 에 적힌 절 안은 안 센다.
func Leftovers(doc, tpl string, skip ...string) []string {
	tplLines := strings.Split(normalize(tpl), "\n")
	marks := map[string]bool{}
	for i, ln := range tplLines {
		t := strings.TrimSpace(ln)
		if t == "" || noneLine.MatchString(t) {
			continue
		}
		if isList(t) || (isTableRow(t) && !isTableFrame(tplLines, i)) {
			marks[lineKey(t)] = true
		}
	}
	skipSet := map[string]bool{}
	for _, s := range skip {
		skipSet[s] = true
	}
	out := []string{}
	cur := ""
	for _, ln := range strings.Split(normalize(doc), "\n") {
		// `###` 아래도 그 위 `##` 절로 본다.
		if level, name := heading(ln); level > 0 {
			if level <= 2 {
				cur = name
			}
			continue
		}
		t := strings.TrimSpace(ln)
		if !skipSet[cur] && marks[lineKey(t)] {
			out = append(out, t)
		}
	}
	return out
}

// Carry 는 지난 기록 「미룬 것」 절의 목록 줄과 그 아래 들여 쓴 줄을 그대로 베낀다.
// 틀에 똑같이 있는 줄(자리표)과 「- 없음」 은 빼고, 뺀 줄 아래 들여 쓴 줄도 같이 뺀다.
// 둘째 값은 베끼지 못한 목록 아닌 내용 줄(글줄 · 표 데이터 줄) 수다.
func Carry(prev, tpl string) ([]string, int) {
	known := lineSet(tpl)
	out := []string{}
	other := 0
	keep := false
	sec := Section(prev, SecDeferred)
	for i, ln := range sec {
		t := strings.TrimSpace(ln)
		if t == "" {
			continue
		}
		if ln[0] == ' ' || ln[0] == '\t' {
			if keep {
				out = append(out, ln)
			}
			continue
		}
		keep = isList(t) && !known[lineKey(t)] && !noneLine.MatchString(t)
		if keep {
			out = append(out, ln)
			continue
		}
		if !isList(t) && !isHeading(t) && !known[lineKey(t)] && !isTableFrame(sec, i) {
			other++
		}
	}
	return out, other
}

// CarryBlock 은 {{지난미룬것}} 에 넣을 글이다. 옮길 것도 못 옮긴 것도 없으면 「- 없음」 한 줄이다.
func CarryBlock(lines []string, other int, source string) string {
	if len(lines) == 0 && other == 0 {
		return "- 없음"
	}
	head := "원문 : " + source
	if other > 0 {
		head += fmt.Sprintf(" (목록 아닌 줄 %d개는 원문 참고)", other)
	}
	if len(lines) == 0 {
		return head
	}
	return head + "\n\n" + strings.Join(lines, "\n")
}

// heading 은 제목 줄이면 깊이와 이름을 돌려준다. 아니면 0.
func heading(ln string) (int, string) {
	t := strings.TrimSpace(ln)
	if !isHeading(t) {
		return 0, ""
	}
	level := len(t) - len(strings.TrimLeft(t, "#"))
	return level, strings.TrimSpace(t[level:])
}

func isHeading(t string) bool {
	rest := strings.TrimLeft(t, "#")
	return len(rest) < len(t) && (rest == "" || rest[0] == ' ')
}

func isList(t string) bool {
	return strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || numbered.MatchString(t)
}

func isTableRow(t string) bool {
	return strings.HasPrefix(t, "|")
}

// isSep 는 `| --- |` 같은 표 가름 줄인지 본다.
func isSep(t string) bool {
	if !isTableRow(t) || !strings.Contains(t, "-") {
		return false
	}
	return strings.Trim(t, "|-: ") == ""
}

// isTableFrame 은 표의 머리 · 가름 · 칸이 다 빈 줄인지 본다. 데이터가 든 표 줄은 아니다.
func isTableFrame(lines []string, i int) bool {
	t := strings.TrimSpace(lines[i])
	if !isTableRow(t) {
		return false
	}
	if isSep(t) || strings.Trim(t, "| \t") == "" {
		return true
	}
	return i+1 < len(lines) && isSep(strings.TrimSpace(lines[i+1]))
}

// lineSet 은 틀의 비지 않은 줄을 앞뒤 빈칸을 깎아 모은다.
func lineSet(tpl string) map[string]bool {
	set := map[string]bool{}
	for _, ln := range strings.Split(normalize(tpl), "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			set[lineKey(t)] = true
		}
	}
	return set
}

// lineKey 는 틀 줄과 견줄 열쇠다. 빈칸 수만 다른 줄을 같게 본다 (표 줄은 칸마다 깎는다).
func lineKey(t string) string {
	if isTableRow(t) {
		cells := strings.Split(t, "|")
		for i, c := range cells {
			cells[i] = strings.Join(strings.Fields(c), " ")
		}
		return strings.Join(cells, "|")
	}
	return strings.Join(strings.Fields(t), " ")
}

func normalize(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
