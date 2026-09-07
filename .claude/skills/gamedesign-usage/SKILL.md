---
name: gamedesign-usage
description: Use when checking or moving the current design stage, recording a prototype verdict, or baking the design dashboard — 지금 몇 단계인지 볼 때 · 단계를 넘기거나 되돌아갈 때 · 통과/불통을 적을 때 · 대시보드를 구울 때 · 기획 문서를 쓸 때.
---

# 기획 단계는 GamedesignTool 로 지킨다

**1판이 돌아간다.** 아래 명령은 다 쓸 수 있다. 자세한 설계는 `Docs/Design/2026-09-07-1판설계.md`.

## 어디서 부르나 — 명령 두 벌

| 어디서 쓰나 | 명령 앞자리 |
| --- | --- |
| 스튜디오(Officina) 저장소에서 | `.\GamedesignTool\bin\stage.exe …` |
| 게임 저장소에서 (서브모듈) | `.\Tools\GamedesignTool\bin\stage.exe …` |
| GamedesignTool 저장소 단독에서 | `.\bin\stage.exe …` |

아래 표는 짧은 쪽(`stage …`)으로 적는다. 실제로 칠 때는 위 앞자리를 붙인다.
**`bin/` 은 git 에 안 올라간다** — 받은 직후에 `$env:CGO_ENABLED="0"; go build -o bin/stage.exe ./cmd/stage` 를 한 번 친다.

## 지금도 지키는 규칙

- **단계를 넘기는 것은 사용자만 한다.** AI 가 임의로 `stage next` 를 치지 않는다.
- **툴은 모양을 검사하고, 판단은 사람이 한다.** 「재미있나」는 툴이 못 본다.
- **상태의 정본은 상태 파일 하나**다 — 붙은 저장소의 `Docs/Todo/기획상태.json`.
- 되돌아갈 때는 **까닭을 같이 남긴다.** 몇 판 돌았는지가 판단 재료다.

## 명령

| 이럴 때 | 이렇게 |
| --- | --- |
| 처음 시작할 때 | `stage init` |
| 세션을 열었을 때 · 지금 어디쯤인지 볼 때 | `stage show` (`--json` 으로 값만) |
| 이번 판 물음을 적을 때 (4단계) | `stage ask "<물음>" --crit "<기준>" --crit "<기준>"` |
| 검증에서 통과/불통을 적을 때 (6단계) | `stage check <물음id> pass\|fail --note "…"` |
| 단계를 넘길 때 (**사용자만**) | `stage next` |
| 되돌아갈 때 | `stage back <단계> --why "…"` |
| 판이 끝났을 때 빠진 것 보기 | `stage done` |
| 문서·코드를 쓰기 직전 모양 검사 | `stage lint` — 훅이 알아서 부른다 |
| 지금 상태를 한 장으로 보고 싶을 때 | `stage dash` → 구운 HTML 을 브라우저로 연다 |

- **옵션은 명령 뒤에 둔다** : `stage check wait-time pass --note "5분 넘게 돌았다"`.
- 다른 저장소를 볼 때는 `--root <경로>`.
- 종료 코드 : 0 잘 됨 · 1 쓰는 법 · **2 검사에 걸림** · 3 읽기 실패 · 4 쓰기 실패 · 5 파일 깨짐.

## 기획 문서가 지켜야 할 규약

훅이 **셀 수 있는 것만** 본다. 그래서 세는 자리가 정해져 있다. 기획 문서를 쓸 때 이 모양을 지킨다.

| 무엇 | 어디에 | 어떻게 |
| --- | --- | --- |
| 기둥 3~4개 | `00-컨셉.md` | `## 기둥` 제목 아래 **표**로 적는다. 그 절의 **첫 표의 본문 줄 수**로 센다 |
| Won't 칸 | `02-기능목록.md` | **첫 표**의 첫 칸이 `Won't` 인 줄이 하나 이상 있어야 한다 |
| 파일 이름 | `Docs/Design/` **바로 아래** | `NN-이름.md` 꼴 (`00-컨셉.md` · `03-시스템-<이름>.md`). 하위 폴더는 검사 밖 |
| 문서 순서 | 같은 곳 | 앞 번호 문서가 없으면 뒤 번호 문서를 **새로 못 만든다** (이미 있는 문서 고치기는 괜찮다) |

- 기둥·Won't 검사는 **그 문서를 쓰는 단계가 지난 뒤에만 막는다** (기둥은 3단계부터, Won't 는 8단계부터).
  그 전에는 경고만 한다 — 안 그러면 문서를 시작조차 못 한다.
- **프로토타입 폴더는 검사에서 뺀다.** 버릴 코드를 막으면 5단계가 답답해진다.

## 훅 셋 (붙은 저장소 `.claude/settings.json`)

| 훅 | 부르는 것 | 무엇을 |
| --- | --- | --- |
| `SessionStart` | `stage show --hook` | 지금 단계를 문맥에 넣는다 (상태 파일이 없어도 안 죽는다) |
| `PreToolUse` (Write\|Edit) | `stage lint --hook` | 순서·모양을 검사해 어긋나면 **종료 2 로 막는다** |
| `Stop` | `stage done --hook` | 판 끝 할 일이 빠졌으면 **경고만** 한다 |

자세한 몫 나누기와 대시보드 화면 배치는 `README.md`.
