#!/usr/bin/env python3
"""공개 GitHub 저장소에 비밀값·개인정보·금칙어가 올라가는 것을 막는 검사기."""

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

# 규칙 이름 -> 정규식. 순서대로 검사해 걸린 것을 모두 보고한다.
RULES = [
    ("token", re.compile(r"gh[pousr]_[A-Za-z0-9]{16,}|github_pat_[A-Za-z0-9_]{20,}")),
    ("token", re.compile(r"AKIA[0-9A-Z]{16}")),
    ("token", re.compile(r"sk-[A-Za-z0-9_-]{20,}")),
    ("token", re.compile(r"xox[baprs]-[A-Za-z0-9-]+")),
    ("token", re.compile(r"AIza[0-9A-Za-z_-]{35}")),
    ("token", re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----")),
    ("token", re.compile(r"Bearer [A-Za-z0-9._-]{20,}")),
    (
        "token",
        re.compile(
            r"(?:api[_-]?key|secret|token|passw(?:or)?d)\s*[:=]\s*[\"']?[^\s\"']{8,}",
            re.IGNORECASE,
        ),
    ),
    ("ip", re.compile(r"\b(?:\d{1,3}\.){3}\d{1,3}\b")),
    ("mac", re.compile(r"\b(?:[0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}\b")),
    ("homepath", re.compile(r"[A-Za-z]:\\Users\\[A-Za-z0-9._-]+|/home/[A-Za-z0-9._-]+|/Users/[A-Za-z0-9._-]+")),
    ("email", re.compile(r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b")),
]

IP_OK = {"0.0.0.0", "127.0.0.1"}
EMAIL_OK = ("@example.com", "noreply@anthropic.com", "@users.noreply.github.com")
LOCAL_FILE_PATTERNS = (".local.md", ".pem", ".key")


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
    if any(part in normalized for part in SKIP_DIRS):
        return True
    if os.path.basename(normalized) == os.path.basename(script_path):
        return True
    return False


def read_words_file(path):
    """목록 파일 한 개를 읽는다. 주석·빈 줄은 버리고 소문자로 맞춘다."""
    if not os.path.isfile(path):
        return []

    words = []
    with open(path, encoding="utf-8", errors="replace") as handle:
        for raw in handle:
            line = raw.strip()
            if not line or line.startswith("#"):
                continue
            words.append(line.lower())
    return words


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


def word_list_paths(script_dir, extra_path=None, start_dir=None):
    """목록을 찾을 자리들. 스크립트 폴더 -> 저장소 최상위에서 부모로 거슬러 올라감 -> --words."""
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

    if extra_path:
        paths.append(os.path.abspath(extra_path))
    return paths


def load_words(script_dir, extra_path=None, start_dir=None):
    """찾은 목록을 전부 합친다 (중복 제거). 하나도 없으면 빈 목록이라 word 규칙을 건너뛴다."""
    words = []
    for path in word_list_paths(script_dir, extra_path, start_dir):
        for word in read_words_file(path):
            if word not in words:
                words.append(word)

    # 자기 저장소 이름은 부모 목록에 있어도 뺀다. 없으면 아무것도 안 뺀다.
    own = read_words_file(os.path.join(script_dir, SELF_FILE))
    return [word for word in words if word not in own]


def scan_text(path, text, words):
    """내용을 줄 단위로 검사해 (파일, 줄번호, 규칙, 가린값) 목록을 돌려준다."""
    hits = []
    for number, line in enumerate(text.splitlines(), start=1):
        if SKIP_MARK in line:
            continue

        for name, pattern in RULES:
            for found in pattern.findall(line):
                if name == "ip" and found in IP_OK:
                    continue
                if name == "email" and any(found.endswith(ok) or found == ok for ok in EMAIL_OK):
                    continue
                hits.append((path, number, name, mask(found)))

        lowered = line.lower()
        for word in words:
            if word in lowered:
                hits.append((path, number, "word", mask(word)))
    return hits


def scan_file(path, words, script_path):
    """파일 하나를 검사한다. 이름 규칙 -> 크기 -> 이진 -> 내용 순."""
    if should_skip_path(path, script_path):
        return []
    if is_local_file(path):
        return [(path, 0, "localfile", os.path.basename(path))]
    if not os.path.isfile(path):
        return []
    if os.path.getsize(path) > MAX_BYTES:
        print(f"건너뜀 (2MB 초과) : {path}")
        return []

    with open(path, "rb") as handle:
        raw = handle.read()
    if b"\x00" in raw:
        return []

    return scan_text(path, raw.decode("utf-8", errors="replace"), words)


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


def run_hook(words, script_path):
    """Claude Code PreToolUse 훅. 걸리면 stderr 에 이유를 쓰고 2 로 끝난다."""
    try:
        payload = json.loads(sys.stdin.read() or "{}")
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
    hits.extend(scan_text(path, content, words))

    if not hits:
        return 0

    for hit_path, number, name, value in hits:
        print(f"{hit_path}:{number}  {name}  {value}", file=sys.stderr)
    print("걸림 %d건 — 커밋을 막았다. 정말 괜찮은 줄이면 guard:ok 를 붙인다." % len(hits), file=sys.stderr)
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
    """.githooks/pre-commit 을 쓰고 git 이 그 폴더를 보게 한다."""
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
    parser.add_argument("--staged", action="store_true", help="스테이지된 파일 검사 (pre-commit)")
    parser.add_argument("--all", action="store_true", help="git 이 아는 파일 전부 검사")
    parser.add_argument("--hook", action="store_true", help="Claude Code PreToolUse 훅 모드")
    parser.add_argument("--files", nargs="+", help="지정한 파일만 검사")
    parser.add_argument("--repo", default=".", help="저장소 경로 (기본 현재 폴더)")
    parser.add_argument("--install", action="store_true", help="git pre-commit 훅 설치")
    parser.add_argument("--words", help="금칙어 목록 파일을 더 준다 (찾은 목록과 합친다)")
    args = parser.parse_args(argv)

    script_path = os.path.abspath(__file__)
    words = load_words(os.path.dirname(script_path), args.words, args.repo)

    if args.install:
        return install(args.repo)
    if args.hook:
        return run_hook(words, script_path)

    if args.files:
        targets = args.files
    elif args.staged:
        targets = [
            os.path.join(args.repo, name)
            for name in git_files(args.repo, ["diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z"])
        ]
    elif args.all:
        targets = [os.path.join(args.repo, name) for name in git_files(args.repo, ["ls-files", "-z"])]
    else:
        parser.print_usage(sys.stderr)
        print("--staged · --all · --files · --hook · --install 중 하나가 필요하다.", file=sys.stderr)
        return 1

    hits = []
    for target in targets:
        hits.extend(scan_file(target, words, script_path))
    return report(hits)


if __name__ == "__main__":
    sys.exit(main())
