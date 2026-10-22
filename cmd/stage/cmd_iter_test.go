package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/config"
	"github.com/mirusona/officina-game-design-tool/internal/state"
)

const iterRel = "Docs/History/반복"

// iterRoot 는 마지막 단계(9)에 있는 임시 저장소를 만든다. iters 가 있으면 상태 파일에 넣는다.
func iterRoot(t *testing.T, stage int, iters ...state.Iter) string {
	t.Helper()
	root := t.TempDir()
	st := state.New(time.Now())
	st.Stage = stage
	st.Iters = append(st.Iters, iters...)
	if err := state.Save(root, st, time.Now()); err != nil {
		t.Fatalf("상태 쓰기 실패 : %v", err)
	}
	return root
}

// putFile 은 뿌리 기준 자리에 파일을 쓴다.
func putFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("폴더 만들기 실패 : %v", err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("파일 쓰기 실패 : %v", err)
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("파일 읽기 실패 (%s) : %v", rel, err)
	}
	return string(raw)
}

func loadSt(t *testing.T, root string) *state.State {
	t.Helper()
	st, err := state.Load(root)
	if err != nil {
		t.Fatalf("상태 읽기 실패 : %v", err)
	}
	return st
}

func today() string { return time.Now().Format("2006-01-02") }

// I1 : 마지막 단계가 아니면 거절하고 아무것도 안 바꾼다.
func TestIterNewRefusesBeforeLastStage(t *testing.T) {
	root := iterRoot(t, 8)
	before := readFile(t, root, state.RelPath)
	if got := exitCode(cmdIter([]string{"new", "--root", root})); got != exitUsage {
		t.Fatalf("종료 %d 여야 한다 (지금 %d)", exitUsage, got)
	}
	if readFile(t, root, state.RelPath) != before {
		t.Fatal("상태 파일이 바뀌었다")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(iterRel))); err == nil {
		t.Fatal("기록 폴더를 만들면 안 된다")
	}
}

func TestIterWithoutSubcommandIsUsage(t *testing.T) {
	if got := exitCode(cmdIter(nil)); got != exitUsage {
		t.Fatalf("iter 만 치면 종료 1 (지금 %d)", got)
	}
}

// I2 : 손 기록 01~09 가 있고 상태에 반복이 없으면 10 을 낸다. 00 은 셈에서 뺀다.
func TestIterNewContinuesFolderNumbers(t *testing.T) {
	root := iterRoot(t, 9)
	putFile(t, root, iterRel+"/00-틀.md", "# 틀\n")
	putFile(t, root, iterRel+"/01-2026-09-01.md", "# 01\n")
	putFile(t, root, iterRel+"/09-2026-10-01.md", "# 09\n")
	if err := cmdIter([]string{"new", "--root", root}); err != nil {
		t.Fatalf("iter new 실패 : %v", err)
	}
	rel := iterRel + "/10-" + today() + ".md"
	body := readFile(t, root, rel)
	if !strings.Contains(body, "# 반복 10") {
		t.Fatalf("번호가 안 채워졌다 : %s", body)
	}
	st := loadSt(t, root)
	if len(st.Iters) != 1 || st.Iters[0].No != 10 || st.Iters[0].File != rel || st.Format != state.FormatIter {
		t.Fatalf("상태 반복 줄이 틀렸다 : %+v (형식 %d)", st.Iters, st.Format)
	}
}

// I3 : 상태 마지막 회가 폴더보다 크면 상태를 따른다.
func TestIterNewUsesStateWhenLarger(t *testing.T) {
	root := iterRoot(t, 9, state.Iter{No: 12, Start: "2026-10-01", Played: "v1", File: iterRel + "/12-2026-10-01.md"})
	putFile(t, root, iterRel+"/09-2026-09-01.md", "# 09\n")
	if err := cmdIter([]string{"new", "--root", root}); err != nil {
		t.Fatalf("iter new 실패 : %v", err)
	}
	if st := loadSt(t, root); st.LastIter().No != 13 {
		t.Fatalf("13 이어야 한다 : %+v", st.Iters)
	}
}

// I4 : 같은 이름이 있으면 덮지 않는다.
func TestIterNewRefusesExistingFile(t *testing.T) {
	root := iterRoot(t, 9)
	// 번호 셈은 .md 파일만 보므로, 같은 이름의 폴더로 다음 자리(01)를 미리 막아 둔다.
	taken := filepath.Join(root, filepath.FromSlash(iterRel), "01-"+today()+".md")
	if err := os.MkdirAll(taken, 0o755); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, root, state.RelPath)
	if got := exitCode(cmdIter([]string{"new", "--root", root})); got != exitUsage {
		t.Fatalf("같은 이름이면 종료 %d (지금 %d)", exitUsage, got)
	}
	if readFile(t, root, state.RelPath) != before {
		t.Fatal("상태 파일이 바뀌었다")
	}
	if info, err := os.Stat(taken); err != nil || !info.IsDir() {
		t.Fatal("있던 것을 덮었다")
	}
	// 같은 이름 보통 파일 : 번호 셈이 늘 비켜 가므로 writeNew 를 바로 부른다.
	plain := filepath.Join(t.TempDir(), "02-x.md")
	putFile(t, filepath.Dir(plain), "02-x.md", "손 기록\n")
	if got := exitCode(writeNew(plain, "새 글")); got != exitUsage || readFile(t, filepath.Dir(plain), "02-x.md") != "손 기록\n" {
		t.Fatalf("보통 파일도 종료 1 · 안 덮음이어야 한다 (지금 %d)", got)
	}
}

// I5 : 지난 「미룬 것」 목록 줄과 들여 쓴 줄을 베끼고, 틀 자리표는 뺀다.
func TestIterNewCarriesDeferred(t *testing.T) {
	root := iterRoot(t, 9)
	prev := "# 반복 03\n\n## 미룬 것\n\n- 하나\n  - 하나 밑 설명\n- 둘\n- 셋\n- (피드백 번호) — 왜 미뤘나\n\n## 넘긴 것\n\n- 넘긴 것\n"
	putFile(t, root, iterRel+"/03-2026-10-01.md", prev)
	if err := cmdIter([]string{"new", "--root", root}); err != nil {
		t.Fatalf("iter new 실패 : %v", err)
	}
	body := readFile(t, root, iterRel+"/04-"+today()+".md")
	carried := strings.Split(body, "## 피드백")[0]
	for _, want := range []string{"원문 : 반복 03 기록 (" + iterRel + "/03-2026-10-01.md)", "- 하나\n  - 하나 밑 설명\n- 둘\n- 셋"} {
		if !strings.Contains(carried, want) {
			t.Fatalf("옮긴 절에 %q 가 없다 :\n%s", want, carried)
		}
	}
	if strings.Contains(carried, "(피드백 번호)") || strings.Contains(carried, "넘긴 것\n") {
		t.Fatalf("자리표·다른 절이 따라왔다 :\n%s", carried)
	}
}

// I6 : 지난 「미룬 것」 이 비면 「- 없음」.
func TestIterNewCarriesNone(t *testing.T) {
	root := iterRoot(t, 9)
	putFile(t, root, iterRel+"/02-2026-10-01.md", "## 미룬 것\n\n## 넘긴 것\n- x\n")
	if err := cmdIter([]string{"new", "--root", root}); err != nil {
		t.Fatalf("iter new 실패 : %v", err)
	}
	body := readFile(t, root, iterRel+"/03-"+today()+".md")
	if !strings.Contains(body, "## 지난 반복에서 미룬 것\n\n- 없음\n") || strings.Contains(body, "원문 :") {
		t.Fatalf("「- 없음」 한 줄이어야 한다 :\n%s", body)
	}
}

// I7 : 게임 틀에 {{지난미룬것}} 이 없으면 「## 피드백」 위에, 그것도 없으면 끝에 절을 끼운다.
func TestIterNewInsertsCarryIntoGameTemplate(t *testing.T) {
	cases := []struct {
		name  string
		tpl   string
		check func(body string) bool
	}{
		{"피드백 위", "# 반복 {{회}} {x}\n\n## 피드백\n\n## 고친 것\n", func(b string) bool {
			c, f := strings.Index(b, "## 지난 반복에서 미룬 것"), strings.Index(b, "## 피드백")
			return c >= 0 && c < f && strings.Contains(b, "# 반복 01 {x}")
		}},
		{"끝에", "# 반복 {{회}}\n\n## 고친 것\n", func(b string) bool {
			return strings.HasSuffix(strings.TrimSpace(b), "## 지난 반복에서 미룬 것\n\n- 없음")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := iterRoot(t, 9)
			putFile(t, root, "Docs/틀.md", c.tpl)
			putFile(t, root, config.RelPath, `{"반복틀": "Docs/틀.md"}`)
			if err := cmdIter([]string{"new", "--root", root}); err != nil {
				t.Fatalf("iter new 실패 : %v", err)
			}
			if body := readFile(t, root, iterRel+"/01-"+today()+".md"); !c.check(body) {
				t.Fatalf("절 자리가 틀렸다 :\n%s", body)
			}
		})
	}
}

// I8 : 설정 경로가 뿌리 밖이면 종료 1.
func TestIterNewRefusesPathsOutsideRoot(t *testing.T) {
	outside := filepath.ToSlash(filepath.Join(t.TempDir(), "틀.md"))
	for _, body := range []string{
		`{"반복틀": "../틀.md"}`,
		`{"반복틀": "` + outside + `"}`,
		`{"반복기록폴더": "../반복"}`,
		`{"반복기록폴더": "` + filepath.ToSlash(t.TempDir()) + `"}`,
	} {
		root := iterRoot(t, 9)
		putFile(t, root, config.RelPath, body)
		if got := exitCode(cmdIter([]string{"new", "--root", root})); got != exitUsage {
			t.Fatalf("%s → 종료 %d 여야 한다 (지금 %d)", body, exitUsage, got)
		}
	}
}

// I9 : --played 가 없으면 지난 올린판 → 지난 놀았던판 → ? 차례다.
func TestIterNewDefaultPlayed(t *testing.T) {
	cases := []struct {
		prev []state.Iter
		want string
	}{
		{[]state.Iter{{No: 1, Start: "2026-10-01", Played: "v1", Shipped: "v2", File: iterRel + "/01.md"}}, "v2"},
		{[]state.Iter{{No: 1, Start: "2026-10-01", Played: "v1", File: iterRel + "/01.md"}}, "v1"},
		{nil, "?"},
	}
	for _, c := range cases {
		root := iterRoot(t, 9, c.prev...)
		if err := cmdIter([]string{"new", "--root", root}); err != nil {
			t.Fatalf("iter new 실패 : %v", err)
		}
		if got := loadSt(t, root).LastIter().Played; got != c.want {
			t.Fatalf("놀았던 판 = %q, %q 여야 한다", got, c.want)
		}
	}
}

// I10 : --played 를 주면 지난 반복의 빈 올린판을 채운다.
func TestIterNewFillsPrevShipped(t *testing.T) {
	root := iterRoot(t, 9, state.Iter{No: 1, Start: "2026-10-01", Played: "v16", File: iterRel + "/01.md"})
	if err := cmdIter([]string{"new", "--played", "v17", "--title", "메인 화면", "--root", root}); err != nil {
		t.Fatalf("iter new 실패 : %v", err)
	}
	st := loadSt(t, root)
	if st.Iters[0].Shipped != "v17" || st.Iters[1].Played != "v17" || st.Iters[1].Title != "메인 화면" {
		t.Fatalf("올린판·놀았던판이 틀렸다 : %+v", st.Iters)
	}
}

// I11 : 상태 저장이 실패하면 종료 4 이고 방금 쓴 기록 파일을 지운다.
func TestIterNewRollsBackOnSaveFailure(t *testing.T) {
	root := iterRoot(t, 9)
	// 읽은 뒤 저장 직전에 상태 파일 자리를 같은 이름 폴더로 바꿔 진짜 Save 가 실패하게 한다 (POSIX 에서도 깨진다).
	saveIterState = func(dir string, st *state.State, now time.Time) error {
		if err := os.Remove(state.Path(dir)); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(state.Path(dir), "막음"), 0o755); err != nil {
			t.Fatal(err)
		}
		return saveState(dir, st, now)
	}
	t.Cleanup(func() { saveIterState = saveState })
	if got := exitCode(cmdIter([]string{"new", "--root", root})); got != exitWrite {
		t.Fatalf("종료 %d 여야 한다 (지금 %d)", exitWrite, got)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(iterRel), "01-"+today()+".md")); err == nil {
		t.Fatal("기록 파일이 남았다")
	}
}

// H1 : 반복이 있으면 show 에 반복 줄과 Won't 줄이 나오고 ask 안내는 없다.
func TestShowIterLine(t *testing.T) {
	root := iterRoot(t, 9, state.Iter{No: 10, Start: "2026-10-22", Played: "v16", File: iterRel + "/10-2026-10-22.md"})
	out := captureOut(t, func() {
		if err := cmdShow([]string{"--root", root}); err != nil {
			t.Fatalf("show 실패 : %v", err)
		}
	})
	for _, want := range []string{"반복 10 · 2026-10-22 부터 · 놀았던 판 v16 · " + iterRel + "/10-2026-10-22.md", "Won't 표에 대 보고"} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q 가 없다 :\n%s", want, out)
		}
	}
	if strings.Contains(out, "stage ask") || strings.Contains(out, "되돌아가는 조건") {
		t.Fatalf("반복 중에는 물음 안내·되돌아가는 조건이 없어야 한다 :\n%s", out)
	}
}

// H2 : show --json 에 반복 배열과 지금반복이 나온다.
func TestShowJSONIter(t *testing.T) {
	root := iterRoot(t, 9, state.Iter{No: 10, Start: "2026-10-22", Played: "v16", File: iterRel + "/10-2026-10-22.md"})
	out := captureOut(t, func() {
		if err := cmdShow([]string{"--json", "--root", root}); err != nil {
			t.Fatalf("show 실패 : %v", err)
		}
	})
	var got struct {
		Iters []state.Iter `json:"반복"`
		Now   int          `json:"지금반복"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("JSON 이 아니다 : %v\n%s", err, out)
	}
	if len(got.Iters) != 1 || got.Now != 10 {
		t.Fatalf("반복·지금반복이 틀렸다 : %+v", got)
	}
}

// 리뷰 6 : --title · --played 에 줄바꿈이 있으면 종료 1.
func TestIterNewRefusesNewlineArgs(t *testing.T) {
	root := iterRoot(t, 9)
	for _, args := range [][]string{{"--title", "한 줄\n둘째 줄"}, {"--played", "v1\r"}} {
		if got := exitCode(cmdIter(append([]string{"new", "--root", root}, args...))); got != exitUsage {
			t.Fatalf("%v → 종료 %d 여야 한다 (지금 %d)", args, exitUsage, got)
		}
	}
}

// B : --title 이 비면 머리에 꼬리가 없고 인용 줄도 없다.
func TestIterNewEmptyTitleHead(t *testing.T) {
	root := iterRoot(t, 9)
	if err := cmdIter([]string{"new", "--played", "v3", "--root", root}); err != nil {
		t.Fatalf("iter new 실패 : %v", err)
	}
	body := readFile(t, root, iterRel+"/01-"+today()+".md")
	if !strings.HasPrefix(body, "# 반복 01 — "+today()+" (v3)\n\n| 회 |") {
		t.Fatalf("머리 꼴이 틀렸다 :\n%s", body)
	}
}
