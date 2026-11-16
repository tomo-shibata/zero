package csvinput_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/interface/csvinput"
)

// 仕様: プラン（docs/plans/trade-history/plan.md）4.7（PR③）の「Taskfile は -csv … -workdir … を渡す。main は -csv が空でなく
// filepath.IsAbs でないときだけ filepath.Join(workdir, csv) にして RunInput.CSVPath に入れる」、判断10（CSV_FILE の相対パスは、
// task を実行したディレクトリからのパス）、5.5 の「resolveCSVPath(workdir, csv string) string（テストは cmd/import-trades の中の単体）。
// 絶対パスは t.TempDir() か filepath.Abs で作る（Windows で /tmp/… は絶対パスにならないため）」。
// 「ファイル <パス> を開けません」の <パス> は、この解決後のパス（要件21・仕様の指摘4）。
// 価格の CSV の登録のプラン（docs/plans/price-import/plan.md）4.3・5.2 で、cmd/import-trades の resolveCSVPath を csvinput.ResolvePath に移した
// （ケースの入力と期待値は変えていない）。売買履歴と価格の取り込みの両方が使う。

// 相対パスは、task を実行したディレクトリ（workdir）からのパスとして、workdir につなぐ。
func Test要件19_ResolvePathは相対パスをworkdirにつなぐ(t *testing.T) {
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
			got := csvinput.ResolvePath(workdir, tc.csv)
			want := filepath.Join(workdir, tc.csv)
			if got != want {
				t.Errorf("ResolvePath(%q, %q): got %q, want %q", workdir, tc.csv, got, want)
			}
		})
	}
}

// 絶対パスは、workdir をつながずそのまま使う。
func Test要件19_ResolvePathは絶対パスをそのまま返す(t *testing.T) {
	t.Parallel()
	workdir := t.TempDir()
	abs, err := filepath.Abs(filepath.Join(t.TempDir(), "trades.local.csv"))
	if err != nil {
		t.Fatalf("絶対パスを作れない: %v", err)
	}
	if !filepath.IsAbs(abs) {
		t.Fatalf("テストの前提: %q が絶対パスでない", abs)
	}
	if got := csvinput.ResolvePath(workdir, abs); got != abs {
		t.Errorf("ResolvePath(%q, %q): got %q, want %q（絶対パスに workdir をつながない）", workdir, abs, got, abs)
	}
}

// Windows のドライブの根からのパス（\data\t.csv）とドライブだけが付いた相対パス（C:t.csv）は、filepath.IsAbs では絶対パスにならないが、
// workdir につながず、そのまま返す（つなぐと、別のドライブや根を指す意味が変わるため）。Windows のときだけ意味がある。
func Test要件19_ResolvePathはWindowsのドライブの根とドライブ付きの相対パスをそのまま返す(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("Windows のパスの決まりのテスト")
	}
	workdir := t.TempDir()
	for _, csv := range []string{`\data\t.csv`, `C:t.csv`} {
		if got := csvinput.ResolvePath(workdir, csv); got != csv {
			t.Errorf("ResolvePath(%q, %q): got %q, want %q（workdir につながない）", workdir, csv, got, csv)
		}
	}
}

// 空のパス（CSV_FILE の指定なし）は、workdir があっても空のまま（呼び出し側が「CSV_FILE を指定してください」にするため）。
func Test要件19_ResolvePathは空のパスを空のまま返す(t *testing.T) {
	t.Parallel()
	workdir := t.TempDir()
	if got := csvinput.ResolvePath(workdir, ""); got != "" {
		t.Errorf("ResolvePath(%q, \"\"): got %q, want 空（workdir だけのパスにしない）", workdir, got)
	}
}
