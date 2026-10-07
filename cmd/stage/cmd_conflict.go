package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/config"
	"github.com/mirusona/officina-game-design-tool/internal/conflict"
	"github.com/mirusona/officina-game-design-tool/internal/gitchanged"
	"github.com/mirusona/officina-game-design-tool/internal/mdscan"
	"github.com/mirusona/officina-game-design-tool/internal/paths"
)

// conflictDir 은 「무엇을 보냈나」 를 남기는 기록 폴더다. 붙은 저장소의 gitignore 에 넣는다. 자동 삭제는 없다.
const conflictDir = ".gamedesign/conflict"

// 경고 문턱. 확률이 0.2 쯤 흔들려 문턱을 둘로 둔다 (설계 0절).
const (
	warnProb      = 0.8
	ambiguousProb = 0.5
	showRunes     = 120 // 화면에 찍는 문장 길이. 전체는 기록 파일에 있다
	listRunes     = 60  // 후보 목록에 찍는 문장 길이
)

var conflictDocName = regexp.MustCompile(`^\d\d-.+\.md$`)

// judgeWait 는 쌍 수로 제한 시간을 정한다. 시험이 짧은 값으로 바꿔 끼운다.
var judgeWait = conflict.Wait

// conflictRun 은 한 번 돌린 재료와 결과를 모아 둔 것이다. 출력과 기록이 같이 쓴다.
type conflictRun struct {
	root      string
	docs      []string
	sentences int
	total     int
	limited   bool // --limit 에 잘렸나. 비밀 꼴로 뺀 것과 섞지 않는다
	rows      []conflict.Row
	pairs     map[string]conflict.Pair
	dropped   []string // 비밀 꼴로 뺀 자리 (`문서:줄`). 기록 파일에만 적는다
	sent      string   // 보낸 jsonl 의 뿌리 기준 경로
	result    string   // 결과 파일. 판정기를 실제로 돌린 판에만 채운다
	skipped   string
	outcome   conflict.Outcome
	started   time.Time
}

// hit 은 경고·애매 한 줄이다 (`--json` 꼴).
type hit struct {
	ID    string  `json:"id"`
	ADoc  string  `json:"a_doc"`
	ALine int     `json:"a_line"`
	BDoc  string  `json:"b_doc"`
	BLine int     `json:"b_line"`
	A     string  `json:"a"`
	B     string  `json:"b"`
	Prob  float64 `json:"prob"`
}

// conflictJSON 은 `--json` 출력이다.
type conflictJSON struct {
	Pairs         int    `json:"pairs"`
	Total         int    `json:"total"`
	Judged        int    `json:"judged"`
	Unread        int    `json:"unread"`
	Skipped       string `json:"skipped"`
	Warn          []hit  `json:"warn"`
	Ambiguous     []hit  `json:"ambiguous"`
	Sent          string `json:"sent"`
	SecretDropped int    `json:"secret_dropped"`
	MS            int64  `json:"ms"`
}

// cmdConflict 는 문서끼리 어긋난 문장 짝을 찾는다. 경고 목록이지 막음이 아니다 — 종료 2 는 안 쓴다.
func cmdConflict(args []string) error {
	fs, root := newFlags("conflict")
	changed := fs.Bool("changed", false, "git 이 본 변경 문서 × 나머지 문서 짝만 본다")
	pairsOnly := fs.Bool("pairs-only", false, "판정기를 안 부르고 후보 쌍 목록만 낸다")
	asJSON := fs.Bool("json", false, "결과를 JSON 으로 낸다 (--pairs-only 면 jsonl 줄 그대로)")
	limit := fs.Int("limit", conflict.DefaultLimit, "판정할 쌍 상한")
	out := fs.String("out", "", "후보 쌍 jsonl 을 이 자리에도 쓴다 (저장소 뿌리 아래만)")
	fresh := fs.Bool("fresh", false, "판정기의 지난 기록을 안 쓰고 다시 묻는다")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *limit < 1 {
		return fail(exitUsage, "--limit 은 1 이상이어야 합니다")
	}
	dir, cfg, err := openRoot(*root)
	if err != nil {
		return err
	}
	outAbs, err := checkOut(dir, *out)
	if err != nil {
		return err
	}
	docs, err := loadDesignDocs(dir, cfg)
	if err != nil {
		return err
	}
	changedSet, err := changedDocs(dir, cfg, *changed)
	if err != nil {
		return err
	}
	run := &conflictRun{root: dir, started: time.Now(), pairs: map[string]conflict.Pair{}}
	for _, d := range docs {
		run.docs = append(run.docs, d.Name)
	}
	if *changed && len(changedSet) == 0 {
		// 안내는 stderr 로 — stdout 은 --json 일 때 기계가 읽는다.
		return printNothing(run, *asJSON, "바뀐 문서가 없습니다 (작업트리·스테이지에 기획 문서 변경 없음)")
	}
	res := conflict.Pairs(docs, conflict.Options{Limit: *limit, Changed: changedSet, Skip: hasSecret})
	run.sentences, run.total = res.Sentences, res.Total
	run.limited = res.Total > len(res.Pairs)
	run.dropped = droppedKeys(res.Dropped)
	run.rows = rowsOf(run, res.Pairs)
	if len(run.rows) == 0 {
		return printNothing(run, *asJSON, fmt.Sprintf("어긋남 후보 0쌍 — 문서 %d장 · 문장 %d · 짝 조건(내용어 2개 겹침 + 숫자·부정어)에 맞는 것 %d · 비밀 꼴로 뺀 문장 %d",
			len(docs), res.Sentences, res.Total, len(run.dropped)))
	}
	if err := writeSent(run, outAbs); err != nil {
		return err
	}
	if *pairsOnly {
		printPairsOnly(run, *asJSON)
		return writeLast(run)
	}
	exe := ""
	if !cfg.Conflict {
		run.skipped = "설정으로 꺼짐 (기획설정.json 어긋남판정: false)"
	} else {
		exe, run.skipped = conflict.Lookup()
	}
	if run.skipped != "" {
		printSkipped(run, *asJSON)
		return writeLast(run)
	}
	run.outcome = conflict.Run(exe, dir, paths.Join(dir, run.sent), judgeWait(len(run.rows)), *fresh)
	if reason := run.outcome.SkipReason(); reason != "" {
		// 판정기가 설정 없이 넘어간 것 — 실패가 아니라 건너뜀으로 찍는다.
		run.skipped = reason
		printSkipped(run, *asJSON)
		return writeLast(run)
	}
	if err := writeResult(run); err != nil {
		return err
	}
	printOutcome(run, *asJSON)
	return writeLast(run)
}

// loadDesignDocs 는 기획 문서 폴더 바로 아래 `NN-이름.md` 를 읽는다. 링크·폴더는 건너뛴다.
func loadDesignDocs(root string, cfg config.Config) ([]conflict.Doc, error) {
	entries, err := os.ReadDir(paths.Join(root, cfg.DesignDir))
	if err != nil {
		return nil, fail(exitRead, "기획 문서 폴더를 못 읽었습니다 (%s) : %v", cfg.DesignDir, err)
	}
	docs := []conflict.Doc{}
	for _, e := range entries {
		if !e.Type().IsRegular() || !conflictDocName.MatchString(e.Name()) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(paths.Join(root, cfg.DesignDir), e.Name()))
		if err != nil {
			return nil, fail(exitRead, "문서를 못 읽었습니다 (%s) : %v", e.Name(), err)
		}
		src := string(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}))
		docs = append(docs, conflict.Doc{Name: e.Name(), Sentences: mdscan.Sentences(src)})
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Name < docs[j].Name })
	return docs, nil
}

// changedDocs 는 git 이 본 변경 중 기획 문서 이름만 모은다. --changed 가 아니면 nil 이다.
func changedDocs(root string, cfg config.Config, changed bool) (map[string]bool, error) {
	if !changed {
		return nil, nil
	}
	entries, prefix, err := gitchanged.Load(root)
	if err != nil {
		return nil, fail(exitRead, "git 변경분을 못 읽었습니다 (%v). --changed 없이 전체를 보세요.", err)
	}
	set := map[string]bool{}
	for _, e := range entries {
		if e.Deleted {
			continue
		}
		rel, err := relUnderRoot(root, prefix, e.Path)
		if err != nil || !paths.InDir(rel, cfg.DesignDir) {
			continue
		}
		name := filepath.Base(rel)
		if conflictDocName.MatchString(name) {
			set[name] = true
		}
	}
	return set, nil
}

// checkOut 은 --out 이 뿌리 아래의 .jsonl 인지 본다. 링크를 푼 실제 경로로 접두 비교한다 (경로 감옥).
// 돌려주는 것은 쓸 실제 절대 경로다.
func checkOut(root, out string) (string, error) {
	if out == "" {
		return "", nil
	}
	if !strings.HasSuffix(strings.ToLower(out), ".jsonl") {
		return "", fail(exitUsage, "--out 은 .jsonl 로 끝나야 합니다 — 기획 문서를 덮지 않게 막습니다")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fail(exitRead, "뿌리를 못 폈습니다 : %v", err)
	}
	abs := out
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, out)
	}
	real, err := realPath(abs)
	if err != nil {
		return "", fail(exitUsage, "--out 경로를 못 폈습니다 : %v", err)
	}
	if !underReal(realRoot, real) {
		return "", fail(exitUsage, "--out 은 저장소 뿌리 아래여야 합니다 : %s", out)
	}
	if info, statErr := os.Lstat(real); statErr == nil && !info.Mode().IsRegular() {
		return "", fail(exitUsage, "--out 자리에 보통 파일이 아닌 것이 있습니다 : %s", out)
	}
	return real, nil
}

// realPath 는 링크를 푼 절대 경로다. 아직 없는 파일은 가장 가까운 있는 조상 폴더를 풀고 나머지를 붙인다.
func realPath(abs string) (string, error) {
	rest := []string{}
	cur := filepath.Clean(abs)
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(append([]string{resolved}, rest...)...), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("있는 조상 폴더가 없습니다 : %s", abs)
		}
		rest = append([]string{filepath.Base(cur)}, rest...)
		cur = parent
	}
}

// underReal 은 둘 다 링크를 푼 경로일 때 target 이 root 아래인지 본다. 대소문자는 접어 비교한다.
func underReal(root, target string) bool {
	rel, err := filepath.Rel(strings.ToLower(root), strings.ToLower(target))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// hasSecret 은 짝의 두 문장(문맥 포함)에 비밀 꼴이 있는지 본다. 값은 어디에도 안 찍는다.
func hasSecret(p conflict.Pair) bool {
	for _, s := range []mdscan.Sentence{p.A, p.B} {
		if conflict.SecretShape(s.Text) || conflict.SecretShape(s.Ctx) {
			return true
		}
	}
	return false
}

// droppedKeys 는 비밀 꼴로 뺀 짝에서 걸린 문장 자리(`문서:줄`)만 모은다. 기록 파일에만 적는다.
func droppedKeys(dropped []conflict.Pair) []string {
	keys := []string{}
	seen := map[string]bool{}
	for _, p := range dropped {
		for _, side := range []struct {
			doc  string
			sent mdscan.Sentence
		}{{p.ADoc, p.A}, {p.BDoc, p.B}} {
			key := fmt.Sprintf("%s:%d", side.doc, side.sent.Line)
			if (conflict.SecretShape(side.sent.Text) || conflict.SecretShape(side.sent.Ctx)) && !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
		}
	}
	return keys
}

// rowsOf 는 짝을 jsonl 줄로 바꾸고 id 로 되찾을 표를 채운다.
func rowsOf(run *conflictRun, pairs []conflict.Pair) []conflict.Row {
	rows := []conflict.Row{}
	for _, p := range pairs {
		row := conflict.RowOf(p)
		run.pairs[row.ID] = p
		rows = append(rows, row)
	}
	return rows
}

// printNothing 은 후보가 없을 때의 안내다. 사람용은 stderr, --json 이면 stdout 에 요약 한 줄.
func printNothing(run *conflictRun, asJSON bool, why string) error {
	fmt.Fprintln(os.Stderr, why)
	if asJSON {
		run.skipped = why
		printJSON(run, nil, nil, 0)
	}
	return nil
}

// writeSent 는 보낼 jsonl 을 기록 폴더에 쓰고, --out 이 있으면 거기에도 쓴다. 이 파일이 그대로 판정기 입력이다.
func writeSent(run *conflictRun, outAbs string) error {
	var buf strings.Builder
	for _, r := range run.rows {
		raw, err := json.Marshal(r)
		if err != nil {
			return fail(exitWrite, "jsonl 을 못 만들었습니다 : %v", err)
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}
	stamp := freeStamp(run.root, run.started)
	run.sent = conflictDir + "/" + stamp + ".jsonl"
	if err := writeAtomic(paths.Join(run.root, run.sent), []byte(buf.String())); err != nil {
		return err
	}
	if outAbs != "" {
		return writeAtomic(outAbs, []byte(buf.String()))
	}
	return nil
}

// freeStamp 은 초 단위 시각 이름이다. 같은 초에 두 판이 돌면 `-2`·`-3` 을 붙여 안 덮는다.
func freeStamp(root string, at time.Time) string {
	base := at.Format("2006-01-02T150405")
	stamp := base
	for n := 2; paths.Exists(paths.Join(root, conflictDir+"/"+stamp+".jsonl")); n++ {
		stamp = fmt.Sprintf("%s-%d", base, n)
	}
	return stamp
}

// writeResult 는 읽힌 판정 JSON 줄만 남긴다. 읽힌 판정이 없으면 파일을 안 만들고 result 는 빈 값이다.
func writeResult(run *conflictRun) error {
	if len(run.outcome.Verdicts) == 0 {
		return nil
	}
	run.result = strings.TrimSuffix(run.sent, ".jsonl") + ".result.jsonl"
	return writeAtomic(paths.Join(run.root, run.result), run.outcome.Accepted)
}

// writeLast 는 이번 판 요약을 last.json 에 남긴다. 비밀 꼴로 뺀 자리는 여기에만 적는다.
// judgeError 는 last.json 에 남길 판정기 쪽 까닭이다. 건너뜀·판정 안 돌림은 빈 값, 읽힌 판정이 0 이면 실패 까닭 전부.
func judgeError(run *conflictRun) string {
	if run.skipped != "" || run.sent == "" || run.outcome.Verdicts == nil {
		return ""
	}
	if len(run.outcome.Verdicts) == 0 {
		return run.outcome.FailReason()
	}
	return run.outcome.ExitErr
}

func writeLast(run *conflictRun) error {
	last := map[string]any{
		"at":             run.started.Format(time.RFC3339),
		"docs":           run.docs,
		"sentences":      run.sentences,
		"pairs":          len(run.rows),
		"total":          run.total,
		"secret_dropped": run.dropped,
		"sent":           run.sent,
		"result":         run.result,
		"skipped":        run.skipped,
		"judge_error":    judgeError(run),
		"cmd":            os.Args,
		"ms":             time.Since(run.started).Milliseconds(),
	}
	raw, err := json.MarshalIndent(last, "", "  ")
	if err != nil {
		return fail(exitWrite, "last.json 을 못 만들었습니다 : %v", err)
	}
	return writeAtomic(paths.Join(run.root, conflictDir+"/last.json"), append(raw, '\n'))
}

// writeAtomic 은 tmp 에 쓰고 rename 한다. 반쯤 쓰인 파일이 남지 않는다.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fail(exitWrite, "폴더를 못 만들었습니다 (%s) : %v", filepath.Dir(path), err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fail(exitWrite, "파일을 못 썼습니다 (%s) : %v", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fail(exitWrite, "파일을 못 바꿔 넣었습니다 (%s) : %v", path, err)
	}
	return nil
}

// printPairsOnly 는 후보 쌍만 찍는다. --json 이면 보낸 jsonl 줄 그대로다 (사람이 want 를 채우면 측정 셋이 된다).
func printPairsOnly(run *conflictRun, asJSON bool) {
	if asJSON {
		for _, r := range run.rows {
			raw, _ := json.Marshal(r)
			fmt.Println(string(raw))
		}
		return
	}
	fmt.Printf("어긋남 후보 %d쌍 (판정 안 함 · --pairs-only)\n", len(run.rows))
	printList(run)
	printFooter(run)
}

// printSkipped 는 판정을 건너뛴 까닭과 후보 목록을 찍는다. 종료 0 이다.
func printSkipped(run *conflictRun, asJSON bool) {
	if asJSON {
		printJSON(run, nil, nil, 0)
		return
	}
	fmt.Printf("판정 건너뜀 — %s · 후보 쌍 %d개\n", run.skipped, len(run.rows))
	printList(run)
	printFooter(run)
}

// printOutcome 은 판정 결과를 경고·애매로 갈라 찍는다. A·C 는 안 찍는다.
func printOutcome(run *conflictRun, asJSON bool) {
	warn, amb, judged := grade(run)
	if asJSON {
		printJSON(run, warn, amb, judged)
		return
	}
	oc := run.outcome
	if len(oc.Verdicts) == 0 {
		fmt.Printf("판정 실패 — %s · 후보 쌍 %d개\n", oc.FailReason(), len(run.rows))
		printFooter(run)
		return
	}
	quiet := judged - len(warn) - len(amb)
	fmt.Printf("어긋남 후보 %d쌍 → 판정 %d (경고 %d · 애매 %d · 조용 %d) · %s\n",
		len(run.rows), judged, len(warn), len(amb), quiet, elapsed(run.started))
	if oc.ExitErr != "" || oc.Unread > 0 || judged < len(oc.Verdicts) {
		fmt.Printf("⚠ 판정기 : %s · 못 읽은 줄 %d · 판정 못 받은 줄 %d\n",
			orDash(oc.ExitErr), oc.Unread, len(oc.Verdicts)-judged)
	}
	fmt.Printf("┌ 경고 (B ≥ %.1f)\n", warnProb)
	printHits(warn)
	fmt.Printf("├ 애매 (%.1f ≤ B < %.1f)\n", ambiguousProb, warnProb)
	printHits(amb)
	printFooter(run)
}

// grade 는 판정을 B 확률로 가른다. judged 는 problem 없이 판정받은 수다.
func grade(run *conflictRun) ([]hit, []hit, int) {
	warn, amb := []hit{}, []hit{}
	judged := 0
	for _, r := range run.rows {
		v, ok := run.outcome.Verdicts[r.ID]
		if !ok || v.Problem != "" {
			continue
		}
		judged++
		p := run.pairs[r.ID]
		h := hit{ID: r.ID, ADoc: p.ADoc, ALine: p.A.Line, BDoc: p.BDoc, BLine: p.B.Line,
			A: p.Evidence(), B: p.Claim(), Prob: v.ProbB()}
		switch {
		case h.Prob >= warnProb:
			warn = append(warn, h)
		case h.Prob >= ambiguousProb:
			amb = append(amb, h)
		}
	}
	sort.SliceStable(warn, func(i, j int) bool { return warn[i].Prob > warn[j].Prob })
	sort.SliceStable(amb, func(i, j int) bool { return amb[i].Prob > amb[j].Prob })
	return warn, amb, judged
}

func printJSON(run *conflictRun, warn, amb []hit, judged int) {
	out := conflictJSON{
		Pairs: len(run.rows), Total: run.total, Judged: judged, Unread: run.outcome.Unread,
		Skipped: run.skipped, Warn: warn, Ambiguous: amb, Sent: run.sent,
		SecretDropped: len(run.dropped), MS: time.Since(run.started).Milliseconds(),
	}
	if out.Skipped == "" && len(run.outcome.Verdicts) == 0 {
		out.Skipped = "판정 실패 — " + run.outcome.FailReason()
	}
	if out.Warn == nil {
		out.Warn = []hit{}
	}
	if out.Ambiguous == nil {
		out.Ambiguous = []hit{}
	}
	raw, _ := json.Marshal(out)
	fmt.Println(string(raw))
}

func printHits(hits []hit) {
	for _, h := range hits {
		fmt.Printf("│ %s:%d ↔ %s:%d   B %.2f\n", h.ADoc, h.ALine, h.BDoc, h.BLine, h.Prob)
		fmt.Printf("│   A : 「%s」\n", cut(h.A, showRunes))
		fmt.Printf("│   B : 「%s」\n", cut(h.B, showRunes))
	}
}

// printList 는 후보 쌍을 한 줄씩 찍는다 (문서 줄 둘 · 문장 앞 60자).
func printList(run *conflictRun) {
	for _, r := range run.rows {
		p := run.pairs[r.ID]
		fmt.Printf("  %s:%d ↔ %s:%d  겹침 %d\n", p.ADoc, p.A.Line, p.BDoc, p.B.Line, p.Overlap)
		fmt.Printf("    A : %s\n    B : %s\n", cut(p.A.Text, listRunes), cut(p.B.Text, listRunes))
	}
	if run.limited {
		fmt.Printf("  후보 %d개 중 %d개만 판정 (--limit)\n", run.total, len(run.rows))
	}
}

func printFooter(run *conflictRun) {
	fmt.Printf("└ 보낸 것 : %s (비밀 꼴로 뺀 문장 %d)\n", run.sent, len(run.dropped))
}

// cut 은 앞 n 글자만 남기고 「…」 를 붙인다.
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func elapsed(start time.Time) string {
	d := time.Since(start).Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%d초", int(d.Seconds()))
	}
	return fmt.Sprintf("%d분 %02d초", int(d.Minutes()), int(d.Seconds())%60)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
