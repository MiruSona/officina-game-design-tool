package conflict

import "regexp"

// secretShapes 는 밖으로 보내기 전에 거르는 비밀 꼴이다. 걸린 문장은 보내지 않고 **개수만** 센다.
// 우회 옵션은 없다 (보안 불변조건 2).
var secretShapes = []*regexp.Regexp{
	regexp.MustCompile(`(^|[^A-Za-z0-9])sk-[A-Za-z0-9]`),
	regexp.MustCompile(`AKIA[0-9A-Z]{8}`),
	regexp.MustCompile(`Bearer [A-Za-z0-9._\-]+`),
	// 일반 낱말은 값(영숫자 8자 이상)이 따라올 때만 건다 — 「Token: 턴마다 1개」 같은 기획 문장을 안 걸려고.
	regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_-]?key)\s*[:=]\s*["']?[A-Za-z0-9_\-./+]{8,}`),
	// 아래 넷은 ModTool commit-prep public_guard.py 의 접두 토큰 꼴을 그대로 옮긴 것이다.
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{16,}|github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]+`),
	regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+`),
	regexp.MustCompile(`[0-9a-fA-F]{40,}`),
	regexp.MustCompile(`-----BEGIN`),
	regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
	regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`),
}

// SecretShape 은 글에 비밀 꼴이 있는지 본다. 값은 돌려주지 않는다.
func SecretShape(text string) bool {
	for _, re := range secretShapes {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}
