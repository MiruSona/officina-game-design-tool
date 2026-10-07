// Package conflict 는 서로 다른 기획 문서에서 어긋날 수 있는 문장 짝을 추리고, 바깥 판정기에 묻는다.
// 판정 글은 모른다 — 통째로 판정기 몫이다. 설계는 Docs/Design/2026-10-08-stage-conflict설계.md.
package conflict

import (
	"regexp"
	"strings"
	"unicode"
)

// Bag 은 문장 하나의 낱말 토막이다. Words 는 내용어(중복 없음), Nums 는 숫자 토막, Neg 는 부정어가 있는가다.
type Bag struct {
	Words []string
	Nums  []string
	Neg   bool
}

// particles 는 토막 끝에서 한 번만 떼는 조사다. 긴 것부터 맞춘다.
var particles = []string{
	"에서", "으로", "로는", "에는", "까지", "부터", "에게", "처럼", "보다", "이다",
	"은", "는", "이", "가", "을", "를", "의", "에", "로", "와", "과", "도", "만", "다",
}

// stopWords 는 내용어로 안 세는 멈춤말이다.
var stopWords = map[string]bool{
	"그": true, "이": true, "저": true, "것": true, "수": true, "등": true, "때": true,
	"및": true, "또는": true, "그리고": true, "하다": true, "한다": true, "있다": true,
	"없다": true, "된다": true, "같다": true, "경우": true, "대한": true, "위한": true, "통해": true,
}

var digitRe = regexp.MustCompile(`[0-9]`)

// startsWithDigit 은 토막이 숫자로 시작하는지 본다.
func startsWithDigit(tok string) bool {
	return tok != "" && tok[0] >= '0' && tok[0] <= '9'
}

// Tokens 는 문장을 낱말 토막으로 나눈다. 순수 함수다.
func Tokens(text string) Bag {
	bag := Bag{}
	seen := map[string]bool{}
	for _, raw := range strings.FieldsFunc(text, isSeparator) {
		tok := strings.ToLower(raw)
		if startsWithDigit(tok) {
			// 「3초로」 처럼 숫자로 시작하는 토막은 조사만 떼고 숫자 쪽에 둔다. 내용어로 안 센다.
			bag.Nums = append(bag.Nums, stripParticle(tok))
			continue
		}
		if isNegation(tok) {
			bag.Neg = true
		}
		word := stripParticle(tok)
		if len([]rune(word)) < 2 || stopWords[word] || seen[word] {
			continue
		}
		seen[word] = true
		bag.Words = append(bag.Words, word)
	}
	return bag
}

// HasNumber 는 문장에 숫자가 하나라도 있는지 본다.
func HasNumber(text string) bool {
	return digitRe.MatchString(text)
}

// isSeparator 는 토막을 자르는 글자인지 본다.
func isSeparator(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	return strings.ContainsRune("·,()/:「」『』\"'“”‘’[]{}<>;!?…", r)
}

// isNegation 은 부정어 토막인지 본다. 「안」·「못」은 홀로 설 때만, 나머지는 앞머리로 본다.
func isNegation(tok string) bool {
	if tok == "안" || tok == "못" || tok == "never" || tok == "not" || tok == "no" {
		return true
	}
	for _, p := range []string{"않", "없", "아니", "금지"} {
		if strings.HasPrefix(tok, p) {
			return true
		}
	}
	return false
}

// stripParticle 은 끝 조사를 한 번만 뗀다. 뗀 뒤 두 글자 미만이면 되돌린다.
func stripParticle(tok string) string {
	for _, p := range particles {
		if !strings.HasSuffix(tok, p) || tok == p {
			continue
		}
		cut := strings.TrimSuffix(tok, p)
		// 「것은」→「것」 은 한 글자지만 멈춤말이라 되돌리지 않는다 — 되돌리면 내용어로 새어 들어간다.
		if len([]rune(cut)) < 2 && !stopWords[cut] {
			return tok
		}
		return cut
	}
	return tok
}
