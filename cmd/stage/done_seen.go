package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// doneSeen 은 Stop 훅이 마지막으로 띄운 말이다. 세션 id 는 **파일 내용으로만** 두고 경로에 안 넣는다.
type doneSeen struct {
	Session string `json:"세션"`
	Text    string `json:"내용해시"`
}

// cacheDir 는 상태를 둘 자리를 돌려준다. 시험이 갈아 끼운다.
var cacheDir = os.UserCacheDir

// doneAlreadySent 는 같은 세션에서 같은 말을 이미 띄웠는지 본다.
// 상태를 못 읽거나 못 쓰거나 세션을 모르면 **띄우는 쪽**으로 답한다 — 실패는 시끄러운 쪽이 안전하다.
func doneAlreadySent(root, session, text string) bool {
	if session == "" {
		return false
	}
	path, err := seenPath(root)
	if err != nil {
		return false
	}
	now := doneSeen{Session: session, Text: sum(text)}
	if raw, readErr := os.ReadFile(path); readErr == nil {
		var old doneSeen
		if json.Unmarshal(raw, &old) == nil && old == now {
			return true
		}
	}
	// 못 써도 띄운다. 다음 판에 또 뜨는 것이 조용히 사라지는 것보다 낫다.
	_ = saveSeen(path, now)
	return false
}

// seenPath 는 저장소마다 하나인 상태 파일 자리다. 뿌리 절대경로를 해시해 이름으로 쓴다.
func seenPath(root string) (string, error) {
	base, err := cacheDir()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "officina-stage")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("done-%s.json", sum(abs)[:12])), nil
}

// saveSeen 은 tmp 에 쓰고 rename 한다 (Go 의 Rename 은 Windows 에서도 덮어쓴다).
func saveSeen(path string, s doneSeen) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	// 이름을 고정하면 두 세션의 Stop 훅이 같은 tmp 를 두고 부딪힌다. 매번 새 이름을 받는다.
	f, err := os.CreateTemp(filepath.Dir(path), "done-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(raw); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// sum 은 글의 sha256 을 16진수로 돌려준다.
func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
