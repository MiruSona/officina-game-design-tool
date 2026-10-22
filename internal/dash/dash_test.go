package dash

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mirusona/officina-game-design-tool/internal/config"
	"github.com/mirusona/officina-game-design-tool/internal/state"
)

const okRepo = "../../testdata/repo-ok"

func build(t *testing.T) Model {
	t.Helper()
	cfg, err := config.Load(okRepo)
	if err != nil {
		t.Fatalf("설정 읽기 실패 : %v", err)
	}
	st, err := state.Load(okRepo)
	if err != nil {
		t.Fatalf("상태 읽기 실패 : %v", err)
	}
	return Build(okRepo, cfg, st, time.Date(2026, 9, 7, 21, 10, 0, 0, time.UTC))
}

func TestBuildReadsStateAndDocs(t *testing.T) {
	m := build(t)
	if m.StageNo != 5 || m.Round != 2 {
		t.Fatalf("단계·판 = %d · %d", m.StageNo, m.Round)
	}
	if len(m.Steps) != 9 || m.Steps[4].Class != "now" || m.Steps[0].Class != "done" || m.Steps[8].Class != "todo" {
		t.Fatalf("스테퍼가 틀렸다 : %+v", m.Steps)
	}
	if m.AskCount != 2 || m.PassCount != 1 {
		t.Fatalf("이번 판 물음 = %d개, 통과 %d", m.AskCount, m.PassCount)
	}
	if m.BackCount != 1 {
		t.Fatalf("되돌아간 횟수 = %d", m.BackCount)
	}
	if len(m.Pillars) != 3 {
		t.Fatalf("기둥 = %d개, 3개여야 한다 : %v", len(m.Pillars), m.Pillars)
	}
	if len(m.Docs) != 6 || !m.Docs[0].Exists || m.Docs[3].Exists {
		t.Fatalf("문서 체크가 틀렸다 : %+v", m.Docs)
	}
}

func TestWriteHasSixBlocks(t *testing.T) {
	var sb strings.Builder
	if err := Write(&sb, build(t)); err != nil {
		t.Fatalf("굽기 실패 : %v", err)
	}
	html := sb.String()
	for _, want := range []string{"① 9단계", "되돌아간 횟수", "③ 이번 판 물음", "④ 기둥", "⑤ 문서 뼈대", "⑥ 되돌아가는 고리", "stateDiagram-v2", "구웠습니다"} {
		if !strings.Contains(html, want) {
			t.Fatalf("덩어리가 빠졌다 : %s", want)
		}
	}
}

func TestWriteEscapesAngleBrackets(t *testing.T) {
	m := build(t)
	m.Asks = append(m.Asks, AskRow{ID: "x", Question: "<script>나쁜 것</script>", Status: "대기"})
	var sb strings.Builder
	if err := Write(&sb, m); err != nil {
		t.Fatalf("굽기 실패 : %v", err)
	}
	if strings.Contains(sb.String(), "<script>나쁜") {
		t.Fatal("꺾쇠를 안 막았다")
	}
}

func TestBakeWritesFile(t *testing.T) {
	dir := t.TempDir()
	path, err := Bake(dir, build(t), "")
	if err != nil {
		t.Fatalf("굽기 실패 : %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("구운 파일을 못 읽었다 : %v", err)
	}
	if !strings.HasPrefix(string(raw), "<!doctype html>") {
		t.Fatal("HTML 이 아니다")
	}
}

func TestPillarsNoteWhenMissing(t *testing.T) {
	cfg := config.Default()
	got, note := pillars(t.TempDir(), cfg)
	if got != nil || note == "" {
		t.Fatalf("컨셉 문서가 없으면 안내만 한다 : %v / %q", got, note)
	}
}

// B1 : 반복 둘이면 덩어리 하나에 최근 것이 위, 반복이 없으면 덩어리가 없다.
func TestWriteIterBlock(t *testing.T) {
	m := build(t)
	var plain strings.Builder
	if err := Write(&plain, m); err != nil {
		t.Fatalf("굽기 실패 : %v", err)
	}
	if strings.Contains(plain.String(), "반복 기록") {
		t.Fatal("반복이 없으면 덩어리를 안 그린다")
	}
	st := state.New(time.Now())
	st.Iters = []state.Iter{
		{No: 10, Start: "2026-10-01", Played: "v16", File: "Docs/History/반복/10-2026-10-01.md"},
		{No: 11, Start: "2026-10-22", Played: "v17", Title: "메인 화면", File: "Docs/History/반복/11-2026-10-22.md"},
	}
	st.Stage = 5
	var back strings.Builder
	if err := Write(&back, Build(t.TempDir(), config.Default(), st, time.Now())); err != nil {
		t.Fatalf("굽기 실패 : %v", err)
	}
	if strings.Contains(back.String(), "반복 기록") || !strings.Contains(back.String(), "되돌아가는 고리") {
		t.Fatal("앞 단계로 되돌아간 뒤에는 고리 그대로다")
	}
	st.Stage = 9
	m = Build(t.TempDir(), config.Default(), st, time.Now())
	if m.Iters[0].Link != "../History/반복/11-2026-10-22.md" {
		t.Fatalf("링크는 구운 자리에서 본 상대 경로다 : %q", m.Iters[0].Link)
	}
	if len(m.Iters) != 2 || m.Iters[0].No != 11 {
		t.Fatalf("최근 것이 위여야 한다 : %+v", m.Iters)
	}
	var sb strings.Builder
	if err := Write(&sb, m); err != nil {
		t.Fatalf("굽기 실패 : %v", err)
	}
	html := sb.String()
	if strings.Count(html, "반복 기록") != 1 || strings.Index(html, "11-2026-10-22") > strings.Index(html, "10-2026-10-01") {
		t.Fatalf("덩어리 하나 · 최근 것 위가 아니다")
	}
	if strings.Contains(html, "되돌아가는 고리") {
		t.Fatal("반복 중에는 고리 덩어리 자리를 반복 기록이 쓴다 (여섯 덩어리 상한)")
	}
}
