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

// 설계 8절 #5 : NLI 길은 정리 필요를 안 매긴다 (10-19 실측) · 요청에 힌트 없음 · label_set nli4 · 판정기·모델 칸.
func TestConflictNLINoTidy(t *testing.T) {
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
	// A .93 · B .02 짝도 정리 필요에 안 들고 조용으로 간다. tidy 는 null 이 아닌 빈 배열이다.
	if got.Tidy == nil || len(got.Tidy) != 0 {
		t.Fatalf("NLI 길의 정리 필요는 빈 배열이어야 한다 : %+v", got.Tidy)
	}
	if !strings.Contains(out, `"tidy":[]`) {
		t.Fatalf("JSON 의 tidy 는 [] 여야 한다\n%s", out)
	}
	if strings.Contains(got.Ambiguous[0].B, "[다른 토막") {
		t.Fatalf("NLI 길의 claim 에는 힌트가 없다 : %q", got.Ambiguous[0].B)
	}
	// 쪼개 묻기 기본 켬 : 짝 4 × (원래 꼴 1 + 토막 1). 이 시험 문장은 이음 기호가 없어 토막이 하나다.
	if len(fake.reqs) != 8 || !got.Split || got.Asks != 8 {
		t.Fatalf("요청 8번 · split 켬이어야 한다 : %d · %+v", len(fake.reqs), got)
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
	if strings.Contains(text, "├ 정리 필요") || strings.Contains(text, "A 0.93") || !strings.Contains(text, "정리 필요 0 · 조용 2)") ||
		!strings.Contains(text, "· 판정 nli · 모델 nli-test (시험 모델))") || strings.Contains(text, "(D ≥") {
		t.Fatalf("사람용 출력이 다르다\n%s", text)
	}
	// 원래 꼴이 이긴 경고(토막과 b 가 같음)에는 「b 낸 토막」 줄이 없다.
	if !strings.Contains(text, "B 0.92") || strings.Contains(text, "b 낸 토막") {
		t.Fatalf("원래 꼴이 이긴 경고엔 토막 줄이 없어야 한다\n%s", text)
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

// NLI 자 : 정리 필요를 안 매긴다 (10-19 실측) · B 경고가 먼저 · 높은 A 도 B 대로 조용/경고/애매로 간다 · R(d)은 통을 안 바꾼다.
func TestGradeNLINeverTidy(t *testing.T) {
	run := &conflictRun{kind: conflict.BackendNLI, pairs: map[string]conflict.Pair{}, outcome: conflict.Outcome{Verdicts: map[string]conflict.Verdict{
		"warn":  {Letter: "B", Probs: map[string]float64{"A": 0.1, "B": 0.85, "C": 0.05}},
		"sure":  {Letter: "A", Probs: map[string]float64{"A": 0.908, "B": 0.03, "C": 0.062}},
		"below": {Letter: "A", Probs: map[string]float64{"A": 0.88, "B": 0.06, "C": 0.06}},
		"amb":   {Letter: "B", Probs: map[string]float64{"A": 0.4, "B": 0.55, "C": 0.05}},
		"r":     {Letter: "R", Probs: map[string]float64{"A": 0.05, "B": 0.05, "C": 0.05, "R": 0.85}},
		"d":     {Letter: "D", Probs: map[string]float64{"A": 0.05, "B": 0.05, "C": 0.05, "D": 0.85}},
	}}}
	for _, id := range []string{"warn", "sure", "below", "amb", "r", "d"} {
		run.rows = append(run.rows, conflict.Row{ID: id})
	}
	g, judged := grade(run)
	if judged != 6 || len(g.warn) != 1 || len(g.amb) != 1 {
		t.Fatalf("judged %d · 경고 %d · 애매 %d", judged, len(g.warn), len(g.amb))
	}
	// A .908 · B .03 짝(실측의 진짜 모순 자리)이 tidy 에 안 든다. tidy 는 빈 배열(nil 아님)이다.
	if g.tidy == nil || len(g.tidy) != 0 {
		t.Fatalf("NLI 길의 정리 필요는 빈 배열이어야 한다 : %+v", g.tidy)
	}
	if g.warn[0].ID != "warn" || g.amb[0].ID != "amb" {
		t.Fatalf("통이 다르다 : %+v · %+v", g.warn, g.amb)
	}
}

// 쪼개묻기 설계 결정 6·7 : nli 길 기본 켬 · 이긴 토막이 경고 줄 split 칸과 사람용 출력에 · --no-split 은 원래 꼴만 · last.json 칸.
func TestConflictNLISplitDefaultAndNoSplit(t *testing.T) {
	root := pairsRepo(t)
	var mu sync.Mutex
	asks := 0
	// 머리 없는 근거(= 토막)와 「7초」 주장일 때만 b .85 — 원래 꼴은 .6(애매)이라 토막이 이겨야 경고로 오른다.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			_, _ = w.Write([]byte(`{"ok":true,"model":"nli-test"}`))
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		asks++
		mu.Unlock()
		claim, _ := req["claim"].(string)
		ev, _ := req["evidence"].(string)
		switch {
		case strings.Contains(claim, "7초") && !strings.HasPrefix(ev, "["):
			_, _ = w.Write([]byte(`{"a":0.1,"b":0.85,"c":0.05,"model":"nli-test"}`))
		case strings.Contains(claim, "7초"):
			_, _ = w.Write([]byte(`{"a":0.3,"b":0.6,"c":0.1,"model":"nli-test"}`))
		default:
			_, _ = w.Write([]byte(`{"a":0.5,"b":0.05,"c":0.45,"model":"nli-test"}`))
		}
	}))
	defer srv.Close()
	setNLIConfig(t, root, srv.URL)
	out, code := runConflict(t, root, "--judge", "--json")
	got := decodeConflictJSON(t, out)
	if code != exitOK || !got.Split || got.Asks != 8 || len(got.Warn) != 1 || len(got.Ambiguous) != 0 {
		t.Fatalf("기본 켬이면 토막이 이겨 경고로 올라야 한다 (%d) : %+v", code, got)
	}
	sp := got.Warn[0].Split
	if sp == nil || sp.Best != 0 || sp.BaseB != 0.6 || len(sp.Pieces) != 1 || sp.Pieces[0].B != 0.85 || got.Warn[0].Prob != 0.85 {
		t.Fatalf("경고 줄 split 칸 = %+v", sp)
	}
	last := readLastJSON(t, root)
	if last["split"] != true || last["asks"] != float64(8) || last["split_capped"] != float64(0) {
		t.Fatalf("last.json 의 split·asks·split_capped : %v", last)
	}
	text, _ := runConflict(t, root, "--judge")
	if !strings.Contains(text, "│   b 낸 토막 : 「") || !strings.Contains(text, "(원래 꼴 B 0.60)") {
		t.Fatalf("사람용 출력에 이긴 토막 줄이 있어야 한다\n%s", text)
	}
	asks = 0
	out, _ = runConflict(t, root, "--judge", "--json", "--no-split")
	got = decodeConflictJSON(t, out)
	if got.Split || got.Asks != 4 || asks != 4 || len(got.Warn) != 0 || len(got.Ambiguous) != 1 || got.Ambiguous[0].Split != nil {
		t.Fatalf("--no-split 은 원래 꼴만 한 번 : 물음 %d : %+v", asks, got)
	}
	if strings.Contains(out, `"split":{`) {
		t.Fatalf("끈 판의 줄에는 split 칸이 없다\n%s", out)
	}
}

// --no-split 은 localharness 길에서 무시된다 (그 길은 쪼개 묻지 않는다).
func TestConflictNoSplitIgnoredOffNLI(t *testing.T) {
	root := pairsRepo(t)
	t.Setenv("PATH", "")
	out, code := runConflict(t, root, "--judge", "--json", "--no-split")
	if got := decodeConflictJSON(t, out); code != exitOK || got.Split || got.Asks != 0 {
		t.Fatalf("localharness 길은 split 이 꺼져 있어야 한다 (%d) : %+v", code, got)
	}
}
