package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

// 仕様: プラン（docs/plans/trade-history/plan.md）4.7（PR③）の「Taskfile は -csv … -workdir … を渡す。main は -csv が空でなく
// filepath.IsAbs でないときだけ filepath.Join(workdir, csv) にして RunInput.CSVPath に入れる」、判断10（CSV_FILE の相対パスは、
// task を実行したディレクトリからのパス）、5.5 の「resolveCSVPath(workdir, csv string) string（テストは cmd/import-trades の中の単体）。
// 絶対パスは t.TempDir() か filepath.Abs で作る（Windows で /tmp/… は絶対パスにならないため）」。
// 「ファイル <パス> を開けません」の <パス> は、この解決後のパス（要件21・仕様の指摘4）。

// 相対パスは、task を実行したディレクトリ（workdir）からのパスとして、workdir につなぐ。
func Test要件19_resolveCSVPathは相対パスをworkdirにつなぐ(t *testing.T) {
	t.Parallel()
	workdir := t.TempDir()
	cases := []struct {
		name string
		csv  string
	}{
		{"ファイル名だけ", "trades.local.csv"},
		{"サブディレクトリ", filepath.Join("backend", "db", "seed", "trades.example.csv")},
		{"スラッシュ区切り", "backend/test/integration/testdata/trades.csv"},
		{"カレントの指定", filepath.Join(".", "trades.local.csv")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolveCSVPath(workdir, tc.csv)
			want := filepath.Join(workdir, tc.csv)
			if got != want {
				t.Errorf("resolveCSVPath(%q, %q): got %q, want %q", workdir, tc.csv, got, want)
			}
		})
	}
}

// 絶対パスは、workdir をつながずそのまま使う。
func Test要件19_resolveCSVPathは絶対パスをそのまま返す(t *testing.T) {
	t.Parallel()
	workdir := t.TempDir()
	abs, err := filepath.Abs(filepath.Join(t.TempDir(), "trades.local.csv"))
	if err != nil {
		t.Fatalf("絶対パスを作れない: %v", err)
	}
	if !filepath.IsAbs(abs) {
		t.Fatalf("テストの前提: %q が絶対パスでない", abs)
	}
	if got := resolveCSVPath(workdir, abs); got != abs {
		t.Errorf("resolveCSVPath(%q, %q): got %q, want %q（絶対パスに workdir をつながない）", workdir, abs, got, abs)
	}
}

// Windows のドライブの根からのパス（\data\t.csv）とドライブだけが付いた相対パス（C:t.csv）は、filepath.IsAbs では絶対パスにならないが、
// workdir につながず、そのまま返す（つなぐと、別のドライブや根を指す意味が変わるため）。Windows のときだけ意味がある。
func Test要件19_resolveCSVPathはWindowsのドライブの根とドライブ付きの相対パスをそのまま返す(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("Windows のパスの決まりのテスト")
	}
	workdir := t.TempDir()
	for _, csv := range []string{`\data\t.csv`, `C:t.csv`} {
		if got := resolveCSVPath(workdir, csv); got != csv {
			t.Errorf("resolveCSVPath(%q, %q): got %q, want %q（workdir につながない）", workdir, csv, got, csv)
		}
	}
}

// 空の -csv（CSV_FILE の指定なし）は、workdir があっても空のまま（「CSV_FILE と USER_ID を指定してください」にするため）。
func Test要件19_resolveCSVPathは空のcsvを空のまま返す(t *testing.T) {
	t.Parallel()
	workdir := t.TempDir()
	if got := resolveCSVPath(workdir, ""); got != "" {
		t.Errorf("resolveCSVPath(%q, \"\"): got %q, want 空（workdir だけのパスにしない）", workdir, got)
	}
}
