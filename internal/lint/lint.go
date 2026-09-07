// Package lint 는 셀 수 있는 것만 검사한다. 판단이 섞이는 것은 넣지 않는다.
package lint

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/mirusona/officina-game-design-tool/internal/config"
	"github.com/mirusona/officina-game-design-tool/internal/mdscan"
	"github.com/mirusona/officina-game-design-tool/internal/paths"
	"github.com/mirusona/officina-game-design-tool/internal/state"
)

// 막기 시작하는 단계. 그 문서를 쓰는 단계에서 막으면 문서를 시작조차 못 한다.
const (
	pillarBlockStage = 3
	wontBlockStage   = 8
)

var designName = regexp.MustCompile(`^\d\d-.+\.md$`)

// Target 은 「이제 쓰려는 파일」이다. Content 는 쓰고 난 뒤의 내용이다.
type Target struct {
	Rel        string
	Content    string
	HasContent bool
	Existed    bool
}

// Finding 은 검사 결과 한 줄이다. Block 이면 막는다.
type Finding struct {
	Code  string
	Msg   string
	Block bool
}

// Input 은 검사에 필요한 재료다. State 는 없을 수도 있다 (아직 init 을 안 했을 때).
type Input struct {
	Root    string
	Cfg     config.Config
	State   *state.State
	Targets []Target
}

// Run 은 검사를 돌려 걸린 것을 돌려준다. 아무것도 안 고친다.
func Run(in Input) []Finding {
	out := []Finding{}
	if len(in.Targets) == 0 {
		return append(out, scanRepo(in)...)
	}
	for _, t := range in.Targets {
		if paths.Excluded(t.Rel, in.Cfg.ProtoDir) {
			continue
		}
		out = append(out, checkTarget(in, t)...)
	}
	return out
}

// Blocked 는 막는 것이 하나라도 있는지 본다.
func Blocked(fs []Finding) bool {
	for _, f := range fs {
		if f.Block {
			return true
		}
	}
	return false
}

// checkTarget 은 파일 하나를 검사한다.
func checkTarget(in Input, t Target) []Finding {
	out := []Finding{}
	design := in.Cfg.DesignDir
	inDesign := paths.InDir(t.Rel, design)
	name := path.Base(t.Rel)

	if inDesign && strings.HasSuffix(name, ".md") && !designName.MatchString(name) {
		out = append(out, Finding{"L3", fmt.Sprintf("%s : 기획서 이름은 `NN-이름.md` 꼴이어야 합니다 (예 : 00-컨셉.md)", t.Rel), true})
	}
	if inDesign && !t.Existed && designName.MatchString(name) {
		out = append(out, orderFindings(in, name)...)
	}
	if !t.Existed && in.Cfg.CodeDir != "" && underDir(t.Rel, in.Cfg.CodeDir) {
		out = append(out, codeOrderFindings(in, t.Rel)...)
	}
	if !t.HasContent {
		return out
	}
	if inDesign && name == skeletonName(in.Cfg, 0) {
		out = append(out, conceptFindings(in, t.Content)...)
	}
	if inDesign && name == skeletonName(in.Cfg, 2) {
		out = append(out, wontFindings(in, t.Content)...)
	}
	return out
}

// scanRepo 는 대상 파일 없이 저장소의 모양만 본다. 순서 검사는 안 한다.
func scanRepo(in Input) []Finding {
	out := []Finding{}
	design := in.Cfg.DesignDir
	entries, err := os.ReadDir(paths.Join(in.Root, design))
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			if designName.MatchString(e.Name()) {
				continue
			}
			rel := design + "/" + e.Name()
			out = append(out, Finding{"L3", fmt.Sprintf("%s : 기획서 이름은 `NN-이름.md` 꼴이어야 합니다", rel), true})
		}
	}
	if src, ok := readDoc(in, skeletonName(in.Cfg, 0)); ok {
		out = append(out, conceptFindings(in, src)...)
	}
	if src, ok := readDoc(in, skeletonName(in.Cfg, 2)); ok {
		out = append(out, wontFindings(in, src)...)
	}
	out = append(out, AskCount(in)...)
	out = append(out, stateFindings(in)...)
	return out
}

// AskCount 는 이번 판 물음 개수가 범위 안인지 본다. `next` 도 이걸 쓴다.
func AskCount(in Input) []Finding {
	if in.State == nil {
		return nil
	}
	if !in.Cfg.InLoop(in.State.Stage) {
		return nil
	}
	n := len(in.State.RoundAsks())
	if n >= in.Cfg.AskMin && n <= in.Cfg.AskMax {
		return nil
	}
	msg := fmt.Sprintf("이번 판 물음이 %d개입니다. 한 묶음은 %d~%d개가 눈금입니다", n, in.Cfg.AskMin, in.Cfg.AskMax)
	return []Finding{{"L7", msg, false}}
}

// conceptFindings 는 기둥 개수(L4)와 컨셉 길이(L5)를 본다.
func conceptFindings(in Input, src string) []Finding {
	out := []Finding{}
	stage := stageOf(in)
	rows, ok := mdscan.PillarRows(src)
	block := stage >= pillarBlockStage
	if !ok {
		msg := fmt.Sprintf("%s 에 기둥 표가 없습니다. `## 기둥` 절 안에 표로 %d~%d개를 적습니다", skeletonName(in.Cfg, 0), in.Cfg.PillarMin, in.Cfg.PillarMax)
		out = append(out, Finding{"L4", msg, block})
	}
	if ok && (rows < in.Cfg.PillarMin || rows > in.Cfg.PillarMax) {
		msg := fmt.Sprintf("기둥이 %d개입니다. %d~%d개여야 합니다 (`## 기둥` 절 안 첫 표의 본문 줄로 셉니다)", rows, in.Cfg.PillarMin, in.Cfg.PillarMax)
		out = append(out, Finding{"L4", msg, block})
	}
	if n := mdscan.Lines(src); n > in.Cfg.ConceptMaxLines {
		msg := fmt.Sprintf("%s 이 %d줄입니다. %d줄 안으로 줄이세요", skeletonName(in.Cfg, 0), n, in.Cfg.ConceptMaxLines)
		out = append(out, Finding{"L5", msg, false})
	}
	return out
}

// wontFindings 는 Won't 칸이 비었는지 본다(L6). 비어 있으면 아무것도 안 자른 것이다.
func wontFindings(in Input, src string) []Finding {
	if mdscan.WontRows(src) > 0 {
		return nil
	}
	block := stageOf(in) >= wontBlockStage
	msg := fmt.Sprintf("%s 의 Won't 칸이 비었습니다. 첫 표의 첫 칸이 `Won't` 인 줄이 하나는 있어야 합니다", skeletonName(in.Cfg, 2))
	return []Finding{{"L6", msg, block}}
}

// orderFindings 는 문서 뼈대 순서를 본다(L1). 아직 없는 파일을 새로 쓸 때만 본다.
func orderFindings(in Input, name string) []Finding {
	idx := skeletonIndex(in.Cfg, name)
	if idx <= 0 {
		return nil
	}
	missing := []string{}
	for i := 0; i < idx; i++ {
		if !skeletonExists(in, in.Cfg.DocSkeleton[i]) {
			missing = append(missing, in.Cfg.DocSkeleton[i])
		}
	}
	if len(missing) == 0 {
		return nil
	}
	msg := fmt.Sprintf("%s 을 쓰기 전에 앞 문서가 먼저입니다 : %s", name, strings.Join(missing, " · "))
	return []Finding{{"L1", msg, true}}
}

// codeOrderFindings 는 시스템 문서 없이 코드를 새로 쓰는 것을 막는다(L2).
func codeOrderFindings(in Input, rel string) []Finding {
	for _, entry := range in.Cfg.DocSkeleton {
		if !strings.Contains(entry, "*") {
			continue
		}
		if skeletonExists(in, entry) {
			return nil
		}
	}
	msg := fmt.Sprintf("%s : 시스템 코드를 새로 쓰기 전에 `03-시스템-<이름>.md` 를 먼저 씁니다", rel)
	return []Finding{{"L2", msg, true}}
}

// stateFindings 는 상태 파일의 앞뒤가 맞는지 본다(L8).
func stateFindings(in Input) []Finding {
	st := in.State
	if st == nil {
		return nil
	}
	out := []Finding{}
	if st.Stage < 1 || st.Stage > len(in.Cfg.Stages) {
		out = append(out, Finding{"L8", fmt.Sprintf("단계 값이 범위 밖입니다 : %d", st.Stage), false})
	}
	seen := map[string]bool{}
	for _, a := range st.Asks {
		if seen[a.ID] {
			out = append(out, Finding{"L8", fmt.Sprintf("물음 id 가 겹칩니다 : %s", a.ID), false})
		}
		seen[a.ID] = true
		if len(a.Verdicts) == 0 {
			continue
		}
		want := state.StatusFromVerdict(a.Verdicts[len(a.Verdicts)-1].Value)
		if a.Status != want {
			msg := fmt.Sprintf("%s : 상태(%s)가 마지막 판정(%s)과 다릅니다", a.ID, a.Status, want)
			out = append(out, Finding{"L8", msg, false})
		}
	}
	return out
}

// stageOf 는 지금 단계를 돌려준다. 상태 파일이 없으면 0 이라 아무것도 막지 않는다.
func stageOf(in Input) int {
	if in.State == nil {
		return 0
	}
	return in.State.Stage
}

// skeletonName 은 뼈대 목록의 n 번째 파일 이름이다.
func skeletonName(cfg config.Config, n int) string {
	if n < 0 || n >= len(cfg.DocSkeleton) {
		return ""
	}
	return cfg.DocSkeleton[n]
}

// skeletonIndex 는 그 파일이 뼈대 목록의 몇 번째인지 찾는다. 없으면 -1 이다.
func skeletonIndex(cfg config.Config, name string) int {
	for i, entry := range cfg.DocSkeleton {
		if entry == name {
			return i
		}
		if strings.Contains(entry, "*") && globMatch(entry, name) {
			return i
		}
	}
	return -1
}

// skeletonExists 는 뼈대 한 칸에 해당하는 파일이 실제로 있는지 본다.
func skeletonExists(in Input, entry string) bool {
	dir := paths.Join(in.Root, in.Cfg.DesignDir)
	if !strings.Contains(entry, "*") {
		return paths.Exists(path.Join(dir, entry))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && globMatch(entry, e.Name()) {
			return true
		}
	}
	return false
}

// globMatch 는 별표 하나짜리 무늬를 맞춰 본다.
func globMatch(pattern, name string) bool {
	ok, err := path.Match(pattern, name)
	if err != nil {
		return false
	}
	return ok
}

// readDoc 은 기획문서폴더 아래 문서를 읽는다.
func readDoc(in Input, name string) (string, bool) {
	if name == "" || strings.Contains(name, "*") {
		return "", false
	}
	raw, err := os.ReadFile(paths.Join(in.Root, in.Cfg.DesignDir+"/"+name))
	if err != nil {
		return "", false
	}
	return string(raw), true
}

// underDir 은 그 폴더 아래인지 본다 (하위 폴더 포함).
func underDir(rel, dir string) bool {
	d := strings.TrimSuffix(dir, "/")
	return rel == d || strings.HasPrefix(rel, d+"/")
}
