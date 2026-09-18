// Package hookio 는 훅이 stdin 으로 주는 JSON 을 읽고, 훅이 알아듣는 출력을 만든다.
// 규약 출처 : https://code.claude.com/docs/en/hooks (2026-09-07 확인)
package hookio

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ToolInput 은 훅이 알려주는 도구 인자다. 우리가 쓰는 칸만 받는다.
type ToolInput struct {
	FilePath   string `json:"file_path"`
	Content    string `json:"content"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

// Event 는 훅 입력이다. 모르는 칸은 그냥 버린다.
type Event struct {
	HookEventName string    `json:"hook_event_name"`
	SessionID     string    `json:"session_id"`
	CWD           string    `json:"cwd"`
	ToolName      string    `json:"tool_name"`
	ToolInput     ToolInput `json:"tool_input"`
}

// Read 는 stdin 을 **끝까지** 읽고 JSON 으로 푼다. 안 읽으면 부르는 쪽 파이프가 막힐 수 있다.
func Read(r io.Reader) (Event, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return Event{}, fmt.Errorf("훅 입력을 못 읽었습니다 : %v", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return Event{}, nil
	}
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		return Event{}, fmt.Errorf("훅 입력이 JSON 이 아닙니다 : %v", err)
	}
	return ev, nil
}

// Drain 은 쓰지 않을 stdin 을 끝까지 읽어 버린다.
func Drain(r io.Reader) {
	_, _ = io.Copy(io.Discard, r)
}

// SystemMessage 는 사람에게 보여줄 한 줄짜리 훅 출력을 만든다.
func SystemMessage(text string) ([]byte, error) {
	out := map[string]string{"systemMessage": text}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("훅 출력을 못 만들었습니다 : %v", err)
	}
	return raw, nil
}

// Applied 는 「쓰고 난 뒤」의 내용을 미리 만든다. Write 는 그대로, Edit 는 고침을 적용한 결과다.
// 저장소가 CRLF 로 체크아웃되면 디스크는 CRLF, old_string 은 LF 라 그냥은 안 맞는다.
// 검사에만 쓰는 값이라 양쪽을 LF 로 맞춰 대조하고 결과도 LF 로 둔다.
func Applied(tool string, in ToolInput, disk string) string {
	if tool == "Write" {
		return in.Content
	}
	src := toLF(disk)
	old := toLF(in.OldString)
	if old == "" || !strings.Contains(src, old) {
		return src
	}
	replacement := toLF(in.NewString)
	if in.ReplaceAll {
		return strings.ReplaceAll(src, old, replacement)
	}
	return strings.Replace(src, old, replacement, 1)
}

// toLF 는 줄끝을 LF 로 맞춘다.
func toLF(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}
