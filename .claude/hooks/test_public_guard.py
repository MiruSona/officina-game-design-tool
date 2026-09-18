"""public_guard 시험. 가짜 토큰은 진짜 꼴이 아니게 X 를 반복해 만든다."""

import json
import os
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import public_guard  # noqa: E402

SCRIPT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "public_guard.py")
FAKE_GH = "ghp_" + "X" * 36
FAKE_AWS = "AKIA" + "X" * 16


def rules_of(hits):
    return sorted({hit[2] for hit in hits})


def scan(text, words=None):
    return public_guard.scan_text("a.md", text, words or [])


class ScanTextTest(unittest.TestCase):
    def test_github_token(self):
        self.assertIn("token", rules_of(scan(f"key = {FAKE_GH}")))

    def test_aws_key(self):
        self.assertIn("token", rules_of(scan(f"aws = {FAKE_AWS}")))

    def test_private_key_block(self):
        self.assertIn("token", rules_of(scan("-----BEGIN RSA PRIVATE KEY-----")))

    def test_generic_password(self):
        self.assertIn("token", rules_of(scan('password = "hunter2hunter2"')))

    def test_clean_line(self):
        self.assertEqual([], scan("print('안녕하세요')"))

    def test_ip_caught(self):
        self.assertIn("ip", rules_of(scan("서버 = 192.168.0.31")))

    def test_ip_allowed(self):
        self.assertEqual([], scan("bind 0.0.0.0 / 127.0.0.1 / localhost"))

    def test_mac_caught(self):
        self.assertIn("mac", rules_of(scan("nic = 3C:5A:B4:00:11:22")))

    def test_homepath_caught(self):
        hits = scan("C:\\Users\\hongdev\\a.txt 와 /home/hongdev/b.txt 와 /Users/hongdev/c.txt")
        self.assertEqual(3, len([hit for hit in hits if hit[2] == "homepath"]))

    def test_email_caught(self):
        self.assertIn("email", rules_of(scan("문의 hong@somecorp.co.kr")))

    def test_email_allowed(self):
        text = "a@example.com noreply@anthropic.com me@users.noreply.github.com"
        self.assertEqual([], scan(text))

    def test_guard_ok_skips_line(self):
        self.assertEqual([], scan(f"key = {FAKE_GH}  # guard:ok"))

    def test_mask_shows_four_chars(self):
        hits = scan(f"key = {FAKE_GH}")
        self.assertEqual("ghp_…", hits[0][3])


class WordRuleTest(unittest.TestCase):
    def test_word_caught_and_case_insensitive(self):
        with tempfile.TemporaryDirectory() as folder:
            path = os.path.join(folder, "public_guard.words")
            with open(path, "w", encoding="utf-8") as handle:
                handle.write("# 주석\n\n가제달빛농장\nAcmeEngineSDK\n")
            words = public_guard.load_words(folder, start_dir=folder)
            self.assertEqual(["가제달빛농장", "acmeenginesdk"], words)
            self.assertIn("word", rules_of(scan("우리 가제달빛농장 이야기", words)))
            self.assertIn("word", rules_of(scan("uses ACMEENGINESDK", words)))

    def test_no_words_file_skips_rule(self):
        with tempfile.TemporaryDirectory() as folder:
            words = public_guard.load_words(folder, start_dir=folder)
            self.assertEqual([], words)
            self.assertEqual([], scan("우리 가제달빛농장 이야기", words))


class WordLookupTest(unittest.TestCase):
    """서브모듈 꼴 — 상위/.claude/hooks/public_guard.words 를 거슬러 올라가 찾는다."""

    def build(self, folder, parent_words, child_words, self_words=None, doc_text=None, child_name="sub"):
        child = os.path.join(folder, child_name)
        for base, content in ((folder, parent_words), (child, child_words)):
            if content is None:
                continue
            hook_dir = os.path.join(base, ".claude", "hooks")
            os.makedirs(hook_dir, exist_ok=True)
            with open(os.path.join(hook_dir, "public_guard.words"), "w", encoding="utf-8") as handle:
                handle.write(content)

        hook_dir = os.path.join(child, ".claude", "hooks")
        os.makedirs(hook_dir, exist_ok=True)
        if self_words is not None:
            with open(os.path.join(hook_dir, "public_guard.self"), "w", encoding="utf-8") as handle:
                handle.write(self_words)
        guard = os.path.join(hook_dir, "public_guard.py")
        with open(SCRIPT, encoding="utf-8") as src, open(guard, "w", encoding="utf-8") as dst:
            dst.write(src.read())

        target = os.path.join(child, "doc.md")
        with open(target, "w", encoding="utf-8") as handle:
            handle.write(doc_text if doc_text is not None else "위쪽낱말 이야기\n아래쪽낱말 이야기\n")
        return child, guard, target

    def run_guard(self, cwd, guard, target):
        proc = subprocess.run(
            [sys.executable, guard, "--files", target],
            cwd=cwd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        return (
            proc.returncode,
            proc.stdout.decode("utf-8", errors="replace"),
            proc.stderr.decode("utf-8", errors="replace"),
        )

    def test_parent_words_found(self):
        with tempfile.TemporaryDirectory() as folder:
            child, guard, target = self.build(folder, "# 부모 목록\n위쪽낱말\n", None)
            code, out, err = self.run_guard(child, guard, target)
            self.assertEqual(2, code, out + err)
            self.assertIn("word", out)
            self.assertIn("doc.md:1", out)
            self.assertNotIn("doc.md:2", out)

    def test_self_file_removes_own_name(self):
        """.self 에 적은 낱말이 저장소 이름과 같으면 목록에서 뺀다."""
        with tempfile.TemporaryDirectory() as folder:
            child, guard, target = self.build(
                folder,
                "우리툴\n",
                None,
                self_words="# 자기 이름\n우리툴\n",
                doc_text="우리툴 이야기\n",
                child_name="우리툴",
            )
            code, out, err = self.run_guard(child, guard, target)
            self.assertEqual(0, code, out + err)
            self.assertEqual("", out)

    def test_self_file_ignores_other_words(self):
        """저장소 이름이 아닌 낱말은 .self 에 넣어도 여전히 잡힌다 (경고까지 찍는다)."""
        with tempfile.TemporaryDirectory() as folder:
            child, guard, target = self.build(
                folder,
                "위쪽낱말\n",
                None,
                self_words="위쪽낱말\n",
                child_name="우리툴",
            )
            code, out, err = self.run_guard(child, guard, target)
            self.assertEqual(2, code, out + err)
            self.assertIn("doc.md:1", out)
            self.assertIn("저장소 이름", err)

    def test_self_file_is_scanned(self):
        """.self 자체는 훅 폴더 안이어도 검사 대상에서 빼지 않는다."""
        self.assertFalse(public_guard.should_skip_path(".claude/hooks/public_guard.self", SCRIPT))
        self.assertTrue(public_guard.should_skip_path(".claude/hooks/other.py", SCRIPT))

    def test_parent_hook_uses_submodule_self(self):
        """부모 저장소 훅이 서브모듈 문서를 검사할 때 서브모듈의 .self 가 먹는다."""
        with tempfile.TemporaryDirectory() as folder:
            parent_hooks = os.path.join(folder, ".claude", "hooks")
            os.makedirs(parent_hooks)
            with open(os.path.join(parent_hooks, "public_guard.words"), "w", encoding="utf-8") as handle:
                handle.write("우리툴\n위쪽낱말\n")
            guard = os.path.join(parent_hooks, "public_guard.py")
            with open(SCRIPT, encoding="utf-8") as src, open(guard, "w", encoding="utf-8") as dst:
                dst.write(src.read())

            child = os.path.join(folder, "우리툴")
            child_hooks = os.path.join(child, ".claude", "hooks")
            os.makedirs(child_hooks)
            with open(os.path.join(child_hooks, "public_guard.self"), "w", encoding="utf-8") as handle:
                handle.write("우리툴\n")
            target = os.path.join(child, "doc.md")
            with open(target, "w", encoding="utf-8") as handle:
                handle.write("우리툴 설명\n")

            code, out, err = self.run_guard(folder, guard, target)
            self.assertEqual(0, code, out + err)

            # 같은 문서라도 저장소 이름이 아닌 금칙어는 여전히 잡힌다.
            with open(target, "w", encoding="utf-8") as handle:
                handle.write("우리툴 설명\n위쪽낱말 설명\n")
            code, out, err = self.run_guard(folder, guard, target)
            self.assertEqual(2, code, out + err)
            self.assertIn("doc.md:2", out)
            self.assertNotIn("doc.md:1", out)

    def test_two_lists_merged(self):
        with tempfile.TemporaryDirectory() as folder:
            child, guard, target = self.build(folder, "위쪽낱말\n", "아래쪽낱말\n")
            code, out, err = self.run_guard(child, guard, target)
            self.assertEqual(2, code, out + err)
            self.assertIn("doc.md:1", out)
            self.assertIn("doc.md:2", out)

    def test_words_file_with_bom(self):
        """BOM 이 붙은 목록 파일도 첫 낱말을 제대로 읽는다."""
        with tempfile.TemporaryDirectory() as folder:
            path = os.path.join(folder, "public_guard.words")
            with open(path, "w", encoding="utf-8-sig") as handle:
                handle.write("가제달빛농장\n아래쪽낱말\n")
            words = public_guard.load_words(folder, start_dir=folder)
            self.assertEqual(["가제달빛농장", "아래쪽낱말"], words)
            self.assertIn("word", rules_of(scan("우리 가제달빛농장 이야기", words)))


class ScanFileTest(unittest.TestCase):
    def test_local_file_name_caught(self):
        with tempfile.TemporaryDirectory() as folder:
            for name in (".env", ".env.production", "note.local.md", "server.pem", "id.key"):
                path = os.path.join(folder, name)
                with open(path, "w", encoding="utf-8") as handle:
                    handle.write("아무 내용\n")
                hits = public_guard.scan_file(path, [], SCRIPT)
                self.assertEqual("localfile", hits[0][2], name)

    def test_binary_file_skipped(self):
        with tempfile.TemporaryDirectory() as folder:
            path = os.path.join(folder, "blob.dat")
            with open(path, "wb") as handle:
                handle.write(b"\x00\x01" + FAKE_GH.encode())
            self.assertEqual([], public_guard.scan_file(path, [], SCRIPT))

    def test_hook_folder_skipped(self):
        with tempfile.TemporaryDirectory() as folder:
            hook_dir = os.path.join(folder, ".claude", "hooks")
            os.makedirs(hook_dir)
            path = os.path.join(hook_dir, "sample.py")
            with open(path, "w", encoding="utf-8") as handle:
                handle.write(f"key = {FAKE_GH}\n")
            self.assertEqual([], public_guard.scan_file(path, [], SCRIPT))

    def test_text_file_caught(self):
        with tempfile.TemporaryDirectory() as folder:
            path = os.path.join(folder, "doc.md")
            with open(path, "w", encoding="utf-8") as handle:
                handle.write(f"한 줄\nkey = {FAKE_GH}\n")
            hits = public_guard.scan_file(path, [], SCRIPT)
            self.assertEqual(2, hits[0][1])


class HookModeTest(unittest.TestCase):
    def run_hook(self, payload):
        proc = subprocess.run(
            [sys.executable, SCRIPT, "--hook"],
            input=json.dumps(payload).encode("utf-8"),
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        return proc.returncode, proc.stderr.decode("utf-8", errors="replace")

    def test_hook_blocks_write(self):
        code, err = self.run_hook(
            {"tool_name": "Write", "tool_input": {"file_path": "a.md", "content": f"key = {FAKE_GH}"}}
        )
        self.assertEqual(2, code)
        self.assertIn("token", err)

    def test_hook_blocks_edit_new_string(self):
        code, err = self.run_hook(
            {"tool_name": "Edit", "tool_input": {"file_path": "a.md", "new_string": "ip = 10.0.0.7"}}
        )
        self.assertEqual(2, code)
        self.assertIn("ip", err)

    def test_hook_blocks_local_file_name(self):
        code, err = self.run_hook(
            {"tool_name": "Write", "tool_input": {"file_path": ".env", "content": "빈 내용"}}
        )
        self.assertEqual(2, code)
        self.assertIn("localfile", err)

    def test_hook_passes_clean(self):
        code, err = self.run_hook(
            {"tool_name": "Write", "tool_input": {"file_path": "a.md", "content": "그냥 글이다"}}
        )
        self.assertEqual(0, code)
        self.assertEqual("", err)


if __name__ == "__main__":
    unittest.main()
