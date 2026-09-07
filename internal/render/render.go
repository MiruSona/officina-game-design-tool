// Package render 는 터미널에 글을 가지런히 찍는 것을 돕는다.
package render

import (
	"strings"
	"unicode"
)

// Width 는 글자가 터미널에서 차지하는 칸 수다. 한글·한자는 두 칸이다.
func Width(s string) int {
	n := 0
	for _, r := range s {
		n += runeWidth(r)
	}
	return n
}

// Pad 는 오른쪽을 채워 폭을 맞춘다.
func Pad(s string, width int) string {
	gap := width - Width(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// Badge 는 상태 딱지를 대괄호로 감싼다.
func Badge(status string) string {
	return "[" + status + "]"
}

// runeWidth 는 글자 하나의 칸 수다.
func runeWidth(r rune) int {
	if r == 0 {
		return 0
	}
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
		return 0
	}
	if wide(r) {
		return 2
	}
	return 1
}

// wide 는 두 칸을 차지하는 글자인지 본다. 우리가 쓰는 범위만 본다.
func wide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F:
		return true
	case r >= 0x2E80 && r <= 0xA4CF:
		return true
	case r >= 0xAC00 && r <= 0xD7A3:
		return true
	case r >= 0xF900 && r <= 0xFAFF:
		return true
	case r >= 0xFF00 && r <= 0xFF60:
		return true
	case r >= 0xFFE0 && r <= 0xFFE6:
		return true
	}
	return false
}
