package main

import "fmt"

// printHelp 는 도움말을 찍는다. 도움말과 오류 글은 한국어다.
func printHelp(topic string) {
	if topic != "" {
		if text, ok := topics[topic]; ok {
			fmt.Print(text)
			return
		}
		fmt.Printf("그런 명령이 없습니다 : %s\n\n", topic)
	}
	fmt.Print(general)
}

const general = `stage — 기획 단계 관문과 상태 한 장

쓰는 법 : stage <명령> [옵션…]

  init                      상태 파일(Docs/Todo/기획상태.json)을 새로 만든다
  show                      지금 단계 · 판 · 이번 판 물음을 보여준다
  next                      다음 단계로 넘긴다 (사용자만 한다)
  back <단계> --why "…"     앞 단계로 되돌아가고 까닭을 남긴다
  ask "<물음>" --crit "…"   이번 판 물음을 적는다
  check <id> pass|fail      통과/불통을 적는다
  lint [<파일…>]            순서와 모양을 검사한다 (막을 것이 있으면 종료 2)
                            --changed 로 git 변경분을, --since <ref> 로 커밋된 변경분을 모은다
  done                      판 끝 할 일이 빠졌는지 본다 (경고만)
  dash                      상태를 정적 HTML 한 장으로 굽는다
  version | help [<명령>]

공통 옵션 : --root <경로>   저장소 뿌리 (기본 : 지금 폴더)
옵션은 명령 뒤에 둔다 — 보기 : stage show --root C:\어디\저장소

종료 코드 : 0 잘 됨 · 1 쓰는 법 · 2 검사에 걸림 · 3 읽기 실패 · 4 쓰기 실패 · 5 파일 깨짐

자세한 것은 stage help <명령>.
`

var topics = map[string]string{
	"init": `stage init [--force]

상태 파일을 1단계 · 1판으로 새로 만든다. 이미 있으면 아무것도 안 하고 멈춘다(--force 로 덮어쓴다).
`,
	"show": `stage show [--json] [--hook]

지금 단계 · 판 번호 · 이번 판 물음 상태를 보여준다. 아무것도 안 고친다.
  --json   셈한 값(며칠째 · 되돌아간 횟수)을 얹어 JSON 으로 낸다
  --hook   세션 훅에서 부른다. 상태 파일이 없어도 종료 0 으로 안내만 한다
`,
	"next": `stage next

다음 단계로 넘긴다. **단계를 넘기는 것은 사용자만 한다** — 툴은 막지 않고,
지금 단계의 「끝났다고 보는 기준」과 걸리는 모양 검사를 경고로 되뇐 뒤 넘긴다.
`,
	"back": `stage back <단계> --why "까닭"

앞 단계면 어디로든 되돌아가고 되돌아간 기록을 남긴다. 까닭이 없으면 멈춘다.
고리(3~6) 안으로 되돌아가면 판 번호가 하나 오르고, 1~2단계로 되돌아가면 판 번호가 1 이 된다.
고리 뒤(7~9)로 되돌아가면 판 번호와 판시작일은 그대로고 기록만 한 줄 늘어난다.
`,
	"ask": `stage ask "<물음>" --crit "기준" [--crit "기준"] [--id <손잡이>]

이번 판에 시험할 물음을 적는다. 물음 하나 = 프로토타입 하나 = 답 하나다.
판정 기준은 예/아니오나 숫자로 답할 수 있게 적는다. 「재미있으면 통과」는 기준이 아니다.
`,
	"check": `stage check <물음id> pass|fail [--note "…"]

물음 하나에 통과/불통을 적는다. 불통이면 어디로 되돌아가는지 같이 알려준다.
`,
	"lint": `stage lint [<파일…>] [--changed] [--since <ref>] [--verbose] [--hook]

셀 수 있는 것만 검사한다. 파일을 주면 「이제 그 파일을 쓴다」고 치고 순서와 모양을 본다.
아무것도 안 주면 저장소의 모양만 본다. --hook 은 stdin 으로 훅 JSON 을 받는다.
막을 것이 있으면 종료 2 와 함께 까닭을 stderr 에 낸다. 프로토타입 폴더는 검사에서 뺀다.
  --changed   git 이 본 변경분(작업 트리 + 스테이지)을 스스로 모아 검사한다.
              파일 인자·--hook 과 같이 못 쓴다. git 을 못 부르면 종료 3 이다 (0개 통과로 안 떨어진다)
  --since <ref>  <ref> 에서 HEAD 까지 **커밋된** 변경도 검사한다 (보기 : --since HEAD~1).
              이미 커밋해 버려 --changed 가 0개로 떨어지는 판에 쓴다. ref 가 틀리면 종료 3 이다.
              --changed 와 같이 주면 둘을 합쳐서 본다
  --verbose   파일마다 어느 검사가 돌고 무엇을 왜 건너뛰었나를 같이 찍는다 (--hook 이면 무시)
`,
	"done": `stage done [--hook]

판 끝 할 일 셋(플레이 기록 · 기억 1건 · 진행 상황 문서 고치기)이 빠졌는지 본다.
이번 판 물음이 다 판정된 뒤에만 본다. **막지 않고 경고만** 한다.
--hook 으로 부른 것은 **같은 세션에서 같은 내용이면 한 번만** 뜬다. 손으로 치면 늘 찍는다.
`,
	"dash": `stage dash [--out <경로>]

상태를 정적 HTML 한 장으로 굽는다 (기본 : Docs/Todo/대시보드.html).
구운 파일은 사진이다 — 정본은 상태 파일이다. gitignore 에 넣어 둔다.
`,
}
