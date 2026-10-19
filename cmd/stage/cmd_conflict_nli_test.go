package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mirusona/officina-game-design-tool/internal/conflict"
)

// nliFake 는 가짜 NLI 판정 서버다. 실제 서버(8092)는 시험에서 절대 안 부른다.
// claim 에 「않」 이 들면 b .92 · 「7초」 면 b .60 · claim 「그대로」+evidence 「3초」 면 a .93 · 나머지는 a .5 (조용).
type nliFake struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []map[string]any
}

func newNLIFake(t *testing.T) *nliFake {
	t.Helper()
	f := &nliFake{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"ok":true,"model":"nli-test","backend":"onnx","name":"시험 모델"}`))
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.reqs = append(f.reqs, req)
		f.mu.Unlock()
		claim, _ := req["claim"].(string)
		evidence, _ := req["evidence"].(string)
		a, b, c := 0.5, 0.05, 0.45
		switch {
		case strings.Contains(claim, "않"):
			a, b, c = 0.04, 0.92, 0.04
		case strings.Contains(claim, "7초"):
			a, b, c = 0.3, 0.6, 0.1
		case strings.Contains(claim, "그대로") && strings.Contains(evidence, "3초"):
			a, b, c = 0.93, 0.02, 0.05
		}
		raw, _ := json.Marshal(map[string]any{"a": a, "b": b, "c": c, "model": "nli-test", "label_set": "nli3"})
		_, _ = w.Write(raw)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func setNLIConfig(t *testing.T, root, url string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "Docs", "Todo", "기획설정.json"), `{"어긋남판정기": "nli", "판정서버": "`+url+`"}`)
}

// closedURL 은 방금 닫은 루프백 서버 주소다 — health 가 바로 실패한다.
func closedURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	return url
}

func decodeConflictJSON(t *testing.T, out string) conflictJSON {
	t.Helper()
	var got conflictJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("--json 이 JSON 이 아니다 : %v\n%s", err, out)
	}
	return got
}

// 설계 8절 #5 : NLI 면 정리 필요 = A ≥ .89 · 요청에 힌트 없음 · label_set nli4 · 판정기·모델 칸.
func TestConflictNLIGradesTidyBySupport(t *testing.T) {
	root := pairsRepo(t)
	fake := newNLIFake(t)
	setNLIConfig(t, root, fake.srv.URL)
	t.Setenv("PATH", "")
	out, code := runConflict(t, root, "--judge", "--json")
	if code != exitOK {
		t.Fatalf("종료 %d\n%s", code, out)
	}
	got := decodeConflictJSON(t, out)
	if got.Kind != "nli" || got.JudgeBy != "nli" || got.Model != "nli-test (시험 모델)" || got.Judged != 4 || got.Skipped != "" {
		t.Fatalf("요약이 다르다 : %+v", got)
	}
	if len(got.Warn) != 1 || got.Warn[0].Prob != 0.92 || len(got.Ambiguous) != 1 || got.Ambiguous[0].Prob != 0.6 {
		t.Fatalf("경고·애매는 B 자 그대로여야 한다 : %+v · %+v", got.Warn, got.Ambiguous)
	}
	if len(got.Tidy) != 1 || got.Tidy[0].ID != "00-바탕.md:9:1|02-수치.md:5:1" || got.Tidy[0].Prob != 0.93 {
		t.Fatalf("정리 필요는 A ≥ .89 여야 한다 : %+v", got.Tidy)
	}
	if strings.Contains(got.Ambiguous[0].B, "[다른 토막") {
		t.Fatalf("NLI 길의 claim 에는 힌트가 없다 : %q", got.Ambiguous[0].B)
	}
	if len(fake.reqs) != 4 {
		t.Fatalf("요청 4번이어야 한다 : %d", len(fake.reqs))
	}
	for _, req := range fake.reqs {
		claim, _ := req["claim"].(string)
		if req["label_set"] != "nli4" || strings.Contains(claim, "[다른 토막") || len(req) != 3 {
			t.Fatalf("요청 몸통이 다르다 (nli4 · 힌트 없음 · 세 칸) : %v", req)
		}
	}
	sent, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(got.Sent)))
	if strings.Contains(string(sent), "다른 토막") {
		t.Fatalf("보낸 기록 파일도 실제로 보낸 글(힌트 없음)이어야 한다 : %s", sent)
	}
	result, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(got.Sent, ".jsonl")+".result.jsonl")))
	if err != nil || strings.Count(string(result), `"backend":"nli"`) != 4 || !strings.Contains(string(result), `"model":"nli-test"`) {
		t.Fatalf("결과 jsonl 줄마다 backend·model 이 있어야 한다 : %v %s", err, result)
	}
	last := readLastJSON(t, root)
	if last["kind"] != "nli" || last["판정기"] != "nli" || last["모델"] != "nli-test (시험 모델)" ||
		last["hint_avg_runes"] != float64(0) || last["hint_off"] != float64(0) || last["judge_error"] != "" {
		t.Fatalf("last.json 에 판정기·모델이 있어야 한다 : %v", last)
	}
	text, _ := runConflict(t, root, "--judge")
	if !strings.Contains(text, "├ 정리 필요 (A ≥ 0.89 · 같은 말 두 자리)") || !strings.Contains(text, "A 0.93") ||
		!strings.Contains(text, "· 판정 nli · 모델 nli-test (시험 모델))") || strings.Contains(text, "(D ≥") {
		t.Fatalf("사람용 출력이 다르다\n%s", text)
	}
}

// 설계 8절 #4 : health 가 실패하면 「판정 건너뜀」 길 그대로 · 종료 코드도 localharness 건너뜀과 같다.
func TestConflictNLIHealthFailureSkips(t *testing.T) {
	root := pairsRepo(t)
	installFakeJudge(t, "ok")
	mark := filepath.Join(t.TempDir(), "called")
	t.Setenv(fakeJudgeMarkEnv, mark)
	setNLIConfig(t, root, closedURL(t))
	out, code := runConflict(t, root, "--judge")
	if code != exitOK || !strings.HasPrefix(out, "판정 건너뜀 — 판정 서버 응답 없음 · 후보 쌍 4개") || strings.Contains(out, "판정 실패") {
		t.Fatalf("health 실패는 건너뜀 + 종료 0 이어야 한다 (%d)\n%s", code, out)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Fatal("nli 를 골랐으면 localharness 로 되돌아가면 안 된다")
	}
	out, code = runConflict(t, root, "--judge", "--json")
	got := decodeConflictJSON(t, out)
	if code != exitOK || got.Skipped != "판정 서버 응답 없음" || got.JudgeBy != "nli" || got.Judged != 0 {
		t.Fatalf("--json 건너뜀 = %d %+v", code, got)
	}
	last := readLastJSON(t, root)
	judgeErr, _ := last["judge_error"].(string)
	if !strings.HasPrefix(judgeErr, "판정 서버 health 실패 : ") || last["result"] != "" {
		t.Fatalf("health 실패 까닭은 judge_error 에 남고 result 는 비어야 한다 : %v", last)
	}
}

// 설계 8절 #1 : 기본 localharness · 설정 nli · --judge-by 가 설정보다 먼저 · 모르는 값 실패.
func TestConflictJudgeBySelection(t *testing.T) {
	root := pairsRepo(t)
	installFakeJudge(t, "ok")
	out, _ := runConflict(t, root, "--judge", "--json")
	if got := decodeConflictJSON(t, out); got.JudgeBy != "localharness" || got.Kind != "conflict" || got.Model != "" {
		t.Fatalf("설정이 없으면 localharness 길이어야 한다 : %+v", got)
	}
	setNLIConfig(t, root, closedURL(t))
	out, _ = runConflict(t, root, "--judge", "--judge-by", "localharness", "--json")
	if got := decodeConflictJSON(t, out); got.JudgeBy != "localharness" || got.Kind != "conflict" || got.Judged != 4 {
		t.Fatalf("--judge-by 가 설정 nli 보다 먼저여야 한다 : %+v", got)
	}
	fake := newNLIFake(t)
	writeFile(t, filepath.Join(root, "Docs", "Todo", "기획설정.json"), `{"판정서버": "`+fake.srv.URL+`"}`)
	out, _ = runConflict(t, root, "--judge", "--judge-by", "nli", "--json")
	if got := decodeConflictJSON(t, out); got.JudgeBy != "nli" || got.Kind != "nli" || got.Judged != 4 {
		t.Fatalf("--judge-by nli 가 설정 기본보다 먼저여야 한다 : %+v", got)
	}
	if _, code := runConflict(t, root, "--judge", "--judge-by", "llama"); code != exitUsage {
		t.Fatalf("모르는 --judge-by 는 종료 %d 여야 한다 (지금 %d)", exitUsage, code)
	}
	writeFile(t, filepath.Join(root, "Docs", "Todo", "기획설정.json"), `{"어긋남판정기": "llama"}`)
	if _, code := runConflict(t, root, "--judge"); code != exitCorrupt {
		t.Fatalf("설정의 모르는 판정기는 종료 %d 여야 한다 (지금 %d)", exitCorrupt, code)
	}
	// 판정 길이 아니면 판정기 칸이 비고, 후보 jsonl 은 힌트가 붙은 옛 꼴 그대로다 (측정 셋 재료).
	setNLIConfig(t, root, fake.srv.URL)
	out, _ = runConflict(t, root, "--json")
	if !strings.Contains(out, "[다른 토막") {
		t.Fatalf("판정 안 하는 길의 후보 줄은 힌트를 그대로 단다\n%s", out)
	}
}

// 설계 8절 #2 : 비루프백 · http 아님 주소는 설정을 읽을 때 바로 실패한다 (판정 서버를 부르기 전).
func TestConflictNLIRefusesNonLoopbackURL(t *testing.T) {
	root := pairsRepo(t)
	for _, url := range []string{"http://203.0.113.10:8092", "https://127.0.0.1:8092"} { // guard:ok 문서용 주소(TEST-NET-3)
		setNLIConfig(t, root, url)
		if _, code := runConflict(t, root, "--judge"); code != exitCorrupt {
			t.Fatalf("%s 는 종료 %d 여야 한다 (지금 %d)", url, exitCorrupt, code)
		}
	}
}

// 설계 8절 #3 (명령 쪽) : 줄 하나가 실패해도 나머지는 판정되고, 못 받은 줄은 ⚠ 줄에 센다.
func TestConflictNLIRowFailureCountsUnjudged(t *testing.T) {
	root := pairsRepo(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"ok":true,"model":"nli-test"}`))
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if claim, _ := req["claim"].(string); strings.Contains(claim, "않") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"a":0.5,"b":0.1,"c":0.4,"model":"nli-test"}`))
	}))
	defer srv.Close()
	setNLIConfig(t, root, srv.URL)
	out, code := runConflict(t, root, "--judge")
	if code != exitOK || !strings.Contains(out, "판정 3 (경고 0 · 애매 0 · 정리 필요 0 · 조용 3)") || !strings.Contains(out, "판정 못 받은 줄 1") {
		t.Fatalf("줄 실패는 셈에서 빠지고 ⚠ 줄에 남아야 한다 (%d)\n%s", code, out)
	}
}

// NLI 자 : B 경고가 먼저 · 정리 필요는 A ≥ .89 · R(d)은 적기만 하고 통을 안 바꾼다.
func TestGradeNLIUsesSupportForTidy(t *testing.T) {
	run := &conflictRun{kind: conflict.BackendNLI, pairs: map[string]conflict.Pair{}, outcome: conflict.Outcome{Verdicts: map[string]conflict.Verdict{
		"warn":  {Letter: "B", Probs: map[string]float64{"A": 0.1, "B": 0.85, "C": 0.05}},
		"tidy":  {Letter: "A", Probs: map[string]float64{"A": 0.9, "B": 0.05, "C": 0.05}},
		"below": {Letter: "A", Probs: map[string]float64{"A": 0.88, "B": 0.06, "C": 0.06}},
		"amb":   {Letter: "B", Probs: map[string]float64{"A": 0.4, "B": 0.55, "C": 0.05}},
		"r":     {Letter: "R", Probs: map[string]float64{"A": 0.05, "B": 0.05, "C": 0.05, "R": 0.85}},
	}}}
	for _, id := range []string{"warn", "tidy", "below", "amb", "r"} {
		run.rows = append(run.rows, conflict.Row{ID: id})
	}
	g, judged := grade(run)
	if judged != 5 || len(g.warn) != 1 || len(g.tidy) != 1 || len(g.amb) != 1 {
		t.Fatalf("judged %d · 경고 %d · 정리 %d · 애매 %d", judged, len(g.warn), len(g.tidy), len(g.amb))
	}
	if g.warn[0].ID != "warn" || g.tidy[0].ID != "tidy" || g.tidy[0].Prob != 0.9 || g.amb[0].ID != "amb" {
		t.Fatalf("통이 다르다 : %+v · %+v · %+v", g.warn, g.tidy, g.amb)
	}
}
