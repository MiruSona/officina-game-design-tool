package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/config"
	"github.com/mirusona/officina-game-design-tool/internal/lint"
	"github.com/mirusona/officina-game-design-tool/internal/render"
	"github.com/mirusona/officina-game-design-tool/internal/state"
)

// cmdInit 은 상태 파일을 새로 만든다.
func cmdInit(args []string) error {
	fs, root := newFlags("init")
	force := fs.Bool("force", false, "이미 있어도 덮어쓴다")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	dir, cfg, err := openRoot(*root)
	if err != nil {
		return err
	}
	if _, err := state.Load(dir); err == nil && !*force {
		return fail(exitUsage, "%s 이 이미 있습니다. 덮어쓰려면 --force 를 붙이세요.", state.RelPath)
	}
	now := time.Now()
	st := state.New(now)
	if err := saveState(dir, st, now); err != nil {
		return err
	}
	fmt.Printf("%s 을 만들었습니다 — 1단계 「%s」 · 1판.\n", state.RelPath, cfg.StageName(1))
	return nil
}

// showJSON 은 --json 으로 내보내는 모양이다. 셈한 값을 얹어 준다.
type showJSON struct {
	*state.State
	StageName string `json:"단계이름"`
	StageDone string `json:"끝난기준"`
	Days      int    `json:"이번판며칠째"`
	BackCount int    `json:"되돌아간횟수"`
	Pass      int    `json:"통과"`
	Fail      int    `json:"불통"`
	Rest      int    `json:"남음"`
	Doubt     bool   `json:"컨셉의심"`
}

// cmdShow 는 지금 단계와 이번 판 물음을 보여준다. 아무것도 안 고친다.
func cmdShow(args []string) error {
	fs, root := newFlags("show")
	asJSON := fs.Bool("json", false, "JSON 으로 내보낸다")
	hook := fs.Bool("hook", false, "세션 훅에서 부른다 (상태 파일이 없어도 종료 0)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	dir, cfg, err := openRoot(*root)
	if err != nil {
		return err
	}
	st, err := state.Load(dir)
	if errors.Is(err, state.ErrMissing) {
		if *hook {
			fmt.Printf("아직 %s 이 없습니다 — `stage init` 으로 기획 단계 기록을 시작하세요.\n", state.RelPath)
			return nil
		}
		return fail(exitRead, "%s 이 없습니다. 먼저 `stage init` 을 치세요.", state.RelPath)
	}
	if err != nil {
		if *hook {
			fmt.Printf("기획 상태를 못 읽었습니다 : %v\n", err)
			return nil
		}
		return fail(exitCorrupt, "%v", err)
	}
	if *asJSON {
		return printShowJSON(cfg, st)
	}
	printShow(cfg, st, time.Now())
	return nil
}

// printShowJSON 은 셈한 값을 얹어 JSON 으로 찍는다.
func printShowJSON(cfg config.Config, st *state.State) error {
	pass, fail2, rest := st.Counts()
	out := showJSON{
		State:     st,
		StageName: cfg.StageName(st.Stage),
		Days:      st.DaysIn(time.Now()),
		BackCount: len(st.Backs),
		Pass:      pass,
		Fail:      fail2,
		Rest:      rest,
		Doubt:     st.Doubt(cfg),
	}
	if s, ok := cfg.Stage(st.Stage); ok {
		out.StageDone = s.Done
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fail(exitWrite, "JSON 으로 못 바꿨습니다 : %v", err)
	}
	fmt.Println(string(raw))
	return nil
}

// printShow 는 사람이 읽는 꼴로 찍는다. 세션 훅이 이 글을 문맥에 넣는다.
func printShow(cfg config.Config, st *state.State, now time.Time) {
	fmt.Printf("지금 %d단계 「%s」 · %d판 · %d일째\n", st.Stage, cfg.StageName(st.Stage), st.Round, st.DaysIn(now))
	if s, ok := cfg.Stage(st.Stage); ok {
		fmt.Printf("끝났다고 보는 기준 : %s\n", s.Done)
	}
	asks := st.RoundAsks()
	pass, fail2, rest := st.Counts()
	fmt.Println()
	if len(asks) == 0 {
		fmt.Println("이번 판 물음이 아직 없습니다 — `stage ask \"<물음>\" --crit \"…\"` 로 적습니다.")
	}
	if len(asks) > 0 {
		fmt.Printf("이번 판 물음 %d개 — 통과 %d · 불통 %d · 남음 %d\n", len(asks), pass, fail2, rest)
		width := 0
		for _, a := range asks {
			if w := render.Width(a.ID); w > width {
				width = w
			}
		}
		for _, a := range asks {
			fmt.Printf("  %s %s  %s\n", render.Badge(a.Status), render.Pad(a.ID, width), a.Question)
		}
	}
	fmt.Println()
	fmt.Printf("되돌아간 횟수 %d회.\n", len(st.Backs))
	if st.Doubt(cfg) {
		fmt.Printf("%d판째입니다 — 이번에도 통과 못 하면 컨셉이나 기둥을 의심할 때입니다.\n", st.Round)
	}
	fmt.Println("되돌아가는 조건 : 모양이 틀렸으면 3단계 · 숫자만 틀렸으면 5단계 · 기준이 틀렸으면 4단계.")
	fmt.Println("단계를 넘기는 것은 사용자만 합니다.")
}

// cmdNext 는 다음 단계로 넘긴다. 기준은 되뇌기만 하고 막지 않는다.
func cmdNext(args []string) error {
	fs, root := newFlags("next")
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
	was := st.Stage
	if s, ok := cfg.Stage(was); ok {
		fmt.Printf("%d단계 「%s」 를 끝냅니다 — 기준 : %s\n", was, s.Name, s.Done)
	}
	warnAll(lint.Run(lint.Input{Root: dir, Cfg: cfg, State: st}))
	if err := st.Next(cfg); err != nil {
		return fail(exitUsage, "%v", err)
	}
	now := time.Now()
	if err := saveState(dir, st, now); err != nil {
		return err
	}
	fmt.Printf("%d단계 → %d단계 「%s」 로 넘어갑니다.\n", was, st.Stage, cfg.StageName(st.Stage))
	return nil
}

// cmdBack 은 앞 단계로 되돌아가고 까닭을 남긴다.
func cmdBack(args []string) error {
	fs, root := newFlags("back")
	why := fs.String("why", "", "되돌아가는 까닭 (반드시 적는다)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fail(exitUsage, "쓰는 법 : stage back <단계> --why \"까닭\"")
	}
	to, convErr := strconv.Atoi(fs.Arg(0))
	if convErr != nil {
		return fail(exitUsage, "단계는 숫자로 적습니다 : %s", fs.Arg(0))
	}
	dir, cfg, err := openRoot(*root)
	if err != nil {
		return err
	}
	st, err := loadState(dir)
	if err != nil {
		return err
	}
	was := st.Stage
	now := time.Now()
	if err := st.GoBack(to, *why, cfg, now); err != nil {
		return fail(exitUsage, "%v", err)
	}
	if err := saveState(dir, st, now); err != nil {
		return err
	}
	fmt.Printf("%d단계 → %d단계 「%s」 로 되돌아갑니다. %d판 시작 (%s).\n", was, st.Stage, cfg.StageName(st.Stage), st.Round, st.RoundStart)
	fmt.Printf("까닭 : %s\n", *why)
	if st.Doubt(cfg) {
		fmt.Printf("%d판째입니다 — 이번에도 통과 못 하면 컨셉이나 기둥을 의심할 때입니다.\n", st.Round)
	}
	return nil
}

// cmdAsk 는 이번 판 물음을 하나 적는다.
func cmdAsk(args []string) error {
	fs, root := newFlags("ask")
	var crit repeated
	fs.Var(&crit, "crit", "판정 기준 (여러 번 쓴다)")
	id := fs.String("id", "", "짧은 손잡이 (없으면 물음에서 만든다)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fail(exitUsage, "쓰는 법 : stage ask \"<물음>\" --crit \"기준\" [--crit \"기준\"]")
	}
	dir, cfg, err := openRoot(*root)
	if err != nil {
		return err
	}
	st, err := loadState(dir)
	if err != nil {
		return err
	}
	a, addErr := st.AddAsk(*id, fs.Arg(0), crit, time.Now())
	if addErr != nil {
		return fail(exitUsage, "%v", addErr)
	}
	now := time.Now()
	if err := saveState(dir, st, now); err != nil {
		return err
	}
	fmt.Printf("물음을 적었습니다 — %s : %s (기준 %d줄)\n", a.ID, a.Question, len(a.Crit))
	warnAll(lint.AskCount(lint.Input{Root: dir, Cfg: cfg, State: st}))
	return nil
}

// cmdCheck 는 물음 하나에 통과/불통을 적는다.
func cmdCheck(args []string) error {
	fs, root := newFlags("check")
	note := fs.String("note", "", "무엇을 보고 그렇게 판정했나")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return fail(exitUsage, "쓰는 법 : stage check <물음id> pass|fail --note \"…\"")
	}
	dir, _, err := openRoot(*root)
	if err != nil {
		return err
	}
	st, err := loadState(dir)
	if err != nil {
		return err
	}
	id, value := fs.Arg(0), fs.Arg(1)
	a, checkErr := st.Check(id, value, *note, time.Now())
	if checkErr != nil {
		if st.FindAsk(id) == nil {
			near := st.NearAsks(id, 3)
			if len(near) > 0 {
				return fail(exitUsage, "%v\n이런 id 가 있습니다 : %v", checkErr, near)
			}
		}
		return fail(exitUsage, "%v", checkErr)
	}
	now := time.Now()
	if err := saveState(dir, st, now); err != nil {
		return err
	}
	fmt.Printf("%s → %s : %s\n", a.ID, a.Status, a.Question)
	if value == "fail" && *note == "" {
		fmt.Fprintln(os.Stderr, "⚠ 왜 불통인지 --note 로 적어 두세요. 다음 판에 못 씁니다.")
	}
	if value == "fail" {
		fmt.Println("어디로 되돌아가나 : 고리 모양이 틀렸으면 3단계 · 숫자만 틀렸으면 5단계 · 기준이 틀렸으면 4단계.")
	}
	return nil
}

// warnAll 은 걸린 것을 경고로 찍는다. 막는 것은 여기서 다루지 않는다.
func warnAll(findings []lint.Finding) {
	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "⚠ [%s] %s\n", f.Code, f.Msg)
	}
}
