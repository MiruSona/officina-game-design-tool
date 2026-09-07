// Package paths 는 저장소 뿌리와 그 아래 경로를 다룬다.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Root 는 저장소 뿌리를 정한다. 빈 값이면 현재 폴더다.
func Root(flagValue string) (string, error) {
	dir := flagValue
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("현재 폴더를 못 읽었습니다 : %v", err)
		}
		dir = wd
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("경로를 못 폈습니다 (%s) : %v", dir, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("그런 폴더가 없습니다 : %s", abs)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("폴더가 아닙니다 : %s", abs)
	}
	return abs, nil
}

// Rel 은 뿌리 기준 상대 경로를 슬래시 꼴로 돌려준다.
func Rel(root, target string) (string, error) {
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("경로를 못 폈습니다 (%s) : %v", target, err)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", fmt.Errorf("뿌리 밖의 경로입니다 : %s", target)
	}
	slashed := filepath.ToSlash(rel)
	if slashed == ".." || strings.HasPrefix(slashed, "../") {
		return "", fmt.Errorf("뿌리 밖의 경로입니다 : %s", target)
	}
	return slashed, nil
}

// Join 은 뿌리 기준 상대 경로를 실제 경로로 바꾼다.
func Join(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}

// Excluded 는 검사에서 빼는 자리인지 본다. 프로토타입 폴더는 버릴 코드라 안 본다.
func Excluded(rel, protoDir string) bool {
	clean := strings.TrimPrefix(filepath.ToSlash(rel), "./")
	if clean == "" {
		return false
	}
	if under(clean, ".git") || under(clean, "bin") || under(clean, "node_modules") {
		return true
	}
	if protoDir != "" && under(clean, strings.TrimSuffix(filepath.ToSlash(protoDir), "/")) {
		return true
	}
	return clean == "Docs/Todo/대시보드.html"
}

// under 는 경로가 그 폴더이거나 그 아래인지 본다.
func under(rel, dir string) bool {
	if dir == "" {
		return false
	}
	return rel == dir || strings.HasPrefix(rel, dir+"/")
}

// InDir 은 그 폴더 **바로 아래**의 파일인지 본다. 하위 폴더는 아니다.
func InDir(rel, dir string) bool {
	d := strings.TrimSuffix(filepath.ToSlash(dir), "/")
	if !strings.HasPrefix(rel, d+"/") {
		return false
	}
	return !strings.Contains(rel[len(d)+1:], "/")
}

// Exists 는 파일이나 폴더가 있는지 본다.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
