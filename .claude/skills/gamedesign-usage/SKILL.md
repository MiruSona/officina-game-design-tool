---
name: gamedesign-usage
description: Use when checking or moving the current design stage, recording a prototype verdict, or baking the design dashboard — 지금 몇 단계인지 볼 때 · 단계를 넘기거나 되돌아갈 때 · 통과/불통을 적을 때 · 대시보드를 구울 때.
---

# 기획 단계는 GamedesignTool 로 지킨다

**아직 뼈대다.** 코드가 없어서 아래 명령은 전부 **예정**이다. **1판이 나오면 채운다.**
지금은 절차 문서를 손으로 지킨다.

## 어디서 부르나 — 명령 두 벌

| 어디서 쓰나 | 명령 앞자리 |
| --- | --- |
| 스튜디오(Officina) 저장소에서 | `.\GamedesignTool\bin\stage.exe …` |
| 게임 저장소에서 (서브모듈) | `.\Tools\GamedesignTool\bin\stage.exe …` |
| GamedesignTool 저장소 단독에서 | `.\bin\stage.exe …` |

아래 표는 짧은 쪽(`stage …`)으로 적는다. 실제로 칠 때는 위 앞자리를 붙인다.

## 지금도 지키는 규칙

- **단계를 넘기는 것은 사용자만 한다.** AI 가 임의로 다음 단계로 가지 않는다.
- **툴은 모양을 검사하고, 판단은 사람이 한다.** 「재미있나」는 툴이 못 본다.
- **상태의 정본은 상태 파일 하나**다 — 게임 저장소의 `Docs/Todo/기획상태.json`.
  같은 것을 두 군데 적지 않는다.
- 되돌아갈 때는 **까닭을 같이 남긴다.** 몇 판 돌았는지가 판단 재료다.

## 예정 명령 (1판이 나오면 여기 채운다)

| 이럴 때 | 이렇게 (예정) |
| --- | --- |
| 세션을 열었을 때 · 지금 어디쯤인지 볼 때 | `stage show` |
| 단계를 넘길 때 (**사용자만**) | `stage next` |
| 되돌아갈 때 | `stage back <단계> --why "…"` |
| 검증에서 통과/불통을 적을 때 | `stage check <물음> pass\|fail --note "…"` |
| 문서·코드를 쓰기 직전 모양 검사 | `stage lint` — 훅이 알아서 부른다 |
| 지금 상태를 한 장으로 보고 싶을 때 | `stage dash` → 구운 HTML 을 브라우저로 연다 |

## 훅 셋 (게임 저장소 `.claude/settings.json` 에 건다)

| 훅 | 무엇을 |
| --- | --- |
| `SessionStart` | `stage show` 로 지금 단계를 문맥에 넣는다 |
| `PreToolUse` (Write/Edit) | `stage lint` 로 순서·모양을 검사해 어긋나면 막는다 |
| `Stop` | 판 끝에 할 일이 빠졌으면 경고한다 |

**프로토타입 폴더는 검사에서 뺀다.** 버릴 코드를 막으면 프로토타입 단계가 답답해진다.

자세한 몫 나누기와 대시보드 화면 배치는 `README.md`.
