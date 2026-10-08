package conflict

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// JudgeName 은 PATH 에서 찾는 바깥 판정기 이름이다. 코드·경로로 묶지 않고 이름으로만 안다.
const JudgeName = "localharness"

// 판정기에 넘기는 물음 종류. conflict 는 글자 넷(A 지지 · B 어긋남 · C 무관 · D 정리 필요), support 는 셋이다 (설계 14절).
const (
	KindConflict = "conflict"
	KindSupport  = "support"
)

// 판정 한 번의 시간 제한 눈금. 쌍 하나 1.2초 실측(조사 0절)에 여유를 둔 값이다.
const (
	MaxWait     = 5 * time.Minute
	WaitPerPair = 6 * time.Second
	WaitBase    = 30 * time.Second
)

// Row 는 판정기에 보내는 jsonl 한 줄이다. 네 칸뿐이다 — 모르는 칸을 판정기가 거부할 수 있다.
type Row struct {
	ID       string `json:"id"`
	Evidence string `json:"evidence"`
	Claim    string `json:"claim"`
	Want     string `json:"want"`
}

// RowOf 는 짝 하나를 jsonl 줄 꼴로 만든다.
func RowOf(p Pair) Row {
	return Row{ID: p.ID(), Evidence: p.Evidence(), Claim: p.Claim()}
}

// Verdict 는 판정기 stdout 한 줄에서 읽는 칸이다. 나머지 칸은 안 본다.
type Verdict struct {
	ID      string             `json:"id"`
	Letter  string             `json:"letter"`
	Prob    float64            `json:"prob"`
	Probs   map[string]float64 `json:"probs"`
	MS      int64              `json:"ms"`
	Problem string             `json:"problem"`
}

// ProbB 는 「반대(B)」 확률이다. probs 가 있으면 그것을, 없으면 letter 가 B 일 때의 prob 을 쓴다.
func (v Verdict) ProbB() float64 {
	return v.probOf("B")
}

// ProbD 는 「정리 필요(D)」 확률이다. support 물음엔 D 가 없어 0 이다.
func (v Verdict) ProbD() float64 {
	return v.probOf("D")
}

func (v Verdict) probOf(letter string) float64 {
	if p, ok := v.Probs[letter]; ok {
		return p
	}
	if v.Letter == letter {
		return v.Prob
	}
	return 0
}

// Outcome 은 판정기를 한 번 돌린 결과다. Verdicts 는 id 로 찾는다.
type Outcome struct {
	Verdicts    map[string]Verdict
	Unread      int    // JSON 으로 못 읽은 stdout 줄 수 (같은 id 가 두 번 온 줄도 센다)
	FirstUnread string // 못 읽은 첫 줄 앞 120자. 판정기가 JSON 대신 「판정 건너뜀 : …」 을 찍었을 때 까닭이 된다
	ExitErr     string // 종료 코드 ≠ 0 이거나 시간이 넘었을 때의 까닭. 읽힌 줄이 있으면 참고용이다
	Accepted    []byte // 읽힌 판정 JSON 줄만 모은 것 (결과 파일용). JSON 아닌 줄은 안 들어간다
	UnknownKind bool   // 판정기가 --kind 값을 몰라 종료 1 — 옛 하네스다. support 로 한 번 되돌아 묻는다
}

// unknownKindMark 는 하네스가 모르는 --kind 를 받았을 때 stderr 에 찍는 글의 머리다.
const unknownKindMark = "--kind 는"

// skipPrefix 는 판정기가 설정 없이 그냥 넘어갈 때 stdout 에 찍는 머리글이다 (하네스 설계 A 절).
const skipPrefix = "판정 건너뜀"

// SkipReason 은 판정기가 「판정 건너뜀 : …」 한 줄만 찍고 종료 0 했을 때의 까닭이다. 아니면 빈 글이다.
// 이것은 실패가 아니라 건너뜀이다 — 설정이 없는 기계에서도 명령이 「실패」로 보이면 안 된다.
func (o Outcome) SkipReason() string {
	if o.ExitErr != "" || len(o.Verdicts) > 0 || !strings.HasPrefix(o.FirstUnread, skipPrefix) {
		return ""
	}
	reason := strings.TrimPrefix(o.FirstUnread, skipPrefix)
	if _, after, ok := strings.Cut(reason, " : "); ok {
		reason = after
	}
	reason = strings.TrimSpace(strings.TrimLeft(reason, " :"))
	if reason == "" {
		return "판정기가 까닭 없이 건너뜀"
	}
	return reason
}

// FailReason 은 읽힌 판정이 하나도 없을 때의 까닭이다. 종료 까닭이 없으면 못 읽은 첫 줄을 쓴다.
func (o Outcome) FailReason() string {
	if o.ExitErr != "" {
		return o.ExitErr
	}
	if o.FirstUnread != "" {
		return "판정기 출력 : " + o.FirstUnread
	}
	return "판정기가 아무 줄도 안 찍음"
}

// Lookup 은 PATH 에서 판정기를 찾는다. 못 찾거나 같은 이름이 둘 이상이면 두 번째 값에 까닭을 적는다.
func Lookup() (string, string) {
	found := candidates()
	if len(found) == 0 {
		return "", JudgeName + " 가 PATH 에 없음"
	}
	if len(found) > 1 {
		// 그림자 exe — 어느 것이 불릴지 모르면 안 부른다 (보안 불변조건 9).
		return "", fmt.Sprintf("PATH 에 %s 가 %d개", JudgeName, len(found))
	}
	// 센 그 파일을 그대로 부른다. 다시 찾으면 센 것과 다른 것이 불릴 수 있다.
	return found[0], ""
}

// candidates 는 PATH 폴더마다 판정기 이름의 실행 파일이 있는지 센다. 같은 폴더에 .exe·.cmd 가 둘이면 둘로 센다.
func candidates() []string {
	exts := []string{""}
	if runtime.GOOS == "windows" {
		exts = windowsExts()
	}
	out := []string{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		for _, ext := range exts {
			p := filepath.Join(dir, JudgeName+ext)
			info, err := os.Stat(p)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
				continue
			}
			out = append(out, p)
		}
	}
	return out
}

// windowsExts 는 PATHEXT 의 확장자다. 없으면 흔한 넷이다.
func windowsExts() []string {
	raw := os.Getenv("PATHEXT")
	if raw == "" {
		return []string{".exe", ".cmd", ".bat", ".com"}
	}
	out := []string{}
	for _, e := range strings.Split(raw, ";") {
		if e != "" {
			out = append(out, strings.ToLower(e))
		}
	}
	return out
}

// Wait 는 쌍 수에 맞춘 시간 제한이다 : min(5분, 쌍 × 6초 + 30초).
func Wait(pairs int) time.Duration {
	d := time.Duration(pairs)*WaitPerPair + WaitBase
	if d > MaxWait {
		return MaxWait
	}
	return d
}

// Run 은 판정기를 인자 배열로 부른다 (셸 안 거침). 종료 코드가 0 이 아니어도 읽힌 줄은 돌려준다.
func Run(exe, workDir, file, kind string, wait time.Duration, fresh bool) Outcome {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	args := []string{"judge", "--kind", kind, "--file", file, "--json"}
	if fresh {
		args = append(args, "--fresh")
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = workDir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	oc := parseVerdicts(outBuf.Bytes())
	exitMsg := ""
	if ctx.Err() != nil {
		exitMsg = fmt.Sprintf("판정기가 %s 안에 안 끝났습니다", wait)
	} else if err != nil {
		exitMsg = fmt.Sprintf("판정기 종료 %v%s", err, lastLine(errBuf.String()))
		oc.UnknownKind = isUnknownKind(err, errBuf.String())
	}
	oc.ExitErr = joinReasons(exitMsg, oc.ExitErr)
	return oc
}

// isUnknownKind 는 「종료 1 + stderr 마지막 줄에 `--kind 는`」 일 때만 참이다. 다른 까닭의 종료 1 로 헛되이 두 번 돌지 않게 좁힌다.
func isUnknownKind(err error, stderr string) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		return false
	}
	return strings.Contains(lastLine(stderr), unknownKindMark)
}

// parseVerdicts 는 stdout 을 줄마다 JSON 으로 읽는다. 모르는 줄은 센다.
func parseVerdicts(raw []byte) Outcome {
	oc := Outcome{Verdicts: map[string]Verdict{}}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var v Verdict
		if err := json.Unmarshal([]byte(line), &v); err != nil || v.ID == "" {
			oc.Unread++
			if oc.FirstUnread == "" {
				oc.FirstUnread = head(line, 120)
			}
			continue
		}
		if _, dup := oc.Verdicts[v.ID]; dup {
			// 같은 id 두 줄은 첫 것을 믿는다. 두 번째는 못 읽은 줄로 센다.
			oc.Unread++
			continue
		}
		oc.Verdicts[v.ID] = v
		oc.Accepted = append(oc.Accepted, line...)
		oc.Accepted = append(oc.Accepted, '\n')
	}
	if err := sc.Err(); err != nil {
		oc.ExitErr = "stdout 을 끝까지 못 읽음 : " + err.Error()
	}
	return oc
}

// joinReasons 는 빈 것을 빼고 「 · 」 로 잇는다.
func joinReasons(parts ...string) string {
	kept := []string{}
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " · ")
}

// head 는 앞 n 글자다.
func head(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// lastLine 은 stderr 의 마지막 줄을 「 — 」 뒤에 붙인다. 없으면 빈 글이다.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" {
		return ""
	}
	return " — " + last
}
