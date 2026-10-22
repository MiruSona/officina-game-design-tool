package conflict

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxPieces 는 근거 하나를 쪼갠 토막 상한이다. 10-22 실측 두 셋(80쌍)의 최대가 7토막이라 실측 판은 안 잘리고,
// 짝 하나가 최악 13물음(≈2.6초)으로 묶인다 (쪼개묻기 설계 결정 5).
const MaxPieces = 12

// pieceHead 는 근거 앞의 문맥 머리(`[절 : …]` · `[줄 머리 : …]` · `[칸 머리 : …]`)다. 첫 `]` 까지 뗀다.
var pieceHead = regexp.MustCompile(`^\[(?:절|줄 머리|칸 머리) :[^\]]*\]\s*`)

// sentenceEnd 는 문장 끝 `.` `!` `?` 뒤의 빈칸이다. 「다 사면」 처럼 「다」 뒤는 안 자른다.
var sentenceEnd = regexp.MustCompile(`[.!?][\s\p{Z}]+`) // 파이썬 \s 처럼 유니코드 빈칸도 본다

// pieceSeps 는 토막 이음 기호다. 앞뒤를 띄어 쓴 것만 — 붙여 쓴 `4·단축` 의 `·` 는 낱말 안이다.
var pieceSeps = []string{" · ", " — ", " ↔ "}

// datedParen 은 날짜 괄호 주석 후보다 : 안에 괄호가 없는 `(…)` · `（…）` 와 그 앞 빈칸.
var datedParen = regexp.MustCompile(`[\s\p{Z}]*[(（][^()（）]*[)）]`)

// dateLike 는 날짜 꼴 `YYYY-MM-DD` · `MM-DD`(두 자리) · `M월 D일` 이다. 앞뒤가 숫자면 날짜가 아니다 (`110-08`).
var dateLike = regexp.MustCompile(`(?:^|\D)(?:\d{4}-\d{2}-\d{2}|(?:0[1-9]|1[0-2])-(?:0[1-9]|[12]\d|3[01])|\d{1,2}월\s*\d{1,2}일)(?:\D|$)`)

// listMark 는 목록 항목 표시 `①`~`⑳` · 앞이 빈칸이거나 맨 앞인 `1)` 이다.
var listMark = regexp.MustCompile(`[①-⑳]|(?:^|[\s\p{Z}])\d{1,2}\)`)

// maxListMarks 를 넘는 항목 표시가 든 토막은 목록 꼴이라 안 묻는다 — 전체 판 실측에서 목록 토막이 b 를 띄웠다 (2번째 고리).
const maxListMarks = 2

// EvidencePieces 는 근거 글을 「괄호 지킨 토막」 으로 쪼갠다 (JudgeModel 10-22 입력 꼴 실측을 따른다).
// 머리를 떼고 → 괄호 밖 문장 끝에서 자르고 → 괄호 밖의 이음 기호에서 한 번 더 자르고 → 날짜 괄호를 지우고 →
// 목록 꼴 토막을 건너뛴다. 돌려주는 값 : 토막 · 상한으로 버린 수 · 목록 꼴로 건너뛴 수.
func EvidencePieces(evidence string) ([]string, int, int) {
	body := pieceHead.ReplaceAllString(evidence, "")
	out, skipped := []string{}, 0
	for _, s := range splitSentences(body) {
		for _, p := range splitOutsideParens(s) {
			p = stripDatedParens(p)
			switch {
			case p == "":
			case len(listMark.FindAllStringIndex(p, -1)) > maxListMarks:
				skipped++
			default:
				out = append(out, p)
			}
		}
	}
	if len(out) > MaxPieces {
		return out[:MaxPieces], len(out) - MaxPieces, skipped
	}
	return out, 0, skipped
}

// stripDatedParens 는 날짜 꼴이 든 괄호 덩어리를 지운다. 짧은 토막에선 「(10-08 결정)」 끼리의 날짜 차이가
// 어긋남으로 읽혀 b 가 튄다 (전체 판 실측 2번째 고리). 안쪽 괄호부터 한 겹만 본다.
func stripDatedParens(p string) string {
	return strings.TrimSpace(datedParen.ReplaceAllStringFunc(p, func(m string) string {
		if dateLike.MatchString(m) {
			return ""
		}
		return m
	}))
}

// splitSentences 는 괄호 밖의 문장 끝 기호 뒤 빈칸에서 자른다. 기호는 앞 토막에 남는다.
// 괄호 깊이는 splitOutsideParens 와 같은 규칙이다 — 「(10-08 결정. 다음 …)」 안에서 자르면 닫히지 않은 토막이 생긴다.
func splitSentences(s string) []string {
	out := []string{}
	start, depth, seen := 0, 0, 0
	for _, m := range sentenceEnd.FindAllStringIndex(s, -1) {
		depth += parenDelta(s[seen:m[0]])
		seen = m[0]
		if depth > 0 {
			continue
		}
		out = append(out, s[start:m[0]+1])
		start = m[1]
	}
	return append(out, s[start:])
}

// parenDelta 는 여는 괄호 수 − 닫는 괄호 수다.
func parenDelta(s string) int {
	d := 0
	for _, r := range s {
		switch r {
		case '(', '（', '「':
			d++
		case ')', '）', '」':
			d--
		}
	}
	return d
}

// splitOutsideParens 는 괄호 깊이가 0 이하일 때만 이음 기호에서 자른다. 닫는 괄호가 남아도(깊이 음수) 멈추지 않는다.
func splitOutsideParens(s string) []string {
	out := []string{}
	var buf strings.Builder
	depth := 0
	for i := 0; i < len(s); {
		if depth <= 0 {
			if sep := sepAt(s[i:]); sep != "" {
				out = appendPiece(out, buf.String())
				buf.Reset()
				i += len(sep)
				continue
			}
		}
		ch := s[i:]
		switch {
		case strings.HasPrefix(ch, "("), strings.HasPrefix(ch, "（"), strings.HasPrefix(ch, "「"):
			depth++
		case strings.HasPrefix(ch, ")"), strings.HasPrefix(ch, "）"), strings.HasPrefix(ch, "」"):
			depth--
		}
		_, n := utf8.DecodeRuneInString(ch)
		buf.WriteString(s[i : i+n])
		i += n
	}
	return appendPiece(out, buf.String())
}

func sepAt(s string) string {
	for _, sep := range pieceSeps {
		if strings.HasPrefix(s, sep) {
			return sep
		}
	}
	return ""
}

func appendPiece(out []string, p string) []string {
	if p = strings.TrimSpace(p); p != "" {
		return append(out, p)
	}
	return out
}
