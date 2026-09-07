// stage 는 기획 단계를 지키게 막고 상태를 한 장으로 굽는 명령이다.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/config"
	"github.com/mirusona/officina-game-design-tool/internal/paths"
	"github.com/mirusona/officina-game-design-tool/internal/state"
)

// Version 은 이 툴의 판이다.
const Version = "0.1.0"

// 종료 코드.
const (
	exitOK      = 0
	exitUsage   = 1
	exitLint    = 2
	exitRead    = 3
	exitWrite   = 4
	exitCorrupt = 5
)

// codedError 는 종료 코드를 달고 다니는 오류다.
type codedError struct {
	code int
	msg  string
}

func (e *codedError) Error() string { return e.msg }

func fail(code int, format string, a ...any) error {
	return &codedError{code: code, msg: fmt.Sprintf(format, a...)}
}

// repeated 는 여러 번 쓸 수 있는 문자열 옵션이다 (--crit).
type repeated []string

func (r *repeated) String() string { return strings.Join(*r, " · ") }

func (r *repeated) Set(v string) error {
	*r = append(*r, v)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printHelp("")
		return exitUsage
	}
	cmd := args[0]
	rest := args[1:]
	var err error
	switch cmd {
	case "init":
		err = cmdInit(rest)
	case "show":
		err = cmdShow(rest)
	case "next":
		err = cmdNext(rest)
	case "back":
		err = cmdBack(rest)
	case "ask":
		err = cmdAsk(rest)
	case "check":
		err = cmdCheck(rest)
	case "lint":
		err = cmdLint(rest)
	case "done":
		err = cmdDone(rest)
	case "dash":
		err = cmdDash(rest)
	case "version", "--version", "-v":
		fmt.Println("stage " + Version)
		return exitOK
	case "help", "--help", "-h":
		topic := ""
		if len(rest) > 0 {
			topic = rest[0]
		}
		printHelp(topic)
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "모르는 명령 : %s\n", cmd)
		printHelp("")
		return exitUsage
	}
	if err == nil {
		return exitOK
	}
	var ce *codedError
	if errors.As(err, &ce) {
		fmt.Fprintln(os.Stderr, ce.msg)
		return ce.code
	}
	if errors.Is(err, flag.ErrHelp) {
		return exitUsage
	}
	fmt.Fprintln(os.Stderr, err.Error())
	return exitRead
}

// newFlags 는 모르는 옵션이 오면 바로 실패하는 플래그 묶음을 만든다.
func newFlags(name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := fs.String("root", "", "저장소 뿌리 (기본 : 지금 폴더)")
	return fs, root
}

// parseFlags 는 옵션을 읽는다. Go 의 flag 은 첫 인자에서 멈추므로 옵션을 앞으로 모아 준다.
func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(reorder(fs, args)); err != nil {
		return fail(exitUsage, "옵션이 잘못됐습니다 (stage %s)", fs.Name())
	}
	return nil
}

// reorder 는 옵션을 앞으로, 인자를 뒤로 모은다. 값이 붙는 옵션은 다음 낱말까지 같이 옮긴다.
func reorder(fs *flag.FlagSet, args []string) []string {
	opts := []string{}
	rest := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !isFlagToken(a) {
			rest = append(rest, a)
			continue
		}
		opts = append(opts, a)
		if strings.Contains(a, "=") || !needsValue(fs, a) {
			continue
		}
		if i+1 < len(args) {
			opts = append(opts, args[i+1])
			i++
		}
	}
	return append(opts, rest...)
}

// isFlagToken 은 옵션처럼 생긴 낱말인지 본다. 음수 숫자는 옵션이 아니다.
func isFlagToken(a string) bool {
	if len(a) < 2 || a[0] != '-' {
		return false
	}
	return !(a[1] >= '0' && a[1] <= '9')
}

// needsValue 는 그 옵션이 값을 따로 받는지 본다. 참/거짓 옵션은 안 받는다.
func needsValue(fs *flag.FlagSet, token string) bool {
	name := strings.TrimLeft(token, "-")
	f := fs.Lookup(name)
	if f == nil {
		return false
	}
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	if ok && b.IsBoolFlag() {
		return false
	}
	return true
}

// openRoot 는 뿌리와 설정을 함께 연다.
func openRoot(rootFlag string) (string, config.Config, error) {
	root, err := paths.Root(rootFlag)
	if err != nil {
		return "", config.Config{}, fail(exitRead, "%v", err)
	}
	cfg, err := config.Load(root)
	if err != nil {
		return "", config.Config{}, fail(exitCorrupt, "%v", err)
	}
	return root, cfg, nil
}

// loadState 는 상태 파일을 연다. 없으면 init 을 안내한다.
func loadState(root string) (*state.State, error) {
	st, err := state.Load(root)
	if errors.Is(err, state.ErrMissing) {
		return nil, fail(exitRead, "%s 이 없습니다. 먼저 `stage init` 을 치세요.", state.RelPath)
	}
	if err != nil {
		return nil, fail(exitCorrupt, "%v", err)
	}
	return st, nil
}

// saveState 는 상태 파일을 쓴다.
func saveState(root string, st *state.State, now time.Time) error {
	if err := state.Save(root, st, now); err != nil {
		return fail(exitWrite, "%v", err)
	}
	return nil
}
