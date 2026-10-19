package conflict

import (
	"strings"
	"testing"
)

func TestSecretShape(t *testing.T) {
	bad := []string{
		"열쇠는 sk-abc123 이다",
		"AKIAABCDEFGH1234 를 쓴다",
		"머리에 Bearer abc.def 를 붙인다",
		"password = 12345678",
		"token=abcdefgh",
		"password: hunter2hunter2",
		"access_token=abcdefghij12345",
		"api_key: \"AbCd1234EfGh\"",
		"ghp_" + strings.Repeat("a", 20),
		"github_pat_" + strings.Repeat("a", 22),
		"xoxb-123-abc",
		"AIza" + strings.Repeat("a", 35),
		"eyJ" + strings.Repeat("a", 10) + "." + strings.Repeat("b", 10) + ".c",
		"해시 " + strings.Repeat("ab", 20),
		"-----BEGIN RSA",
		"주소 10.0.0.1 로 간다",                 // guard:ok 가짜
		"메일 someone@example.invalid 로 보낸다", // guard:ok 가짜
	}
	for _, b := range bad {
		if !SecretShape(b) {
			t.Fatalf("비밀 꼴을 못 잡았다 : %q", b)
		}
	}
	good := []string{"대기 시간은 3초로 둔다.", "비율은 1.5 배다.", "토큰 수는 세 개다.", "task-1 과 desk-2 를 본다.", "mask-3 은 셋이다.",
		"Token: 턴마다 1개 얻는다", "입장 token : 5개"}
	for _, g := range good {
		if SecretShape(g) {
			t.Fatalf("보통 문장을 비밀로 봤다 : %q", g)
		}
	}
}
