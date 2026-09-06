# public_guard — 공개 저장소 새는 것 막기

공개 GitHub 저장소(툴 저장소)에 **비밀값·개인정보·금칙어**가 올라가는 것을 막는다.
파이썬 3.10+ 표준 라이브러리만 쓴다.

## 무엇을 막나

| 규칙 | 잡는 것 | 통과시키는 것 |
| --- | --- | --- |
| `token` | GitHub·AWS·OpenAI/Anthropic·Slack·Google 키, 개인키 블록, `Bearer …`, `api_key = …` 꼴 | — |
| `ip` | IPv4 주소 | `0.0.0.0` · `127.0.0.1` · `localhost` |
| `mac` | MAC 주소 | — |
| `homepath` | `C:\Users\이름` · `/home/이름` · `/Users/이름` | — |
| `email` | 이메일 주소 | `@example.com` · `noreply@anthropic.com` · `@users.noreply.github.com` |
| `localfile` | 파일 이름이 `*.local.md` · `.env` · `.env.*` · `*.pem` · `*.key` | — |
| `word` | `public_guard.words` 에 적은 금칙어 | 목록 파일이 없으면 규칙을 건너뛴다 |

`.claude/hooks/` · `.githooks/` 아래 파일, 검사기 자신, 이진 파일, 2MB 넘는 파일은 건너뛴다.

## 설치

1. `public_guard.py` 를 저장소의 `.claude/hooks/` 에 복사한다.
   금칙어를 쓸 거면 `public_guard.words.example` 을 같은 폴더에 `public_guard.words` 로 복사해 채운다.
2. `python .claude/hooks/public_guard.py --install` — `.githooks/pre-commit` 을 만들고
   `git config core.hooksPath .githooks` 를 걸어 준다. 훅이 이미 있으면 덮어쓰지 않고 알린다.

## 훅 둘

- **git pre-commit** — 커밋할 때 스테이지된 파일을 검사한다 (`--staged`). 걸리면 커밋이 멈춘다.
- **Claude Code PreToolUse** — `settings.snippet.json` 을 저장소 `.claude/settings.json` 에 합치면
  Claude 가 Write·Edit 하기 전에 검사한다 (`--hook`). 걸리면 편집 자체가 막히고 이유가 모델에게 보인다.

## 목록은 어디서 읽나

`word` 규칙의 금칙어 목록은 세 자리에서 찾아 **전부 합친다** (중복은 한 번만).

1. 스크립트와 같은 폴더의 `public_guard.words` — 이 저장소 전용 목록
2. 저장소 최상위(`git rev-parse --show-toplevel`, 실패하면 현재 폴더)에서 시작해
   **부모 폴더를 차례로 올라가며** 각 폴더의 `.claude/hooks/public_guard.words` — 드라이브 뿌리까지 본다.
   툴이 게임 저장소에 `Tools/<툴>/` 서브모듈로 붙으면 게임 저장소의 목록이 여기서 걸린다.
3. `--words <경로>` 로 직접 준 파일

하나도 없으면 `word` 규칙 자체를 건너뛴다.

**공개 저장소에는 실제 목록을 올리지 않는다.** 목록에 적힌 낱말 자체가 새는 것이기 때문이다.
`.gitignore` 에 `.claude/hooks/public_guard.words` 를 넣고 `public_guard.words.example` 만 올린다.
진짜 목록은 비공개 저장소(게임 저장소)의 `.claude/hooks/public_guard.words` 에 둔다 — 서브모듈이 알아서 찾는다.

## 손으로 돌리기

```
python public_guard.py --files a.md b.py    # 지정 파일
python public_guard.py --all                # git 이 아는 파일 전부
python public_guard.py --staged --repo ../x # 다른 저장소의 스테이지된 파일
python public_guard.py --files a.md --words ../비공개/.claude/hooks/public_guard.words
```

## 예외 — `guard:ok`

정말 괜찮은 줄이면 그 줄에 `guard:ok` 를 적는다. 그 줄만 건너뛴다.

```
DEFAULT_HOST = "192.168.0.1"  # 문서용 예시다  guard:ok
```

## 출력과 종료 코드

걸린 것마다 `파일:줄  규칙  가린값` 한 줄이 나온다. 값은 앞 4자만 보인다.

| 코드 | 뜻 |
| --- | --- |
| 0 | 깨끗함 |
| 1 | 사용법·환경 오류 |
| 2 | 걸림 (커밋·편집을 막는다) |
