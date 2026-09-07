// Package dash 는 상태를 정적 HTML 한 장으로 굽는다. 서버도 상주 프로세스도 없다.
package dash

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/config"
	"github.com/mirusona/officina-game-design-tool/internal/mdscan"
	"github.com/mirusona/officina-game-design-tool/internal/paths"
	"github.com/mirusona/officina-game-design-tool/internal/state"
	"github.com/mirusona/officina-game-design-tool/templates"
)

// RelPath 는 구운 HTML 이 놓이는 자리다. 파생물이라 gitignore 대상이다.
const RelPath = "Docs/Todo/대시보드.html"

// Step 은 스테퍼 한 칸이다.
type Step struct {
	Number int
	Name   string
	Class  string
}

// AskRow 는 물음 표 한 줄이다.
type AskRow struct {
	ID        string
	Question  string
	Status    string
	CritCount int
}

// DocRow 는 문서 뼈대 체크 한 줄이다.
type DocRow struct {
	Name   string
	Exists bool
}

// Model 은 템플릿에 넣는 값 전부다.
type Model struct {
	StageNo    int
	StageName  string
	StageDone  string
	Round      int
	RoundStart string
	Days       int
	Doubt      bool
	BackCount  int
	PassCount  int
	AskCount   int
	Steps      []Step
	Asks       []AskRow
	Pillars    []string
	PillarNote string
	Docs       []DocRow
	BakedAt    string
}

// Build 는 상태·설정·문서를 읽어 화면에 넣을 값을 만든다.
func Build(root string, cfg config.Config, st *state.State, now time.Time) Model {
	pass, _, _ := st.Counts()
	m := Model{
		StageNo:    st.Stage,
		StageName:  cfg.StageName(st.Stage),
		Round:      st.Round,
		RoundStart: st.RoundStart,
		Days:       st.DaysIn(now),
		Doubt:      st.Doubt(cfg),
		BackCount:  len(st.Backs),
		PassCount:  pass,
		BakedAt:    now.Format("2006-01-02 15:04"),
	}
	if s, ok := cfg.Stage(st.Stage); ok {
		m.StageDone = s.Done
	}
	for _, s := range cfg.Stages {
		m.Steps = append(m.Steps, Step{Number: s.Number, Name: s.Name, Class: stepClass(s.Number, st.Stage)})
	}
	for _, a := range st.RoundAsks() {
		m.Asks = append(m.Asks, AskRow{ID: a.ID, Question: a.Question, Status: a.Status, CritCount: len(a.Crit)})
	}
	m.AskCount = len(m.Asks)
	m.Pillars, m.PillarNote = pillars(root, cfg)
	for _, name := range cfg.DocSkeleton {
		m.Docs = append(m.Docs, DocRow{Name: name, Exists: docExists(root, cfg, name)})
	}
	return m
}

// Bake 는 HTML 을 파일로 굽는다. 폴더가 없으면 만든다.
func Bake(root string, m Model, outRel string) (string, error) {
	if outRel == "" {
		outRel = RelPath
	}
	path := outRel
	if !filepath.IsAbs(path) {
		path = paths.Join(root, outRel)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("폴더를 못 만들었습니다 (%s) : %v", filepath.Dir(path), err)
	}
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("HTML 을 못 썼습니다 (%s) : %v", path, err)
	}
	defer f.Close()
	if err := Write(f, m); err != nil {
		return "", err
	}
	return path, nil
}

// Write 는 틀에 값을 넣어 흘려 보낸다.
func Write(w io.Writer, m Model) error {
	tpl, err := template.ParseFS(templates.FS, "dash.html")
	if err != nil {
		return fmt.Errorf("틀을 못 읽었습니다 : %v", err)
	}
	if err := tpl.Execute(w, m); err != nil {
		return fmt.Errorf("틀에 값을 못 넣었습니다 : %v", err)
	}
	return nil
}

// stepClass 는 스테퍼 한 칸의 꾸밈을 정한다.
func stepClass(n, now int) string {
	if n < now {
		return "done"
	}
	if n == now {
		return "now"
	}
	return "todo"
}

// pillars 는 컨셉 문서에서 기둥 이름을 읽는다. 세는 규약은 검사와 같다.
func pillars(root string, cfg config.Config) ([]string, string) {
	name := ""
	if len(cfg.DocSkeleton) > 0 {
		name = cfg.DocSkeleton[0]
	}
	raw, err := os.ReadFile(paths.Join(root, cfg.DesignDir+"/"+name))
	if err != nil {
		return nil, fmt.Sprintf("%s 이 아직 없습니다.", name)
	}
	src := string(raw)
	if _, ok := mdscan.PillarRows(src); !ok {
		return nil, fmt.Sprintf("%s 에 `## 기둥` 절의 표가 없습니다.", name)
	}
	out := []string{}
	for _, t := range mdscan.Tables(pillarSection(src)) {
		for _, row := range t.Rows {
			if len(row) > 0 && row[0] != "" {
				out = append(out, row[0])
			}
		}
		break
	}
	if len(out) == 0 {
		return nil, "기둥 표가 비었습니다."
	}
	return out, ""
}

// pillarSection 은 `## 기둥` 절만 잘라 낸다.
func pillarSection(src string) string {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	start := -1
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "#") && strings.HasPrefix(strings.TrimSpace(strings.TrimLeft(t, "#")), "기둥") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	for i := start; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "#") && strings.Contains(t, " ") {
			return strings.Join(lines[start:i], "\n")
		}
	}
	return strings.Join(lines[start:], "\n")
}

// docExists 는 뼈대 한 칸의 문서가 있는지 본다. 별표가 있으면 하나만 있어도 된다.
func docExists(root string, cfg config.Config, name string) bool {
	dir := paths.Join(root, cfg.DesignDir)
	if !strings.Contains(name, "*") {
		return paths.Exists(filepath.Join(dir, name))
	}
	matches, err := filepath.Glob(filepath.Join(dir, name))
	if err != nil {
		return false
	}
	return len(matches) > 0
}
