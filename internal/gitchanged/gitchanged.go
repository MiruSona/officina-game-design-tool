// Package gitchanged 는 git 이 본 변경 파일을 모은다.
// git 을 인자 배열로 부르고(셸을 안 거친다), 돌려받은 이름은 자료로만 쓴다.
package gitchanged

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"time"
)

// Wait 는 git 한 번을 기다려 주는 한도다. 훅에서도 부르므로 짧게 둔다.
const Wait = 10 * time.Second

// Entry 는 변경 파일 하나다. Path 는 **git 뿌리 기준** 슬래시 경로다.
type Entry struct {
	Path    string
	New     bool
	Deleted bool
}

// Load 는 뿌리 아래 변경분과 git 뿌리에서 뿌리까지의 앞머리를 돌려준다.
func Load(root string) ([]Entry, string, error) {
	prefix, err := run(root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, "", err
	}
	raw, err := run(root, "status", "--porcelain=v1", "-z", "--untracked-files=all",
		"--ignore-submodules=all", "--no-renames", "--", ".")
	if err != nil {
		return nil, "", err
	}
	return Parse(raw), strings.TrimSpace(string(prefix)), nil
}

// LoadSince 는 <ref> 에서 HEAD 까지 바뀐 파일을 모은다. **이미 커밋된 판**을 볼 때 쓴다.
func LoadSince(root, ref string) ([]Entry, string, error) {
	prefix, err := run(root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, "", err
	}
	raw, err := run(root, "diff", "--name-status", "--no-renames", "-z", ref+"...HEAD")
	if err != nil {
		return nil, "", err
	}
	return ParseDiff(raw), strings.TrimSpace(string(prefix)), nil
}

// ParseDiff 는 `diff --name-status -z` 출력을 자른다. 「딱지\x00경로」가 한 짝이다.
func ParseDiff(raw []byte) []Entry {
	out := []Entry{}
	toks := strings.Split(string(raw), "\x00")
	for i := 0; i+1 < len(toks); i += 2 {
		status, p := strings.TrimSpace(toks[i]), toks[i+1]
		if status == "" || p == "" {
			continue
		}
		e := Entry{Path: path.Clean(strings.ReplaceAll(p, "\\", "/"))}
		switch status[0] {
		case 'D':
			e.Deleted = true
		case 'A':
			e.New = true
		}
		out = append(out, e)
	}
	return out
}

// Parse 는 `status --porcelain=v1 -z` 출력을 자른다. 순수 함수다 — 파일도 git 도 안 만진다.
func Parse(raw []byte) []Entry {
	out := []Entry{}
	toks := strings.Split(string(raw), "\x00")
	for i := 0; i < len(toks); i++ {
		rec := toks[i]
		if len(rec) < 4 || rec[2] != ' ' {
			continue
		}
		x, y := rec[0], rec[1]
		// 이름 바꾸기·복사는 「새 이름\x00옛 이름」 두 토막이다. 옛 이름은 버린다.
		if x == 'R' || y == 'R' || x == 'C' || y == 'C' {
			i++
		}
		e := Entry{Path: path.Clean(strings.ReplaceAll(rec[3:], "\\", "/"))}
		if x == 'D' || y == 'D' {
			e.Deleted = true
		}
		if !e.Deleted && (x == 'A' || x == '?') {
			e.New = true
		}
		out = append(out, e)
	}
	return out
}

// run 은 git 한 번을 돌린다. 셸을 안 거치고, 잠금 파일을 안 건드리게 해 둔다.
func run(root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Wait)
	defer cancel()
	full := append([]string{"-C", root}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(cmd.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("git 이 %s 안에 안 끝났습니다", Wait)
	}
	if err != nil {
		return nil, fmt.Errorf("git %s 실패 : %v%s", args[0], err, tail(errBuf.String()))
	}
	return out, nil
}

// tail 은 git 이 낸 까닭 한 줄을 붙인다. 없으면 빈 글이다.
func tail(s string) string {
	line := strings.TrimSpace(s)
	if line == "" {
		return ""
	}
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return " — " + line
}
