package conflict

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/mdscan"
)

// fakeNLI 는 가짜 판정 서버다. claim 글에 든 낱말로 답 꼴을 고른다. 받은 요청 몸통은 got 에 쌓는다.
func fakeNLI(t *testing.T, got *[]map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"ok":true,"model":"nli-test","backend":"onnx","name":"시험 모델"}`))
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if got != nil {
			*got = append(*got, req)
		}
		claim, _ := req["claim"].(string)
		switch {
		case strings.Contains(claim, "넷칸"):
			_, _ = w.Write([]byte(`{"a":0.1,"b":0.2,"c":0.3,"d":0.4,"model":"nli-test","label_set":"nli4"}`))
		case strings.Contains(claim, "합틀림"):
			_, _ = w.Write([]byte(`{"a":0.5,"b":0.5,"c":0.5,"model":"nli-test"}`))
		case strings.Contains(claim, "칸빠짐"):
			_, _ = w.Write([]byte(`{"a":0.5,"b":0.5,"model":"nli-test"}`))
		case strings.Contains(claim, "범위밖"):
			_, _ = w.Write([]byte(`{"a":1.2,"b":-0.2,"c":0,"model":"nli-test"}`))
		case strings.Contains(claim, "터짐"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"judge failed"}`))
		case strings.Contains(claim, "느림"):
			time.Sleep(400 * time.Millisecond)
			_, _ = w.Write([]byte(`{"a":0.9,"b":0.05,"c":0.05,"model":"nli-test"}`))
		default:
			_, _ = w.Write([]byte(`{"a":0.7,"b":0.2,"c":0.1,"model":"nli-test","label_set":"nli3"}`))
		}
	}))
}

func rowsFor(claims ...string) []Row {
	rows := []Row{}
	for i, c := range claims {
		rows = append(rows, Row{ID: string(rune('a' + i)), Evidence: "근거", Claim: c})
	}
	return rows
}

func TestRunNLIThreeAndFourCells(t *testing.T) {
	srv := fakeNLI(t, nil)
	defer srv.Close()
	oc := RunNLI(srv.URL, rowsFor("세칸", "넷칸"), 5*time.Second, false)
	three, four := oc.Verdicts["a"], oc.Verdicts["b"]
	if three.Problem != "" || three.Letter != "A" || three.ProbA() != 0.7 || three.ProbB() != 0.2 || len(three.Probs) != 3 {
		t.Fatalf("3칸 답 = %+v", three)
	}
	if four.Problem != "" || four.Probs["R"] != 0.4 || four.ProbD() != 0 || four.Letter != "R" {
		t.Fatalf("4칸 답은 d 를 R 로 적기만 해야 한다 = %+v", four)
	}
	if oc.ExitErr != "" {
		t.Fatalf("ExitErr 가 비어야 한다 : %s", oc.ExitErr)
	}
	lines := strings.Split(strings.TrimSpace(string(oc.Accepted)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"backend":"nli"`) || !strings.Contains(lines[0], `"model":"nli-test"`) {
		t.Fatalf("결과 줄에 backend·model 이 있어야 한다 : %q", oc.Accepted)
	}
}

func TestRunNLIBadRowsBecomeProblemsAndContinue(t *testing.T) {
	srv := fakeNLI(t, nil)
	defer srv.Close()
	oc := RunNLI(srv.URL, rowsFor("합틀림", "칸빠짐", "범위밖", "터짐", "세칸"), 5*time.Second, false)
	for id, want := range map[string]string{"a": "합이", "b": "빠짐", "c": "0~1 밖", "d": "HTTP 500"} {
		if !strings.Contains(oc.Verdicts[id].Problem, want) {
			t.Fatalf("줄 %s 의 Problem 에 %q 가 있어야 한다 : %+v", id, want, oc.Verdicts[id])
		}
	}
	if oc.Verdicts["e"].Problem != "" || oc.Verdicts["e"].Letter != "A" {
		t.Fatalf("실패한 줄 뒤에도 계속 물어야 한다 : %+v", oc.Verdicts["e"])
	}
	if strings.Count(string(oc.Accepted), "\n") != 5 {
		t.Fatalf("실패한 줄도 결과 파일에 남는다 : %q", oc.Accepted)
	}
}

func TestRunNLITimeoutMarksRemainingRows(t *testing.T) {
	srv := fakeNLI(t, nil)
	defer srv.Close()
	oc := RunNLI(srv.URL, rowsFor("느림", "세칸"), 150*time.Millisecond, false)
	if !strings.Contains(oc.ExitErr, "안 끝났습니다") {
		t.Fatalf("기다림을 넘기면 ExitErr 에 까닭이 있어야 한다 : %q", oc.ExitErr)
	}
	for _, id := range []string{"a", "b"} {
		if !strings.Contains(oc.Verdicts[id].Problem, "안 끝났습니다") {
			t.Fatalf("줄 %s 는 기다림 초과 Problem 이어야 한다 : %+v", id, oc.Verdicts[id])
		}
	}
}

func TestRunNLISendsLabelSetWithoutHint(t *testing.T) {
	var got []map[string]any
	srv := fakeNLI(t, &got)
	defer srv.Close()
	p := Pair{ADoc: "00-바탕.md", BDoc: "01-자세.md",
		A: mdscan.Sentence{Line: 3, Seq: 1, Text: "대기 시간은 3초로 둔다.", Ctx: "절 : 규칙"},
		B: mdscan.Sentence{Line: 4, Seq: 1, Text: "대기 시간은 7초로 늘린다."}}
	if p.Hint() == "" {
		t.Fatal("시험 짝에는 힌트가 있어야 한다 (전제)")
	}
	RunNLI(srv.URL+"/", []Row{NLIRowOf(p)}, 5*time.Second, false)
	if len(got) != 1 {
		t.Fatalf("요청 1번이어야 한다 : %v", got)
	}
	req := got[0]
	if req["label_set"] != "nli4" || req["evidence"] != "[절 : 규칙] 대기 시간은 3초로 둔다." || req["claim"] != "대기 시간은 7초로 늘린다." {
		t.Fatalf("요청 몸통이 다르다 (힌트 없음 · Ctx 남김 · nli4) : %v", req)
	}
	if len(req) != 3 {
		t.Fatalf("요청은 세 칸뿐이어야 한다 : %v", req)
	}
}

func TestRunNLIDoesNotFollowRedirect(t *testing.T) {
	hits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"a":0.7,"b":0.2,"c":0.1,"model":"elsewhere"}`))
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/judge", http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	oc := RunNLI(redirector.URL, rowsFor("세칸"), 5*time.Second, false)
	if !strings.Contains(oc.Verdicts["a"].Problem, "HTTP 307") {
		t.Fatalf("307 은 따라가지 말고 Problem 이어야 한다 : %+v", oc.Verdicts["a"])
	}
	if hits != 0 {
		t.Fatalf("리다이렉트 대상 서버가 요청을 받으면 안 된다 (%d번)", hits)
	}
}

func TestHealthReadsModelAndFailsFast(t *testing.T) {
	srv := fakeNLI(t, nil)
	model, name, err := Health(srv.URL)
	if err != nil || model != "nli-test" || name != "시험 모델" {
		t.Fatalf("health = %q %q %v", model, name, err)
	}
	srv.Close()
	if _, _, err := Health(srv.URL); err == nil {
		t.Fatal("닫힌 서버면 오류여야 한다")
	}
	notOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false}`))
	}))
	defer notOK.Close()
	if _, _, err := Health(notOK.URL); err == nil {
		t.Fatal("ok 가 거짓이면 오류여야 한다")
	}
}

func TestNLIWaitIsCapped(t *testing.T) {
	if NLIWait(3) != 13*time.Second || NLIWait(1000) != MaxWait {
		t.Fatalf("NLIWait = %s · %s", NLIWait(3), NLIWait(1000))
	}
}
