package hookio

import (
	"os"
	"strings"
	"testing"
)

func TestReadPreToolUseFixture(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/hooks/pretooluse-write.json")
	if err != nil {
		t.Fatalf("훅 예시를 못 읽었다 : %v", err)
	}
	ev, err := Read(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("훅 입력 풀기 실패 : %v", err)
	}
	if ev.ToolName != "Write" {
		t.Fatalf("도구 이름 = %q", ev.ToolName)
	}
	if ev.ToolInput.FilePath != "Docs/Design/노트.md" {
		t.Fatalf("파일 경로 = %q", ev.ToolInput.FilePath)
	}
	if ev.HookEventName != "PreToolUse" {
		t.Fatalf("이벤트 이름 = %q", ev.HookEventName)
	}
}

func TestReadEmptyIsNotError(t *testing.T) {
	ev, err := Read(strings.NewReader("   \n"))
	if err != nil {
		t.Fatalf("빈 입력은 오류가 아니다 : %v", err)
	}
	if ev.ToolName != "" {
		t.Fatalf("빈 입력인데 값이 있다 : %+v", ev)
	}
}

func TestReadBrokenJSONFails(t *testing.T) {
	if _, err := Read(strings.NewReader("{ 이건 JSON 이 아니다")); err == nil {
		t.Fatal("깨진 입력은 오류다")
	}
}

func TestApplied(t *testing.T) {
	disk := "가\n나\n가\n"
	cases := []struct {
		name string
		tool string
		in   ToolInput
		want string
	}{
		{"Write 는 내용 그대로", "Write", ToolInput{Content: "새 글\n"}, "새 글\n"},
		{"Edit 는 첫 번째만 바꾼다", "Edit", ToolInput{OldString: "가\n", NewString: "다\n"}, "다\n나\n가\n"},
		{"replace_all 이면 다 바꾼다", "Edit", ToolInput{OldString: "가\n", NewString: "다\n", ReplaceAll: true}, "다\n나\n다\n"},
		{"못 찾으면 디스크 그대로", "Edit", ToolInput{OldString: "없는 글", NewString: "x"}, disk},
		{"old_string 이 없으면 디스크 그대로", "Edit", ToolInput{}, disk},
	}
	for _, c := range cases {
		if got := Applied(c.tool, c.in, disk); got != c.want {
			t.Fatalf("%s : %q, %q 여야 한다", c.name, got, c.want)
		}
	}
}

func TestSystemMessage(t *testing.T) {
	raw, err := SystemMessage("판 끝 할 일이 남았습니다")
	if err != nil {
		t.Fatalf("훅 출력 만들기 실패 : %v", err)
	}
	if !strings.Contains(string(raw), `"systemMessage"`) {
		t.Fatalf("칸 이름이 없다 : %s", raw)
	}
}
