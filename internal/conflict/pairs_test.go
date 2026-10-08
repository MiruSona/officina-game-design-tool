package conflict

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mirusona/officina-game-design-tool/internal/mdscan"
)

func sent(line int, text string) mdscan.Sentence {
	return mdscan.Sentence{Line: line, Seq: 1, Text: text}
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
	if res.Pairs[0].ID() != "00-바탕.md:3:1|02-자세.md:10:1" {
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
		{Name: "01-앞.md", Sentences: []mdscan.Sentence{{Line: 4, Seq: 1, Text: "대기 시간은 3초로 둔다.", Ctx: "절 : 규칙 · 줄 머리 : 대기"}}},
	}
	res := Pairs(docs, Options{})
	if len(res.Pairs) != 1 {
		t.Fatalf("짝 %d개", len(res.Pairs))
	}
	p := res.Pairs[0]
	if p.ADoc != "01-앞.md" || p.Evidence() != "[절 : 규칙 · 줄 머리 : 대기] 대기 시간은 3초로 둔다." {
		t.Fatalf("evidence = %q (%s)", p.Evidence(), p.ADoc)
	}
	if p.Claim() != "대기 시간은 5초로 둔다. [다른 토막 : 값 A 3초 ↔ B 5초]" {
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

func TestPairsSkipsMetaAndRecordLinesAndClaimCarriesCtx(t *testing.T) {
	docs := []Doc{
		{Name: "00-바탕.md", Sentences: []mdscan.Sentence{
			{Line: 3, Seq: 1, Head: true, Text: "날짜 2026-10-01 · 정한 사람 사용자 · 상태 임시 · 대기 시간 3초"},
			{Line: 9, Seq: 2, Ctx: "절 : 규칙 · 줄 머리 : 대기", Text: "대기 시간은 3초로 둔다."},
		}},
		{Name: "02-자세.md", Sentences: []mdscan.Sentence{
			{Line: 3, Seq: 1, Head: true, Text: "날짜 2026-10-03 · 정한 사람 사용자 · 상태 확정 · 대기 시간 7초"},
			{Line: 10, Seq: 1, Ctx: "절 : 자세", Text: "대기 시간은 5초로 늘린다."},
			{Line: 20, Seq: 1, Ctx: "절 : 근거", Text: "대기 시간 3초 규칙은 바탕 문서 9줄을 따른다."},
		}},
	}
	res := Pairs(docs, Options{})
	if len(res.Pairs) != 1 || res.Excluded[ExcludeMeta] != 2 || res.Excluded[ExcludeRecord] != 1 {
		t.Fatalf("메타 2 · 근거 1 을 빼고 짝 1 이어야 한다 : 짝 %d · 뺀 것 %v", len(res.Pairs), res.Excluded)
	}
	p := res.Pairs[0]
	if p.ID() != "00-바탕.md:9:2|02-자세.md:10:1" {
		t.Fatalf("id 는 문서:줄:칸 꼴이어야 한다 : %s", p.ID())
	}
	if p.Claim() != "[절 : 자세] 대기 시간은 5초로 늘린다. [다른 토막 : 값 A 3초 ↔ B 5초 · 낱말 A 둔다 ↔ B 늘린다]" {
		t.Fatalf("claim 에도 Ctx 와 힌트가 붙어야 한다 : %q", p.Claim())
	}
	if res.Sentences != 2 {
		t.Fatalf("문장 수는 뺀 뒤 값이어야 한다 : %d", res.Sentences)
	}
}

func TestHintListsOnlyDifferingNumbersAndNegation(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want string
	}{
		{"같은 값 — 값 줄 없음", "대기 시간은 3초로 둔다.", "대기 시간은 3초다.", "다른 토막 : 낱말 A 둔다 ↔ B —"},
		{"부정어 B 에만", "창문은 아침에 두 번 열린다.", "창문은 아침에 두 번 열리지 않는다.", "다른 토막 : 낱말 A 열린다 ↔ B 열리지·않는다 · 부정어 B 에만"},
		{"값과 부정어", "상한은 100 이고 보관함 상한은 없다.", "상한은 200 이다.", "다른 토막 : 값 A 100 ↔ B 200 · 낱말 A 이고… ↔ B — · 부정어 A 에만"},
		{"날짜·id·문서·절 은 값이 아니다", "2026-09-08 에 정했다 (mem 20260908-abcd1234). 값은 3초다.", "값은 3초다 (09-12 고침 · 02-기능목록.md 3절).", "다른 토막 : 낱말 A 정했… ↔ B 고침"},
		{"값 상한 넷 · 겹침 0 이면 낱말 줄 없음", "값 1 2 3 4 5 6 이다.", "값 없음이다.", "다른 토막 : 값 A 1·2·3·4 ↔ B 없음 · 부정어 B 에만"},
		{"둘 다 같으면 힌트 없음", "대기 시간은 3초다.", "대기 시간은 3초다", ""},
	}
	for _, c := range cases {
		p := Pair{A: mdscan.Sentence{Text: c.a}, B: mdscan.Sentence{Text: c.b}}
		if got := p.Hint(); got != c.want {
			t.Errorf("%s : %q, %q 여야 한다", c.name, got, c.want)
		}
	}
}

func TestPairsDefersOverlapTwoAcrossSections(t *testing.T) {
	docs := []Doc{
		{Name: "00-a.md", Sentences: []mdscan.Sentence{
			{Line: 1, Seq: 1, Ctx: "절 : 1. 규칙", Text: "대기 시간은 3초로 둔다."},
			{Line: 2, Seq: 1, Ctx: "절 : 2. 화면", Text: "창문 색은 파랑 1번이다."},
		}},
		{Name: "01-b.md", Sentences: []mdscan.Sentence{
			{Line: 1, Seq: 1, Ctx: "절 : 9. 다른 절", Text: "대기 시간은 5초로 늘린다."},          // 겹침 2 · 절 다름 → 미룸
			{Line: 2, Seq: 1, Ctx: "절 : 2. 화면 · 줄 머리 : 창문", Text: "창문 색은 노랑 2번이다."}, // 겹침 2 · 절 같음 → 안 미룸
		}},
	}
	res := Pairs(docs, Options{})
	if len(res.Pairs) != 2 || res.Pairs[0].Deferred || !res.Pairs[1].Deferred {
		t.Fatalf("절 제목이 다른 겹침 2 짝은 뒤로 가야 한다 : %+v", res.Pairs)
	}
	if res.Pairs[0].B.Line != 2 {
		t.Fatalf("같은 절 짝이 먼저여야 한다 : %s", res.Pairs[0].ID())
	}
}

// 손질 3차 ① — 2차에 놓친 「뒤튼 토막이 낱말」 꼴(실측 10절 synth-03·22·23·26·29·30)이 힌트에 실려야 한다.
func TestHintWordsCarryTwistedWord(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want string
	}{
		{"최소↔최대", "창고 칸은 최소 3칸을 둔다.", "창고 칸은 최대 3칸을 둔다.", "낱말 A 최소 ↔ B 최대"},
		{"고인다↔모자란다", "물은 밤에 통에 고인다.", "물은 밤에 통에 모자란다.", "낱말 A 고인다 ↔ B 모자란다"},
		{"시간↔골드", "값은 시간으로 치른다 (5).", "값은 골드로 치른다 (5).", "낱말 A 시간 ↔ B 골드"},
		{"영구↔일시", "효과는 영구 적용이며 2단계다.", "효과는 일시 적용이며 2단계다.", "낱말 A 영구 ↔ B 일시"},
		{"멈춤↔두 배로", "밤에는 생산이 멈춤 상태가 되고 3분 뒤 깬다.", "밤에는 생산이 두 배로 상태가 되고 3분 뒤 깬다.", "낱말 A 멈춤 ↔ B 배로"},
		{"×1.5↔×3 은 값", "보너스는 ×1.5 로 센다.", "보너스는 ×3 로 센다.", "값 A ×1.5 ↔ B ×3"},
	}
	for _, c := range cases {
		p := Pair{A: mdscan.Sentence{Text: c.a}, B: mdscan.Sentence{Text: c.b}}
		got := p.Hint()
		if !strings.Contains(got, c.want) {
			t.Errorf("%s : %q 에 %q 가 있어야 한다", c.name, got, c.want)
		}
	}
	// 배율은 낱말 줄에 안 들어간다.
	p := Pair{A: mdscan.Sentence{Text: "보너스는 ×1.5 로 센다."}, B: mdscan.Sentence{Text: "보너스는 ×3 로 센다."}}
	if strings.Contains(p.Hint(), "낱말") {
		t.Fatalf("×배율은 값이지 낱말이 아니다 : %q", p.Hint())
	}
}

// 낱말 상한은 min(4, 겹침 수) 이고 잘린 쪽엔 「…」 가 붙는다. 공통 낱말·멈춤말·한 글자는 안 실린다.
func TestHintWordsLimitByOverlap(t *testing.T) {
	// 겹침 2 (창문 · 아침) — 한쪽 2 개까지.
	p := Pair{A: mdscan.Sentence{Text: "창문 아침 파랑 노랑 초록 보라 하나"}, B: mdscan.Sentence{Text: "창문 아침 검정"}}
	if got, want := p.Hint(), "다른 토막 : 낱말 A 파랑·노랑… ↔ B 검정"; got != want {
		t.Fatalf("겹침 2 면 한쪽 2 개에서 잘려야 한다 : %q", got)
	}
	// 겹침 5 — 상한 4 에서 잘린다.
	p = Pair{A: mdscan.Sentence{Text: "하나 두울 세엣 네엣 다섯 파랑 노랑 초록 보라 분홍"}, B: mdscan.Sentence{Text: "하나 두울 세엣 네엣 다섯 검정"}}
	if got, want := p.Hint(), "다른 토막 : 낱말 A 파랑·노랑·초록·보라… ↔ B 검정"; got != want {
		t.Fatalf("상한 4 : %q", got)
	}
	// 공통 낱말만 있고 멈춤말·한 글자 차이뿐이면 낱말 줄이 없다.
	p = Pair{A: mdscan.Sentence{Text: "창문 아침 그리고 것"}, B: mdscan.Sentence{Text: "창문 아침 수"}}
	if got := p.Hint(); got != "" {
		t.Fatalf("멈춤말·한 글자 차이는 힌트가 아니다 : %q", got)
	}
}

// 손질 3차 ② — 단위 붙은 값이 참조 번호보다 먼저 실리고, 비율·좌표는 한 토막이다.
func TestHintOrderUnitValuesFirst(t *testing.T) {
	p := Pair{A: mdscan.Sentence{Text: "총 시간은 합쳐 96분이다."}, B: mdscan.Sentence{Text: "13 14 3 2 를 합쳐 총 시간은 192분이다."}}
	if got, want := p.Hint(), "다른 토막 : 값 A 96분 ↔ B 192분·13·14·3"; got != want {
		t.Fatalf("단위 값이 먼저, 맨숫자는 상한 4 에서 잘려야 한다 : %q", got)
	}
	p = Pair{A: mdscan.Sentence{Text: "비율은 3/4 이고 배율은 ×3 이며 번호는 7 이다 (20골드)."}, B: mdscan.Sentence{Text: "비율은 1/2 이고 배율은 이며 번호는 이다."}}
	if got, want := p.Hint(), "다른 토막 : 값 A 20골드·3/4·×3·7 ↔ B 1/2"; got != want {
		t.Fatalf("순서는 단위 → 비율 → 배율 → 맨숫자 : %q", got)
	}
}

// 손질 3차 ③ — 숫자 토막 합이 8 을 넘으면 힌트를 통째로 끈다 (부정어 줄도).
func TestHintOffOnNumericRow(t *testing.T) {
	nine := Pair{A: mdscan.Sentence{Text: "단계 값은 1 2 3 4 5 이다."}, B: mdscan.Sentence{Text: "단계 값은 6 7 8 9 가 아니다."}}
	if h, off := nine.HintInfo(); h != "" || !off {
		t.Fatalf("숫자 합 9 면 힌트가 꺼져야 한다 : %q off=%v", h, off)
	}
	eight := Pair{A: mdscan.Sentence{Text: "단계 값은 1 2 3 4 이다."}, B: mdscan.Sentence{Text: "단계 값은 6 7 8 9 가 아니다."}}
	if h, off := eight.HintInfo(); h == "" || off {
		t.Fatalf("숫자 합 8 은 경계 안이라 힌트가 있어야 한다 : %q off=%v", h, off)
	}
}
