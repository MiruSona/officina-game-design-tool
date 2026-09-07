package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, filepath.FromSlash(RelPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("폴더 만들기 실패 : %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("설정 쓰기 실패 : %v", err)
	}
	return dir
}

func TestLoadWithoutFileUsesDefault(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("설정 파일이 없어도 돼야 한다 : %v", err)
	}
	if len(cfg.Stages) != 9 {
		t.Fatalf("단계 = %d개, 9개여야 한다", len(cfg.Stages))
	}
	if cfg.Stages[0].Name != "컨셉" || cfg.Stages[8].Name != "확장" {
		t.Fatalf("단계 이름이 절차 정본과 다르다 : %v", cfg.Stages)
	}
	if cfg.PillarMin != 3 || cfg.PillarMax != 4 || cfg.AskMin != 2 || cfg.AskMax != 4 {
		t.Fatalf("기본 범위가 다르다 : %+v", cfg)
	}
}

func TestLoadOverwritesOnlyWrittenFields(t *testing.T) {
	dir := writeConfig(t, `{ "컨셉줄상한": 100, "기둥개수": [2, 5], "코드폴더": "Assets/Scripts", "모르는칸": 1 }`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("읽기 실패 : %v", err)
	}
	if cfg.ConceptMaxLines != 100 || cfg.PillarMin != 2 || cfg.PillarMax != 5 {
		t.Fatalf("덮기가 안 됐다 : %+v", cfg)
	}
	if cfg.CodeDir != "Assets/Scripts" {
		t.Fatalf("코드폴더 = %q", cfg.CodeDir)
	}
	if cfg.DesignDir != "Docs/Design" || cfg.AskMin != 2 {
		t.Fatal("안 적은 칸은 그대로여야 한다")
	}
}

func TestLoadReplacesStageNames(t *testing.T) {
	dir := writeConfig(t, `{ "단계": ["하나", "둘", "셋"], "고리단계": [2, 3] }`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("읽기 실패 : %v", err)
	}
	if len(cfg.Stages) != 3 || cfg.StageName(2) != "둘" {
		t.Fatalf("단계 덮기가 안 됐다 : %+v", cfg.Stages)
	}
	if !cfg.InLoop(3) || cfg.InLoop(1) {
		t.Fatal("고리 범위가 덮이지 않았다")
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	cases := []string{
		`{ "기둥개수": [5, 2] }`,
		`{ "컨셉줄상한": 0 }`,
		`{ "단계": ["하나"] }`,
		`{ "고리단계": [3, 99] }`,
	}
	for _, body := range cases {
		if _, err := Load(writeConfig(t, body)); err == nil {
			t.Fatalf("막았어야 한다 : %s", body)
		}
	}
}

func TestLoadRejectsBrokenJSON(t *testing.T) {
	if _, err := Load(writeConfig(t, "{ 이건 JSON 이 아니다")); err == nil {
		t.Fatal("깨진 설정 파일은 막아야 한다")
	}
}

func TestStageLookup(t *testing.T) {
	cfg := Default()
	if _, ok := cfg.Stage(0); ok {
		t.Fatal("0단계는 없다")
	}
	if _, ok := cfg.Stage(10); ok {
		t.Fatal("10단계는 없다")
	}
	if cfg.StageName(5) != "거친 프로토타입 묶음" {
		t.Fatalf("5단계 이름 = %q", cfg.StageName(5))
	}
}
