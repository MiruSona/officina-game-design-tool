#!/usr/bin/env python3
"""공개 GitHub 저장소에 비밀값·개인정보·금칙어가 올라가는 것을 막는 검사기.

원본은 ModTool 의 commit-prep mod 안 guard/public_guard.py 이다. 이 파일은 그 거울 사본이다 —
검사 핵심(규칙·예외·scan_text·scan_file)은 원본과 글자까지 같게 두고, 고칠 일은 원본에서 고친 뒤 베껴 온다.
GamedesignTool · ProtoTool 의 .claude/hooks/public_guard.py 두 벌은 서로 바이트까지 같다.
원본 주석의 「(사본에서 더함)」 은 원본 쪽에서 본 말이라 그대로 둔다.

툴 저장소 사본이 원본에 다시 붙인 실행 꼴 (원본 mod 는 --files-from · --all 만 쓴다) :
  --hook (Claude Code PreToolUse) · --staged (.githooks/pre-commit) · --files · --install · --words

원본은 거짓 경보 셋을 애초에 안 잡는다 (guard:ok 를 붙이면 자동 모드 분류기가 「보안 완화」 로 커밋을 거부한 일이 있어서) :
  - 시험 경로 (testdata/ · tests/ · test/ · test_* · *_test.* · *.test.*) 는 통째로 건너뛴다
    (단 .env · .env.* · *.env · *.pem · *.key · id_rsa* 같은 비밀 파일 이름은 시험 경로라도 검사한다)
  - 예시 메일 (*.example · *.invalid · example.com · example.org)
  - 사설 IP (10. · 192.168. · 172.16~31.) 와 문서 IP (192.0.2. · 198.51.100. · 203.0.113.)
"""

import argparse
import json
import os
import re
import subprocess
import sys

# 윈도우 콘솔이 cp949 라도 한글·… 이 깨지지 않게 맞춘다.
for stream in (sys.stdout, sys.stderr):
    if hasattr(stream, "reconfigure"):
        stream.reconfigure(encoding="utf-8", errors="replace")

MAX_BYTES = 2 * 1024 * 1024
SKIP_MARK = "guard:ok"
SKIP_DIRS = (".claude/hooks/", ".githooks/")
WORDS_FILE = "public_guard.words"
SELF_FILE = "public_guard.self"
# 꼴이 없는 개인 식별자 목록 (계정 이름 · SSH 별칭 · 기기 이름 · 본명). 목록이 추적되면 그게 유출이라 git 밖에만 둔다
PRIVATE_FILE = "private.words"
PRIVATE_RULE = "private"
# 낱말 목록 머리의 범위 표시. `# scope: submodules` 면 그 목록이 든 저장소 밖(서브모듈 등)의 파일에만 쓴다
SCOPE_HEAD = re.compile(r"^#\s*scope:\s*(\w+)", re.IGNORECASE)
SCOPE_SUBMODULES = "submodules"

# 규칙 이름 -> 정규식. 순서대로 검사해 걸린 것을 모두 보고한다.
# 토큰 규칙의 값 자리가 이 꼴로 시작하면 값이 아니라 참조다 : os.environ · getenv( · process.env · ${…} · $VAR · $env:VAR · .NET · Java
# `$이름` 꼴은 실제 값이어도 참조로 본다 — 알고 둔 구멍 (꼴로는 구분이 안 된다)
ENV_REF = (
    r"os\.environ|os\.getenv\(|getenv\(|process\.env|\$\{|\$env:|\$[A-Za-z_]"
    r"|Environment\.GetEnvironmentVariable\(|System\.getenv\("
)
# 값 자리가 따옴표 없이 아래 꼴이면 값이 아니라 코드다 (Go·C# 대입문 거짓 경보)
#   - 「식별자(.식별자)*」 뒤에 ( 나 { : 함수 호출 secret.New( · 구조체 리터럴 SecretConfig{
#   - 점이 하나 이상 든 식별자 사슬이 값 끝(공백·, ; ) } ] `·줄끝)까지 : 필드 참조 repository.Config.Secret
#     (백틱은 마크다운 코드 조각의 끝 : `secret = repo.Config.Secret`)
# 맨 식별자 하나(`password = hunter2hunter2` 꼴)는 값일 수 있어 그대로 잡는다
# 점 사슬 예외에 걸리는 JWT 는 아래 JWT 규칙이 따로 잡는다
IDENT = r"[A-Za-z_][A-Za-z0-9_]*"
CODE_REF = (
    r"(?:" + IDENT + r"(?:\." + IDENT + r")*[({])"
    r"|(?:" + IDENT + r"(?:\." + IDENT + r")+(?=[\s,;)}\]`]|$))"
)
# 홈 폴더 사용자 이름. 첫 글자가 점이면 이름이 아니다 (`..` 줄임 표시)
USER_NAME = r"[A-Za-z0-9_-][A-Za-z0-9._-]*"
# 일반 「secret = …」 규칙만 주석 줄에서 건너뛴다 (영어 주석 속 글 거짓 경보). 접두사 규칙은 주석에서도 잡는다
GENERIC_RULE = "generic"
# 줄 시작(공백 뒤) 주석 머리. `--` 는 뒤에 공백·줄끝일 때만 (CLI 플래그 `--password=` 와 안 겹치게)
LINE_COMMENT = re.compile(r"^\s*(?://|#|\*|/\*|--(?:\s|$))")
# 마크다운·글 파일은 `#` 제목·`*` 목록이 주석이 아니라 본문이라 주석 예외를 안 쓴다
PROSE_ENDS = (".md", ".markdown", ".txt")

RULES = [
    ("token", re.compile(r"gh[pousr]_[A-Za-z0-9]{16,}|github_pat_[A-Za-z0-9_]{20,}")),
    ("token", re.compile(r"AKIA[0-9A-Z]{16}")),
    ("token", re.compile(r"sk-[A-Za-z0-9_-]{20,}")),
    ("token", re.compile(r"xox[baprs]-[A-Za-z0-9-]+")),
    ("token", re.compile(r"AIza[0-9A-Za-z_-]{35}")),
    ("token", re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----")),
    ("token", re.compile(r"Bearer [A-Za-z0-9._-]{20,}")),
    # JWT (머리.몸.서명). 일반 규칙의 점 사슬 예외에 빠지므로 따로 잡는다
    ("token", re.compile(r"eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]+")),
    (
        GENERIC_RULE,
        re.compile(
            r"(?:api[_-]?key|secret|token|passw(?:or)?d)\s*[:=]\s*[\"']?"
            # 값이 아니라 환경 변수를 읽는 참조 꼴이면 안 잡는다 (고칠 수 없는 거짓 경보)
            r"(?!" + ENV_REF + r")"
            # 따옴표 없는 코드 식(호출·구조체 리터럴·점 사슬 필드)도 값이 아니다. 따옴표 뒤면 문자열이라 이 예외를 안 쓴다
            # `=?` 는 Go 의 붙여 쓴 `:=` 꼴 (`[:=]` 가 `:` 만 먹고 `=` 가 값 자리에 남는다)
            r"(?:(?<=[\"'])|(?!=?(?:" + CODE_REF + r")))"
            # 값 본문에 백틱은 안 넣는다 (마크다운 코드 조각을 닫는 백틱까지 값으로 먹지 않게)
            r"[^\s\"'`]{8,}",
            re.IGNORECASE,
        ),
    ),
    # 참조 뒤에 리터럴 기본값을 붙인 꼴은 그 기본값이 실제 값이라 잡는다 : os.environ.get("KEY", "sk-…")
    (
        "token",
        re.compile(
            # 기본값이 경로 꼴(C:/ · C:\ · / · ./ · ~)이면 비밀이 아니다 : os.environ.get('WINDIR', 'C:/Windows')
            r"(?:environ\.get|getenv|GetEnvironmentVariable)\([^)]*,\s*[\"'](?![A-Za-z]:[/\\]|\.?/|~)[^\s\"']{8,}[\"']",
            re.IGNORECASE,
        ),
    ),
    ("ip", re.compile(r"\b(?:\d{1,3}\.){3}\d{1,3}\b")),
    ("mac", re.compile(r"\b(?:[0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}\b")),
    # 사용자 이름 첫 글자는 점이 아니다 (`/c/Users/..` 같은 줄임 표시를 이름으로 안 잡게)
    ("homepath", re.compile(
        r"[A-Za-z]:\\Users\\" + USER_NAME + r"|/home/" + USER_NAME + r"|/Users/" + USER_NAME
    )),
    ("email", re.compile(r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b")),
]

IP_OK = {"0.0.0.0", "127.0.0.1"}
# 네 토막 숫자가 버전 번호로 보이는 줄 : 단어 머리 version · 붙여 쓴 Version(XboxOneVersion) · VERSION · 버전 · ver
# (conversion 같은 낱말 속 소문자 version 은 안 친다)
VERSION_WORD = re.compile(r"\b[Vv]ersion|Version|VERSION|버전|\b[Vv]er\b|\bVER\b")
# 가명 홈 경로의 사용자 이름 자리 (소문자). 이력 다시 쓰기·문서 예시에서 진짜 이름 대신 쓴다
HOMEPATH_OK = {"user", "username", "<user>", "<이름>", "<사용자>", "name"}
# git@github.com 은 서브모듈·원격 주소(git@github.com:owner/repo)의 SSH 사용자라 사람 메일이 아니다
EMAIL_OK = ("@example.com", "noreply@anthropic.com", "@users.noreply.github.com", "git@github.com")
# 레티나 그림 이름(icon@2x.png)이 메일 꼴에 걸린다
IMAGE_ENDS = (".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg")
LOCAL_FILE_PATTERNS = (".local.md", ".pem", ".key")

# (사본에서 더함) 예시로만 쓰는 메일 도메인 끝 · 사설·문서 IP 머리 · 시험 폴더 이름
EMAIL_EXAMPLE_ENDS = (".example", ".invalid", "example.com", "example.org")
IP_SAFE_PREFIXES = ("10.", "192.168.", "192.0.2.", "198.51.100.", "203.0.113.")
TEST_DIRS = {"testdata", "tests", "test"}
# 시험 경로 아래라도 봐주지 않는 비밀 파일 이름 (머리 · 끝)
SECRET_NAME_HEADS = (".env.", "id_rsa")
SECRET_NAME_ENDS = (".env", ".pem", ".key")

# --repo 값. 시험 경로를 저장소 기준 상대 경로로 보려고 main 이 채운다.
REPO_ROOT = None


def is_safe_ip(ip):
    """사설·문서용 IP 면 참."""
    if ip.startswith(IP_SAFE_PREFIXES):
        return True
    parts = ip.split(".")
    return parts[0] == "172" and parts[1].isdigit() and 16 <= int(parts[1]) <= 31


def is_example_email(mail):
    """예시 도메인 메일이면 참."""
    domain = mail.rsplit("@", 1)[-1].lower()
    for end in EMAIL_EXAMPLE_ENDS:
        bare = end.lstrip(".")
        if domain == bare or domain.endswith("." + bare):
            return True
    return False


def is_test_path(path):
    """시험 폴더 안이거나 시험 파일 이름이면 참 (비밀 파일 이름은 늘 거짓). 저장소 기준 상대 경로로 본다."""
    rel = path
    if REPO_ROOT is not None:
        try:
            rel = os.path.relpath(os.path.abspath(path), os.path.abspath(REPO_ROOT))
        except ValueError:
            rel = path
    parts = rel.replace("\\", "/").lower().split("/")
    name = parts[-1]
    if name.startswith(SECRET_NAME_HEADS) or name.endswith(SECRET_NAME_ENDS):
        return False
    if any(part in TEST_DIRS for part in parts[:-1]):
        return True
    return name.startswith("test_") or "_test." in name or ".test." in name


def is_version_number(line, match):
    """네 토막 숫자가 IP 가 아니라 버전 번호면 참 : 줄에 version·버전·ver · 바로 앞이 v/V · .0.0 으로 끝남."""
    if match.group(0).endswith(".0.0"):
        return True
    start = match.start()
    if start > 0 and line[start - 1] in "vV":
        return True
    return bool(VERSION_WORD.search(line))


def is_alias_home(found):
    """홈 경로의 사용자 이름 자리가 가명이면 참 : C:\\Users\\USER · /home/user 같은 것."""
    name = found.replace("\\", "/").rstrip("/").rsplit("/", 1)[-1]
    return name.lower() in HOMEPATH_OK


def mask_private(value):
    """개인 식별자는 더 세게 가린다 : 첫 글자 + … + (N자). 짧은 값도 다 보이지 않게."""
    return f"{value[:1]}…({len(value)}자)"


def mask(value):
    """값 앞 4자만 남기고 가린다."""
    if len(value) <= 4:
        return value + "…"
    return value[:4] + "…"


def is_local_file(path):
    """이름만 보고 통째로 막을 파일인지 본다."""
    name = os.path.basename(path).lower()
    if name == ".env" or name.startswith(".env."):
        return True
    return any(name.endswith(suffix) for suffix in LOCAL_FILE_PATTERNS)


def should_skip_path(path, script_path):
    """훅 폴더와 검사기 자신은 건너뛴다."""
    normalized = path.replace("\\", "/")
    # .self 만은 훅 폴더 안이어도 검사한다. 여기 적은 낱말은 그대로 공개되기 때문이다.
    if os.path.basename(normalized) == SELF_FILE:
        return False
    # 개인 식별자 목록 자체는 검사하지 않는다 (git 밖에 두는 파일이라 원래 올라갈 일이 없다)
    if os.path.basename(normalized) == PRIVATE_FILE:
        return True
    if any(part in normalized for part in SKIP_DIRS):
        return True
    if is_test_path(path):
        return True
    if os.path.basename(normalized) == os.path.basename(script_path):
        return True
    return False


def read_words_file(path):
    """목록 파일 한 개를 읽는다. 주석·빈 줄은 버리고 소문자로 맞춘다."""
    if not os.path.isfile(path):
        return []

    words = []
    with open(path, encoding="utf-8-sig", errors="replace") as handle:
        for raw in handle:
            line = raw.strip()
            if not line or line.startswith("#"):
                continue
            words.append(line.lower())
    return words


def read_words_scope(path):
    """목록 파일 머리(앞쪽 주석 줄들)의 `# scope: …` 값. 없거나 모르는 값이면 None (= 전부 적용)."""
    if not os.path.isfile(path):
        return None
    with open(path, encoding="utf-8-sig", errors="replace") as handle:
        for raw in handle:
            line = raw.strip()
            if not line:
                continue
            if not line.startswith("#"):
                return None
            found = SCOPE_HEAD.match(line)
            if found:
                value = found.group(1).lower()
                return value if value == SCOPE_SUBMODULES else None
    return None


# 폴더 -> 저장소 최상위 (정규화). git 을 파일마다 부르지 않으려고 둔다.
ROOT_CACHE = {}


def norm_path(path):
    """비교용 경로. 절대 경로 + 윈도우 대소문자·구분자 맞춤."""
    return os.path.normcase(os.path.normpath(os.path.abspath(path)))


def cached_repo_root(folder):
    """repo_root 를 폴더별로 캐시해 정규화한 값으로 돌려준다."""
    key = norm_path(folder)
    if key not in ROOT_CACHE:
        ROOT_CACHE[key] = norm_path(repo_root(folder)) if os.path.isdir(folder) else key
    return ROOT_CACHE[key]


def list_owner_dir(path):
    """목록 파일을 가진 저장소 자리. `<x>/.claude/hooks/<목록>` 이면 <x>, 아니면 목록이 든 폴더."""
    folder = os.path.dirname(os.path.abspath(path))
    hooks = os.path.basename(folder).lower()
    claude = os.path.basename(os.path.dirname(folder)).lower()
    if hooks == "hooks" and claude == ".claude":
        return os.path.dirname(os.path.dirname(folder))
    return folder


def repo_root(start):
    """저장소 최상위. git 이 답을 못 주면 준 폴더를 그대로 쓴다."""
    try:
        out = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            cwd=start,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            check=True,
        ).stdout
    except (OSError, subprocess.CalledProcessError):
        return os.path.abspath(start)
    return out.decode("utf-8", errors="replace").strip() or os.path.abspath(start)


def word_list_paths(script_dir, start_dir=None):
    """목록을 찾을 자리들. 스크립트 폴더 -> 저장소 최상위에서 부모로 거슬러 올라감."""
    paths = [os.path.join(script_dir, WORDS_FILE)]

    if start_dir is None:
        start_dir = os.getcwd()
    current = os.path.abspath(repo_root(start_dir))
    while True:
        paths.append(os.path.join(current, ".claude", "hooks", WORDS_FILE))
        parent = os.path.dirname(current)
        if parent == current:
            break
        current = parent

    return paths


# `# scope: submodules` 목록의 낱말 : [(낱말, 그 목록을 가진 저장소 루트)]. load_words 가 채우고 words_for_path 가 쓴다.
SCOPED_WORDS = []


def load_words(script_dir, start_dir=None):
    """찾은 목록을 전부 합친다 (중복 제거). 하나도 없으면 빈 목록이라 word 규칙을 건너뛴다.

    범위 머리가 붙은 목록의 낱말은 돌려주는 목록에 안 넣고 SCOPED_WORDS 에 따로 둔다.
    """
    words = []
    SCOPED_WORDS.clear()
    for path in word_list_paths(script_dir, start_dir):
        if read_words_scope(path) == SCOPE_SUBMODULES:
            owner = cached_repo_root(list_owner_dir(path))
            for word in read_words_file(path):
                if (word, owner) not in SCOPED_WORDS:
                    SCOPED_WORDS.append((word, owner))
            continue
        for word in read_words_file(path):
            if word not in words:
                words.append(word)
    return words


# 폴더 -> 그 자리의 .claude/local/private.words 낱말. 같은 폴더를 여러 번 읽지 않으려고 둔다.
PRIVATE_CACHE = {}


def account_private_path():
    """계정 목록 자리 : ~/.claude/private.words."""
    return os.path.join(os.path.expanduser("~"), ".claude", PRIVATE_FILE)


def read_local_private(folder):
    """<folder>/.claude/local/private.words 를 읽는다 (캐시)."""
    key = norm_path(folder)
    if key not in PRIVATE_CACHE:
        PRIVATE_CACHE[key] = read_words_file(os.path.join(key, ".claude", "local", PRIVATE_FILE))
    return PRIVATE_CACHE[key]


def private_words_for(path):
    """그 파일에 쓸 개인 식별자 : 계정 목록 + 파일 자리에서 부모로 거슬러 올라가며 만난 .claude/local 목록."""
    words = []
    for word in read_words_file(account_private_path()):
        if word not in words:
            words.append(word)
    current = os.path.dirname(os.path.abspath(path))
    while True:
        for word in read_local_private(current):
            if word not in words:
                words.append(word)
        parent = os.path.dirname(current)
        if parent == current:
            break
        current = parent
    return words


def repo_names(base):
    """그 저장소를 가리키는 이름들 (폴더 이름 · git remote 이름). 전부 소문자."""
    root = repo_root(base)
    names = {
        os.path.basename(os.path.abspath(root)).lower(),
        os.path.basename(os.path.abspath(base)).lower(),
    }

    try:
        out = subprocess.run(
            ["git", "remote", "get-url", "origin"],
            cwd=root,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            check=True,
        ).stdout
    except (OSError, subprocess.CalledProcessError):
        out = b""
    url = out.decode("utf-8", errors="replace").strip().rstrip("/")
    if url:
        name = url.rsplit("/", 1)[-1].rsplit(":", 1)[-1]
        if name.endswith(".git"):
            name = name[:-4]
        names.add(name.lower())

    names.discard("")
    return names


# 폴더 -> 검증을 통과한 .self 낱말. 같은 폴더를 여러 번 읽지 않으려고 둔다.
SELF_CACHE = {}


def read_self_words(base):
    """<base>/.claude/hooks/public_guard.self 에서 그 저장소 이름과 맞는 낱말만 읽는다."""
    key = os.path.abspath(base)
    if key in SELF_CACHE:
        return SELF_CACHE[key]

    path = os.path.join(key, ".claude", "hooks", SELF_FILE)
    words = read_words_file(path)
    kept = []
    if words:
        names = repo_names(key)
        for word in words:
            if word in names:
                kept.append(word)
            else:
                print(
                    f"{path} : 저장소 이름이 아니라 무시한다 : {mask(word)}",
                    file=sys.stderr,
                )

    SELF_CACHE[key] = kept
    return kept


def self_words_for(path):
    """검사 대상 파일 자리에서 부모로 거슬러 올라가며 .self 낱말을 모은다."""
    words = []
    current = os.path.dirname(os.path.abspath(path))
    while True:
        for word in read_self_words(current):
            if word not in words:
                words.append(word)
        parent = os.path.dirname(current)
        if parent == current:
            break
        current = parent
    return words


def words_for_path(path, words):
    """그 파일에 쓸 금칙어. 범위 붙은 낱말은 목록 주인과 다른 저장소의 파일에만 더하고, 파일 저장소의 이름은 뺀다."""
    if SCOPED_WORDS:
        root = cached_repo_root(os.path.dirname(os.path.abspath(path)))
        merged = list(words)
        for word, owner in SCOPED_WORDS:
            if owner != root and word not in merged:
                merged.append(word)
        words = merged
    own = self_words_for(path)
    if not own:
        return words
    return [word for word in words if word not in own]


def block_comment_open(line, in_block):
    """이 줄 뒤에 /* … */ 블록 주석 안인지 돌려준다. 마지막 /* 와 */ 중 뒤에 온 쪽이 이긴다."""
    start = line.rfind("/*")
    end = line.rfind("*/")
    if start < 0 and end < 0:
        return in_block
    return start > end


def scan_text(path, text, words, private=None):
    """내용을 줄 단위로 검사해 (파일, 줄번호, 규칙, 가린값) 목록을 돌려준다. private 는 개인 식별자 목록."""
    hits = []
    use_comments = not path.lower().endswith(PROSE_ENDS)
    in_block = False
    for number, line in enumerate(text.splitlines(), start=1):
        comment = use_comments and (in_block or bool(LINE_COMMENT.match(line)))
        if use_comments:
            in_block = block_comment_open(line, in_block)
        if SKIP_MARK in line:
            continue

        for rule, pattern in RULES:
            if rule == GENERIC_RULE and comment:
                continue
            name = "token" if rule == GENERIC_RULE else rule
            for match in pattern.finditer(line):
                found = match.group(0)
                if name == "ip" and (found in IP_OK or is_safe_ip(found) or is_version_number(line, match)):
                    continue
                if name == "homepath" and is_alias_home(found):
                    continue
                if name == "email" and (
                    any(found.endswith(ok) or found == ok for ok in EMAIL_OK)
                    or is_example_email(found)
                    or found.lower().endswith(IMAGE_ENDS)
                ):
                    continue
                hits.append((path, number, name, mask(found)))

        lowered = line.lower()
        for word in words:
            if word in lowered:
                hits.append((path, number, "word", mask(word)))
        for word in private or ():
            if word in lowered:
                hits.append((path, number, PRIVATE_RULE, mask_private(word)))
    return hits


def index_bytes(path, repo):
    """(사본에서 더함) 올라가 있는(staged) 내용 = git show :<경로>. 못 읽으면 None."""
    try:
        rel = os.path.relpath(os.path.abspath(path), os.path.abspath(repo)).replace("\\", "/")
        return subprocess.run(
            ["git", "show", ":" + rel], cwd=repo, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, check=True
        ).stdout
    except (OSError, ValueError, subprocess.CalledProcessError):
        return None


def scan_file(path, words, script_path, index_repo=None):
    """파일 하나를 검사한다. 이름 규칙 -> 크기 -> 이진 -> 내용 순.

    index_repo 를 주면 작업 트리 대신 올라가 있는 내용을 본다 (못 읽으면 작업 트리로 떨어진다).
    """
    if should_skip_path(path, script_path):
        return []
    if is_local_file(path):
        return [(path, 0, "localfile", os.path.basename(path))]
    raw = index_bytes(path, index_repo) if index_repo is not None else None
    if raw is None:
        if not os.path.isfile(path):
            return []
        if os.path.getsize(path) > MAX_BYTES:
            print(f"건너뜀 (2MB 초과) : {path}")
            return []
        with open(path, "rb") as handle:
            raw = handle.read()
    if len(raw) > MAX_BYTES:
        print(f"건너뜀 (2MB 초과) : {path}")
        return []
    if b"\x00" in raw:
        return []

    text = raw.decode("utf-8", errors="replace")
    return scan_text(path, text, words_for_path(path, words), private_words_for(path))


def git_files(repo, args):
    """git 명령으로 파일 목록을 받는다. NUL 로 끊어 공백 있는 경로도 안전하다."""
    try:
        out = subprocess.run(
            ["git"] + args, cwd=repo, stdout=subprocess.PIPE, check=True
        ).stdout
    except (OSError, subprocess.CalledProcessError) as error:
        print(f"git 실행 실패 : {error}", file=sys.stderr)
        sys.exit(1)
    return [name for name in out.decode("utf-8", errors="replace").split("\0") if name]


def report(hits):
    """걸린 것을 한 줄씩 찍고 종료 코드를 정한다."""
    if not hits:
        return 0
    for path, number, name, value in hits:
        print(f"{path}:{number}  {name}  {value}")
    print("걸림 %d건 — 커밋을 막았다. 정말 괜찮은 줄이면 guard:ok 를 붙인다." % len(hits))
    return 2


def read_list(where):
    """(사본에서 더함) --files-from 목록 : [(I 또는 W, 경로)]. 표준 입력은 utf-8 로 읽는다."""
    if where == "-":
        text = sys.stdin.buffer.read().decode("utf-8", errors="replace")
    else:
        with open(where, encoding="utf-8-sig", errors="replace") as handle:
            text = handle.read()
    out = []
    for line in text.splitlines():
        if not line.strip():
            continue
        if len(line) > 2 and line[1] == "\t" and line[0] in "IW":
            out.append((line[0], line[2:]))
        else:
            out.append(("W", line))
    return out


def run_hook(words, script_path):
    """(툴 사본) Claude Code PreToolUse 훅. 걸리면 stderr 에 이유를 쓰고 2 로 끝난다."""
    try:
        payload = json.loads(sys.stdin.buffer.read().decode("utf-8", errors="replace") or "{}")
    except ValueError as error:
        print(f"훅 입력을 읽지 못했다 : {error}", file=sys.stderr)
        return 1

    tool_input = payload.get("tool_input") or {}
    path = tool_input.get("file_path") or "(이름 없음)"
    content = tool_input.get("content")
    if content is None:
        content = tool_input.get("new_string") or ""

    if should_skip_path(path, script_path):
        return 0

    hits = []
    if is_local_file(path):
        hits.append((path, 0, "localfile", os.path.basename(path)))
    hits.extend(scan_text(path, content, words_for_path(path, words), private_words_for(path)))

    if not hits:
        return 0

    for hit_path, number, name, value in hits:
        print(f"{hit_path}:{number}  {name}  {value}", file=sys.stderr)
    print("걸림 %d건 — 쓰기를 막았다. 정말 괜찮은 줄이면 guard:ok 를 붙인다." % len(hits), file=sys.stderr)
    return 2


HOOK_SCRIPT = """#!/bin/sh
# 공개 저장소 검사 — 스테이지된 파일에서 비밀값·개인정보·금칙어를 찾는다.
ROOT=$(git rev-parse --show-toplevel)
GUARD="$ROOT/.claude/hooks/public_guard.py"
if command -v python >/dev/null 2>&1; then
    python "$GUARD" --staged || exit 1
else
    py -3 "$GUARD" --staged || exit 1
fi
"""


def install(repo):
    """(툴 사본) .githooks/pre-commit 을 쓰고 git 이 그 폴더를 보게 한다."""
    hook_dir = os.path.join(repo, ".githooks")
    hook_path = os.path.join(hook_dir, "pre-commit")
    if os.path.exists(hook_path):
        print(f"이미 있다 — 덮어쓰지 않았다 : {hook_path}")
    else:
        os.makedirs(hook_dir, exist_ok=True)
        with open(hook_path, "w", encoding="utf-8", newline="\n") as handle:
            handle.write(HOOK_SCRIPT)
        os.chmod(hook_path, 0o755)
        print(f"만들었다 : {hook_path}")

    try:
        subprocess.run(["git", "config", "core.hooksPath", ".githooks"], cwd=repo, check=True)
    except (OSError, subprocess.CalledProcessError) as error:
        print(f"git config 실패 : {error}", file=sys.stderr)
        return 1
    print("core.hooksPath 를 .githooks 로 맞췄다.")
    return 0


def main(argv=None):
    parser = argparse.ArgumentParser(description="공개 저장소용 비밀값·개인정보 검사기")
    parser.add_argument("--staged", action="store_true", help="(툴 사본) 스테이지된 내용 검사 (pre-commit)")
    parser.add_argument("--all", action="store_true", help="git 이 아는 파일 전부 검사")
    parser.add_argument("--hook", action="store_true", help="(툴 사본) Claude Code PreToolUse 훅 모드")
    parser.add_argument("--files", nargs="+", help="(툴 사본) 지정한 파일만 검사")
    parser.add_argument(
        "--files-from",
        help="파일 목록을 이 파일에서 읽는다 (- 면 표준 입력). 한 줄에 하나, "
        "'I<탭>경로' 는 올라가 있는(index) 내용, 'W<탭>경로'·그냥 경로는 작업 트리 내용을 본다",
    )
    parser.add_argument("--repo", default=".", help="저장소 경로 (기본 현재 폴더)")
    parser.add_argument("--install", action="store_true", help="(툴 사본) git pre-commit 훅 설치")
    parser.add_argument("--words", help="(툴 사본) 금칙어 목록 파일을 더 준다 (찾은 목록과 합친다)")
    args = parser.parse_args(argv)

    global REPO_ROOT
    REPO_ROOT = args.repo

    script_path = os.path.abspath(__file__)
    words = load_words(os.path.dirname(script_path), args.repo)
    if args.words:
        words += [word for word in read_words_file(os.path.abspath(args.words)) if word not in words]

    if args.install:
        return install(args.repo)
    if args.hook:
        return run_hook(words, script_path)

    hits = []
    if args.files_from:
        for source, target in read_list(args.files_from):
            hits.extend(scan_file(target, words, script_path, args.repo if source == "I" else None))
    elif args.files:
        for target in args.files:
            hits.extend(scan_file(target, words, script_path))
    elif args.staged:
        # 작업 트리가 아니라 올라가 있는 내용을 본다 (일부만 add 한 파일도 실제로 커밋될 내용을 검사)
        names = git_files(args.repo, ["diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z"])
        for name in names:
            hits.extend(scan_file(os.path.join(args.repo, name), words, script_path, args.repo))
    elif args.all:
        for name in git_files(args.repo, ["ls-files", "-z"]):
            hits.extend(scan_file(os.path.join(args.repo, name), words, script_path))
    else:
        parser.print_usage(sys.stderr)
        print("--staged · --all · --files · --files-from · --hook · --install 중 하나가 필요하다.", file=sys.stderr)
        return 1
    return report(hits)


if __name__ == "__main__":
    sys.exit(main())
