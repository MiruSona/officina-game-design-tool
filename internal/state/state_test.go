package state

import (
	"testing"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/config"
)

func now() time.Time {
	return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
}

func TestNextKeepsRound(t *testing.T) {
	cfg := config.Default()
	st := New(now())
	st.Stage = 6
	st.Round = 2
	if err := st.Next(cfg); err != nil {
		t.Fatalf("넘기기 실패 : %v", err)
	}
	if st.Stage != 7 {
		t.Fatalf("단계 = %d, 7 이어야 한다", st.Stage)
	}
	if st.Round != 2 {
		t.Fatalf("판번호 = %d, next 로는 안 오른다", st.Round)
	}
}

func TestNextAtLastStageFails(t *testing.T) {
	cfg := config.Default()
	st := New(now())
	st.Stage = len(cfg.Stages)
	if err := st.Next(cfg); err == nil {
		t.Fatal("마지막 단계에서는 못 넘긴다")
	}
}

func TestBackRules(t *testing.T) {
	cfg := config.Default()
	cases := []struct {
		name      string
		from      int
		to        int
		why       string
		wantErr   bool
		wantRound int
	}{
		{"고리 안으로 되돌아가면 판이 오른다", 6, 3, "고리가 안 닫혔다", false, 3},
		{"숫자만 틀렸으면 5단계", 6, 5, "숫자가 안 맞는다", false, 3},
		{"컨셉으로 되돌아가면 판이 1 이 된다", 6, 1, "컨셉이 틀렸다", false, 1},
		{"까닭이 없으면 막는다", 6, 3, "", true, 2},
		{"앞 단계가 아니면 막는다", 3, 6, "왜", true, 2},
		{"고리 밖으로는 못 되돌아간다", 9, 8, "왜", true, 2},
	}
	for _, c := range cases {
		st := New(now())
		st.Stage = c.from
		st.Round = 2
		err := st.GoBack(c.to, c.why, cfg, now())
		if c.wantErr && err == nil {
			t.Fatalf("%s : 막았어야 한다", c.name)
		}
		if !c.wantErr && err != nil {
			t.Fatalf("%s : %v", c.name, err)
		}
		if st.Round != c.wantRound {
			t.Fatalf("%s : 판번호 = %d, %d 이어야 한다", c.name, st.Round, c.wantRound)
		}
		if !c.wantErr && len(st.Backs) != 1 {
			t.Fatalf("%s : 되돌아간 기록이 안 남았다", c.name)
		}
	}
}

func TestBackKeepsRecordWhenRoundResets(t *testing.T) {
	cfg := config.Default()
	st := New(now())
	st.Stage = 6
	st.Round = 3
	if err := st.GoBack(2, "기둥이 틀렸다", cfg, now()); err != nil {
		t.Fatalf("되돌아가기 실패 : %v", err)
	}
	if len(st.Backs) != 1 {
		t.Fatal("판을 되돌려도 기록은 남는다")
	}
	if st.Backs[0].Round != 3 {
		t.Fatalf("기록의 판번호 = %d, 되돌아가기 직전 값이어야 한다", st.Backs[0].Round)
	}
}

func TestAddAskAndCheck(t *testing.T) {
	st := New(now())
	st.Round = 2
	a, err := st.AddAsk("", "시작 칸을 3칸으로 하는 게 맞나", []string{"5분 동안 돌렸다"}, now())
	if err != nil {
		t.Fatalf("물음 넣기 실패 : %v", err)
	}
	if a.Status != StatusMaking || a.Round != 2 {
		t.Fatalf("새 물음 = %+v", a)
	}
	if _, err := st.AddAsk("", "기준 없는 물음", nil, now()); err == nil {
		t.Fatal("기준이 없으면 막는다")
	}
	got, err := st.Check(a.ID, "fail", "멈춘 횟수가 많다", now())
	if err != nil {
		t.Fatalf("판정 실패 : %v", err)
	}
	if got.Status != StatusFail || len(got.Verdicts) != 1 {
		t.Fatalf("판정 뒤 = %+v", got)
	}
	if _, err := st.Check(a.ID, "maybe", "", now()); err == nil {
		t.Fatal("pass·fail 이 아니면 막는다")
	}
	if _, err := st.Check("없는id", "pass", "", now()); err == nil {
		t.Fatal("없는 물음은 막는다")
	}
}

func TestMakeIDAvoidsClash(t *testing.T) {
	st := New(now())
	first, _ := st.AddAsk("", "start slots", []string{"기준"}, now())
	second, _ := st.AddAsk("", "start slots", []string{"기준"}, now())
	if first.ID == second.ID {
		t.Fatalf("id 가 겹쳤다 : %s", first.ID)
	}
	korean, _ := st.AddAsk("", "한글만 있는 물음", []string{"기준"}, now())
	if korean.ID == "" {
		t.Fatal("한글만 있어도 id 는 있어야 한다")
	}
}

func TestCountsAndDays(t *testing.T) {
	st := New(now())
	st.Round = 2
	st.RoundStart = "2026-09-05"
	a1, _ := st.AddAsk("a1", "물음 하나", []string{"기준"}, now())
	a2, _ := st.AddAsk("a2", "물음 둘", []string{"기준"}, now())
	_, _ = st.Check(a1.ID, "pass", "", now())
	_, _ = st.Check(a2.ID, "fail", "", now())
	_, _ = st.AddAsk("a3", "물음 셋", []string{"기준"}, now())
	pass, fail, rest := st.Counts()
	if pass != 1 || fail != 1 || rest != 1 {
		t.Fatalf("셈 = %d/%d/%d", pass, fail, rest)
	}
	if got := st.DaysIn(now()); got != 3 {
		t.Fatalf("며칠째 = %d, 3 이어야 한다", got)
	}
}

func TestDoubtOnlyInLoop(t *testing.T) {
	cfg := config.Default()
	st := New(now())
	st.Round = 3
	st.Stage = 5
	if !st.Doubt(cfg) {
		t.Fatal("고리 안에서 3판이면 의심할 때다")
	}
	st.Stage = 8
	if st.Doubt(cfg) {
		t.Fatal("고리 밖에서는 안 띄운다")
	}
}

func TestLoadMissingAndSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err != ErrMissing {
		t.Fatalf("없는 파일 = %v, ErrMissing 이어야 한다", err)
	}
	st := New(now())
	st.Stage = 4
	if err := Save(dir, st, now()); err != nil {
		t.Fatalf("쓰기 실패 : %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("읽기 실패 : %v", err)
	}
	if got.Stage != 4 || got.Format != Format {
		t.Fatalf("다시 읽은 값 = %+v", got)
	}
}
