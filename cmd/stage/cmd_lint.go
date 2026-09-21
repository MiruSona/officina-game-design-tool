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
	"github.com/mirusona/officina-game-design-tool/internal/gitchanged"
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
	changed := fs.Bool("changed", false, "git 이 본 변경 파일을 스스로 모아 검사한다")
	since := fs.String("since", "", "그 ref 에서 HEAD 까지 **커밋된** 변경도 같이 검사한다 (보기 : HEAD~1)")
	verbose := fs.Bool("verbose", false, "무엇을 검사하고 무엇을 왜 건너뛰었나를 같이 찍는다")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	git := *changed || *since != ""
	if *hook && git {
		return fail(exitUsage, "--changed·--since 와 --hook 은 같이 못 씁니다. 훅은 파일 하나를 받습니다.")
	}
	if *hook {
		// 훅은 찍을 자리가 없다. --verbose 는 무시한다.
		return lintHook(*root)
	}
	if git && fs.NArg() > 0 {
		return fail(exitUsage, "--changed·--since 는 파일 인자와 같이 못 씁니다. 하나만 고르세요.")
	}
	dir, cfg, err := openRoot(*root)
	if err != nil {
		return err
	}
	if git {
		return lintChanged(dir, cfg, *changed, *since, *verbose)
	}
	targets, err := fileTargets(dir, cfg, fs.Args())
	if err != nil {
		return err
	}
	findings, traces := lint.RunTrace(lint.Input{Root: dir, Cfg: cfg, State: loadStateOrNil(dir), Targets: targets})
	if *verbose {
		printTraces(traces)
	}
	return report(findings, len(targets))
}

// lintChanged 는 git 변경분을 모아 검사한다. 못 읽으면 **종료 3** 이다 — 0개 통과로 떨어지지 않는다.
// changed 는 작업 트리·스테이지, since 는 그 ref 에서 HEAD 까지다. 둘 다 주면 합쳐서 본다.
func lintChanged(root string, cfg config.Config, changed bool, since string, verbose bool) error {
	entries := []gitchanged.Entry{}
	prefix := ""
	if since != "" {
		got, p, err := gitchanged.LoadSince(root, since)
		if err != nil {
			return fail(exitRead, "커밋된 변경분을 못 읽었습니다 (%v). ref 를 다시 보세요 : stage lint --since HEAD~1", err)
		}
		entries, prefix = got, p
	}
	if changed {
		got, p, err := gitchanged.Load(root)
		if err != nil {
			return fail(exitRead, "git 변경분을 못 읽었습니다 (%v). 파일을 직접 넘기세요 : stage lint <파일…>", err)
		}
		entries, prefix = mergeEntries(entries, got), p
	}
	if verbose {
		fmt.Printf("git 뿌리에서 이 폴더까지 : %q\n", prefix)
	}
	if len(entries) == 0 {
		fmt.Println("바뀐 파일이 없습니다 (작업트리·스테이지 깨끗함).")
		if since == "" {
			fmt.Println("이미 커밋된 판은 `stage lint --since <ref>` 로 봅니다 (보기 : stage lint --since HEAD~1).")
		}
		return nil
	}
	targets, skip := changedTargets(root, cfg, prefix, entries)
	fmt.Printf("git 변경 %d개 중 %d개 검사 (지운 것 %d · 제외 %d · 파일 아님 %d)\n",
		len(entries), len(targets), skip.deleted, skip.excluded, skip.notFile)
	if skip.outside > 0 {
		fmt.Printf("⚠ 뿌리 밖 경로 %d개는 검사하지 않았습니다 — 저장소 뿌리(--root)가 맞는지 보세요.\n", skip.outside)
	}
	if verbose {
		for _, t := range targets {
			fmt.Printf("  · %s\n", t.Rel)
		}
	}
	if len(targets) == 0 {
		fmt.Println("검사할 파일이 남지 않았습니다 — 저장소 전체 검사는 `stage lint` 로 따로 돌립니다.")
		return nil
	}
	findings, traces := lint.RunTrace(lint.Input{Root: root, Cfg: cfg, State: loadStateOrNil(root), Targets: targets})
	if verbose {
		printTraces(traces)
	}
	return report(findings, len(targets))
}

// skipCount 는 git 이 준 것 중 검사에서 뺀 까닭별 개수다.
type skipCount struct {
	deleted  int
	excluded int
	notFile  int
	outside  int
}

// mergeEntries 는 두 목록을 경로로 합친다. 한 쪽이라도 새 파일이면 새 파일로 보고,
// 지워졌나는 나중 목록(작업 트리)이 정본이다.
func mergeEntries(older, newer []gitchanged.Entry) []gitchanged.Entry {
	out := []gitchanged.Entry{}
	at := map[string]int{}
	for _, e := range append(append([]gitchanged.Entry{}, older...), newer...) {
		if i, ok := at[e.Path]; ok {
			out[i].New = out[i].New || e.New
			out[i].Deleted = e.Deleted
			continue
		}
		at[e.Path] = len(out)
		out = append(out, e)
	}
	return out
}

// changedTargets 는 git 이 준 경로를 뿌리 기준으로 바꿔 검사 대상으로 만든다.
func changedTargets(root string, cfg config.Config, prefix string, entries []gitchanged.Entry) ([]lint.Target, skipCount) {
	out := []lint.Target{}
	skip := skipCount{}
	for _, e := range entries {
		if e.Deleted {
			skip.deleted++
			continue
		}
		rel, err := relUnderRoot(root, prefix, e.Path)
		if err != nil {
			// 뿌리 밖은 「제외」와 뜻이 다르다. 섞어 세면 --root 를 잘못 준 것이 안 보인다.
			skip.outside++
			continue
		}
		if paths.Excluded(rel, cfg.ProtoDir) {
			skip.excluded++
			continue
		}
		// 폴더·서브모듈·링크는 검사할 내용이 없다. 링크를 따라가지 않게 Lstat 로 본다.
		info, statErr := os.Lstat(paths.Join(root, rel))
		if statErr != nil || !info.Mode().IsRegular() {
			skip.notFile++
			continue
		}
		content, read := readIfExists(paths.Join(root, rel))
		out = append(out, lint.Target{Rel: rel, Content: content, HasContent: read, Existed: !e.New})
	}
	return out, skip
}

// relUnderRoot 는 git 뿌리 기준 경로를 이 저장소 뿌리 기준으로 바꾼다. 뿌리 밖이면 오류다.
func relUnderRoot(root, prefix, gitPath string) (string, error) {
	p := gitPath
	if prefix != "" {
		if !strings.HasPrefix(p, prefix) {
			return "", fmt.Errorf("뿌리 밖의 경로입니다 : %s", gitPath)
		}
		p = strings.TrimPrefix(p, prefix)
	}
	if p == "" {
		return "", fmt.Errorf("빈 경로입니다")
	}
	return paths.Rel(root, filepath.Join(root, filepath.FromSlash(p)))
}

// printTraces 는 무엇이 돌고 무엇을 왜 건너뛰었나를 찍는다.
func printTraces(traces []lint.Trace) {
	for _, tr := range traces {
		fmt.Printf("· %s\n", tr.Rel)
		for _, r := range tr.Ran {
			fmt.Printf("    돌았음 : %s\n", r)
		}
		for _, s := range tr.Skipped {
			fmt.Printf("    건너뜀 : %s\n", s)
		}
	}
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
	session := ""
	if *hook {
		// stdin 은 끝까지 읽어야 부르는 쪽 파이프가 안 막힌다. 세션 id 는 여기서만 얻는다.
		ev, readErr := hookio.Read(os.Stdin)
		if readErr == nil {
			session = ev.SessionID
		}
	}
	dir, cfg, err := openRoot(*root)
	if err != nil {
		return quietOrFail(*hook, err)
	}
	st, err := state.Load(dir)
	if err != nil {
		return quietOrFail(*hook, fail(exitRead, "%v", err))
	}
	// 할 일에 「기억 1건」이 늘 들어가므로, 빈 목록은 아직 판이 안 끝났다는 뜻뿐이다.
	missing := roundTodos(dir, cfg, st)
	if len(missing) == 0 {
		if !*hook {
			fmt.Println("아직 판이 안 끝났습니다 — 이번 판 물음이 없거나 판정이 남았습니다.")
		}
		return nil
	}
	text := todoText(missing)
	if !*hook {
		// 손으로 친 것은 늘 찍는다. 되풀이를 줄이는 것은 훅뿐이다.
		fmt.Println(text)
		return nil
	}
	if doneAlreadySent(dir, session, text) {
		return nil
	}
	raw, err := hookio.SystemMessage(text)
	if err != nil {
		return nil
	}
	fmt.Println(string(raw))
	return nil
}

// todoText 는 할 일과 「무엇을 하면 사라지나」를 한 덩이 글로 만든다.
func todoText(missing []todo) string {
	lines := []string{"판 끝 할 일이 남았습니다 :"}
	for _, m := range missing {
		lines = append(lines, fmt.Sprintf("- %s → %s", m.name, m.fix))
	}
	lines = append(lines, "(같은 내용은 세션당 한 번만 뜹니다)")
	return strings.Join(lines, "\n")
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

// todo 는 판 끝 할 일 하나와 「무엇을 하면 사라지나」다.
type todo struct {
	name string
	fix  string
}

// roundTodos 는 판 끝 할 일 중 빠진 것을 모은다. 물음이 다 판정된 뒤에만 본다.
func roundTodos(root string, cfg config.Config, st *state.State) []todo {
	asks := st.RoundAsks()
	if len(asks) == 0 {
		return nil
	}
	_, _, rest := st.Counts()
	if rest > 0 {
		return nil
	}
	// 파일 mtime 이 로컬 시각이라 기준 시각도 로컬로 읽는다 (DaysIn 과 같은 꼴).
	start, err := st.RoundBase(time.Local)
	if err != nil {
		return nil
	}
	missing := []todo{}
	if !newerThan(paths.Join(root, historyDir), start, true) {
		missing = append(missing, todo{"플레이 기록 한 편", "`" + historyDir + "/` 에 .md 한 편"})
	}
	// 기억 저장소를 부르지 않으므로 확인할 길이 없다. 판마다 그냥 다시 묻는다 (설계 그대로).
	missing = append(missing, todo{"기억 1건 (결정과 공수 눈금)", "툴이 확인 못 합니다. 이 세션에서는 다시 안 뜹니다"})
	if !newerThan(paths.Join(root, progressDoc), start, false) {
		missing = append(missing, todo{"진행 상황 문서의 「이미 정한 것」·「지금 할 일」 고치기", "`" + progressDoc + "` 고치기"})
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
