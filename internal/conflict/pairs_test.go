package conflict

import (
	"fmt"
	"testing"

	"github.com/mirusona/officina-game-design-tool/internal/mdscan"
)

func sent(line int, text string) mdscan.Sentence {
	return mdscan.Sentence{Line: line, Text: text}
}

func TestPairsNeedsOverlapAndNumberOrNegation(t *testing.T) {
	docs := []Doc{
		{Name: "00-바탕.md", Sentences: []mdscan.Sentence{
			sent(3, "대기 시간은 3초로 둔다."),
			sent(5, "색은 파랑으로 정한다."),
			sent(7, "창문은 아침에 두 번 열린다."),
		}},
		{Name: "02-자세.md", Sentences: []mdscan.Sentence{
			sent(10, "대기 시간은 5초로 늘린다."),      // 00:3 과 겹침 2 + 숫자
			sent(12, "색은 노랑으로 정한다."),         // 00:5 와 겹침 2 이지만 숫자·부정어 없음
			sent(14, "대기 시간은 3초로 둔다."),       // 00:3 과 같은 글 → 뺀다
			sent(16, "창문은 아침에 두 번 열리지 않는다."), // 00:7 과 겹침 2 + 부정어
		}},
	}
	res := Pairs(docs, Options{})
	if len(res.Pairs) != 2 || res.Total != 2 {
		t.Fatalf("짝 %d개(전체 %d), 2개여야 한다 : %+v", len(res.Pairs), res.Total, res.Pairs)
	}
	for _, p := range res.Pairs {
		if p.ADoc != "00-바탕.md" || p.BDoc != "02-자세.md" {
			t.Fatalf("evidence 는 앞 번호 문서여야 한다 : %s", p.ID())
		}
	}
	if res.Pairs[0].ID() != "00-바탕.md:3|02-자세.md:10" {
		t.Fatalf("id 꼴이 다르다 : %s", res.Pairs[0].ID())
	}
}

func TestPairsSkipsSameDocument(t *testing.T) {
	docs := []Doc{{Name: "00-바탕.md", Sentences: []mdscan.Sentence{
		sent(1, "대기 시간은 3초로 둔다."), sent(2, "대기 시간은 5초로 둔다."),
	}}}
	if res := Pairs(docs, Options{}); len(res.Pairs) != 0 {
		t.Fatalf("같은 문서 안은 안 봐야 한다 : %+v", res.Pairs)
	}
}

func TestPairsEvidenceCarriesCtxAndDocOrderIsByName(t *testing.T) {
	docs := []Doc{
		{Name: "03-뒤.md", Sentences: []mdscan.Sentence{sent(2, "대기 시간은 5초로 둔다.")}},
		{Name: "01-앞.md", Sentences: []mdscan.Sentence{{Line: 4, Text: "대기 시간은 3초로 둔다.", Ctx: "절 : 규칙 · 줄 머리 : 대기"}}},
	}
	res := Pairs(docs, Options{})
	if len(res.Pairs) != 1 {
		t.Fatalf("짝 %d개", len(res.Pairs))
	}
	p := res.Pairs[0]
	if p.ADoc != "01-앞.md" || p.Evidence() != "[절 : 규칙 · 줄 머리 : 대기] 대기 시간은 3초로 둔다." {
		t.Fatalf("evidence = %q (%s)", p.Evidence(), p.ADoc)
	}
	if p.Claim() != "대기 시간은 5초로 둔다." {
		t.Fatalf("claim = %q", p.Claim())
	}
}

func TestPairsSortsByOverlapThenStrongAndCutsAtLimit(t *testing.T) {
	docs := []Doc{
		{Name: "00-a.md", Sentences: []mdscan.Sentence{
			sent(1, "창문 색은 파랑 유리로 둔다."),
			sent(2, "문짝 수는 둘이다."),
		}},
		{Name: "01-b.md", Sentences: []mdscan.Sentence{
			sent(1, "문짝 수는 3개로 둔다."),        // 00:2 와 겹침 2 (문짝·수는→수 는 멈춤말… 문짝·둔다?)
			sent(2, "창문 색은 파랑 유리로 안 둔다."),   // 00:1 과 겹침 4 + 부정어 + 숫자 없음
			sent(3, "창문 색은 파랑 유리로 3번 바꾼다."), // 00:1 과 겹침 3 + 숫자
		}},
	}
	res := Pairs(docs, Options{Limit: 2})
	if res.Total < 2 || len(res.Pairs) != 2 {
		t.Fatalf("상한 2 로 잘라야 한다 : %d/%d", len(res.Pairs), res.Total)
	}
	if res.Pairs[0].Overlap < res.Pairs[1].Overlap {
		t.Fatalf("겹침 많은 순이어야 한다 : %+v", res.Pairs)
	}
	if res.Pairs[0].B.Line != 2 {
		t.Fatalf("겹침 4 짝이 먼저여야 한다 : %s", res.Pairs[0].ID())
	}
}

func TestPairsSkipRunsBeforeLimitSoGapIsFilled(t *testing.T) {
	docs := []Doc{
		{Name: "00-a.md", Sentences: []mdscan.Sentence{sent(1, "창문 색은 파랑 유리로 둔다.")}},
		{Name: "01-b.md", Sentences: []mdscan.Sentence{
			sent(1, "창문 색은 파랑 유리로 안 둔다."),   // 겹침 4 — Skip 으로 뺀다
			sent(2, "창문 색은 파랑 유리로 3번 바꾼다."), // 겹침 3 — 이것이 상한 1 을 채워야 한다
		}},
	}
	res := Pairs(docs, Options{Limit: 1, Skip: func(p Pair) bool { return p.B.Line == 1 }})
	if len(res.Pairs) != 1 || res.Pairs[0].B.Line != 2 {
		t.Fatalf("Skip 으로 빠진 자리를 뒤 짝이 채워야 한다 : %+v", res.Pairs)
	}
	if res.Total != 1 || len(res.Dropped) != 1 {
		t.Fatalf("Total 은 뺀 뒤 수(1), Dropped 는 1 이어야 한다 : %d %d", res.Total, len(res.Dropped))
	}
}

func TestPairsTrimsHubs(t *testing.T) {
	hub := Doc{Name: "00-a.md", Sentences: []mdscan.Sentence{sent(1, "자원 건물 수는 3개다.")}}
	others := Doc{Name: "01-b.md"}
	for i := 0; i < 25; i++ {
		others.Sentences = append(others.Sentences, sent(i+1, fmt.Sprintf("자원 건물 수는 %d개로 바꾼다.", i+10)))
	}
	res := Pairs([]Doc{hub, others}, Options{})
	if len(res.Pairs) != hubKeep {
		t.Fatalf("허브 문장은 %d개만 남겨야 한다 (지금 %d)", hubKeep, len(res.Pairs))
	}
}

func TestPairsChangedFilter(t *testing.T) {
	docs := []Doc{
		{Name: "00-a.md", Sentences: []mdscan.Sentence{sent(1, "대기 시간은 3초로 둔다.")}},
		{Name: "01-b.md", Sentences: []mdscan.Sentence{sent(1, "대기 시간은 5초로 둔다.")}},
		{Name: "02-c.md", Sentences: []mdscan.Sentence{sent(1, "대기 시간은 7초로 둔다.")}},
	}
	res := Pairs(docs, Options{Changed: map[string]bool{"02-c.md": true}})
	if len(res.Pairs) != 2 {
		t.Fatalf("바뀐 문서가 낀 짝만 2개여야 한다 : %+v", res.Pairs)
	}
	for _, p := range res.Pairs {
		if p.BDoc != "02-c.md" {
			t.Fatalf("바뀐 문서가 안 낀 짝이 있다 : %s", p.ID())
		}
	}
}
