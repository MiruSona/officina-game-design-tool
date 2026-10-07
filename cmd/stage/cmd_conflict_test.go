package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/conflict"
)

// fakeJudgeEnv 가 켜져 있으면 이 시험 실행 파일이 가짜 localharness 로 돈다.
// 실제 판정기·서버는 시험에서 절대 안 부른다.
const (
	fakeJudgeEnv     = "STAGE_FAKE_JUDGE"
	fakeJudgeMarkEnv = "STAGE_FAKE_JUDGE_MARK"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeJudgeEnv); mode != "" {
		os.Exit(fakeJudgeMain(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeJudgeMain 은 가짜 판정기다. 입력 jsonl 을 읽어 claim 글에 따라 정해진 글자·확률을 찍는다.
//   - ok   : 「않」 이 들면 B 0.92 · 「7초」 가 들면 B 0.60 · 나머지 A 0.88
//   - fail : 첫 줄만 찍고 JSON 아닌 줄 하나를 더 찍은 뒤 종료 3
//   - hang : 아무것도 안 찍고 30초 잔다
//   - skip : 설정이 없을 때처럼 「판정 건너뜀 : …」 한 줄만 찍고 종료 0 (JSON 아님)
//   - garbage : JSON 도 건너뜀도 아닌 줄 하나 찍고 종료 0
//
// STAGE_FAKE_JUDGE_MARK 에 경로가 있으면 불릴 때 그 파일을 만든다 — 「안 불렸다」 를 시험하는 표시다.
func fakeJudgeMain(mode string, args []string) int {
	if mark := os.Getenv(fakeJudgeMarkEnv); mark != "" {
		_ = os.WriteFile(mark, []byte("called\n"), 0o644)
	}
	if len(args) < 1 || args[0] != "judge" || !hasArgs(args, "--kind", "support") || !hasFlag(args, "--json") {
		fmt.Fprintln(os.Stderr, "가짜 판정기 : 인자 꼴이 다르다 :", args)
		return 2
	}
	file := argValue(args, "--file")
	f, err := os.Open(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "가짜 판정기 : 파일 없음", err)
		return 2
	}
	defer f.Close()
	if mode == "hang" {
		time.Sleep(30 * time.Second)
		return 0
	}
	if mode == "skip" {
		fmt.Println("판정 건너뜀 : llm.toml 이 없습니다 (어딘가/llm.toml)")
		return 0
	}
	if mode == "garbage" {
		fmt.Println("알 수 없는 줄")
		return 0
	}
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		var row conflict.Row
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			continue
		}
		fmt.Println(fakeVerdict(row))
		n++
		if mode == "fail" {
			fmt.Println("이건 JSON 이 아닌 줄이다")
			fmt.Fprintln(os.Stderr, "서버가 끊겼다")
			return 3
		}
	}
	fmt.Fprintf(os.Stderr, "판정 %d줄\n", n)
	return 0
}

func fakeVerdict(row conflict.Row) string {
	letter, b := "A", 0.05
	switch {
	case strings.Contains(row.Claim, "않"):
		letter, b = "B", 0.92
	case strings.Contains(row.Claim, "7초"):
		letter, b = "B", 0.60
	}
	a := 1 - b - 0.03
	raw, _ := json.Marshal(map[string]any{
		"id": row.ID, "letter": letter, "prob": map[bool]float64{true: b, false: a}[letter == "B"],
		"probs": map[string]float64{"A": a, "B": b, "C": 0.03}, "ms": 12,
	})
	return string(raw)
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

func hasArgs(args []string, name, value string) bool {
	return argValue(args, name) == value
}

func argValue(args []string, name string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

// installFakeJudge 는 이 시험 실행 파일을 localharness 이름으로 복사해 PATH 를 그 폴더 하나로 바꾼다.
func installFakeJudge(t *testing.T, mode string) string {
	t.Helper()
	dir := t.TempDir()
	copyExecutable(t, dir)
	t.Setenv("PATH", dir)
	t.Setenv(fakeJudgeEnv, mode)
	return dir
}

func copyExecutable(t *testing.T, dir string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Skipf("실행 파일 경로를 모른다 : %v", err)
	}
	name := conflict.JudgeName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	src, err := os.Open(self)
	if err != nil {
		t.Fatalf("실행 파일을 못 연다 : %v", err)
	}
	defer src.Close()
	dst, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("가짜 판정기를 못 만든다 : %v", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatalf("복사 실패 : %v", err)
	}
	_ = dst.Close()
}

// pairsRepo 는 testdata 의 예시 저장소를 임시 폴더로 베낀다. 시험은 사본에만 쓴다.
func pairsRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join("..", "..", "testdata", "conflict", "repo-pairs")
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		writeFile(t, filepath.Join(root, rel), string(raw))
		return nil
	})
	if err != nil {
		t.Fatalf("예시 저장소 베끼기 실패 : %v", err)
	}
	return root
}

func runConflict(t *testing.T, root string, args ...string) (string, int) {
	t.Helper()
	code := 0
	out := captureOut(t, func() {
		code = run(append([]string{"conflict", "--root", root}, args...))
	})
	return out, code
}

func TestConflictPairsOnlyJSONPrintsFourFieldRows(t *testing.T) {
	root := pairsRepo(t)
	out, code := runConflict(t, root, "--pairs-only", "--json")
	if code != exitOK {
		t.Fatalf("종료 %d, 0 이어야 한다\n%s", code, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("후보 쌍 4줄이어야 한다 (지금 %d)\n%s", len(lines), out)
	}
	ids := []string{}
	for _, ln := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatalf("jsonl 이 아니다 : %s", ln)
		}
		if len(m) != 4 || m["id"] == nil || m["evidence"] == nil || m["claim"] == nil || m["want"] == nil {
			t.Fatalf("네 칸(id·evidence·claim·want)이어야 한다 : %s", ln)
		}
		ids = append(ids, m["id"].(string))
	}
	joined := strings.Join(ids, " ")
	if !strings.Contains(joined, "00-바탕.md:7|01-자세.md:3") || !strings.Contains(joined, "00-바탕.md:8|01-자세.md:4") {
		t.Fatalf("id 꼴·줄 번호가 다르다 : %v", ids)
	}
	if strings.Contains(out, "example.invalid") {
		t.Fatal("비밀 꼴 문장이 stdout 에 나오면 안 된다")
	}
	rec := filepath.Join(root, ".gamedesign", "conflict")
	entries, err := os.ReadDir(rec)
	if err != nil || len(entries) < 2 {
		t.Fatalf("기록 폴더에 보낸 jsonl 과 last.json 이 있어야 한다 : %v %v", err, entries)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("tmp 파일이 남으면 안 된다 : %s", e.Name())
		}
	}
	last, _ := os.ReadFile(filepath.Join(rec, "last.json"))
	if !strings.Contains(string(last), `"02-수치.md:6"`) {
		t.Fatalf("비밀 꼴로 뺀 자리는 last.json 에 줄 번호로만 남는다 : %s", last)
	}
}

func TestConflictSkipsWhenNoJudgeOnPath(t *testing.T) {
	root := pairsRepo(t)
	t.Setenv("PATH", "")
	out, code := runConflict(t, root)
	if code != exitOK || !strings.Contains(out, "판정 건너뜀") || !strings.Contains(out, "후보 쌍 4개") {
		t.Fatalf("PATH 가 비면 건너뜀 + 종료 0 이어야 한다 (%d)\n%s", code, out)
	}
	if !strings.Contains(out, "비밀 꼴로 뺀 문장 1") {
		t.Fatalf("비밀 꼴로 뺀 개수를 찍어야 한다\n%s", out)
	}
}

func TestConflictRefusesShadowJudge(t *testing.T) {
	root := pairsRepo(t)
	first := installFakeJudge(t, "ok")
	second := t.TempDir()
	copyExecutable(t, second)
	t.Setenv("PATH", first+string(os.PathListSeparator)+second)
	out, code := runConflict(t, root)
	if code != exitOK || !strings.Contains(out, "PATH 에 localharness 가 2개") {
		t.Fatalf("같은 이름이 둘이면 거절해야 한다 (%d)\n%s", code, out)
	}
}

func TestConflictConfigOffSkipsBeforeLookup(t *testing.T) {
	root := pairsRepo(t)
	installFakeJudge(t, "ok")
	writeFile(t, filepath.Join(root, "Docs", "Todo", "기획설정.json"), `{"어긋남판정": false}`)
	out, code := runConflict(t, root)
	if code != exitOK || !strings.Contains(out, "설정으로 꺼짐") {
		t.Fatalf("설정이 꺼져 있으면 판정기를 안 불러야 한다 (%d)\n%s", code, out)
	}
	writeFile(t, filepath.Join(root, "Docs", "Todo", "기획설정.json"), `{"어긋남판정": "예"}`)
	if _, code := runConflict(t, root); code != exitCorrupt {
		t.Fatalf("모르는 값은 즉시 실패(종료 %d)여야 한다 (지금 %d)", exitCorrupt, code)
	}
}

func TestConflictFakeJudgeGradesWarnAndAmbiguous(t *testing.T) {
	root := pairsRepo(t)
	installFakeJudge(t, "ok")
	out, code := runConflict(t, root, "--json")
	if code != exitOK {
		t.Fatalf("경고가 있어도 종료 0 이어야 한다 (%d)\n%s", code, out)
	}
	var got conflictJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("--json 이 JSON 이 아니다 : %v\n%s", err, out)
	}
	if got.Pairs != 4 || got.Judged != 4 || got.Skipped != "" || got.SecretDropped != 1 {
		t.Fatalf("요약이 다르다 : %+v", got)
	}
	if len(got.Warn) != 1 || got.Warn[0].ID != "00-바탕.md:8|01-자세.md:4" || got.Warn[0].Prob != 0.92 {
		t.Fatalf("경고 = %+v", got.Warn)
	}
	if !strings.HasPrefix(got.Warn[0].A, "[절 : 규칙 · 줄 머리 : 창문 · 칸 머리 : 설명] ") {
		t.Fatalf("evidence 앞에 Ctx 가 붙어야 한다 : %q", got.Warn[0].A)
	}
	if len(got.Ambiguous) != 1 || got.Ambiguous[0].ID != "00-바탕.md:7|01-자세.md:3" {
		t.Fatalf("애매 = %+v", got.Ambiguous)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(got.Sent))); err != nil {
		t.Fatalf("보낸 파일이 있어야 한다 : %v", err)
	}
	result := strings.TrimSuffix(got.Sent, ".jsonl") + ".result.jsonl"
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(result))); err != nil {
		t.Fatalf("결과 파일이 있어야 한다 : %v", err)
	}
	// 사람용 출력도 한 번 본다.
	text, _ := runConflict(t, root)
	if !strings.Contains(text, "경고 1 · 애매 1 · 조용 2") || !strings.Contains(text, "B 0.92") {
		t.Fatalf("사람용 표가 다르다\n%s", text)
	}
}

func TestConflictJudgeFailureStillUsesReadLines(t *testing.T) {
	root := pairsRepo(t)
	installFakeJudge(t, "fail")
	out, code := runConflict(t, root, "--json")
	if code != exitOK {
		t.Fatalf("판정기 실패도 종료 0 이어야 한다 (%d)\n%s", code, out)
	}
	var got conflictJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("JSON 아님 : %v\n%s", err, out)
	}
	if got.Judged != 1 || got.Unread != 1 {
		t.Fatalf("읽힌 1줄은 쓰고 못 읽은 1줄은 세야 한다 : %+v", got)
	}
	text, _ := runConflict(t, root)
	if !strings.Contains(text, "⚠ 판정기") || !strings.Contains(text, "서버가 끊겼다") {
		t.Fatalf("판정기 종료 까닭을 찍어야 한다\n%s", text)
	}
}

func TestConflictJudgeTimeoutReportsFailure(t *testing.T) {
	root := pairsRepo(t)
	installFakeJudge(t, "hang")
	old := judgeWait
	judgeWait = func(int) time.Duration { return 500 * time.Millisecond }
	defer func() { judgeWait = old }()
	out, code := runConflict(t, root)
	if code != exitOK || !strings.Contains(out, "판정 실패") || !strings.Contains(out, "안 끝났습니다") {
		t.Fatalf("제한 시간을 넘기면 판정 실패 + 종료 0 이어야 한다 (%d)\n%s", code, out)
	}
}

func TestConflictOutOutsideRootRefused(t *testing.T) {
	root := pairsRepo(t)
	if _, code := runConflict(t, root, "--pairs-only", "--out", filepath.Join(root, "..", "밖.jsonl")); code != exitUsage {
		t.Fatalf("--out 이 뿌리 밖이면 종료 %d 여야 한다 (지금 %d)", exitUsage, code)
	}
	out, code := runConflict(t, root, "--pairs-only", "--out", "안.jsonl")
	if code != exitOK {
		t.Fatalf("뿌리 아래 --out 은 돼야 한다 (%d)\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, "안.jsonl")); err != nil {
		t.Fatalf("--out 파일이 있어야 한다 : %v", err)
	}
}

func TestConflictChangedOnlyPairsChangedDoc(t *testing.T) {
	root := newGitRepo(t)
	for _, name := range []string{"00-바탕.md", "01-자세.md", "02-수치.md"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "conflict", "repo-pairs", "Docs", "Design", name))
		if err != nil {
			t.Fatalf("예시 문서 읽기 실패 : %v", err)
		}
		writeFile(t, filepath.Join(root, "Docs", "Design", name), string(raw))
	}
	gitDo(t, root, "add", ".")
	gitDo(t, root, "commit", "-q", "-m", "처음")
	out, code := runConflict(t, root, "--changed", "--pairs-only", "--json")
	var got conflictJSON
	if code != exitOK || json.Unmarshal([]byte(strings.TrimSpace(out)), &got) != nil {
		t.Fatalf("깨끗하면 종료 0 + --json 요약 한 줄이어야 한다 (%d)\n%s", code, out)
	}
	if got.Pairs != 0 || !strings.Contains(got.Skipped, "바뀐 문서가 없습니다") {
		t.Fatalf("요약 = %+v", got)
	}
	if out, _ := runConflict(t, root, "--changed", "--pairs-only"); strings.TrimSpace(out) != "" {
		t.Fatalf("--json 이 아니면 stdout 은 비어야 한다(안내는 stderr)\n%s", out)
	}
	writeFile(t, filepath.Join(root, "Docs", "Design", "01-자세.md"), "# 자세\n\n대기 시간은 7초로 늘린다.\n")
	out, code = runConflict(t, root, "--changed", "--pairs-only", "--json")
	if code != exitOK {
		t.Fatalf("종료 %d\n%s", code, out)
	}
	for _, ln := range strings.Split(strings.TrimSpace(out), "\n") {
		if !strings.Contains(ln, "01-자세.md") {
			t.Fatalf("바뀐 문서가 안 낀 짝이 있다 : %s", ln)
		}
	}
}

func TestConflictNoPairsNeverCallsJudgeNorWrites(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Docs", "Design", "00-하나.md"), "# 하나\n\n대기 시간은 3초로 둔다.\n")
	installFakeJudge(t, "ok")
	mark := filepath.Join(t.TempDir(), "called")
	t.Setenv(fakeJudgeMarkEnv, mark)
	out, code := runConflict(t, root, "--json")
	if code != exitOK {
		t.Fatalf("종료 %d\n%s", code, out)
	}
	var got conflictJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil || got.Pairs != 0 || !strings.Contains(got.Skipped, "후보 0쌍") {
		t.Fatalf("후보 0쌍 요약이어야 한다 : %v %+v", err, got)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Fatal("쌍이 없으면 판정기를 부르면 안 된다")
	}
	if _, err := os.Stat(filepath.Join(root, ".gamedesign")); err == nil {
		t.Fatal("쌍이 없으면 기록도 안 남겨야 한다")
	}
}

// 실제 judge 는 설정이 없으면 stdout 「판정 건너뜀 : llm.toml 이 없습니다 (<경로>)」 + 종료 0 이다 (2026-10-08 통합 실측).
func TestConflictJudgeSkipLineIsSkipNotFailure(t *testing.T) {
	root := pairsRepo(t)
	installFakeJudge(t, "skip")
	out, code := runConflict(t, root)
	if code != exitOK || !strings.HasPrefix(out, "판정 건너뜀 — llm.toml 이 없습니다 (어딘가/llm.toml) · 후보 쌍 4개") {
		t.Fatalf("건너뜀 줄은 「판정 건너뜀 — <까닭>」 머리글이어야 한다 (%d)\n%s", code, out)
	}
	if strings.Contains(out, "판정 실패") {
		t.Fatalf("건너뜀을 실패로 찍으면 안 된다\n%s", out)
	}
	rec := filepath.Join(root, ".gamedesign", "conflict")
	last, _ := os.ReadFile(filepath.Join(rec, "last.json"))
	for _, want := range []string{`"skipped": "llm.toml 이 없습니다 (어딘가/llm.toml)"`, `"judge_error": ""`, `"result": ""`} {
		if !strings.Contains(string(last), want) {
			t.Fatalf("last.json 에 %s 가 있어야 한다 : %s", want, last)
		}
	}
	entries, _ := os.ReadDir(rec)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".result.") {
			t.Fatalf("읽힌 판정이 0 이면 결과 파일을 안 만든다 : %s", e.Name())
		}
	}
	// --json 도 같은 뜻이다.
	out, _ = runConflict(t, root, "--json")
	var got conflictJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil || got.Skipped != "llm.toml 이 없습니다 (어딘가/llm.toml)" || got.Judged != 0 {
		t.Fatalf("--json skipped 가 다르다 : %v %+v", err, got)
	}
}

func TestConflictGarbageStdoutIsFailureWithoutResultFile(t *testing.T) {
	root := pairsRepo(t)
	installFakeJudge(t, "garbage")
	out, code := runConflict(t, root)
	if code != exitOK || !strings.Contains(out, "판정 실패 — 판정기 출력 : 알 수 없는 줄") {
		t.Fatalf("JSON 도 건너뜀도 아닌 줄은 실패 까닭이다 (%d)\n%s", code, out)
	}
	rec := filepath.Join(root, ".gamedesign", "conflict")
	last, _ := os.ReadFile(filepath.Join(rec, "last.json"))
	if !strings.Contains(string(last), `"judge_error": "판정기 출력 : 알 수 없는 줄`) || !strings.Contains(string(last), `"result": ""`) {
		t.Fatalf("judge_error 는 차고 result 는 비어야 한다 : %s", last)
	}
	entries, _ := os.ReadDir(rec)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".result.") {
			t.Fatalf("읽힌 판정이 0 이면 결과 파일을 안 만든다 : %s", e.Name())
		}
	}
}

func TestConflictResultFileHoldsOnlyJSONLines(t *testing.T) {
	root := pairsRepo(t)
	installFakeJudge(t, "fail")
	out, _ := runConflict(t, root, "--json")
	var got conflictJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
		t.Fatalf("JSON 아님 : %v\n%s", err, out)
	}
	result := strings.TrimSuffix(got.Sent, ".jsonl") + ".result.jsonl"
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result)))
	if err != nil {
		t.Fatalf("읽힌 판정이 1 이면 결과 파일이 있어야 한다 : %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "{") {
		t.Fatalf("결과 파일은 읽힌 JSON 줄만 담는다 : %q", raw)
	}
}

func TestConflictSkippedRunHasNoResultInLast(t *testing.T) {
	root := pairsRepo(t)
	t.Setenv("PATH", "")
	runConflict(t, root)
	last, _ := os.ReadFile(filepath.Join(root, ".gamedesign", "conflict", "last.json"))
	if !strings.Contains(string(last), `"result": ""`) {
		t.Fatalf("판정기를 안 돌린 판은 result 가 비어야 한다 : %s", last)
	}
}

func TestConflictTwoRunsSameSecondKeepBothRecords(t *testing.T) {
	root := pairsRepo(t)
	t.Setenv("PATH", "")
	runConflict(t, root, "--pairs-only")
	runConflict(t, root, "--pairs-only")
	entries, _ := os.ReadDir(filepath.Join(root, ".gamedesign", "conflict"))
	sent := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") && !strings.Contains(e.Name(), ".result.") {
			sent++
		}
	}
	if sent != 2 {
		t.Fatalf("두 판이면 보낸 파일이 둘이어야 한다 (지금 %d) : %v", sent, entries)
	}
}

func TestConflictOutMustBeJSONLAndNotADesignDoc(t *testing.T) {
	root := pairsRepo(t)
	doc := filepath.Join("Docs", "Design", "00-바탕.md")
	before, _ := os.ReadFile(filepath.Join(root, doc))
	if _, code := runConflict(t, root, "--pairs-only", "--out", doc); code != exitUsage {
		t.Fatalf("--out 으로 기획 문서를 가리키면 종료 %d 여야 한다 (지금 %d)", exitUsage, code)
	}
	if _, code := runConflict(t, root, "--pairs-only", "--out", "메모.txt"); code != exitUsage {
		t.Fatalf("--out 은 .jsonl 이어야 한다 (지금 %d)", code)
	}
	after, _ := os.ReadFile(filepath.Join(root, doc))
	if string(before) != string(after) {
		t.Fatal("기획 문서가 바뀌면 안 된다")
	}
	if _, code := runConflict(t, root, "--pairs-only", "--out", filepath.Join("새폴더", "안.jsonl")); code != exitOK {
		t.Fatalf("아직 없는 폴더 아래 .jsonl 은 돼야 한다 (지금 %d)", code)
	}
}

func TestConflictRefusesExeAndCmdInSameFolder(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PATHEXT 는 Windows 몫이다")
	}
	root := pairsRepo(t)
	dir := installFakeJudge(t, "ok")
	writeFile(t, filepath.Join(dir, conflict.JudgeName+".cmd"), "@echo off\r\n")
	out, code := runConflict(t, root)
	if code != exitOK || !strings.Contains(out, "PATH 에 localharness 가 2개") {
		t.Fatalf("같은 폴더의 .exe 와 .cmd 도 둘로 세어 거절해야 한다 (%d)\n%s", code, out)
	}
}

func TestConflictLimitCutsAndSaysSo(t *testing.T) {
	root := pairsRepo(t)
	t.Setenv("PATH", "")
	out, code := runConflict(t, root, "--limit", "1")
	if code != exitOK || !strings.Contains(out, "후보 쌍 1개") {
		t.Fatalf("--limit 1 이면 쌍 1개여야 한다 (%d)\n%s", code, out)
	}
	if _, code := runConflict(t, root, "--limit", "0"); code != exitUsage {
		t.Fatalf("--limit 0 은 쓰는 법 오류여야 한다 (지금 %d)", code)
	}
}
