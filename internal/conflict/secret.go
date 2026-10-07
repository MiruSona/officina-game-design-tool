package conflict

import "regexp"

// secretShapes 는 밖으로 보내기 전에 거르는 비밀 꼴이다. 걸린 문장은 보내지 않고 **개수만** 센다.
// 우회 옵션은 없다 (보안 불변조건 2).
var secretShapes = []*regexp.Regexp{
	regexp.MustCompile(`(^|[^A-Za-z0-9])sk-[A-Za-z0-9]`),
	regexp.MustCompile(`AKIA[0-9A-Z]{8}`),
	regexp.MustCompile(`Bearer [A-Za-z0-9._\-]+`),
	regexp.MustCompile(`(?i)password\s*=`),
	regexp.MustCompile(`(?i)token\s*=`),
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
