package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/config"
	"github.com/mirusona/officina-game-design-tool/internal/iter"
	"github.com/mirusona/officina-game-design-tool/internal/paths"
	"github.com/mirusona/officina-game-design-tool/internal/state"
	"github.com/mirusona/officina-game-design-tool/templates"
)

// saveIterState 는 상태 저장이다. 시험이 저장 실패를 흉내 내려고 바꿔 끼운다.
var saveIterState = saveState

const iterUsage = "쓰는 법 : stage iter new [--played <판>] [--title \"<한 줄>\"]"

// cmdIter 는 마지막 단계 뒤 반복을 다룬다. 이번 판 하위명령은 new 하나다.
func cmdIter(args []string) error {
	if len(args) == 0 || args[0] != "new" {
		return fail(exitUsage, "%s", iterUsage)
	}
	return cmdIterNew(args[1:])
}

// cmdIterNew 는 반복 하나를 연다 — 번호 매기기 · 틀 채워 기록 파일 쓰기 · 상태 파일에 한 줄.
func cmdIterNew(args []string) error {
	fs, root := newFlags("iter new")
	played := fs.String("played", "", "이번에 놀았던 판 (없으면 지난 올린판 → 지난 놀았던판 → ?)")
	title := fs.String("title", "", "이번 반복 한 줄")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if strings.ContainsAny(*played+*title, "\r\n") {
		return fail(exitUsage, "--played · --title 은 한 줄로 적습니다")
	}
	if fs.NArg() != 0 {
		return fail(exitUsage, "%s", iterUsage)
	}
	dir, cfg, err := openRoot(*root)
	if err != nil {
		return err
	}
	st, err := loadState(dir)
	if err != nil {
		return err
	}
	last := len(cfg.Stages)
	if st.Stage != last {
		return fail(exitUsage, "반복은 %d단계 「%s」 뒤에 합니다 (지금 %d단계)", last, cfg.StageName(last), st.Stage)
	}
	recDir, err := iterPath(dir, "반복기록폴더", cfg.IterDir)
	if err != nil {
		return err
	}
	tpl, err := iterTemplate(dir, cfg)
	if err != nil {
		return err
	}
	maxNo, prevName, err := scanIterDir(recDir)
	if err != nil {
		return fail(exitRead, "%v", err)
	}
	prev := st.LastIter()
	no := maxNo
	if prev != nil && prev.No > no {
		no = prev.No
	}
	no++

	relDir := path.Clean(filepath.ToSlash(cfg.IterDir))
	carried := []string{}
	other := 0
	source := ""
	if prevName != "" {
		raw, readErr := os.ReadFile(filepath.Join(recDir, prevName))
		if readErr != nil {
			return fail(exitRead, "지난 기록을 못 읽었습니다 (%s) : %v", prevName, readErr)
		}
		carried, other = iter.Carry(string(raw), tpl)
		prevNo, _ := iter.FileNo(prevName)
		source = fmt.Sprintf("반복 %02d 기록 (%s)", prevNo, path.Join(relDir, prevName))
	}

	playedVal := strings.TrimSpace(*played)
	if playedVal == "" {
		playedVal = defaultPlayed(prev)
	}
	now := time.Now()
	today := now.Format("2006-01-02")
	name := fmt.Sprintf("%02d-%s.md", no, today)
	body := iter.Fill(tpl, map[string]string{
		"{{회}}":       fmt.Sprintf("%02d", no),
		"{{날짜}}":      today,
		"{{놀았던판}}":    playedVal,
		"{{한줄}}":      strings.TrimSpace(*title),
		iter.CarryKey: iter.CarryBlock(carried, other, source),
	})

	// 지난 반복을 닫기 전에 빠진 것을 본다. 막지는 않는다.
	if prev != nil {
		for _, m := range iterTodos(dir, *prev, tpl, false) {
			fmt.Fprintf(os.Stderr, "⚠ 반복 %02d : %s → %s\n", prev.No, m.name, m.fix)
		}
	}

	filePath := filepath.Join(recDir, name)
	if err := writeNew(filePath, body); err != nil {
		return err
	}
	if prev != nil && prev.Shipped == "" && strings.TrimSpace(*played) != "" {
		prev.Shipped = playedVal
	}
	rel := path.Join(relDir, name)
	st.Iters = append(st.Iters, state.Iter{No: no, Start: today, Played: playedVal, Title: strings.TrimSpace(*title), File: rel})
	if err := saveIterState(dir, st, now); err != nil {
		// 반쪽 상태를 안 남긴다 — 상태에 안 적힌 기록 파일은 지운다.
		_ = os.Remove(filePath)
		return err
	}
	fmt.Printf("반복 %02d 열림 — %s · 놀았던 판 %s · 지난 반복에서 미룬 것 %d줄 옮김\n", no, rel, playedVal, len(carried))
	return nil
}

// defaultPlayed 는 --played 가 없을 때 놀았던 판을 정한다. 지난 올린판 → 지난 놀았던판 → ?.
func defaultPlayed(prev *state.Iter) string {
	if prev != nil && prev.Shipped != "" {
		return prev.Shipped
	}
	if prev != nil && prev.Played != "" {
		return prev.Played
	}
	return "?"
}

// writeNew 는 새 파일만 쓴다. 같은 이름이 있으면 덮지 않고 멈춘다.
func writeNew(filePath, body string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fail(exitWrite, "폴더를 못 만들었습니다 (%s) : %v", filepath.Dir(filePath), err)
	}
	// O_EXCL 이 정본이다. 폴더가 그 이름이면 Windows 는 ErrExist 대신 접근 거부를 내므로 Lstat 으로 먼저 본다.
	_, statErr := os.Lstat(filePath)
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil && (statErr == nil || errors.Is(err, fs.ErrExist)) {
		return fail(exitUsage, "같은 이름의 기록 파일이 이미 있습니다 : %s — 덮어쓰지 않습니다", filepath.Base(filePath))
	}
	if err != nil {
		return fail(exitWrite, "기록 파일을 못 만들었습니다 (%s) : %v", filePath, err)
	}
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		_ = os.Remove(filePath)
		return fail(exitWrite, "기록 파일을 못 썼습니다 (%s) : %v", filePath, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(filePath)
		return fail(exitWrite, "기록 파일을 못 썼습니다 (%s) : %v", filePath, err)
	}
	return nil
}

// iterPath 는 설정 경로를 뿌리 안 실제 경로로 바꾼다. 글자로 한 번, 링크를 푼 실제 경로로 한 번 더 본다.
func iterPath(root, name, rel string) (string, error) {
	if err := config.CheckInside(name, rel); err != nil {
		return "", fail(exitUsage, "%v", err)
	}
	abs := paths.Join(root, rel)
	if _, err := paths.Rel(root, abs); err != nil {
		return "", fail(exitUsage, "%s : %v", name, err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fail(exitRead, "뿌리를 못 폈습니다 : %v", err)
	}
	real, err := realPath(abs)
	if err != nil {
		return "", fail(exitUsage, "%s 경로를 못 폈습니다 : %v", name, err)
	}
	if !underReal(realRoot, real) {
		return "", fail(exitUsage, "%s 이 링크를 따라 뿌리 밖으로 나갑니다 : %s", name, rel)
	}
	return real, nil
}

// iterTemplate 은 반복 틀 글을 읽는다. `반복틀` 이 비면 내장 틀이다.
func iterTemplate(root string, cfg config.Config) (string, error) {
	if cfg.IterTemplate == "" {
		raw, err := templates.FS.ReadFile("iter.md")
		if err != nil {
			return "", fail(exitRead, "내장 반복 틀을 못 읽었습니다 : %v", err)
		}
		return string(raw), nil
	}
	p, err := iterPath(root, "반복틀", cfg.IterTemplate)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return "", fail(exitRead, "반복틀을 못 읽었습니다 (%s) : %v", cfg.IterTemplate, err)
	}
	return string(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})), nil
}

// scanIterDir 는 기록 폴더에서 가장 큰 번호와 그 파일 이름을 찾는다. 00 은 틀 자리라 뺀다. 폴더가 없으면 0 이다.
func scanIterDir(dir string) (int, string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("반복 기록 폴더를 못 읽었습니다 (%s) : %v", dir, err)
	}
	maxNo, latest := 0, ""
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		n, ok := iter.FileNo(e.Name())
		if !ok || n == 0 {
			continue
		}
		if n > maxNo || (n == maxNo && e.Name() > latest) {
			maxNo, latest = n, e.Name()
		}
	}
	return maxNo, latest, nil
}

// iterTodos 는 반복 하나의 기록 파일에서 빠진 것을 모은다.
// fromDone 이면 「고친 것」 에 내용이 생긴 뒤에만 보고(반복 중 매 Stop 잔소리를 막는다) 「기억 1건」 을 붙인다.
func iterTodos(root string, it state.Iter, tpl string, fromDone bool) []todo {
	raw, err := os.ReadFile(paths.Join(root, it.File))
	if err != nil {
		return []todo{{"기록 파일이 없습니다 — " + it.File, "다시 만들거나 상태 파일의 `기록파일` 을 고칩니다"}}
	}
	doc := string(raw)
	if fromDone && !iter.HasContent(iter.Section(doc, iter.SecFixed), tpl) {
		return nil
	}
	missing := []todo{}
	skip := []string{}
	for _, sec := range []string{iter.SecDeferred, iter.SecHanded} {
		if !iter.HasContent(iter.Section(doc, sec), tpl) {
			missing = append(missing, todo{"「" + sec + "」", "없으면 「- 없음」"})
			skip = append(skip, sec)
		}
	}
	// 위에서 빈 절로 이미 알린 절의 자리표는 또 세지 않는다.
	if left := iter.Leftovers(doc, tpl, skip...); len(left) > 0 {
		missing = append(missing, todo{fmt.Sprintf("틀 자리표가 남았습니다 (%d줄)", len(left)), "예 : " + left[0]})
	}
	if fromDone {
		missing = append(missing, memoryTodo)
	}
	return missing
}
