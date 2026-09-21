package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/config"
	"github.com/mirusona/officina-game-design-tool/internal/state"
)

// writeState 는 임시 저장소에 상태 파일 내용을 그대로 쓴다.
func writeState(t *testing.T, root, body string) {
	t.Helper()
	path := state.Path(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("폴더 만들기 실패 : %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("상태 쓰기 실패 : %v", err)
	}
}

// exitCode 는 codedError 의 종료 코드를 꺼낸다. 없으면 -1 이다.
func exitCode(err error) int {
	ce, ok := err.(*codedError)
	if !ok {
		return -1
	}
	return ce.code
}

func TestInitRefusesBrokenState(t *testing.T) {
	root := t.TempDir()
	writeState(t, root, "{ 이건 JSON 이 아니다")
	err := cmdInit([]string{"--root", root})
	if got := exitCode(err); got != exitCorrupt {
		t.Fatalf("깨진 상태 파일이면 종료 %d 여야 한다 (지금 %d, %v)", exitCorrupt, got, err)
	}
	raw, readErr := os.ReadFile(state.Path(root))
	if readErr != nil {
		t.Fatalf("상태 파일이 없어졌다 : %v", readErr)
	}
	if string(raw) != "{ 이건 JSON 이 아니다" {
		t.Fatalf("깨진 상태 파일을 덮어썼다 : %s", raw)
	}
}

func TestInitRefusesUnknownFormat(t *testing.T) {
	root := t.TempDir()
	writeState(t, root, `{"형식": 99, "단계": 1, "판번호": 1}`)
	if got := exitCode(cmdInit([]string{"--root", root})); got != exitCorrupt {
		t.Fatalf("모르는 판이면 종료 %d 여야 한다 (지금 %d)", exitCorrupt, got)
	}
}

func TestInitForceOverwritesBrokenState(t *testing.T) {
	root := t.TempDir()
	writeState(t, root, "{ 이건 JSON 이 아니다")
	if err := cmdInit([]string{"--root", root, "--force"}); err != nil {
		t.Fatalf("--force 면 덮어써야 한다 : %v", err)
	}
	if _, err := state.Load(root); err != nil {
		t.Fatalf("덮어쓴 뒤에는 읽혀야 한다 : %v", err)
	}
}

func TestInitOnEmptyRepoWorks(t *testing.T) {
	root := t.TempDir()
	if err := cmdInit([]string{"--root", root}); err != nil {
		t.Fatalf("빈 저장소에서는 새로 만들어야 한다 : %v", err)
	}
}

// 판시작일은 로컬 시각으로 읽어야 파일 mtime 과 눈금이 맞는다 (KST 는 UTC 보다 9시간 이르다).
func TestRoundTodosUsesLocalRoundStart(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	now := time.Now()
	st := state.New(now)
	if _, err := st.AddAsk("a", "물음 하나", []string{"기준"}, now); err != nil {
		t.Fatalf("물음 더하기 실패 : %v", err)
	}
	if _, err := st.Check("a", "pass", "", now); err != nil {
		t.Fatalf("판정 실패 : %v", err)
	}
	// 오늘 새벽(로컬 00:30)에 고친 기록. UTC 로 읽으면 판시작일이 09:00 이 돼 「없음」이 된다.
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 30, 0, 0, time.Local)
	path := filepath.Join(root, filepath.FromSlash(historyDir), "기록.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("폴더 만들기 실패 : %v", err)
	}
	if err := os.WriteFile(path, []byte("# 기록\n"), 0o644); err != nil {
		t.Fatalf("기록 쓰기 실패 : %v", err)
	}
	if err := os.Chtimes(path, start, start); err != nil {
		t.Fatalf("시각 바꾸기 실패 : %v", err)
	}
	for _, m := range roundTodos(root, cfg, st) {
		if m.name == "플레이 기록 한 편" {
			t.Fatal("오늘 고친 기록이 있는데 없다고 본다 (판시작일을 UTC 로 읽었다)")
		}
	}
}

// 고리 뒤(9→8)로 되돌아가면 판시작일이 그대로라 옛 기록이 「다 했음」으로 읽혔다.
func TestRoundTodosUsesLastBackAfterLoop(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	now := time.Now()
	st := state.New(now)
	st.Stage = 9
	if _, err := st.AddAsk("a", "물음 하나", []string{"기준"}, now); err != nil {
		t.Fatalf("물음 더하기 실패 : %v", err)
	}
	if _, err := st.Check("a", "pass", "", now); err != nil {
		t.Fatalf("판정 실패 : %v", err)
	}
	// 판시작일 뒤·되돌아가기 전에 쓴 옛 기록. 되돌아간 뒤에는 「없음」으로 봐야 한다.
	old := time.Date(now.Year(), now.Month(), now.Day(), 1, 0, 0, 0, time.Local)
	path := filepath.Join(root, filepath.FromSlash(historyDir), "옛기록.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("폴더 만들기 실패 : %v", err)
	}
	if err := os.WriteFile(path, []byte("# 옛 기록\n"), 0o644); err != nil {
		t.Fatalf("기록 쓰기 실패 : %v", err)
	}
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("시각 바꾸기 실패 : %v", err)
	}
	if err := st.GoBack(8, "세로 조각을 다시 본다", cfg, old.Add(time.Hour)); err != nil {
		t.Fatalf("되돌아가기 실패 : %v", err)
	}
	found := false
	for _, m := range roundTodos(root, cfg, st) {
		if m.name == "플레이 기록 한 편" {
			found = true
		}
	}
	if !found {
		t.Fatal("되돌아간 뒤에는 그 전 기록을 「다 했음」으로 보면 안 된다")
	}
}

func TestHelpFlagExitsZero(t *testing.T) {
	if got := run([]string{"lint", "-h"}); got != exitOK {
		t.Fatalf("-h 는 종료 0 이어야 한다 (지금 %d)", got)
	}
}

func TestBadFlagExitsUsage(t *testing.T) {
	if got := run([]string{"lint", "--없는옵션"}); got != exitUsage {
		t.Fatalf("모르는 옵션은 종료 %d 여야 한다 (지금 %d)", exitUsage, got)
	}
}
