package conflict

import (
	"testing"

	"github.com/mirusona/officina-game-design-tool/internal/mdscan"
)

func TestExcludeMetaOnlyInHead(t *testing.T) {
	meta := mdscan.Sentence{Line: 4, Seq: 1, Head: true, Text: "날짜 2026-09-12 · 마지막 고침 2026-10-03 · 정한 사람 사용자 · 상태 임시"}
	if got := Exclude(meta); got != ExcludeMeta {
		t.Fatalf("머리말의 날짜·정한 사람 줄은 메타여야 한다 : %q", got)
	}
	body := meta
	body.Head = false
	body.Ctx = "절 : 3. 자원 흐름"
	if got := Exclude(body); got != "" {
		t.Fatalf("본문의 같은 글은 안 뺀다 (상태는 게임 낱말일 수 있다) : %q", got)
	}
	claim := mdscan.Sentence{Head: true, Text: "골드 소비처는 합성소에 몬다 (2026-09-08 사용자 결정)."}
	if got := Exclude(claim); got != "" {
		t.Fatalf("날짜만 있고 메타 낱말이 없으면 주장이다 : %q", got)
	}
}

func TestExcludeRecordSectionAndPathLine(t *testing.T) {
	cases := []struct {
		name string
		s    mdscan.Sentence
		want string
	}{
		{"근거 절", mdscan.Sentence{Ctx: "절 : 14. 근거", Text: "결정 기록 : 12절의 mem id 들 (mem show )"}, ExcludeRecord},
		{"참고 절 표 칸", mdscan.Sentence{Ctx: "절 : 참고 · 줄 머리 : 1 · 칸 머리 : 무엇", Text: "지난 조사 문서를 본다."}, ExcludeRecord},
		{"경로 시작", mdscan.Sentence{Ctx: "절 : 5. 보기", Text: "Docs/Design/02-기능목록.md — Must 1·9·7·8"}, ExcludeRecord},
		{"파일 이름 시작", mdscan.Sentence{Text: "03-시스템-농장.md:12 재료 상한 100 을 본다"}, ExcludeRecord},
		{"근거 머리", mdscan.Sentence{Ctx: "절 : 찹쌀이 농장 설계", Text: "근거 결정 : mem 20260918-ace5b370(재료 상한 종마다 100) · 20260918-756b4011"}, ExcludeRecord},
		{"경로가 안에만", mdscan.Sentence{Ctx: "절 : 3. 규칙", Text: "값의 정본은 Docs/Design/04-수치표.md 다."}, ""},
		{"근거 낱말이 본문에만", mdscan.Sentence{Ctx: "절 : 2. 왜 이렇게 정했나", Text: "근거는 세로 조각 판정 3번이다."}, ""},
		{"근거리 머리", mdscan.Sentence{Ctx: "절 : 3. 전투", Text: "근거리 공격 : 사거리 3칸"}, ""},
		{"근거리 머리 붙은 콜론", mdscan.Sentence{Ctx: "절 : 3. 전투", Text: "근거리 무기: 단검"}, ""},
		{"참고로 머리", mdscan.Sentence{Ctx: "절 : 3. 전투", Text: "참고로 : 보스는 2페이즈다"}, ""},
		{"참고 머리", mdscan.Sentence{Ctx: "절 : 3. 전투", Text: "참고 : 지난 조사"}, ExcludeRecord},
		{"근거리 전투 절", mdscan.Sentence{Ctx: "절 : 4. 근거리 전투", Text: "근거리 공격은 3칸까지 닿는다."}, ""},
		{"최고 기록 보상 절", mdscan.Sentence{Ctx: "절 : 5. 최고 기록 보상", Text: "최고 기록을 넘으면 골드 100 을 준다."}, ""},
		{"기록 갱신 절", mdscan.Sentence{Ctx: "절 : 기록 갱신", Text: "기록을 넘으면 골드 100 을 준다."}, ""},
		{"참고 문서 절", mdscan.Sentence{Ctx: "절 : 9. 참고 문서 · 줄 머리 : 1", Text: "지난 조사 문서를 본다."}, ExcludeRecord},
	}
	for _, c := range cases {
		if got := Exclude(c.s); got != c.want {
			t.Errorf("%s : %q, %q 여야 한다", c.name, got, c.want)
		}
	}
}
