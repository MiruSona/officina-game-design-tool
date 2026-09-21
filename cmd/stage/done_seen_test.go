package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/state"
)

// useTempCache 는 상태 파일 자리를 시험용 폴더로 갈아 끼운다.
func useTempCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := cacheDir
	cacheDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { cacheDir = old })
	return dir
}

func TestDoneAlreadySentRules(t *testing.T) {
	useTempCache(t)
	root := t.TempDir()
	if doneAlreadySent(root, "sess-1", "할 일") {
		t.Fatal("처음에는 띄운다")
	}
	if !doneAlreadySent(root, "sess-1", "할 일") {
		t.Fatal("같은 세션·같은 내용은 두 번째부터 안 띄운다")
	}
	if doneAlreadySent(root, "sess-2", "할 일") {
		t.Fatal("세션이 바뀌면 다시 띄운다")
	}
	if doneAlreadySent(root, "sess-2", "다른 할 일") {
		t.Fatal("내용이 바뀌면 다시 띄운다")
	}
	if doneAlreadySent(root, "", "할 일") {
		t.Fatal("session_id 가 없으면 늘 띄운다")
	}
}

func TestDoneAlreadySentPrintsWhenCacheUnwritable(t *testing.T) {
	// 캐시 자리를 **파일**로 둔다. 그 아래에는 폴더를 못 만든다.
	blocked := filepath.Join(t.TempDir(), "막힌자리")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatalf("파일 쓰기 실패 : %v", err)
	}
	old := cacheDir
	cacheDir = func() (string, error) { return blocked, nil }
	t.Cleanup(func() { cacheDir = old })
	root := t.TempDir()
	if doneAlreadySent(root, "sess-1", "할 일") || doneAlreadySent(root, "sess-1", "할 일") {
		t.Fatal("캐시에 못 쓰면 늘 띄운다")
	}
}

// finishedRound 는 물음이 다 판정된 판 하나를 만든다. 판 끝 할 일이 뜨는 상태다.
func finishedRound(t *testing.T, root string) {
	t.Helper()
	now := time.Now()
	st := state.New(now)
	if _, err := st.AddAsk("a", "물음 하나", []string{"기준"}, now); err != nil {
		t.Fatalf("물음 더하기 실패 : %v", err)
	}
	if _, err := st.Check("a", "pass", "", now); err != nil {
		t.Fatalf("판정 실패 : %v", err)
	}
	if err := state.Save(root, st, now); err != nil {
		t.Fatalf("상태 쓰기 실패 : %v", err)
	}
}

func TestDoneHookRepeatsOnlyOncePerSession(t *testing.T) {
	useTempCache(t)
	root := t.TempDir()
	finishedRound(t, root)
	hookJSON := `{"hook_event_name":"Stop","session_id":"sess-9","cwd":"` + filepath.ToSlash(root) + `"}`
	first := runDoneHook(t, root, hookJSON)
	if !strings.Contains(first, "systemMessage") {
		t.Fatalf("첫 번째는 띄워야 한다 : %q", first)
	}
	second := runDoneHook(t, root, hookJSON)
	if strings.TrimSpace(second) != "" {
		t.Fatalf("같은 세션 두 번째는 아무것도 안 찍는다 : %q", second)
	}
	other := runDoneHook(t, root, strings.Replace(hookJSON, "sess-9", "sess-10", 1))
	if !strings.Contains(other, "systemMessage") {
		t.Fatalf("세션이 바뀌면 다시 띄운다 : %q", other)
	}
}

// runDoneHook 은 `stage done --hook` 을 한 번 돌리고 stdout 을 돌려준다.
func runDoneHook(t *testing.T, root, hookJSON string) string {
	t.Helper()
	inPath := filepath.Join(t.TempDir(), "hook.json")
	if err := os.WriteFile(inPath, []byte(hookJSON), 0o644); err != nil {
		t.Fatalf("훅 입력 쓰기 실패 : %v", err)
	}
	in, err := os.Open(inPath)
	if err != nil {
		t.Fatalf("훅 입력 열기 실패 : %v", err)
	}
	defer in.Close()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("파이프 실패 : %v", err)
	}
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = in, w
	err = cmdDone([]string{"--hook", "--root", root})
	os.Stdin, os.Stdout = oldIn, oldOut
	w.Close()
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	r.Close()
	if err != nil {
		t.Fatalf("done 실패 : %v", err)
	}
	return string(buf[:n])
}
