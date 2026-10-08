package conflict

import (
	"regexp"
	"strings"

	"github.com/mirusona/officina-game-design-tool/internal/mdscan"
)

// 주장이 아닌 줄을 후보에서 빼는 규칙이다 (손질 1차 · 실측 2026-10-08).
// 1차 실측의 오탐 21건이 전부 머리말 메타 줄 짝(20)과 근거 목록 줄 짝(1)이었다 — 판정기가 아니라 후보 고르기 문제다.
//
//   - 메타 줄 : 머리말(첫 `##` 앞)에 있고, 날짜(YYYY-MM-DD)와 「정한 사람 · 상태 · 마지막 고침」 같은 낱말을 같이 품은 문장.
//     날짜 숫자와 공통 낱말로 모든 문서가 허브처럼 묶이고, 판정기는 날짜·상태가 다르니 「반대」라 한다.
//   - 근거 줄 : 절 제목이 「근거 · 참고 · 출처 · 기록」 인 절의 문장, 파일 경로로 시작하는 목록 줄,
//     「근거 결정 : mem …」 처럼 근거·기록 머리로 시작하는 줄.
//     「Docs/Design/02-기능목록.md — Must 1·9」 같은 줄은 문서마다 절 번호·목록이 다를 뿐 주장이 아니다.
//
// 빼는 까닭을 글로 돌려줘 몇 줄을 왜 뺐는지 기록 파일에 남긴다.

// 뺀 까닭. Result.Excluded 의 열쇠다.
const (
	ExcludeMeta   = "meta"   // 머리말 메타 줄
	ExcludeRecord = "record" // 근거·기록 절의 줄 · 경로로 시작하는 목록 줄
)

var (
	dateRe     = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	metaWordRe = regexp.MustCompile(`날짜|쓴 날|잰 날|고친 날|마지막 고침|정한 사람|쓴 사람|잰 사람|작성자|상태`)
	// 경로로 시작하는 줄 : `Docs/…` · `NN-이름.md` · `폴더/이름.md`. 뒤에 공백·콜론·줄끝이 와야 한다.
	pathStartRe = regexp.MustCompile(`^(?:Docs/|[A-Za-z0-9_.\-가-힣]+(?:/[A-Za-z0-9_.\-가-힣]+)*\.md(?:\s|:|$))`)
	// 「근거 결정 : …」 「결정 기록 : …」 「참고 : …」 처럼 기록 머리로 시작하는 줄. 머리 바로 뒤는 공백이나 콜론이어야 하고,
	// 머리와 콜론 사이엔 낱말 하나(네 글자까지)만 봐 준다 — 「근거리 공격 : 3칸」 같은 게임 낱말이 걸리지 않게 (리뷰 10-08).
	recordHeadRe = regexp.MustCompile(`^(?:근거|결정 기록|참고|출처)(?:\s+[^\s:]{1,4})?\s*:`)
	// 절 제목이 이 낱말과 같거나 이 낱말 + 공백으로 시작하면 그 절은 주장이 아니라 기록이다. 번호(「14. 근거」)는 떼고 본다.
	// 「기록」 은 안 넣는다 — 「최고 기록 보상」 같은 기획 절이 흔하다. 「근거리 전투」 도 안 걸린다 (리뷰 10-08).
	recordSections = []string{"근거", "참고", "출처"}
)

// Exclude 는 문장을 후보에서 빼야 하면 그 까닭(ExcludeMeta · ExcludeRecord)을, 아니면 빈 글을 돌려준다.
func Exclude(s mdscan.Sentence) string {
	if s.Head && isMetaLine(s.Text) {
		return ExcludeMeta
	}
	if isRecordSection(s.Ctx) || pathStartRe.MatchString(s.Text) || recordHeadRe.MatchString(s.Text) {
		return ExcludeRecord
	}
	return ""
}

// sectionTitle 은 Ctx 「절 : <제목> · 줄 머리 : …」 에서 번호를 뗀 제목이다. 절이 없으면 빈 글이다.
func sectionTitle(ctx string) string {
	title, ok := strings.CutPrefix(ctx, "절 : ")
	if !ok {
		return ""
	}
	if before, _, found := strings.Cut(title, " · "); found {
		title = before
	}
	return strings.TrimSpace(strings.TrimLeft(title, "0123456789.-) "))
}

// isMetaLine 은 날짜와 메타 낱말을 같이 품은 글인지 본다. 머리말 안에서만 쓴다 — 본문의 「상태」는 게임 낱말일 수 있다.
func isMetaLine(text string) bool {
	return dateRe.MatchString(text) && metaWordRe.MatchString(text)
}

// isRecordSection 은 Ctx 의 절 제목이 기록 절인지 본다.
func isRecordSection(ctx string) bool {
	title := sectionTitle(ctx)
	if title == "" {
		return false
	}
	for _, w := range recordSections {
		if title == w || strings.HasPrefix(title, w+" ") {
			return true
		}
	}
	return false
}
