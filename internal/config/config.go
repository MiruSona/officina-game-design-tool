// Package config 는 단계 정의 기본값과 붙은 저장소의 설정 파일 덮기를 맡는다.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// RelPath 는 설정 파일 자리다. 없어도 된다.
const RelPath = "Docs/Todo/기획설정.json"

// 어긋남 판정기 이름. localharness 는 PATH 의 하네스, nli 는 루프백의 NLI 판정 서버다.
const (
	JudgeByLocalharness = "localharness"
	JudgeByNLI          = "nli"
	DefaultJudgeURL     = "http://127.0.0.1:8092"
)

// Stage 는 단계 하나다.
type Stage struct {
	Number int
	Name   string
	Done   string
}

// Config 는 툴이 쓰는 모든 설정값이다.
type Config struct {
	Stages          []Stage
	DocSkeleton     []string
	DesignDir       string
	PillarMin       int
	PillarMax       int
	AskMin          int
	AskMax          int
	ConceptMaxLines int
	ProtoDir        string
	CodeDir         string
	LoopFrom        int
	LoopTo          int
	DoubtRounds     int
	Conflict        bool   // 어긋남 판정을 바깥 판정기에 보내도 되나. 끄면 후보 쌍까지만 본다
	JudgeBy         string // 어긋남 판정기 (localharness · nli)
	JudgeURL        string // nli 판정 서버 주소. http + 루프백만 받는다
	IterDir         string // 반복 기록 파일을 두는 폴더 (뿌리 기준)
	IterTemplate    string // 게임 자기 반복 틀 (뿌리 기준). 비면 내장 틀
}

// file 은 설정 파일의 모양이다. 적힌 칸만 덮으려고 포인터로 받는다.
type file struct {
	Stages          []string `json:"단계"`
	DocSkeleton     []string `json:"문서뼈대"`
	DesignDir       *string  `json:"기획문서폴더"`
	Pillars         []int    `json:"기둥개수"`
	Asks            []int    `json:"물음개수"`
	ConceptMaxLines *int     `json:"컨셉줄상한"`
	ProtoDir        *string  `json:"프로토타입폴더"`
	CodeDir         *string  `json:"코드폴더"`
	Loop            []int    `json:"고리단계"`
	DoubtRounds     *int     `json:"판의심횟수"`
	Conflict        *bool    `json:"어긋남판정"`
	JudgeBy         *string  `json:"어긋남판정기"`
	JudgeURL        *string  `json:"판정서버"`
	IterDir         *string  `json:"반복기록폴더"`
	IterTemplate    *string  `json:"반복틀"`
}

// Default 는 아무것도 안 정했을 때 쓰는 값이다. 단계 이름과 기준은 절차 정본 「큰 흐름」 표 그대로다.
func Default() Config {
	return Config{
		Stages: []Stage{
			{1, "컨셉", "20초 안에 남에게 말할 수 있다"},
			{2, "기둥 3~4개", "하나를 갈아 끼우면 다른 게임이 된다"},
			{3, "코어 루프", "「행동 → 결과 → 보상 → 다시 행동」이 표로 닫힌다"},
			{4, "물음 묶음과 판정 기준", "물음마다 예/아니오나 숫자로 답할 기준이 있다"},
			{5, "거친 프로토타입 묶음", "각각 5분 동안 손으로 돌아간다"},
			{6, "몰아서 검증", "프로토타입마다 통과 / 불통이 적힌다"},
			{7, "기능 자르기", "Won't 칸이 비어 있지 않다"},
			{8, "세로 조각", "남이 켜서 끝까지 해 본다"},
			{9, "확장", "새 시스템을 안 늘린다"},
		},
		DocSkeleton: []string{
			"00-컨셉.md",
			"01-코어루프.md",
			"02-기능목록.md",
			"03-시스템-*.md",
			"04-수치표.md",
			"05-진행과튜토리얼.md",
		},
		DesignDir:       "Docs/Design",
		PillarMin:       3,
		PillarMax:       4,
		AskMin:          2,
		AskMax:          4,
		ConceptMaxLines: 60,
		ProtoDir:        "Prototypes",
		CodeDir:         "",
		LoopFrom:        3,
		LoopTo:          6,
		DoubtRounds:     3,
		Conflict:        true,
		JudgeBy:         JudgeByLocalharness,
		JudgeURL:        DefaultJudgeURL,
		IterDir:         "Docs/History/반복",
	}
}

// Load 는 기본값을 읽고, 설정 파일이 있으면 적힌 칸만 덮는다.
func Load(root string) (Config, error) {
	cfg := Default()
	path := filepath.Join(root, filepath.FromSlash(RelPath))
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("설정 파일을 못 읽었습니다 (%s) : %v", RelPath, err)
	}
	// PowerShell 의 `Set-Content -Encoding utf8` 이 BOM 을 붙인다. 사람이 손으로 고친 파일이 깨져 보이면 안 된다.
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	var f file
	// 모르는 칸은 오타로 보고 거부한다. 조용히 무시하면 안 먹은 설정을 찾기 어렵다.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return cfg, fmt.Errorf("설정 파일이 깨졌습니다 (%s) : %v", RelPath, err)
	}
	apply(&cfg, f)
	if err := check(cfg, f); err != nil {
		return cfg, fmt.Errorf("설정 파일 값이 잘못됐습니다 (%s) : %v", RelPath, err)
	}
	return cfg, nil
}

// apply 는 설정 파일에 적힌 칸만 덮는다.
func apply(cfg *Config, f file) {
	if len(f.Stages) > 0 {
		stages := make([]Stage, 0, len(f.Stages))
		for i, name := range f.Stages {
			done := ""
			if i < len(cfg.Stages) {
				done = cfg.Stages[i].Done
			}
			stages = append(stages, Stage{Number: i + 1, Name: name, Done: done})
		}
		cfg.Stages = stages
	}
	if len(f.DocSkeleton) > 0 {
		cfg.DocSkeleton = f.DocSkeleton
	}
	if f.DesignDir != nil {
		cfg.DesignDir = *f.DesignDir
	}
	if len(f.Pillars) == 2 {
		cfg.PillarMin, cfg.PillarMax = f.Pillars[0], f.Pillars[1]
	}
	if len(f.Asks) == 2 {
		cfg.AskMin, cfg.AskMax = f.Asks[0], f.Asks[1]
	}
	if f.ConceptMaxLines != nil {
		cfg.ConceptMaxLines = *f.ConceptMaxLines
	}
	if f.ProtoDir != nil {
		cfg.ProtoDir = *f.ProtoDir
	}
	if f.CodeDir != nil {
		cfg.CodeDir = *f.CodeDir
	}
	if len(f.Loop) == 2 {
		cfg.LoopFrom, cfg.LoopTo = f.Loop[0], f.Loop[1]
	}
	if f.DoubtRounds != nil {
		cfg.DoubtRounds = *f.DoubtRounds
	}
	if f.Conflict != nil {
		cfg.Conflict = *f.Conflict
	}
	if f.JudgeBy != nil {
		cfg.JudgeBy = *f.JudgeBy
	}
	if f.JudgeURL != nil {
		cfg.JudgeURL = *f.JudgeURL
	}
	if f.IterDir != nil {
		cfg.IterDir = *f.IterDir
	}
	if f.IterTemplate != nil {
		cfg.IterTemplate = *f.IterTemplate
	}
}

// check 는 덮은 뒤 값이 말이 되는지 본다. 적힌 배열 길이도 여기서 본다.
func check(cfg Config, f file) error {
	if err := checkPairs(f); err != nil {
		return err
	}
	if len(cfg.Stages) < 2 {
		return errors.New("단계는 둘 이상이어야 합니다")
	}
	if cfg.PillarMin < 1 || cfg.PillarMax < cfg.PillarMin {
		return errors.New("기둥개수는 [최소, 최대] 이고 최소가 1 이상이어야 합니다")
	}
	if cfg.AskMin < 1 || cfg.AskMax < cfg.AskMin {
		return errors.New("물음개수는 [최소, 최대] 이고 최소가 1 이상이어야 합니다")
	}
	if cfg.ConceptMaxLines < 1 {
		return errors.New("컨셉줄상한은 1 이상이어야 합니다")
	}
	if cfg.LoopFrom < 1 || cfg.LoopTo < cfg.LoopFrom || cfg.LoopTo > len(cfg.Stages) {
		return errors.New("고리단계는 [시작, 끝] 이고 단계 범위 안이어야 합니다")
	}
	if cfg.DoubtRounds < 1 {
		return errors.New("판의심횟수는 1 이상이어야 합니다")
	}
	if err := CheckJudgeBy(cfg.JudgeBy); err != nil {
		return err
	}
	return CheckJudgeURL(cfg.JudgeURL)
}

// CheckJudgeBy 는 판정기 이름이 아는 값인지 본다. 설정 칸과 `--judge-by` 가 같이 쓴다.
func CheckJudgeBy(name string) error {
	if name == JudgeByLocalharness || name == JudgeByNLI {
		return nil
	}
	return fmt.Errorf("어긋남판정기는 %q 또는 %q 여야 합니다 (지금 %q)", JudgeByLocalharness, JudgeByNLI, name)
}

// CheckJudgeURL 은 판정 서버 주소가 http 이고 호스트가 루프백인지 본다. 기획 글이 이 기계 밖으로 나가지 않게 막는다.
func CheckJudgeURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("판정서버 주소를 못 읽었습니다 (%q) : %v", raw, err)
	}
	if u.Scheme != "http" {
		return fmt.Errorf("판정서버 주소는 http:// 여야 합니다 (지금 %q)", raw)
	}
	if u.User != nil {
		return errors.New("판정서버 주소에 사용자 정보를 넣지 않습니다")
	}
	if u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("판정서버 주소는 http://호스트:포트 까지만 적습니다 (지금 %q)", raw)
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost", "::1":
		return nil
	}
	return fmt.Errorf("판정서버 주소는 루프백(127.0.0.1 · localhost · ::1)만 받습니다 (지금 %q)", raw)
}

// CheckInside 는 설정 경로가 뿌리 기준 상대 경로이고 뿌리 밖으로 안 나가는지 글자로만 본다.
// 설정 읽기에서는 안 막는다 — 반복을 안 쓰는 명령까지 멈추면 안 된다. `stage iter new` 가 부른다.
// 링크를 푼 실제 경로는 부르는 쪽이 한 번 더 본다.
func CheckInside(name, rel string) error {
	slashed := filepath.ToSlash(rel)
	if strings.TrimSpace(slashed) == "" {
		return fmt.Errorf("%s 이 비었습니다", name)
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || strings.HasPrefix(slashed, "/") {
		return fmt.Errorf("%s 은 뿌리 기준 상대 경로여야 합니다 (지금 %q)", name, rel)
	}
	clean := path.Clean(slashed)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%s 이 뿌리 밖으로 나갑니다 (지금 %q)", name, rel)
	}
	return nil
}

// checkPairs 는 [최소, 최대] 꼴 칸이 정말 둘인지 본다. 길이가 다르면 덮기가 조용히 안 먹는다.
func checkPairs(f file) error {
	pairs := []struct {
		name string
		val  []int
	}{
		{"기둥개수", f.Pillars},
		{"물음개수", f.Asks},
		{"고리단계", f.Loop},
	}
	for _, p := range pairs {
		if p.val != nil && len(p.val) != 2 {
			return fmt.Errorf("%s 은 값 두 개짜리 배열이어야 합니다 (지금 %d개)", p.name, len(p.val))
		}
	}
	return nil
}

// StageName 은 번호로 단계 이름을 찾는다. 범위 밖이면 빈 글이다.
func (c Config) StageName(n int) string {
	s, ok := c.Stage(n)
	if !ok {
		return ""
	}
	return s.Name
}

// Stage 는 번호로 단계를 찾는다.
func (c Config) Stage(n int) (Stage, bool) {
	if n < 1 || n > len(c.Stages) {
		return Stage{}, false
	}
	return c.Stages[n-1], true
}

// InLoop 은 그 단계가 되풀이 고리 안인지 본다. 판 번호는 이 안에서만 오른다.
func (c Config) InLoop(n int) bool {
	return n >= c.LoopFrom && n <= c.LoopTo
}
