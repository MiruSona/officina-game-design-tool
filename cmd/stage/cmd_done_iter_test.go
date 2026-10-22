package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 내장 틀의 자리표 줄. 시험이 이 줄을 바꿔 「채웠다」 를 만든다.
const (
	tplFeedback = "1. (사용자 말) [코드]"
	tplDecision = "| 1 | (고친다 · 미룬다 · 안 한다) | (해당 · 없음 · 새로 넣음) |"
	tplFixed    = "| (코드) | | |"
	tplDeferred = "- (피드백 번호) — 왜 미뤘나"
	tplHanded   = "| --- | --- |" + "\n" + "| | |"
	tplLesson   = "- (다음 반복에 쓸 것 한 줄)"
)

// openIter 는 마지막 단계에서 `iter new` 로 반복 하나를 열고, 기록 파일 줄을 바꾼 뒤 그 자리를 돌려준다.
func openIter(t *testing.T, swaps ...string) (string, string) {
	t.Helper()
	root := iterRoot(t, 9)
	if err := cmdIter([]string{"new", "--root", root}); err != nil {
		t.Fatalf("iter new 실패 : %v", err)
	}
	rel := iterRel + "/01-" + today() + ".md"
	body := readFile(t, root, rel)
	for i := 0; i+1 < len(swaps); i += 2 {
		if !strings.Contains(body, swaps[i]) {
			t.Fatalf("틀에 %q 가 없다", swaps[i])
		}
		body = strings.Replace(body, swaps[i], swaps[i+1], 1)
	}
	putFile(t, root, rel, body)
	return root, rel
}

func runDone(t *testing.T, root string) string {
	t.Helper()
	return captureOut(t, func() {
		if err := cmdDone([]string{"--root", root}); err != nil {
			t.Fatalf("done 실패 : %v", err)
		}
	})
}

// filledTables 는 피드백·결정 표를 채우는 바꿈 짝이다.
var filledTables = []string{tplFeedback, "1. 점프가 무겁다 [코드]", tplDecision, "| 1 | 고친다 | 없음 |", tplLesson, "- 눈금 하나"}

// D1 : 「고친 것」 이 틀 그대로면 조용하다.
func TestDoneIterQuietBeforeFixed(t *testing.T) {
	root, _ := openIter(t)
	if out := runDone(t, root); !strings.Contains(out, "아직 반복이 안 끝났습니다") || strings.Contains(out, "할 일이 남았습니다") {
		t.Fatalf("조용해야 한다 : %s", out)
	}
}

// D2 : 고친 것만 채우면 미룬 것 · 넘긴 것 · 기억이 뜬다.
func TestDoneIterAsksDeferredAndHanded(t *testing.T) {
	root, _ := openIter(t, append([]string{tplFixed, "| 코드 | 점프 높이 | Player.cs |"}, filledTables...)...)
	out := runDone(t, root)
	for _, want := range []string{"반복 01 끝 할 일이 남았습니다 :", "「미룬 것」 → 없으면 「- 없음」", "「넘긴 것」 → 없으면 「- 없음」", "기억 1건"} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q 가 없다 :\n%s", want, out)
		}
	}
	if strings.Contains(out, "자리표") {
		t.Fatalf("빈 절로 알린 자리표는 또 세지 않는다 :\n%s", out)
	}
}

// D3 : 미룬 것 「- 없음」 · 넘긴 것 채움 · 자리표 0 이면 기억만 뜬다.
func TestDoneIterOnlyMemory(t *testing.T) {
	swaps := append([]string{tplFixed, "| 코드 | 점프 높이 | a.cs |", tplDeferred, "- 없음", tplHanded, "| --- | --- |" + "\n" + "| 그림 → 코드 | 값 |"}, filledTables...)
	root, _ := openIter(t, swaps...)
	out := runDone(t, root)
	if !strings.Contains(out, "기억 1건") || strings.Contains(out, "「미룬 것」") || strings.Contains(out, "「넘긴 것」") || strings.Contains(out, "자리표") {
		t.Fatalf("기억만 떠야 한다 :\n%s", out)
	}
}

// D4 : 결정 표 첫 줄이 틀 그대로면 자리표가 뜬다 (줄 수와 예).
func TestDoneIterPlaceholderLeft(t *testing.T) {
	swaps := []string{tplFixed, "| 코드 | 점프 | a.cs |", tplDeferred, "- 없음", tplHanded, "| --- | --- |" + "\n" + "| 없음 | |", tplFeedback, "1. 무겁다 [코드]", tplLesson, "- 눈금"}
	root, _ := openIter(t, swaps...)
	out := runDone(t, root)
	if !strings.Contains(out, "틀 자리표가 남았습니다 (1줄) → 예 : "+tplDecision) {
		t.Fatalf("자리표 1줄과 예가 떠야 한다 :\n%s", out)
	}
}

// D5 : 기록 파일이 없으면 「고친 것」 조건 없이 그것만 뜬다.
func TestDoneIterMissingFile(t *testing.T) {
	root, rel := openIter(t)
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
	out := runDone(t, root)
	if !strings.Contains(out, "기록 파일이 없습니다 — "+rel) || strings.Contains(out, "기억 1건") {
		t.Fatalf("기록 파일 없음만 떠야 한다 :\n%s", out)
	}
}

// D6 : 반복이 없으면 판 끝 할 일 그대로다.
func TestDoneWithoutIterKeepsRoundTodos(t *testing.T) {
	root := t.TempDir()
	finishedRound(t, root)
	out := runDone(t, root)
	if !strings.Contains(out, "판 끝 할 일이 남았습니다 :") || strings.Contains(out, "반복") {
		t.Fatalf("판 끝 할 일이어야 한다 :\n%s", out)
	}
}

// D7 : --hook 으로 같은 글이면 두 번째는 안 찍는다.
func TestDoneIterHookOnce(t *testing.T) {
	useTempCache(t)
	root, _ := openIter(t, tplFixed, "| 코드 | 점프 | a.cs |")
	hookJSON := `{"hook_event_name":"Stop","session_id":"sess-iter","cwd":"` + filepath.ToSlash(root) + `"}`
	if first := runDoneHook(t, root, hookJSON); !strings.Contains(first, "systemMessage") {
		t.Fatalf("첫 번째는 띄워야 한다 : %q", first)
	}
	if second := runDoneHook(t, root, hookJSON); strings.TrimSpace(second) != "" {
		t.Fatalf("두 번째는 안 찍는다 : %q", second)
	}
}
