package conflict

import (
	"fmt"
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
	ADoc    string
	BDoc    string
	A       mdscan.Sentence
	B       mdscan.Sentence
	Overlap int
	Strong  bool // 숫자·부정어 둘 다 있다
}

// ID 는 `A.md:줄|B.md:줄` 꼴이다. 판정기 출력에서 짝을 되찾는 열쇠다.
func (p Pair) ID() string {
	return fmt.Sprintf("%s:%d|%s:%d", p.ADoc, p.A.Line, p.BDoc, p.B.Line)
}

// Evidence 는 판정기에 보낼 근거 글이다. 문맥(Ctx)을 앞에 붙인다 — 표 칸만 떼면 주어가 없다.
func (p Pair) Evidence() string {
	if p.A.Ctx == "" {
		return p.A.Text
	}
	return "[" + p.A.Ctx + "] " + p.A.Text
}

// Claim 은 판정기에 보낼 주장 글이다. 문맥은 id 에만 남긴다.
func (p Pair) Claim() string {
	return p.B.Text
}

// Options 는 짝 추리기 선택지다. Changed 가 비어 있지 않으면 그 문서가 낀 짝만 본다.
// Skip 은 정렬·허브 뒤, 상한 앞에서 짝을 빼는 규칙이다 (비밀 꼴). 빠진 자리는 뒤 짝이 채운다.
type Options struct {
	Limit   int
	Changed map[string]bool
	Skip    func(Pair) bool
}

// Result 는 짝 추리기 결과다. Total 은 Skip 으로 뺀 뒤 · 상한으로 자르기 전 개수다.
type Result struct {
	Pairs     []Pair
	Dropped   []Pair // Skip 에 걸려 뺀 짝
	Total     int
	Sentences int
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
	bags := makeBags(ordered)
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
	res := Result{Sentences: len(bags), Dropped: []Pair{}}
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

// makeBags 는 모든 문장에 토막을 붙인다. 내용어가 둘 미만인 문장은 여기서 버린다.
func makeBags(docs []Doc) []bag {
	out := []bag{}
	for d, doc := range docs {
		for _, s := range doc.Sentences {
			b := Tokens(s.Text)
			if len(b.Words) < minOverlap {
				continue
			}
			out = append(out, bag{doc: d, sent: s, bag: b, flat: flatten(s.Text), num: HasNumber(s.Text)})
		}
	}
	return out
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

// sortPairs 는 겹침 많은 순 → 숫자·부정어 둘 다 → 문서·줄 순으로 늘어놓는다.
func sortPairs(pairs []Pair) {
	sort.SliceStable(pairs, func(i, j int) bool {
		p, q := pairs[i], pairs[j]
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

func sideKey(doc string, line int) string {
	return fmt.Sprintf("%s:%d", doc, line)
}

// flatten 은 「같은 글인가」를 볼 때 공백을 접은 꼴이다.
func flatten(s string) string {
	return strings.Join(strings.Fields(s), "")
}
