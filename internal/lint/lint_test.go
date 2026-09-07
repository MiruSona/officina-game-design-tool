package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/config"
	"github.com/mirusona/officina-game-design-tool/internal/state"
)

const okRepo = "../../testdata/repo-ok"
const badRepo = "../../testdata/repo-bad"

// load 는 testdata 저장소 하나를 검사 재료로 연다.
func load(t *testing.T, root string) Input {
	t.Helper()
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("설정 읽기 실패 : %v", err)
	}
	st, err := state.Load(root)
	if err != nil {
		t.Fatalf("상태 읽기 실패 : %v", err)
	}
	return Input{Root: root, Cfg: cfg, State: st}
}

// codes 는 걸린 것의 이름만 모은다.
func codes(fs []Finding) string {
	out := []string{}
	for _, f := range fs {
		mark := ""
		if f.Block {
			mark = "!"
		}
		out = append(out, f.Code+mark)
	}
	return strings.Join(out, ",")
}

func TestRepoOKHasNothing(t *testing.T) {
	fs := Run(load(t, okRepo))
	if len(fs) != 0 {
		t.Fatalf("갖춰진 저장소인데 걸렸다 : %s", codes(fs))
	}
}

func TestRepoBadCatchesShape(t *testing.T) {
	fs := Run(load(t, badRepo))
	got := codes(fs)
	for _, want := range []string{"L3!", "L4!", "L6!", "L8"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%s 가 안 걸렸다 : %s", want, got)
		}
	}
}

func TestPillarBlocksOnlyFromStage3(t *testing.T) {
	in := load(t, badRepo)
	in.State.Stage = 2
	for _, f := range Run(in) {
		if f.Code == "L4" && f.Block {
			t.Fatal("2단계에서는 기둥을 막지 않는다")
		}
	}
	in.State.Stage = 3
	blocked := false
	for _, f := range Run(in) {
		if f.Code == "L4" && f.Block {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("3단계부터는 기둥을 막는다")
	}
}

func TestWontBlocksOnlyFromStage8(t *testing.T) {
	in := load(t, badRepo)
	in.State.Stage = 7
	for _, f := range Run(in) {
		if f.Code == "L6" && f.Block {
			t.Fatal("7단계에서는 Won't 를 막지 않는다")
		}
	}
	in.State.Stage = 8
	blocked := false
	for _, f := range Run(in) {
		if f.Code == "L6" && f.Block {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("8단계부터는 Won't 를 막는다")
	}
}

func TestOrderBlocksNewFileOnly(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "Docs", "Design"))
	cfg := config.Default()
	newFile := Target{Rel: "Docs/Design/01-코어루프.md", Existed: false}
	fs := Run(Input{Root: root, Cfg: cfg, Targets: []Target{newFile}})
	if !Blocked(fs) {
		t.Fatalf("00-컨셉.md 가 없으면 막아야 한다 : %s", codes(fs))
	}

	old := Target{Rel: "Docs/Design/01-코어루프.md", Existed: true}
	if Blocked(Run(Input{Root: root, Cfg: cfg, Targets: []Target{old}})) {
		t.Fatal("이미 있는 문서를 고치는 것은 순서를 안 본다")
	}

	write(t, filepath.Join(root, "Docs", "Design", "00-컨셉.md"), "# 컨셉\n")
	if Blocked(Run(Input{Root: root, Cfg: cfg, Targets: []Target{newFile}})) {
		t.Fatal("앞 문서가 생기면 통과해야 한다")
	}
}

func TestNameCheckOnlyDirectChildren(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	sub := Target{Rel: "Docs/Design/옛자료/지난 설계.md", Existed: true}
	if Blocked(Run(Input{Root: root, Cfg: cfg, Targets: []Target{sub}})) {
		t.Fatal("하위 폴더는 이름 규칙을 안 본다")
	}
	direct := Target{Rel: "Docs/Design/노트.md", Existed: true}
	if !Blocked(Run(Input{Root: root, Cfg: cfg, Targets: []Target{direct}})) {
		t.Fatal("바로 아래 문서는 이름 규칙을 본다")
	}
}

func TestPrototypeFolderExcluded(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	tgt := Target{Rel: "Prototypes/2026-09-13-무엇/index.html", Existed: false}
	if len(Run(Input{Root: root, Cfg: cfg, Targets: []Target{tgt}})) != 0 {
		t.Fatal("프로토타입 폴더는 검사에서 뺀다")
	}
}

func TestCodeOrderNeedsSystemDoc(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "Docs", "Design"))
	cfg := config.Default()
	cfg.CodeDir = "Src"
	tgt := Target{Rel: "Src/새것.cs", Existed: false}
	if !Blocked(Run(Input{Root: root, Cfg: cfg, Targets: []Target{tgt}})) {
		t.Fatal("시스템 문서 없이 코드를 새로 쓰면 막는다")
	}
	write(t, filepath.Join(root, "Docs", "Design", "03-시스템-무엇.md"), "# 시스템\n")
	if Blocked(Run(Input{Root: root, Cfg: cfg, Targets: []Target{tgt}})) {
		t.Fatal("시스템 문서가 있으면 통과한다")
	}
	cfg.CodeDir = ""
	if Blocked(Run(Input{Root: root, Cfg: cfg, Targets: []Target{tgt}})) {
		t.Fatal("코드폴더를 안 정했으면 안 본다")
	}
}

func TestAskCountWarnsOutsideRange(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	st := state.New(time.Now())
	st.Stage = 4
	if len(AskCount(Input{Root: root, Cfg: cfg, State: st})) != 1 {
		t.Fatal("물음이 0개면 경고한다")
	}
	_, _ = st.AddAsk("a", "물음 하나", []string{"기준"}, time.Now())
	_, _ = st.AddAsk("b", "물음 둘", []string{"기준"}, time.Now())
	if len(AskCount(Input{Root: root, Cfg: cfg, State: st})) != 0 {
		t.Fatal("2개면 범위 안이다")
	}
	st.Stage = 8
	if len(AskCount(Input{Root: root, Cfg: cfg, State: st})) != 0 {
		t.Fatal("고리 밖에서는 안 본다")
	}
}

func TestConceptLineWarnDoesNotBlock(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.ConceptMaxLines = 3
	long := "# 컨셉\n\n## 기둥\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n| 1 | 2 |\n| 1 | 2 |\n"
	tgt := Target{Rel: "Docs/Design/00-컨셉.md", Content: long, HasContent: true, Existed: true}
	fs := Run(Input{Root: root, Cfg: cfg, Targets: []Target{tgt}})
	if Blocked(fs) {
		t.Fatalf("길이는 경고만이다 : %s", codes(fs))
	}
	if !strings.Contains(codes(fs), "L5") {
		t.Fatalf("길이 경고가 없다 : %s", codes(fs))
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("폴더 만들기 실패 : %v", err)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("파일 쓰기 실패 : %v", err)
	}
}
