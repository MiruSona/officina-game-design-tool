package gitchanged

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// joinNUL 은 토막들을 실제 `-z` 출력처럼 NUL 로 잇는다.
func joinNUL(parts ...string) []byte {
	if len(parts) == 0 {
		return []byte{}
	}
	return []byte(strings.Join(parts, "\x00") + "\x00")
}

func TestParseTable(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want []Entry
	}{
		{"빈 입력", []byte{}, []Entry{}},
		{"고친 파일", joinNUL(" M Docs/할일.md"), []Entry{{Path: "Docs/할일.md"}}},
		{"한글 경로 새 파일", joinNUL("?? Docs/설계/새 문서.md"), []Entry{{Path: "Docs/설계/새 문서.md", New: true}}},
		{"스테이지에 올린 새 파일", joinNUL("A  Src/새것.cs"), []Entry{{Path: "Src/새것.cs", New: true}}},
		{"지운 파일", joinNUL(" D Src/옛것.cs"), []Entry{{Path: "Src/옛것.cs", Deleted: true}}},
		{"스테이지에 올렸다 지움", joinNUL("AD Src/잠깐.cs"), []Entry{{Path: "Src/잠깐.cs", Deleted: true}}},
		{"이름 바꾸기는 두 토막", joinNUL("R  새이름.md", "옛이름.md", " M 뒤.md"),
			[]Entry{{Path: "새이름.md"}, {Path: "뒤.md"}}},
		{"공백 든 이름", joinNUL(" M Docs/이름 에 공백.md"), []Entry{{Path: "Docs/이름 에 공백.md"}}},
		{"서브모듈 줄", joinNUL(" M Tools/GamedesignTool"), []Entry{{Path: "Tools/GamedesignTool"}}},
	}
	for _, c := range cases {
		got := Parse(c.raw)
		if len(got) != len(c.want) {
			t.Fatalf("%s : %d개, %d개여야 한다 (%+v)", c.name, len(got), len(c.want), got)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s : %+v, %+v 여야 한다", c.name, got[i], c.want[i])
			}
		}
	}
}

func TestParseDiffTable(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want []Entry
	}{
		{"빈 입력", []byte{}, []Entry{}},
		{"새 파일", joinNUL("A", "Src/새것.cs"), []Entry{{Path: "Src/새것.cs", New: true}}},
		{"고친 파일", joinNUL("M", "Docs/할일.md"), []Entry{{Path: "Docs/할일.md"}}},
		{"지운 파일", joinNUL("D", "Src/옛것.cs"), []Entry{{Path: "Src/옛것.cs", Deleted: true}}},
		{"여럿", joinNUL("A", "새것.md", "M", "있던것.md"),
			[]Entry{{Path: "새것.md", New: true}, {Path: "있던것.md"}}},
		{"한글·공백 경로", joinNUL("M", "Docs/이름 에 공백.md"), []Entry{{Path: "Docs/이름 에 공백.md"}}},
	}
	for _, c := range cases {
		got := ParseDiff(c.raw)
		if len(got) != len(c.want) {
			t.Fatalf("%s : %d개, %d개여야 한다 (%+v)", c.name, len(got), len(c.want), got)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s : %+v, %+v 여야 한다", c.name, got[i], c.want[i])
			}
		}
	}
}

// 실제 git 한 판. git 이 없는 기계에서는 건너뛴다.
func TestLoadOnRealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}
	root := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git 준비 실패 : %v %s", err, out)
		}
	}
	write(t, filepath.Join(root, "있던것.md"), "# 있던 것\n")
	write(t, filepath.Join(root, "지울것.md"), "# 지울 것\n")
	gitDo(t, root, "add", "-A")
	gitDo(t, root, "commit", "-m", "첫 커밋")

	write(t, filepath.Join(root, "있던것.md"), "# 있던 것\n고쳤다\n")
	write(t, filepath.Join(root, "새것.md"), "# 새것\n")
	if err := os.Remove(filepath.Join(root, "지울것.md")); err != nil {
		t.Fatalf("지우기 실패 : %v", err)
	}

	entries, prefix, err := Load(root)
	if err != nil {
		t.Fatalf("읽기 실패 : %v", err)
	}
	if prefix != "" {
		t.Fatalf("앞머리 = %q, 뿌리에서는 빈 값이어야 한다", prefix)
	}
	got := map[string]Entry{}
	for _, e := range entries {
		got[e.Path] = e
	}
	if e, ok := got["있던것.md"]; !ok || e.New || e.Deleted {
		t.Fatalf("고친 파일 = %+v", e)
	}
	if e, ok := got["새것.md"]; !ok || !e.New {
		t.Fatalf("새 파일 = %+v", e)
	}
	if e, ok := got["지울것.md"]; !ok || !e.Deleted {
		t.Fatalf("지운 파일 = %+v", e)
	}
}

// 커밋해 버린 변경도 --since 로는 보여야 한다.
func TestLoadSinceOnRealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}
	root := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git 준비 실패 : %v %s", err, out)
		}
	}
	write(t, filepath.Join(root, "있던것.md"), "# 있던 것\n")
	gitDo(t, root, "add", "-A")
	gitDo(t, root, "commit", "-m", "첫 커밋")
	write(t, filepath.Join(root, "새것.md"), "# 새것\n")
	write(t, filepath.Join(root, "있던것.md"), "# 있던 것\n고쳤다\n")
	gitDo(t, root, "add", "-A")
	gitDo(t, root, "commit", "-m", "둘째 커밋")

	entries, _, err := LoadSince(root, "HEAD~1")
	if err != nil {
		t.Fatalf("읽기 실패 : %v", err)
	}
	got := map[string]Entry{}
	for _, e := range entries {
		got[e.Path] = e
	}
	if e, ok := got["새것.md"]; !ok || !e.New {
		t.Fatalf("커밋된 새 파일 = %+v", e)
	}
	if e, ok := got["있던것.md"]; !ok || e.New || e.Deleted {
		t.Fatalf("커밋된 고친 파일 = %+v", e)
	}
	if _, _, err := LoadSince(root, "없는ref"); err == nil {
		t.Fatal("없는 ref 는 오류여야 한다")
	}
}

func TestLoadFailsOutsideRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 이 없다")
	}
	if _, _, err := Load(t.TempDir()); err == nil {
		t.Fatal("git 저장소가 아니면 오류여야 한다")
	}
}

func gitDo(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v 실패 : %v %s", args, err, out)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("파일 쓰기 실패 : %v", err)
	}
}
