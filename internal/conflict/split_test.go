package conflict

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// 자르기 규칙 (쪼개묻기 설계 2절) : 머리 떼기 · 문장 끝 · 띄어 쓴 이음 기호 · 괄호 보호 · 빈 토막 버림 ·
// 2번째 고리 : 괄호 안 문장 끝 안 자름 · 날짜 괄호 지움 · 목록 꼴 건너뜀.
func TestEvidencePieces(t *testing.T) {
	cases := []struct {
		name, in string
		want     []string
	}{
		{"설계 보기", "[절 : 3] 칸 4·단축 5단계를 다 사면(사람 판에서 96분) 골드가 고인다. 그 뒤 후보 : 농장(번식 · 상한) · 가판대 강화.",
			[]string{"칸 4·단축 5단계를 다 사면(사람 판에서 96분) 골드가 고인다.", "그 뒤 후보 : 농장(번식 · 상한)", "가판대 강화."}},
		{"문장 끝 셋", "하나다. 둘인가? 셋이다!  넷", []string{"하나다.", "둘인가?", "셋이다!", "넷"}},
		{"「다」 뒤는 안 자름", "다 사면 고인다", []string{"다 사면 고인다"}},
		{"빈칸 없는 마침표는 안 자름", "값 1.5 배", []string{"값 1.5 배"}},
		{"이음 기호 셋", "가 — 나 ↔ 다 · 라", []string{"가", "나", "다", "라"}},
		{"붙여 쓴 · 는 안 자름", "칸 4·단축", []string{"칸 4·단축"}},
		{"전각 괄호·낫표 보호", "가（나 · 다）· 라 · 「마 — 바」", []string{"가（나 · 다）· 라", "「마 — 바」"}},
		{"닫는 괄호만 있어도 계속 자름", "가) · 나 · 다", []string{"가)", "나", "다"}},
		{"줄 머리 꼴 머리도 뗌", "[줄 머리 : 대기 · 칸 머리 : 값] 3초", []string{"3초"}},
		{"머리 아닌 대괄호는 남김", "[임시] 값 · 둘", []string{"[임시] 값", "둘"}},
		{"빈 토막 버림", "  · 가 ·  ", []string{"가"}},
		{"빈 글", "", []string{}},
		{"괄호 안 마침표는 안 자름", "값 (처음. 둘째) 이다. 끝", []string{"값 (처음. 둘째) 이다.", "끝"}},
		{"낫표 안 물음표도 안 자름", "「됐나? 응」 하고 묻는다. 끝", []string{"「됐나? 응」 하고 묻는다.", "끝"}},
		{"날짜 괄호 YYYY-MM-DD", "넘치면 보관함 (2026-10-08 사용자 결정).", []string{"넘치면 보관함."}},
		{"날짜 괄호 MM-DD · 전각", "넘치면 보관함（10-08 결정） · 둘", []string{"넘치면 보관함", "둘"}},
		{"날짜 괄호 M월 D일", "넘치면 보관함 (10월 8일 결정)", []string{"넘치면 보관함"}},
		{"날짜 아닌 괄호는 남김", "다 사면(96분) 고인다 (3-5개) (110-08)", []string{"다 사면(96분) 고인다 (3-5개) (110-08)"}},
		{"날짜 괄호뿐인 토막은 버림", "(10-08 결정) · 가", []string{"가"}},
		{"닫히지 않은 날짜 괄호 안 생김", "상한은 재료별 (10-08 결정. 다음 판에 본다). 끝", []string{"상한은 재료별.", "끝"}},
	}
	for _, c := range cases {
		got, capped, skipped := EvidencePieces(c.in)
		if !reflect.DeepEqual(got, c.want) || capped != 0 || skipped != 0 {
			t.Errorf("%s : %q → %q (버림 %d) · 기대 %q", c.name, c.in, got, capped, c.want)
		}
	}
}

func TestEvidencePiecesCapped(t *testing.T) {
	parts := []string{}
	for i := 0; i < MaxPieces+3; i++ {
		parts = append(parts, string(rune('가'+i)))
	}
	got, capped, _ := EvidencePieces(strings.Join(parts, " · "))
	if len(got) != MaxPieces || capped != 3 || got[0] != "가" || got[MaxPieces-1] != parts[MaxPieces-1] {
		t.Fatalf("상한 = %d 토막 · 버림 %d · %q", len(got), capped, got)
	}
}

// 목록 꼴 (2번째 고리) : 항목 표시가 2개를 넘는 토막은 건너뛰고 센다. 2개까지는 묻는다. 상한은 건너뛴 뒤에 센다.
func TestEvidencePiecesSkipsLists(t *testing.T) {
	cases := []struct {
		name, in string
		want     []string
		skipped  int
	}{
		{"동그라미 셋", "① 가 ② 나 ③ 다 · 끝", []string{"끝"}, 1},
		{"동그라미 둘은 묻는다", "① 가 ② 나", []string{"① 가 ② 나"}, 0},
		{"괄호 번호 셋", "1) 가 2) 나 3) 다", []string{}, 1},
		{"낱말 붙은 괄호는 번호가 아님", "값1) 가 값2) 나 값3) 다", []string{"값1) 가 값2) 나 값3) 다"}, 0},
	}
	for _, c := range cases {
		got, capped, skipped := EvidencePieces(c.in)
		if !reflect.DeepEqual(got, c.want) || capped != 0 || skipped != c.skipped {
			t.Errorf("%s : %q → %q (버림 %d · 건너뜀 %d) · 기대 %q · 건너뜀 %d", c.name, c.in, got, capped, skipped, c.want, c.skipped)
		}
	}
	parts := []string{"① 가 ② 나 ③ 다"}
	for i := 0; i < MaxPieces; i++ {
		parts = append(parts, string(rune('가'+i)))
	}
	if got, capped, skipped := EvidencePieces(strings.Join(parts, " · ")); len(got) != MaxPieces || capped != 0 || skipped != 1 {
		t.Fatalf("건너뛴 토막은 상한에 안 센다 : %d 토막 · 버림 %d · 건너뜀 %d", len(got), capped, skipped)
	}
}

// 리뷰가 짚은 두 길 : 원래 꼴 물음이 실패하면 토막은 안 묻는다(물음 1) · 토막 0개 줄은 빈 pieces 와 best -1 · 건너뛴 수 기록.
func TestRunNLISplitBaseFailureAndNoPieces(t *testing.T) {
	var got []map[string]any
	srv := splitFake(t, &got)
	oc := RunNLI(srv.URL, []Row{{ID: "a", Evidence: "터짐 · 가 · 나", Claim: "주장"}}, 5*time.Second, true)
	if v := oc.Verdicts["a"]; oc.Asks != 1 || len(got) != 1 || !strings.Contains(v.Problem, "HTTP 500") || v.Split != nil {
		t.Fatalf("원래 꼴 실패면 물음 1 · 기록 없음 : 물음 %d · %+v", oc.Asks, v)
	}
	got = nil
	oc = RunNLI(srv.URL, []Row{{ID: "b", Evidence: "[절 : 규칙] ① 가 ② 나 ③ 다", Claim: "주장"}}, 5*time.Second, true)
	v := oc.Verdicts["b"]
	want := &SplitRecord{BaseB: 0.3, Pieces: []PieceAnswer{}, Best: -1, Skipped: 1}
	if oc.Asks != 1 || v.Problem != "" || !reflect.DeepEqual(v.Split, want) {
		t.Fatalf("토막 0개 줄 : 물음 %d · %+v", oc.Asks, v.Split)
	}
	if !strings.Contains(string(oc.Accepted), `"split":{"base_b":0.3,"pieces":[],"best":-1,"skipped":1}`) {
		t.Fatalf("결과 줄에 빈 pieces 와 skipped : %s", oc.Accepted)
	}
}

// splitFake 는 근거 글로 답을 고르는 가짜 서버다 : 「뒤집힘」 → b .9 · 「같음」 → b .3 · 「터짐」 → 500 · 나머지 b .3.
func splitFake(t *testing.T, got *[]map[string]any) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		*got = append(*got, req)
		mu.Unlock()
		ev, _ := req["evidence"].(string)
		switch {
		case strings.Contains(ev, "터짐") && !strings.HasPrefix(ev, "[절"):
			w.WriteHeader(http.StatusInternalServerError)
		case strings.Contains(ev, "뒤집힘") && !strings.HasPrefix(ev, "[절"):
			_, _ = w.Write([]byte(`{"a":0.05,"b":0.9,"c":0.05,"model":"nli-test"}`))
		default:
			_, _ = w.Write([]byte(`{"a":0.6,"b":0.3,"c":0.1,"model":"nli-test"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// 합치기 (설계 3절) : 원래 꼴 + 토막마다 묻고 max b 답 묶음을 쓴다 · 기록에 토막 글과 b · 같으면 원래 꼴.
func TestRunNLISplitTakesMaxB(t *testing.T) {
	var got []map[string]any
	srv := splitFake(t, &got)
	rows := []Row{
		{ID: "win", Evidence: "[절 : 규칙] 값은 그대로 · 순서는 뒤집힘", Claim: "[절 : 자세] 주장 글"},
		{ID: "tie", Evidence: "[절 : 규칙] 같음 · 같음 둘", Claim: "주장"},
	}
	oc := RunNLI(srv.URL, rows, 5*time.Second, true)
	if oc.Asks != 6 || len(got) != 6 || NLIAsks(rows, true) != 6 || NLIAsks(rows, false) != 2 {
		t.Fatalf("물음 수 = %d · 요청 %d · NLIAsks %d/%d", oc.Asks, len(got), NLIAsks(rows, true), NLIAsks(rows, false))
	}
	// 차례 : 원래 꼴(머리 있는 근거) → 토막들. 주장은 늘 통째(머리째) 그대로다.
	if got[0]["evidence"] != rows[0].Evidence || got[1]["evidence"] != "값은 그대로" || got[2]["evidence"] != "순서는 뒤집힘" {
		t.Fatalf("묻는 차례가 다르다 : %v", got[:3])
	}
	for _, req := range got[:3] {
		if req["claim"] != "[절 : 자세] 주장 글" || len(req) != 3 {
			t.Fatalf("주장은 머리째 통째여야 한다 : %v", req)
		}
	}
	win := oc.Verdicts["win"]
	if win.ProbB() != 0.9 || win.Letter != "B" || win.ProbA() != 0.05 || win.Split == nil {
		t.Fatalf("max b 답 묶음을 통째로 써야 한다 : %+v", win)
	}
	want := &SplitRecord{BaseB: 0.3, Best: 1, Pieces: []PieceAnswer{{"값은 그대로", 0.3}, {"순서는 뒤집힘", 0.9}}}
	if !reflect.DeepEqual(win.Split, want) {
		t.Fatalf("기록 = %+v", win.Split)
	}
	if tie := oc.Verdicts["tie"]; tie.Split == nil || tie.Split.Best != -1 || tie.ProbB() != 0.3 {
		t.Fatalf("같은 b 면 원래 꼴이 앞서야 한다 : %+v", tie.Split)
	}
	line := strings.Split(string(oc.Accepted), "\n")[0]
	if !strings.Contains(line, `"split":{"base_b":0.3,"pieces":[{"text":"값은 그대로","b":0.3},{"text":"순서는 뒤집힘","b":0.9}],"best":1}`) {
		t.Fatalf("결과 줄에 split 기록이 있어야 한다 : %s", line)
	}
}

// 실패 (설계 결정 8) : 토막 물음 하나라도 실패하면 줄 전체 Problem · 다음 줄은 계속. 끈 판은 기록 없음 · 토막 안 물음.
func TestRunNLISplitPieceFailureAndOff(t *testing.T) {
	var got []map[string]any
	srv := splitFake(t, &got)
	rows := []Row{{ID: "a", Evidence: "[절 : 규칙] 가 · 터짐", Claim: "주장"}, {ID: "b", Evidence: "가 · 뒤집힘", Claim: "주장"}}
	oc := RunNLI(srv.URL, rows, 5*time.Second, true)
	if p := oc.Verdicts["a"].Problem; !strings.Contains(p, "토막 2 물음 실패") || !strings.Contains(p, "HTTP 500") {
		t.Fatalf("토막 실패는 줄 Problem 이어야 한다 : %q", p)
	}
	if v := oc.Verdicts["b"]; v.Problem != "" || v.ProbB() != 0.9 {
		t.Fatalf("실패한 줄 뒤에도 계속 물어야 한다 : %+v", v)
	}
	got = nil
	off := RunNLI(srv.URL, rows[1:], 5*time.Second, false)
	if v := off.Verdicts["b"]; len(got) != 1 || v.Split != nil || v.ProbB() != 0.9 || strings.Contains(string(off.Accepted), "split") {
		t.Fatalf("끈 판은 원래 꼴 한 번만 · 기록 없음 : %d · %+v", len(got), v)
	}
}

func TestRunNLISplitCountsCappedRows(t *testing.T) {
	var got []map[string]any
	srv := splitFake(t, &got)
	ev := strings.Repeat("가 · ", MaxPieces+2) + "끝"
	oc := RunNLI(srv.URL, []Row{{ID: "a", Evidence: ev, Claim: "주장"}}, 5*time.Second, true)
	if oc.Capped != 1 || oc.Asks != 1+MaxPieces || oc.Verdicts["a"].Split.Capped != 3 {
		t.Fatalf("상한 : 걸린 줄 %d · 물음 %d · 버림 %+v", oc.Capped, oc.Asks, oc.Verdicts["a"].Split)
	}
}
