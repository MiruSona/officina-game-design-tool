package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirusona/officina-game-design-tool/internal/config"
	"github.com/mirusona/officina-game-design-tool/internal/gitchanged"
)

// captureOut 은 fn 이 stdout 에 찍은 것을 모은다.
func captureOut(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("파이프 실패 : %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	raw, _ := io.ReadAll(r)
	return string(raw)
}

// gitDo 는 시험 저장소에서 git 한 번을 돌린다.
func gitDo(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v 실패 : %v %s", args, err, out)
	}
}

// writeFile 은 시험 저장소에 파일 하나를 쓴다.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("폴더 만들기 실패 : %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("파일 쓰기 실패 : %v", err)
	}
}

// newGitRepo 는 시험용 git 저장소 하나를 만든다. git 이 없으면 건너뛴다.
func newGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}
	root := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"config", "commit.gpgsign", "false"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Skipf("git 준비 실패 : %v %s", err, out)
		}
	}
	return root
}

func TestLintVerboseExitsZero(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "Docs", "Design", "00-컨셉.md"), "# 컨셉\n")
	if got := run([]string{"lint", "--verbose", "--root", root, filepath.Join(root, "Docs", "Design", "00-컨셉.md")}); got != exitOK {
		t.Fatalf("--verbose 는 종료 0 이어야 한다 (지금 %d)", got)
	}
}

func TestLintChangedOutsideGitRepoExitsRead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}
	root := t.TempDir()
	if got := run([]string{"lint", "--changed", "--root", root}); got != exitRead {
		t.Fatalf("git 저장소가 아니면 종료 %d 여야 한다 (지금 %d)", exitRead, got)
	}
}

func TestLintChangedRefusesFileArgsAndHook(t *testing.T) {
	root := t.TempDir()
	if got := run([]string{"lint", "--changed", "--root", root, "a.md"}); got != exitUsage {
		t.Fatalf("--changed 와 파일 인자는 종료 %d 여야 한다 (지금 %d)", exitUsage, got)
	}
	if got := run([]string{"lint", "--changed", "--hook", "--root", root}); got != exitUsage {
		t.Fatalf("--changed 와 --hook 은 종료 %d 여야 한다 (지금 %d)", exitUsage, got)
	}
}

func TestLintChangedBlocksNewCodeWithoutSystemDoc(t *testing.T) {
	root := newGitRepo(t)
	writeFile(t, filepath.Join(root, "Docs", "Todo", "기획설정.json"), `{"코드폴더": "Src"}`)
	if err := os.MkdirAll(filepath.Join(root, "Docs", "Design"), 0o755); err != nil {
		t.Fatalf("폴더 만들기 실패 : %v", err)
	}
	writeFile(t, filepath.Join(root, "Src", "새것.cs"), "class A {}\n")
	if got := run([]string{"lint", "--changed", "--root", root}); got != exitLint {
		t.Fatalf("시스템 문서 없이 새 코드면 종료 %d 여야 한다 (지금 %d)", exitLint, got)
	}
	// 시스템 문서도 새 파일이라 앞 문서(L1)가 다 있어야 통과한다.
	for _, name := range []string{"00-컨셉.md", "01-코어루프.md", "02-기능목록.md", "03-시스템-무엇.md"} {
		writeFile(t, filepath.Join(root, "Docs", "Design", name), "# 문서\n\n| 무엇 | 값 |\n| --- | --- |\n| Won't | 안 한다 |\n")
	}
	if got := run([]string{"lint", "--changed", "--root", root}); got != exitOK {
		t.Fatalf("시스템 문서가 생기면 종료 0 이어야 한다 (지금 %d)", got)
	}
}

func TestLintChangedOnCleanRepoSaysNothingChanged(t *testing.T) {
	root := newGitRepo(t)
	out := captureOut(t, func() {
		if got := run([]string{"lint", "--changed", "--root", root}); got != exitOK {
			t.Errorf("깨끗한 저장소는 종료 0 이어야 한다 (지금 %d)", got)
		}
	})
	// 0개로 떨어진 판이 「검사했다」로 보이면 안 된다. 커밋된 판을 보는 길을 같이 알려준다.
	if !strings.Contains(out, "--since") {
		t.Fatalf("0개일 때 --since 안내가 있어야 한다 : %s", out)
	}
}

// 서브에이전트가 커밋까지 해 버려도 --since 로는 잡혀야 한다.
func TestLintSinceSeesCommittedCode(t *testing.T) {
	root := newGitRepo(t)
	writeFile(t, filepath.Join(root, "Docs", "Todo", "기획설정.json"), `{"코드폴더": "Src"}`)
	gitDo(t, root, "add", "-A")
	gitDo(t, root, "commit", "-m", "첫 커밋")
	writeFile(t, filepath.Join(root, "Src", "새것.cs"), "class A {}\n")
	gitDo(t, root, "add", "-A")
	gitDo(t, root, "commit", "-m", "둘째 커밋")

	if got := run([]string{"lint", "--changed", "--root", root}); got != exitOK {
		t.Fatalf("커밋 뒤라 --changed 는 0개 종료 0 이다 (지금 %d)", got)
	}
	if got := run([]string{"lint", "--since", "HEAD~1", "--root", root}); got != exitLint {
		t.Fatalf("커밋된 새 코드는 L2 로 막혀 종료 %d 여야 한다 (지금 %d)", exitLint, got)
	}
}

func TestLintSinceBadRefExitsRead(t *testing.T) {
	root := newGitRepo(t)
	if got := run([]string{"lint", "--since", "없는ref", "--root", root}); got != exitRead {
		t.Fatalf("틀린 ref 는 종료 %d 여야 한다 (지금 %d)", exitRead, got)
	}
}

func TestLintSinceRefusesFileArgsAndHook(t *testing.T) {
	root := t.TempDir()
	if got := run([]string{"lint", "--since", "HEAD", "--root", root, "a.md"}); got != exitUsage {
		t.Fatalf("--since 와 파일 인자는 종료 %d 여야 한다 (지금 %d)", exitUsage, got)
	}
	if got := run([]string{"lint", "--since", "HEAD", "--hook", "--root", root}); got != exitUsage {
		t.Fatalf("--since 와 --hook 은 종료 %d 여야 한다 (지금 %d)", exitUsage, got)
	}
}

// 뿌리 밖 경로는 「제외」와 섞어 세면 --root 를 잘못 준 것이 안 보인다.
func TestChangedTargetsCountsOutsideApart(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	entries := []gitchanged.Entry{
		{Path: "다른곳/밖.md"},
		{Path: "여기/Prototypes/버릴것.html"},
	}
	targets, skip := changedTargets(root, cfg, "여기/", entries)
	if len(targets) != 0 {
		t.Fatalf("검사 대상 = %d개, 0개여야 한다", len(targets))
	}
	if skip.outside != 1 {
		t.Fatalf("뿌리 밖 = %d개, 1개여야 한다 (%+v)", skip.outside, skip)
	}
	if skip.excluded != 1 {
		t.Fatalf("제외 = %d개, 1개여야 한다 (%+v)", skip.excluded, skip)
	}
}
