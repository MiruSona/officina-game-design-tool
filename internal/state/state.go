// Package state 는 상태 파일을 읽고 쓰며 판 번호·되돌아간 기록 규칙을 지킨다.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/config"
)

// RelPath 는 상태 파일 자리다. 이 파일이 정본이다.
const RelPath = "Docs/Todo/기획상태.json"

// Format 은 지금 상태 파일 모양의 판이다.
const Format = 1

// 물음의 상태 값.
const (
	StatusMaking = "만드는중"
	StatusWait   = "대기"
	StatusPass   = "통과"
	StatusFail   = "불통"
)

// ErrMissing 은 상태 파일이 아직 없다는 뜻이다.
var ErrMissing = errors.New("상태 파일이 없습니다")

// Verdict 는 물음 하나에 대한 판정 한 줄이다.
type Verdict struct {
	When  string `json:"언제"`
	Value string `json:"값"`
	Note  string `json:"메모,omitempty"`
}

// Ask 는 이번 판에 시험하는 물음 하나다. 물음·기준·만든날은 프로토타입 meta 와 같은 이름이다.
type Ask struct {
	ID       string    `json:"id"`
	Question string    `json:"물음"`
	Crit     []string  `json:"기준"`
	Made     string    `json:"만든날"`
	Round    int       `json:"판번호"`
	Status   string    `json:"상태"`
	Verdicts []Verdict `json:"판정"`
}

// Back 은 되돌아간 기록 한 줄이다. 몇 판 돌았나를 여기서 센다.
type Back struct {
	When  string `json:"언제"`
	From  int    `json:"어디서"`
	To    int    `json:"어디로"`
	Round int    `json:"판번호"`
	Why   string `json:"까닭"`
}

// State 는 상태 파일 전체다.
type State struct {
	Format     int    `json:"형식"`
	Stage      int    `json:"단계"`
	Round      int    `json:"판번호"`
	RoundStart string `json:"판시작일"`
	Updated    string `json:"고침시각"`
	Asks       []Ask  `json:"물음"`
	Backs      []Back `json:"되돌아간기록"`
}

// Path 는 상태 파일의 실제 경로다.
func Path(root string) string {
	return filepath.Join(root, filepath.FromSlash(RelPath))
}

// New 는 1단계 1판짜리 새 상태를 만든다.
func New(now time.Time) *State {
	return &State{
		Format:     Format,
		Stage:      1,
		Round:      1,
		RoundStart: now.Format("2006-01-02"),
		Updated:    now.Format(time.RFC3339),
		Asks:       []Ask{},
		Backs:      []Back{},
	}
}

// Load 는 상태 파일을 읽는다. 없으면 ErrMissing 이다.
func Load(root string) (*State, error) {
	raw, err := os.ReadFile(Path(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrMissing
	}
	if err != nil {
		return nil, fmt.Errorf("상태 파일을 못 읽었습니다 (%s) : %v", RelPath, err)
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("상태 파일이 깨졌습니다 (%s) : %v", RelPath, err)
	}
	if st.Format != Format {
		return nil, fmt.Errorf("모르는 상태 파일 판입니다 (형식 %d, 이 툴은 %d)", st.Format, Format)
	}
	if st.Asks == nil {
		st.Asks = []Ask{}
	}
	if st.Backs == nil {
		st.Backs = []Back{}
	}
	return &st, nil
}

// Save 는 상태 파일을 쓴다. 폴더가 없으면 만든다.
func Save(root string, st *State, now time.Time) error {
	st.Updated = now.Format(time.RFC3339)
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("상태를 JSON 으로 못 바꿨습니다 : %v", err)
	}
	raw = append(raw, '\n')
	path := Path(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("폴더를 못 만들었습니다 (%s) : %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("상태 파일을 못 썼습니다 (%s) : %v", RelPath, err)
	}
	return nil
}

// Next 는 다음 단계로 넘긴다. 막지 않는다 — 단계를 넘기는 것은 사용자만 한다.
func (s *State) Next(cfg config.Config) error {
	if s.Stage >= len(cfg.Stages) {
		return fmt.Errorf("마지막 단계입니다 (%d단계)", s.Stage)
	}
	s.Stage++
	return nil
}

// GoBack 은 앞 단계로 되돌아가고 기록을 남긴다. 판 번호는 여기서만 오른다.
func (s *State) GoBack(to int, why string, cfg config.Config, now time.Time) error {
	if strings.TrimSpace(why) == "" {
		return errors.New("되돌아가는 까닭을 --why 로 적어야 합니다")
	}
	if to < 1 || to > len(cfg.Stages) {
		return fmt.Errorf("그런 단계가 없습니다 : %d", to)
	}
	if to >= s.Stage {
		return fmt.Errorf("되돌아가는 것은 앞 단계로만 합니다 (지금 %d단계)", s.Stage)
	}
	if to > cfg.LoopTo {
		return fmt.Errorf("되돌아갈 수 있는 자리는 %d~%d단계(고리)나 1~%d단계(컨셉·기둥)입니다", cfg.LoopFrom, cfg.LoopTo, cfg.LoopFrom-1)
	}
	s.Backs = append(s.Backs, Back{
		When:  now.Format(time.RFC3339),
		From:  s.Stage,
		To:    to,
		Round: s.Round,
		Why:   strings.TrimSpace(why),
	})
	if cfg.InLoop(to) {
		s.Round++
	} else {
		s.Round = 1
	}
	s.Stage = to
	s.RoundStart = now.Format("2006-01-02")
	return nil
}

var idBad = regexp.MustCompile(`[^a-z0-9-]+`)

// MakeID 는 물음 글에서 짧은 손잡이를 만든다. 영문·숫자가 없으면 판·차례로 짓는다.
func (s *State) MakeID(question string) string {
	base := idBad.ReplaceAllString(strings.ToLower(question), "-")
	base = strings.Trim(base, "-")
	if len(base) > 24 {
		base = strings.Trim(base[:24], "-")
	}
	if base == "" {
		base = fmt.Sprintf("q%d-%d", s.Round, len(s.RoundAsks())+1)
	}
	id := base
	for n := 2; s.FindAsk(id) != nil; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

// AddAsk 는 이번 판 물음을 하나 더한다.
func (s *State) AddAsk(id, question string, crit []string, now time.Time) (*Ask, error) {
	q := strings.TrimSpace(question)
	if q == "" {
		return nil, errors.New("물음을 한 문장으로 적어야 합니다")
	}
	if len(crit) == 0 {
		return nil, errors.New("판정 기준을 --crit 로 하나 이상 적어야 합니다")
	}
	if id == "" {
		id = s.MakeID(q)
	}
	if s.FindAsk(id) != nil {
		return nil, fmt.Errorf("같은 id 가 이미 있습니다 : %s", id)
	}
	s.Asks = append(s.Asks, Ask{
		ID:       id,
		Question: q,
		Crit:     crit,
		Made:     now.Format("2006-01-02"),
		Round:    s.Round,
		Status:   StatusMaking,
		Verdicts: []Verdict{},
	})
	return &s.Asks[len(s.Asks)-1], nil
}

// FindAsk 는 id 로 물음을 찾는다. 없으면 nil 이다.
func (s *State) FindAsk(id string) *Ask {
	for i := range s.Asks {
		if s.Asks[i].ID == id {
			return &s.Asks[i]
		}
	}
	return nil
}

// NearAsks 는 비슷한 id 를 몇 개 골라 준다. 오타를 알려주려는 것이다.
func (s *State) NearAsks(id string, max int) []string {
	out := []string{}
	low := strings.ToLower(id)
	for i := range s.Asks {
		cand := s.Asks[i].ID
		if strings.Contains(strings.ToLower(cand), low) || strings.Contains(low, strings.ToLower(cand)) {
			out = append(out, cand)
		}
	}
	if len(out) == 0 {
		for i := range s.Asks {
			out = append(out, s.Asks[i].ID)
		}
	}
	if len(out) > max {
		out = out[:max]
	}
	return out
}

// Check 는 물음 하나에 통과/불통을 적는다. 판정 배열이 정본이고 상태는 뽑아 둔 값이다.
func (s *State) Check(id, value, note string, now time.Time) (*Ask, error) {
	if value != "pass" && value != "fail" {
		return nil, fmt.Errorf("판정은 pass 나 fail 로 적습니다 : %s", value)
	}
	a := s.FindAsk(id)
	if a == nil {
		return nil, fmt.Errorf("그런 물음이 없습니다 : %s", id)
	}
	a.Verdicts = append(a.Verdicts, Verdict{
		When:  now.Format(time.RFC3339),
		Value: value,
		Note:  strings.TrimSpace(note),
	})
	a.Status = StatusFromVerdict(value)
	return a, nil
}

// StatusFromVerdict 는 판정 값에 맞는 상태 딱지를 돌려준다.
func StatusFromVerdict(value string) string {
	if value == "pass" {
		return StatusPass
	}
	return StatusFail
}

// RoundAsks 는 지금 판의 물음만 골라 준다.
func (s *State) RoundAsks() []Ask {
	out := []Ask{}
	for _, a := range s.Asks {
		if a.Round == s.Round {
			out = append(out, a)
		}
	}
	return out
}

// Counts 는 지금 판 물음의 통과·불통·남은 수를 센다.
func (s *State) Counts() (pass, fail, rest int) {
	for _, a := range s.RoundAsks() {
		switch a.Status {
		case StatusPass:
			pass++
		case StatusFail:
			fail++
		default:
			rest++
		}
	}
	return pass, fail, rest
}

// DaysIn 은 이번 판 며칠째인지 센다. 시작한 날이 1일째다.
func (s *State) DaysIn(now time.Time) int {
	start, err := time.ParseInLocation("2006-01-02", s.RoundStart, now.Location())
	if err != nil {
		return 0
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	days := int(today.Sub(start).Hours()/24) + 1
	if days < 1 {
		return 1
	}
	return days
}

// Doubt 는 판을 너무 많이 돌아 컨셉을 의심할 때인지 본다.
func (s *State) Doubt(cfg config.Config) bool {
	return s.Round >= cfg.DoubtRounds && cfg.InLoop(s.Stage)
}
