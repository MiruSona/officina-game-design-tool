package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/config"
	"github.com/mirusona/officina-game-design-tool/internal/dash"
	"github.com/mirusona/officina-game-design-tool/internal/hookio"
	"github.com/mirusona/officina-game-design-tool/internal/lint"
	"github.com/mirusona/officina-game-design-tool/internal/paths"
	"github.com/mirusona/officina-game-design-tool/internal/state"
)

// 판 끝에 보는 자리.
const (
	historyDir  = "Docs/History"
	progressDoc = "Docs/Todo/진행상황.md"
)

// cmdLint 는 모양과 순서를 검사한다. 아무것도 안 고친다.
func cmdLint(args []string) error {
	fs, root := newFlags("lint")
	hook := fs.Bool("hook", false, "훅에서 부른다 (stdin 으로 훅 JSON 을 받는다)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *hook {
		return lintHook(*root)
	}
	dir, cfg, err := openRoot(*root)
	if err != nil {
		return err
	}
	st := loadStateOrNil(dir)
	targets, err := fileTargets(dir, cfg, fs.Args())
	if err != nil {
		return err
	}
	findings := lint.Run(lint.Input{Root: dir, Cfg: cfg, State: st, Targets: targets})
	return report(findings, len(targets))
}

// lintHook 은 훅 JSON 을 받아 그 파일 하나를 검사한다. 막을 때 종료 2 다.
func lintHook(rootFlag string) error {
	ev, err := hookio.Read(os.Stdin)
	if err != nil {
		return nil
	}
	if ev.ToolName != "Write" && ev.ToolName != "Edit" {
		return nil
	}
	if ev.ToolInput.FilePath == "" {
		return nil
	}
	dir := rootFlag
	if dir == "" {
		dir = ev.CWD
	}
	root, cfg, err := openRoot(dir)
	if err != nil {
		return nil
	}
	// 훅은 보통 절대 경로를 준다. 상대 경로로 오면 저장소 뿌리 기준으로 푼다.
	target := ev.ToolInput.FilePath
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	rel, err := paths.Rel(root, target)
	if err != nil {
		return nil
	}
	disk, existed := readIfExists(paths.Join(root, rel))
	t := lint.Target{
		Rel:        rel,
		Content:    hookio.Applied(ev.ToolName, ev.ToolInput, disk),
		HasContent: true,
		Existed:    existed,
	}
	findings := lint.Run(lint.Input{Root: root, Cfg: cfg, State: loadStateOrNil(root), Targets: []lint.Target{t}})
	if !lint.Blocked(findings) {
		return nil
	}
	msg := []string{}
	for _, f := range findings {
		if f.Block {
			msg = append(msg, fmt.Sprintf("[%s] %s", f.Code, f.Msg))
		}
	}
	return fail(exitLint, "기획 순서·모양 검사에 걸렸습니다.\n%s", strings.Join(msg, "\n"))
}

// cmdDone 은 판 끝에 할 일이 빠졌는지 본다. **막지 않고 경고만** 한다.
func cmdDone(args []string) error {
	fs, root := newFlags("done")
	hook := fs.Bool("hook", false, "Stop 훅에서 부른다")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *hook {
		hookio.Drain(os.Stdin)
	}
	dir, cfg, err := openRoot(*root)
	if err != nil {
		return quietOrFail(*hook, err)
	}
	st, err := state.Load(dir)
	if err != nil {
		return quietOrFail(*hook, fail(exitRead, "%v", err))
	}
	missing := roundTodos(dir, cfg, st)
	if len(missing) == 0 {
		if !*hook {
			fmt.Println("판 끝 할 일이 다 됐거나, 아직 판이 안 끝났습니다.")
		}
		return nil
	}
	text := "판 끝 할 일이 남았습니다 : " + strings.Join(missing, " · ")
	if !*hook {
		fmt.Println(text)
		return nil
	}
	raw, err := hookio.SystemMessage(text)
	if err != nil {
		return nil
	}
	fmt.Println(string(raw))
	return nil
}

// cmdDash 는 상태를 정적 HTML 한 장으로 굽는다.
func cmdDash(args []string) error {
	fs, root := newFlags("dash")
	out := fs.String("out", "", "구울 자리 (기본 : "+dash.RelPath+")")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	dir, cfg, err := openRoot(*root)
	if err != nil {
		return err
	}
	st, err := loadState(dir)
	if err != nil {
		return err
	}
	now := time.Now()
	path, bakeErr := dash.Bake(dir, dash.Build(dir, cfg, st, now), *out)
	if bakeErr != nil {
		return fail(exitWrite, "%v", bakeErr)
	}
	fmt.Printf("%s 를 구웠습니다 (%s). 브라우저로 여세요.\n", path, now.Format("2006-01-02 15:04"))
	return nil
}

// roundTodos 는 판 끝 할 일 중 빠진 것을 모은다. 물음이 다 판정된 뒤에만 본다.
func roundTodos(root string, cfg config.Config, st *state.State) []string {
	asks := st.RoundAsks()
	if len(asks) == 0 {
		return nil
	}
	_, _, rest := st.Counts()
	if rest > 0 {
		return nil
	}
	start, err := time.Parse("2006-01-02", st.RoundStart)
	if err != nil {
		return nil
	}
	missing := []string{}
	if !newerThan(paths.Join(root, historyDir), start, true) {
		missing = append(missing, "플레이 기록 한 편")
	}
	missing = append(missing, "기억 1건 (결정과 공수 눈금)")
	if !newerThan(paths.Join(root, progressDoc), start, false) {
		missing = append(missing, "진행 상황 문서의 「이미 정한 것」·「지금 할 일」 고치기")
	}
	return missing
}

// newerThan 은 그 자리에 판 시작 뒤로 고쳐진 것이 있는지 본다. 폴더가 없으면 「없음」이다.
func newerThan(path string, start time.Time, dir bool) bool {
	if !dir {
		info, err := os.Stat(path)
		if err != nil {
			return false
		}
		return info.ModTime().After(start)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(start) {
			return true
		}
	}
	return false
}

// fileTargets 는 명령줄로 받은 파일들을 검사 대상으로 바꾼다.
func fileTargets(root string, cfg config.Config, args []string) ([]lint.Target, error) {
	out := []lint.Target{}
	for _, a := range args {
		abs := a
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, a)
		}
		rel, err := paths.Rel(root, abs)
		if err != nil {
			return nil, fail(exitUsage, "%v", err)
		}
		content, existed := readIfExists(abs)
		out = append(out, lint.Target{Rel: rel, Content: content, HasContent: existed, Existed: existed})
	}
	return out, nil
}

// report 는 걸린 것을 찍고 종료 코드를 정한다.
func report(findings []lint.Finding, targets int) error {
	blockMsgs := []string{}
	for _, f := range findings {
		if f.Block {
			blockMsgs = append(blockMsgs, fmt.Sprintf("[%s] %s", f.Code, f.Msg))
			continue
		}
		fmt.Fprintf(os.Stderr, "⚠ [%s] %s\n", f.Code, f.Msg)
	}
	if len(blockMsgs) > 0 {
		return fail(exitLint, "기획 순서·모양 검사에 걸렸습니다.\n%s", strings.Join(blockMsgs, "\n"))
	}
	if targets == 0 {
		fmt.Println("검사를 마쳤습니다 — 막을 것은 없습니다.")
		return nil
	}
	fmt.Printf("검사를 마쳤습니다 — 파일 %d개, 막을 것은 없습니다.\n", targets)
	return nil
}

// loadStateOrNil 은 상태 파일을 읽되 없으면 nil 을 준다. 검사는 상태가 없어도 돈다.
func loadStateOrNil(root string) *state.State {
	st, err := state.Load(root)
	if err != nil {
		return nil
	}
	return st
}

// readIfExists 는 파일이 있으면 읽는다.
func readIfExists(path string) (string, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(raw), true
}

// quietOrFail 은 훅에서는 조용히 넘어가고, 손으로 쳤을 때만 오류를 낸다.
func quietOrFail(hook bool, err error) error {
	if hook {
		return nil
	}
	var ce *codedError
	if errors.As(err, &ce) {
		return err
	}
	return fail(exitRead, "%v", err)
}
