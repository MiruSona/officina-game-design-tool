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

func TestCodeOrderSkipsWithoutWildcardEntry(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "Docs", "Design"))
	cfg := config.Default()
	cfg.CodeDir = "Src"
	cfg.DocSkeleton = []string{"00-컨셉.md", "01-코어루프.md"}
	tgt := Target{Rel: "Src/새것.cs", Existed: false}
	if Blocked(Run(Input{Root: root, Cfg: cfg, Targets: []Target{tgt}})) {
		t.Fatal("문서 뼈대에 와일드카드가 없으면 코드를 막지 않는다")
	}
}

func TestCodeOrderMessageUsesConfigEntry(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "Docs", "Design"))
	cfg := config.Default()
	cfg.CodeDir = "Src"
	cfg.DocSkeleton = []string{"00-컨셉.md", "10-구조-*.md"}
	tgt := Target{Rel: "Src/새것.cs", Existed: false}
	fs := Run(Input{Root: root, Cfg: cfg, Targets: []Target{tgt}})
	if !Blocked(fs) {
		t.Fatal("시스템 문서 없이 코드를 새로 쓰면 막는다")
	}
	found := false
	for _, f := range fs {
		if f.Code != "L2" {
			continue
		}
		if strings.Contains(f.Msg, "10-구조-<이름>.md") && strings.Contains(f.Msg, "10-구조-*.md") &&
			strings.Contains(f.Msg, cfg.DesignDir) && strings.Contains(f.Msg, "한 장만 있으면 풀립니다") {
			found = true
		}
	}
	if !found {
		t.Fatalf("설정 항목 이름·찾는 자리·푸는 법이 메시지에 다 없다 : %v", fs)
	}
}

// traceOf 는 첫 자취의 돈 것·건너뛴 것을 한 줄로 이어 붙인다.
func traceOf(t *testing.T, in Input) (string, string) {
	t.Helper()
	_, trs := RunTrace(in)
	if len(trs) != 1 {
		t.Fatalf("자취 = %d개, 1개여야 한다", len(trs))
	}
	return strings.Join(trs[0].Ran, " / "), strings.Join(trs[0].Skipped, " / ")
}

func TestTraceTellsWhyCodeFileSkipsDocChecks(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "Docs", "Design"))
	cfg := config.Default()
	cfg.CodeDir = "Src"
	write(t, filepath.Join(root, "Docs", "Design", "03-시스템-무엇.md"), "# 시스템\n")
	tgt := Target{Rel: "Src/새것.cs", Content: "class A {}", HasContent: true, Existed: false}
	ran, skipped := traceOf(t, Input{Root: root, Cfg: cfg, Targets: []Target{tgt}})
	if !strings.Contains(ran, "L2 코드 순서") {
		t.Fatalf("새 코드 파일이면 L2 가 돈다 : %s", ran)
	}
	if !strings.Contains(skipped, "L3 이름") || !strings.Contains(skipped, "L1 문서 순서") {
		t.Fatalf("코드 파일은 L1·L3 를 건너뛴 까닭이 있어야 한다 : %s", skipped)
	}
}

func TestTraceSaysExistingFileSkipsOrder(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	tgt := Target{Rel: "Docs/Design/00-컨셉.md", Content: "# 컨셉\n", HasContent: true, Existed: true}
	ran, skipped := traceOf(t, Input{Root: root, Cfg: cfg, Targets: []Target{tgt}})
	if !strings.Contains(skipped, "이미 있는 파일") {
		t.Fatalf("있는 파일은 순서 검사를 건너뛴 까닭을 적는다 : %s", skipped)
	}
	if !strings.Contains(ran, "L4 기둥 개수") || !strings.Contains(ran, "L3 이름") {
		t.Fatalf("00-컨셉.md 면 L3·L4 가 돈다 : %s", ran)
	}
}

// 기획문서폴더 밖 파일에 「00-컨셉.md 가 아님」이라고 적으면 엉뚱한 까닭이 된다.
func TestTraceSaysOutsideDesignDir(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	tgt := Target{Rel: "Src/새것.cs", Content: "class A {}", HasContent: true, Existed: true}
	_, skipped := traceOf(t, Input{Root: root, Cfg: cfg, Targets: []Target{tgt}})
	if !strings.Contains(skipped, "L4·L5 : 기획문서폴더(Docs/Design) 아래가 아님") {
		t.Fatalf("L4·L5 는 폴더 밖이라고 갈라 적어야 한다 : %s", skipped)
	}
	if !strings.Contains(skipped, "L6 : 기획문서폴더(Docs/Design) 아래가 아님") {
		t.Fatalf("L6 도 폴더 밖이라고 갈라 적어야 한다 : %s", skipped)
	}
	if strings.Contains(skipped, "00-컨셉.md 가 아님") {
		t.Fatalf("폴더 밖 파일에 파일 이름 까닭을 적으면 안 된다 : %s", skipped)
	}
}

// 기획문서폴더 안이면 예전처럼 파일 이름으로 까닭을 적는다.
func TestTraceSaysWrongDocNameInsideDesignDir(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	tgt := Target{Rel: "Docs/Design/01-코어루프.md", Content: "# 코어루프\n", HasContent: true, Existed: true}
	_, skipped := traceOf(t, Input{Root: root, Cfg: cfg, Targets: []Target{tgt}})
	if !strings.Contains(skipped, "00-컨셉.md 가 아님") {
		t.Fatalf("폴더 안이면 파일 이름 까닭이어야 한다 : %s", skipped)
	}
}

func TestTraceMarksExcludedFolder(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	tgt := Target{Rel: "Prototypes/2026-09-13-무엇/index.html", HasContent: true}
	ran, skipped := traceOf(t, Input{Root: root, Cfg: cfg, Targets: []Target{tgt}})
	if ran != "" || !strings.Contains(skipped, "검사 밖 자리") {
		t.Fatalf("프로토타입 폴더는 통째로 건너뛴다 : ran=%s skip=%s", ran, skipped)
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
