package conflict

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mirusona/officina-game-design-tool/internal/mdscan"
)

// 짝 추리기 눈금. 설계 3절 그대로다.
const (
	DefaultLimit = 200 // 한 번에 판정할 쌍 상한. 1.2초 × 200 = 4분
	minOverlap   = 2   // 내용어가 이만큼 겹쳐야 짝이다
	hubPairs     = 20  // 한 문장이 이만큼 넘게 짝을 맺으면 「허브」다
	hubKeep      = 5   // 허브 문장은 상위 이만큼만 남긴다
)

// Doc 은 문서 한 장이다. Name 은 `NN-이름.md` 파일 이름이다 (번호가 앞선 쪽이 evidence 가 된다).
type Doc struct {
	Name      string
	Sentences []mdscan.Sentence
}

// Pair 는 판정에 보낼 문장 짝이다. A 가 evidence(앞 문서), B 가 claim(뒤 문서)이다.
type Pair struct {
	ADoc     string
	BDoc     string
	A        mdscan.Sentence
	B        mdscan.Sentence
	Overlap  int
	Strong   bool // 숫자·부정어 둘 다 있다
	Deferred bool // 겹침이 최소(2)뿐이고 절 제목까지 다르다 — 상한에서 뒤로 미룬다 (손질 2차 ③)
}

// ID 는 `A.md:줄:칸|B.md:줄:칸` 꼴이다. 판정기 출력에서 짝을 되찾는 열쇠다.
// 칸(같은 줄 안 순번)이 없으면 표 칸이 여럿인 줄에서 id 가 겹쳐 뒤 칸이 앞 칸을 덮는다 (실측 2026-10-08 함정).
func (p Pair) ID() string {
	return SideID(p.ADoc, p.A) + "|" + SideID(p.BDoc, p.B)
}

// SideID 는 문장 하나의 자리 `문서:줄:칸` 이다.
func SideID(doc string, s mdscan.Sentence) string {
	return fmt.Sprintf("%s:%d:%d", doc, s.Line, s.Seq)
}

// Evidence 는 판정기에 보낼 근거 글이다. 문맥(Ctx)을 앞에 붙인다 — 표 칸만 떼면 주어가 없다.
func (p Pair) Evidence() string {
	return withCtx(p.A)
}

// Claim 은 판정기에 보낼 주장 글이다. 1차 실측에서 Ctx 없는 claim 이 Won't 표 칸을 주장처럼 보이게 했고
// 긴 claim 의 바뀐 토막이 「무관」으로 샜다 — evidence 와 같은 꼴로 Ctx 를 붙이고(손질 1차),
// 두 문장의 다른 값·부정어를 꼬리에 힌트로 단다(손질 2차 ② — 긴 글의 곁가지 하나가 묻히는 것을 막는다).
func (p Pair) Claim() string {
	if h := p.Hint(); h != "" {
		return withCtx(p.B) + " [" + h + "]"
	}
	return withCtx(p.B)
}

// Hint 는 두 문장 사이 「다른 토막」이다 : 한쪽에만 있는 숫자 토막(날짜·id 꼴은 뺌) · 한쪽에만 있는 내용어 · 한쪽에만 있는 부정어.
// 셋 다 없으면 빈 글이다. 꼴 : 「다른 토막 : 값 A 96분 ↔ B 192분 · 낱말 A 최소 ↔ B 최대 · 부정어 B 에만」
func (p Pair) Hint() string {
	h, _ := p.HintInfo()
	return h
}

// HintInfo 는 힌트 글과 「수치표 꼴이라 일부러 껐나」 를 같이 돌려준다. 끈 수는 last.json 에 남긴다.
func (p Pair) HintInfo() (string, bool) {
	a, b := Tokens(p.A.Text), Tokens(p.B.Text)
	if len(a.Nums)+len(b.Nums) > hintMaxNums {
		// 글이 이미 숫자뿐이면 힌트가 소음이다 — 판정기가 직접 본다 (손질 3차 ③).
		return "", true
	}
	parts := []string{}
	onlyA, onlyB := onlyIn(a.Nums, b.Nums), onlyIn(b.Nums, a.Nums)
	if len(onlyA)+len(onlyB) > 0 {
		parts = append(parts, "값 A "+orNone(onlyA)+" ↔ B "+orNone(onlyB))
	}
	wordA, wordB := wordDiff(a.Words, b.Words)
	if wordA != "" || wordB != "" {
		parts = append(parts, "낱말 A "+orDash(wordA)+" ↔ B "+orDash(wordB))
	}
	switch {
	case a.Neg && !b.Neg:
		parts = append(parts, "부정어 A 에만")
	case b.Neg && !a.Neg:
		parts = append(parts, "부정어 B 에만")
	}
	if len(parts) == 0 {
		return "", false
	}
	return "다른 토막 : " + strings.Join(parts, " · "), false
}

// 힌트 눈금 (손질 3차). 설계 14절.
const (
	hintNums    = 4 // 힌트에 싣는 숫자 토막 상한 (한쪽당). 수치표 한 줄에 숫자가 열 개씩 들 때 힌트가 글보다 길어지는 것을 막는다
	hintWords   = 4 // 힌트에 싣는 다른 낱말 상한 (한쪽당). 겹침 수가 이보다 적으면 겹침 수까지만
	hintMaxNums = 8 // 두 문장 숫자 토막 합이 이보다 많으면 수치표 꼴로 보고 힌트를 안 붙인다
)

// wordDiff 는 한쪽에만 있는 내용어를 「x·y」 꼴로 돌려준다 (등장 순 · 한쪽당 min(hintWords, 겹침 수)).
// 겹친 만큼만 다른 것을 말한다 — 겹침이 적은 짝은 힌트도 짧아 길이가 안 터진다. 잘린 쪽은 끝에 「…」.
func wordDiff(xs, ys []string) (string, string) {
	limit := countOverlap(xs, ys)
	if limit > hintWords {
		limit = hintWords
	}
	return joinCut(onlyWords(xs, ys), limit), joinCut(onlyWords(ys, xs), limit)
}

// onlyWords 는 xs 에만 있는 내용어다. Words 는 이미 중복 없음·등장 순이다.
// 끝 문장부호를 뗀 열쇠로 견준다 — 「둔다.」 와 「둔다」 는 같은 낱말이다. 떼고 나서 멈춤말이 된 것(「이다.」)은 뺀다.
func onlyWords(xs, ys []string) []string {
	set := map[string]bool{}
	for _, y := range ys {
		set[wordKey(y)] = true
	}
	out := []string{}
	seen := map[string]bool{}
	for _, x := range xs {
		k := wordKey(x)
		if set[k] || seen[k] || stopWords[k] || isParticle(k) || len([]rune(k)) < 2 {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

// wordKey 는 낱말 토막의 견줌 열쇠다 : 끝 문장부호를 떼고 조사를 한 번 더 뗀다 (「없음이다.」 → 「없음」).
// 다만 「다」 하나만 떨어지는 꼴(「늘린다」)은 조사가 아니라 서술어 꼬리라 그대로 둔다 — 「늘린」 은 읽히지 않는다.
func wordKey(tok string) string {
	k := strings.TrimRight(tok, ".,;:!?…")
	cut := stripParticle(k)
	if cut != k && strings.TrimSuffix(k, "다") == cut {
		return k
	}
	return cut
}

// isParticle 은 토막이 조사 그 자체(「이다」)인지 본다. 문장부호 때문에 Tokens 에서 안 떨어진 것이 여기로 온다.
func isParticle(tok string) bool {
	for _, p := range particles {
		if tok == p {
			return true
		}
	}
	return false
}

// joinCut 은 앞 limit 개를 「·」 로 잇고 잘렸으면 「…」 를 붙인다. limit 이 0 이면 빈 글이다.
func joinCut(xs []string, limit int) string {
	if limit <= 0 || len(xs) == 0 {
		return ""
	}
	if len(xs) <= limit {
		return strings.Join(xs, "·")
	}
	return strings.Join(xs[:limit], "·") + "…"
}

// onlyIn 은 xs 에만 있는 숫자 토막이다 (중복 없음 · 날짜·id 꼴 제외 · 상한 hintNums).
// 「3초다.」 와 「3초」 가 같은 값으로 보이게 끝 문장부호와 조사를 떼고 견준다.
// 상한에서 자르기 전에 단위 붙은 값을 앞으로 보낸다 — 참조 번호 `13·14·3·2` 가 `192분` 을 밀어낸 적이 있다 (실측 10절).
func onlyIn(xs, ys []string) []string {
	set := map[string]bool{}
	for _, y := range ys {
		set[numKey(y)] = true
	}
	out := []string{}
	seen := map[string]bool{}
	for _, x := range xs {
		k := numKey(x)
		if set[k] || seen[k] || isNoiseNum(k) {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	out = rankNums(out)
	if len(out) > hintNums {
		out = out[:hintNums]
	}
	return out
}

// rankNums 는 단위 붙은 값 → 비율·좌표 → `×` 배율 → 맨숫자 순으로 늘어놓는다. 같은 묶음 안은 등장 순.
func rankNums(xs []string) []string {
	out := append([]string{}, xs...)
	sort.SliceStable(out, func(i, j int) bool { return numRank(out[i]) < numRank(out[j]) })
	return out
}

var (
	bareNumRe = regexp.MustCompile(`^[0-9][0-9.,]*$`)
	ratioOnly = regexp.MustCompile(`^` + ratioRe.String() + `$`)
)

// numRank 는 숫자 토막의 묶음이다 : 0 단위 붙은 값(`192분`) · 1 비율·좌표(`3/4`) · 2 배율(`×3`) · 3 맨숫자(`13`).
func numRank(tok string) int {
	switch {
	case bareNumRe.MatchString(tok):
		return 3
	case startsWithScale(tok):
		return 2
	case ratioOnly.MatchString(tok):
		return 1
	}
	return 0
}

// numKey 는 숫자 토막의 견줌 열쇠다 : 끝 문장부호를 떼고 조사를 한 번 더 뗀다.
func numKey(tok string) string {
	return stripParticle(strings.TrimRight(tok, ".,;:!?"))
}

// isNoiseNum 은 값이 아니라 표식인 숫자 토막이다 : 날짜(2026-09-08 · 09-08) · mem id(20260908-abcd1234) ·
// 문서 이름(02-기능목록.md) · 절 번호(3절 · 4-1절). 이런 것은 두 문서가 서로 가리키는 자리라 「다른 값」이 아니다.
func isNoiseNum(tok string) bool {
	return noiseNumRe.MatchString(tok)
}

var noiseNumRe = regexp.MustCompile(`^(?:\d{4}-\d{2}-\d{2}|\d{2}-\d{2}|\d{8}-[0-9a-f]{8})|\.md$|절$`)

func orNone(xs []string) string {
	if len(xs) == 0 {
		return "없음"
	}
	return strings.Join(xs, "·")
}

// orDash 는 빈 낱말 쪽을 「—」 로 적는다. 「없음」 은 내용어로도 나와 헷갈린다.
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// withCtx 는 「[Ctx] 문장」 꼴이다. Ctx 가 없으면 문장만이다.
func withCtx(s mdscan.Sentence) string {
	if s.Ctx == "" {
		return s.Text
	}
	return "[" + s.Ctx + "] " + s.Text
}

// Options 는 짝 추리기 선택지다. Changed 가 비어 있지 않으면 그 문서가 낀 짝만 본다.
// Skip 은 정렬·허브 뒤, 상한 앞에서 짝을 빼는 규칙이다 (비밀 꼴). 빠진 자리는 뒤 짝이 채운다.
type Options struct {
	Limit   int
	Changed map[string]bool
	Skip    func(Pair) bool
}

// Result 는 짝 추리기 결과다. Total 은 Skip 으로 뺀 뒤 · 상한으로 자르기 전 개수다.
// Excluded 는 주장이 아닌 줄(메타 · 근거)이라 짝 추리기 전에 뺀 문장 수를 까닭별로 센 것이다.
type Result struct {
	Pairs     []Pair
	Dropped   []Pair // Skip 에 걸려 뺀 짝
	Total     int
	Sentences int
	Excluded  map[string]int
}

// bag 은 문장 하나에 토막을 붙여 둔 것이다.
type bag struct {
	doc  int
	sent mdscan.Sentence
	bag  Bag
	flat string
	num  bool
}

// Pairs 는 서로 다른 문서의 문장 중 어긋날 수 있는 짝을 추린다. 순수 함수다.
func Pairs(docs []Doc, opt Options) Result {
	if opt.Limit <= 0 {
		opt.Limit = DefaultLimit
	}
	ordered := append([]Doc{}, docs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	bags, excluded := makeBags(ordered)
	pairs := []Pair{}
	for i := 0; i < len(bags); i++ {
		for j := i + 1; j < len(bags); j++ {
			if p, ok := makePair(ordered, bags[i], bags[j], opt.Changed); ok {
				pairs = append(pairs, p)
			}
		}
	}
	sortPairs(pairs)
	pairs = trimHubs(pairs)
	res := Result{Sentences: len(bags), Dropped: []Pair{}, Excluded: excluded}
	if opt.Skip != nil {
		pairs, res.Dropped = splitSkipped(pairs, opt.Skip)
	}
	res.Total = len(pairs)
	if len(pairs) > opt.Limit {
		pairs = pairs[:opt.Limit]
	}
	res.Pairs = pairs
	return res
}

// makeBags 는 모든 문장에 토막을 붙인다. 내용어가 둘 미만인 문장과 주장이 아닌 줄(Exclude)은 여기서 버린다.
// 두 번째 값은 뺀 문장 수를 까닭별로 센 것이다.
func makeBags(docs []Doc) ([]bag, map[string]int) {
	out := []bag{}
	excluded := map[string]int{}
	for d, doc := range docs {
		for _, s := range doc.Sentences {
			if why := Exclude(s); why != "" {
				excluded[why]++
				continue
			}
			b := Tokens(s.Text)
			if len(b.Words) < minOverlap {
				continue
			}
			out = append(out, bag{doc: d, sent: s, bag: b, flat: flatten(s.Text), num: HasNumber(s.Text)})
		}
	}
	return out, excluded
}

// makePair 는 두 문장이 짝 조건에 맞는지 본다. bags 는 문서 순이라 a 가 늘 앞 문서다.
func makePair(docs []Doc, a, b bag, changed map[string]bool) (Pair, bool) {
	if a.doc == b.doc || a.flat == b.flat {
		return Pair{}, false
	}
	if len(changed) > 0 && !changed[docs[a.doc].Name] && !changed[docs[b.doc].Name] {
		return Pair{}, false
	}
	hasNum := a.num || b.num
	hasNeg := a.bag.Neg || b.bag.Neg
	if !hasNum && !hasNeg {
		return Pair{}, false
	}
	overlap := countOverlap(a.bag.Words, b.bag.Words)
	if overlap < minOverlap {
		return Pair{}, false
	}
	return Pair{
		ADoc: docs[a.doc].Name, BDoc: docs[b.doc].Name,
		A: a.sent, B: b.sent,
		Overlap: overlap, Strong: hasNum && hasNeg,
		Deferred: overlap <= minOverlap && sectionTitle(a.sent.Ctx) != sectionTitle(b.sent.Ctx),
	}, true
}

// countOverlap 은 두 내용어 목록이 몇 개 겹치는지 센다.
func countOverlap(xs, ys []string) int {
	set := map[string]bool{}
	for _, x := range xs {
		set[x] = true
	}
	n := 0
	for _, y := range ys {
		if set[y] {
			n++
		}
	}
	return n
}

// sortPairs 는 미룬 짝을 맨 뒤로 → 겹침 많은 순 → 숫자·부정어 둘 다 → 문서·줄 순으로 늘어놓는다.
// 미룬 짝(겹침 2 + 절 제목 다름)은 1차 실측에서 후보의 71% 였고 경고는 거의 안 냈다 — 상한 200 을 겹침 3 이상에 먼저 쓴다.
func sortPairs(pairs []Pair) {
	sort.SliceStable(pairs, func(i, j int) bool {
		p, q := pairs[i], pairs[j]
		if p.Deferred != q.Deferred {
			return !p.Deferred
		}
		if p.Overlap != q.Overlap {
			return p.Overlap > q.Overlap
		}
		if p.Strong != q.Strong {
			return p.Strong
		}
		if p.ADoc != q.ADoc {
			return p.ADoc < q.ADoc
		}
		if p.A.Line != q.A.Line {
			return p.A.Line < q.A.Line
		}
		if p.BDoc != q.BDoc {
			return p.BDoc < q.BDoc
		}
		return p.B.Line < q.B.Line
	})
}

// trimHubs 는 짝을 너무 많이 맺는 문장을 상위 몇 개로 줄인다. 정렬된 목록을 받아 순서를 지킨다.
func trimHubs(pairs []Pair) []Pair {
	total := map[string]int{}
	for _, p := range pairs {
		total[sideKey(p.ADoc, p.A.Line)]++
		total[sideKey(p.BDoc, p.B.Line)]++
	}
	kept := map[string]int{}
	out := []Pair{}
	for _, p := range pairs {
		ka, kb := sideKey(p.ADoc, p.A.Line), sideKey(p.BDoc, p.B.Line)
		if isHubFull(total, kept, ka) || isHubFull(total, kept, kb) {
			continue
		}
		kept[ka]++
		kept[kb]++
		out = append(out, p)
	}
	return out
}

// splitSkipped 는 Skip 에 걸린 짝을 따로 모은다. 순서는 지킨다.
func splitSkipped(pairs []Pair, skip func(Pair) bool) ([]Pair, []Pair) {
	kept, dropped := []Pair{}, []Pair{}
	for _, p := range pairs {
		if skip(p) {
			dropped = append(dropped, p)
			continue
		}
		kept = append(kept, p)
	}
	return kept, dropped
}

// isHubFull 은 그 문장이 허브이고 이미 상한만큼 남겼는지 본다.
func isHubFull(total, kept map[string]int, key string) bool {
	return total[key] >= hubPairs && kept[key] >= hubKeep
}

// sideKey 는 허브 셈의 열쇠다. 일부러 칸이 아니라 줄 단위다 — 표 한 줄의 칸들이 같은 낱말을 나눠 가져
// 줄째로 허브가 되는 것을 막는다 (1차 실측 눈금을 그대로 두려는 뜻도 있다).
func sideKey(doc string, line int) string {
	return fmt.Sprintf("%s:%d", doc, line)
}

// flatten 은 「같은 글인가」를 볼 때 공백을 접은 꼴이다.
func flatten(s string) string {
	return strings.Join(strings.Fields(s), "")
}
