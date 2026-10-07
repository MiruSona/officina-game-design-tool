package conflict

import (
	"strings"
	"testing"
)

func TestWaitIsCappedAtFiveMinutes(t *testing.T) {
	if Wait(1) != WaitBase+WaitPerPair {
		t.Fatalf("쌍 1 = %s", Wait(1))
	}
	if Wait(200) != MaxWait || Wait(1000) != MaxWait {
		t.Fatalf("상한 5분을 넘으면 안 된다 : %s", Wait(200))
	}
}

func TestParseVerdictsCountsUnreadLines(t *testing.T) {
	raw := []byte(`{"id":"a.md:1|b.md:2","letter":"B","prob":0.91,"probs":{"A":0.05,"B":0.91,"C":0.04},"ms":1200}
이건 JSON 이 아니다
{"letter":"A"}
{"id":"a.md:3|b.md:4","problem":"timeout"}
`)
	oc := parseVerdicts(raw)
	if oc.Unread != 2 {
		t.Fatalf("못 읽은 줄 = %d, 2 여야 한다 (JSON 아님 · id 없음)", oc.Unread)
	}
	v, ok := oc.Verdicts["a.md:1|b.md:2"]
	if !ok || v.ProbB() != 0.91 || v.MS != 1200 {
		t.Fatalf("판정을 못 읽었다 : %+v", v)
	}
	if p := oc.Verdicts["a.md:3|b.md:4"]; p.Problem != "timeout" || p.ProbB() != 0 {
		t.Fatalf("problem 줄 = %+v", p)
	}
}

func TestParseVerdictsKeepsFirstOfDuplicateID(t *testing.T) {
	raw := []byte(`{"id":"x","letter":"B","prob":0.9}
{"id":"x","letter":"A","prob":0.9}
`)
	oc := parseVerdicts(raw)
	if oc.Verdicts["x"].Letter != "B" || oc.Unread != 1 {
		t.Fatalf("같은 id 는 첫 것을 쓰고 두 번째는 못 읽은 줄로 센다 : %+v %d", oc.Verdicts["x"], oc.Unread)
	}
}

func TestFailReasonUsesFirstUnreadLine(t *testing.T) {
	oc := parseVerdicts([]byte("판정 건너뜀 : llm.toml 이 없습니다\n"))
	if oc.FailReason() != "판정기 출력 : 판정 건너뜀 : llm.toml 이 없습니다" {
		t.Fatalf("까닭 = %q", oc.FailReason())
	}
	oc.ExitErr = "판정기 종료 exit status 3"
	if oc.FailReason() != oc.ExitErr {
		t.Fatal("종료 까닭이 있으면 그것이 먼저다")
	}
	if (Outcome{}).FailReason() == "" {
		t.Fatal("아무것도 없어도 까닭 한 줄은 있어야 한다")
	}
}

func TestSkipReasonOnlyForSkipLineWithNoVerdicts(t *testing.T) {
	oc := parseVerdicts([]byte("판정 건너뜀 : llm.toml 이 없습니다 (어딘가)\n"))
	if oc.SkipReason() != "llm.toml 이 없습니다 (어딘가)" {
		t.Fatalf("까닭 = %q", oc.SkipReason())
	}
	if parseVerdicts([]byte("알 수 없는 줄\n")).SkipReason() != "" {
		t.Fatal("건너뜀 머리글이 아니면 빈 글이다")
	}
	oc.ExitErr = "판정기 종료 exit status 3"
	if oc.SkipReason() != "" {
		t.Fatal("종료 까닭이 있으면 건너뜀이 아니다")
	}
	both := parseVerdicts([]byte("판정 건너뜀 : x\n{\"id\":\"a\",\"letter\":\"A\"}\n"))
	if both.SkipReason() != "" || len(both.Accepted) == 0 {
		t.Fatal("판정이 하나라도 읽혔으면 건너뜀이 아니고 Accepted 에 그 줄이 있다")
	}
	long := parseVerdicts([]byte("판정 건너뜀 : " + strings.Repeat("가", 200) + "\n"))
	if r := []rune(long.SkipReason()); len(r) > 121 || r[len(r)-1] != '…' {
		t.Fatalf("긴 까닭은 잘리고 끝에 … 가 붙는다 (지금 %d자)", len(r))
	}
}

func TestProbBFallsBackToLetter(t *testing.T) {
	if (Verdict{Letter: "B", Prob: 0.7}).ProbB() != 0.7 {
		t.Fatal("probs 가 없으면 letter 가 B 일 때 prob 을 써야 한다")
	}
	if (Verdict{Letter: "A", Prob: 0.7}).ProbB() != 0 {
		t.Fatal("letter 가 A 면 B 확률은 0 이다")
	}
}

func TestLookupWithEmptyPath(t *testing.T) {
	t.Setenv("PATH", "")
	exe, skip := Lookup()
	if exe != "" || skip == "" {
		t.Fatalf("PATH 가 비면 건너뛴 까닭이 있어야 한다 : %q %q", exe, skip)
	}
}
