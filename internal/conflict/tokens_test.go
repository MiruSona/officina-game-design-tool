package conflict

import (
	"reflect"
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
