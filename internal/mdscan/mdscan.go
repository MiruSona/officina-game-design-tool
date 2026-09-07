// Package mdscan 은 마크다운에서 셀 수 있는 것만 센다. 세는 규약은 1판 설계 문서 4장에 있다.
package mdscan

import (
	"strings"
)

// Table 은 마크다운 표 하나다. Rows 는 본문 줄만 담는다 (머리줄과 구분줄은 뺀다).
type Table struct {
	Head []string
	Rows [][]string
}

// Lines 는 글의 줄 수다. 빈 줄도 센다.
func Lines(src string) int {
	if src == "" {
		return 0
	}
	n := strings.Count(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	if !strings.HasSuffix(src, "\n") {
		n++
	}
	return n
}

// Tables 는 글 안의 표를 차례대로 찾는다.
func Tables(src string) []Table {
	lines := splitLines(src)
	out := []Table{}
	for i := 0; i < len(lines); i++ {
		if !isTableLine(lines[i]) {
			continue
		}
		t, next := readTable(lines, i)
		if t != nil {
			out = append(out, *t)
		}
		i = next - 1
	}
	return out
}

// PillarRows 는 `## 기둥` 절 안 첫 표의 본문 줄 수를 센다. 표가 없으면 두 번째 값이 거짓이다.
func PillarRows(src string) (int, bool) {
	lines := splitLines(src)
	start := -1
	for i, ln := range lines {
		if isHeading(ln, "기둥") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return 0, false
	}
	for i := start; i < len(lines); i++ {
		if isAnyHeading(lines[i]) {
			return 0, false
		}
		if !isTableLine(lines[i]) {
			continue
		}
		t, _ := readTable(lines, i)
		if t == nil {
			continue
		}
		return len(t.Rows), true
	}
	return 0, false
}

// WontRows 는 첫 표에서 첫 칸이 Won't 인 줄 수를 센다.
func WontRows(src string) int {
	tables := Tables(src)
	if len(tables) == 0 {
		return 0
	}
	n := 0
	for _, row := range tables[0].Rows {
		if len(row) == 0 {
			continue
		}
		if strings.EqualFold(cellWord(row[0]), "won't") {
			n++
		}
	}
	return n
}

// splitLines 는 줄바꿈 종류를 맞춰 자른다.
func splitLines(src string) []string {
	return strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
}

// isTableLine 은 표 줄인지 본다. 앞 공백은 세 칸까지 봐 준다.
func isTableLine(line string) bool {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 {
		return false
	}
	return strings.HasPrefix(t, "|")
}

// isAnyHeading 은 `#` 으로 시작하는 제목 줄인지 본다.
func isAnyHeading(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "#") && strings.Contains(t, " ")
}

// isHeading 은 그 낱말로 시작하는 제목 줄인지 본다. 뒤에 글자가 더 붙어도 된다.
func isHeading(line, word string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "#") {
		return false
	}
	t = strings.TrimLeft(t, "#")
	t = strings.TrimSpace(t)
	t = strings.Trim(t, "*_")
	return strings.HasPrefix(t, word)
}

// readTable 은 i 번 줄부터 이어지는 표 하나를 읽는다. 두 번째 값은 표 다음 줄 번호다.
func readTable(lines []string, i int) (*Table, int) {
	rows := [][]string{}
	j := i
	for ; j < len(lines) && isTableLine(lines[j]); j++ {
		rows = append(rows, cells(lines[j]))
	}
	if len(rows) < 2 || !isDivider(rows[1]) {
		return nil, j
	}
	body := rows[2:]
	return &Table{Head: rows[0], Rows: body}, j
}

// isDivider 는 `|---|---|` 같은 구분줄인지 본다.
func isDivider(row []string) bool {
	if len(row) == 0 {
		return false
	}
	for _, c := range row {
		t := strings.TrimSpace(c)
		t = strings.Trim(t, ":")
		if t == "" || strings.Trim(t, "-") != "" {
			return false
		}
	}
	return true
}

// cells 는 표 줄을 칸으로 자른다. 앞뒤 파이프는 버린다.
func cells(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	parts := strings.Split(t, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// cellWord 는 칸에서 굵게 표시와 장식을 떼고 첫 낱말만 남긴다.
func cellWord(cell string) string {
	t := strings.TrimSpace(cell)
	t = strings.Trim(t, "*_`")
	t = strings.TrimSpace(t)
	if idx := strings.IndexAny(t, " \t("); idx > 0 {
		t = t[:idx]
	}
	return strings.TrimSpace(t)
}
