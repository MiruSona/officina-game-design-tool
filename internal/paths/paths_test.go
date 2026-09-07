package paths

import (
	"path/filepath"
	"testing"
)

func TestRootDefaultsToWorkingDir(t *testing.T) {
	got, err := Root("")
	if err != nil {
		t.Fatalf("현재 폴더를 못 잡았다 : %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("절대 경로여야 한다 : %s", got)
	}
	if _, err := Root(filepath.Join(t.TempDir(), "없는폴더")); err == nil {
		t.Fatal("없는 폴더는 막는다")
	}
}

func TestRelStaysInsideRoot(t *testing.T) {
	root := t.TempDir()
	got, err := Rel(root, filepath.Join(root, "Docs", "Design", "00-컨셉.md"))
	if err != nil {
		t.Fatalf("상대 경로 실패 : %v", err)
	}
	if got != "Docs/Design/00-컨셉.md" {
		t.Fatalf("상대 경로 = %q", got)
	}
	if _, err := Rel(root, filepath.Join(root, "..", "밖")); err == nil {
		t.Fatal("뿌리 밖은 막는다")
	}
}

func TestExcluded(t *testing.T) {
	cases := []struct {
		rel  string
		want bool
	}{
		{"Prototypes/2026-09-13-무엇/index.html", true},
		{"Prototypes", true},
		{"Docs/Todo/대시보드.html", true},
		{".git/config", true},
		{"Docs/Design/00-컨셉.md", false},
		{"PrototypesX/index.html", false},
	}
	for _, c := range cases {
		if got := Excluded(c.rel, "Prototypes"); got != c.want {
			t.Fatalf("%s : %v, %v 여야 한다", c.rel, got, c.want)
		}
	}
}

func TestInDirOnlyDirectChildren(t *testing.T) {
	if !InDir("Docs/Design/00-컨셉.md", "Docs/Design") {
		t.Fatal("바로 아래는 참이다")
	}
	if InDir("Docs/Design/옛자료/지난것.md", "Docs/Design") {
		t.Fatal("하위 폴더는 거짓이다")
	}
	if InDir("Docs/Todo/진행상황.md", "Docs/Design") {
		t.Fatal("다른 폴더는 거짓이다")
	}
}
