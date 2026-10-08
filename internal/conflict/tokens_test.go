package conflict

import (
	"reflect"
	"strings"
	"testing"
)

func TestTokensStripsParticleOnce(t *testing.T) {
	bag := Tokens("대기 시간은 건물에서 3초로 둔다")
	want := []string{"대기", "시간", "건물", "둔다"}
	if !reflect.DeepEqual(bag.Words, want) {
		t.Fatalf("Words = %v, %v 여야 한다", bag.Words, want)
	}
	if !reflect.DeepEqual(bag.Nums, []string{"3초"}) {
		t.Fatalf("Nums = %v", bag.Nums)
	}
	if bag.Neg {
		t.Fatal("부정어가 없다")
	}
}

func TestTokensRevertsOneLetterAndDropsStopWords(t *testing.T) {
	bag := Tokens("이 것은 그리고 길이 사이 있다")
	// 「길이」 는 조사를 떼면 「길」 한 글자라 되돌린다. 「사이」 도 같다. 「이」·「것」·「그리고」·「있다」 는 멈춤말이다.
	want := []string{"길이", "사이"}
	if !reflect.DeepEqual(bag.Words, want) {
		t.Fatalf("Words = %v, %v 여야 한다", bag.Words, want)
	}
}

func TestTokensNegationAndLowercase(t *testing.T) {
	cases := []struct {
		text string
		neg  bool
	}{
		{"이 기능은 안 한다", true},
		{"이 기능은 하지 않는다", true},
		{"이 기능은 없다", true},
		{"Never Allow This", true},
		{"이 기능은 한다", false},
		{"안전 장치를 둔다", false},
	}
	for _, c := range cases {
		if got := Tokens(c.text).Neg; got != c.neg {
			t.Fatalf("%q : Neg = %v, %v 여야 한다", c.text, got, c.neg)
		}
	}
	bag := Tokens("Never Allow This")
	if !reflect.DeepEqual(bag.Words, []string{"never", "allow", "this"}) {
		t.Fatalf("영문은 소문자로 접어야 한다 : %v", bag.Words)
	}
}

func TestTokensSeparatorsAndNumbers(t *testing.T) {
	bag := Tokens("값(최대)·비율/배수 : 1.5, 20%")
	if !reflect.DeepEqual(bag.Words, []string{"최대", "비율", "배수"}) {
		t.Fatalf("Words = %v", bag.Words)
	}
	if !reflect.DeepEqual(bag.Nums, []string{"1.5", "20%"}) {
		t.Fatalf("Nums = %v", bag.Nums)
	}
	if !HasNumber("3초") || HasNumber("세 초") {
		t.Fatal("HasNumber 가 숫자를 못 가린다")
	}
}

// 손질 3차 ② — 비율·좌표·시각은 한 토막이고 내용어로 안 샌다. `×3` 배율은 값이다.
func TestTokensRatioAndScale(t *testing.T) {
	bag := Tokens("화면은 540×960 이고 비율 3/4 에 시각 9:20 과 1.5/2 를 쓰며 보너스 ×3 과 x1.5 다.")
	wantNums := []string{"540×960", "3/4", "9:20", "1.5/2", "×3", "x1.5"}
	if !reflect.DeepEqual(bag.Nums, wantNums) {
		t.Fatalf("Nums = %v, %v 여야 한다", bag.Nums, wantNums)
	}
	for _, w := range bag.Words {
		if strings.ContainsAny(w, "0123456789") {
			t.Fatalf("숫자가 든 토막이 내용어로 샜다 : %v", bag.Words)
		}
	}
	// 날짜·절 번호는 비율이 아니다 (`-` 는 안 잡힌다).
	bag = Tokens("2026-10-08 에 4-1절을 고쳤다.")
	if !reflect.DeepEqual(bag.Nums, []string{"2026-10-08", "4-1절"}) {
		t.Fatalf("Nums = %v", bag.Nums)
	}
}
