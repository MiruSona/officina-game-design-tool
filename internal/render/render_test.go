package render

import "testing"

func TestWidthKorean(t *testing.T) {
	if Width("가") != 2 {
		t.Fatal("한글은 두 칸이다")
	}
	if Width("ab") != 2 {
		t.Fatal("영문은 한 칸씩이다")
	}
	if Width("가a") != 3 {
		t.Fatalf("섞인 글 = %d", Width("가a"))
	}
}

func TestPadFitsWidth(t *testing.T) {
	if got := Pad("가", 4); Width(got) != 4 {
		t.Fatalf("채운 폭 = %d", Width(got))
	}
	if got := Pad("가나다", 2); got != "가나다" {
		t.Fatal("더 길면 자르지 않는다")
	}
}

func TestBadge(t *testing.T) {
	if Badge("통과") != "[통과]" {
		t.Fatal("딱지 꼴이 다르다")
	}
}
