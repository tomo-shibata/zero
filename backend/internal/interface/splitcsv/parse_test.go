package splitcsv_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/interface/splitcsv"
)

// 仕様: 株式分割の要件（docs/requirements/stock-split.md）の要件14（BOM・CRLF・ダブルクォート・空の行）・要件15（見出し）・要件16（各列の書き方）・
// 要件17（同じ銘柄コード・同じ分割の日）と「分割の CSV の誤りの文言」の表（ファイル・各行・行どうしの3段階。行番号はファイルの行番号）・基準18・基準19・基準20（Parse の部分）、
// プラン（docs/plans/stock-split/plan.md）4.8 の PR② の splitcsv.Parse・Parsed・SplitRow・ParseError・Problem、5.1 の基準18 の行、5.2 の「Parse（単体）」。
// Parse は DB を見ない。「今日」は引数 today で決まる（このテストでは 2026-11-16）。

const (
	header = "銘柄コード,分割の日,分割後の株数"
	today  = "2026-11-16"

	// 要件定義の「分割の CSV の誤りの文言」の表。
	msgCode       = "銘柄コードは半角の英大文字・数字・「.」・「-」で1〜10文字にしてください"
	msgDate       = "分割の日は YYYY-MM-DD か YYYY/M/D の実在する日付にしてください"
	msgFuture     = "未来の分割の日は取り込めません"
	msgRatioSmall = "分割後の株数は2以上の整数にしてください"
	msgRatioLarge = "分割後の株数は1,000以下にしてください"
	msgColumns    = "列の数を3にしてください"

	msgEncoding = "CSV の文字コードが UTF-8 ではありません"
	msgHeader   = "1行目の見出しを「銘柄コード,分割の日,分割後の株数」にしてください"
	msgNoRows   = "取り込む行がありません"
	msgQuote    = "ダブルクォートの使い方が正しくありません"

	// Shift_JIS で書いた見出し「銘柄コード,分割の日,分割後の株数」。実際の Shift_JIS のバイト列。
	shiftJISHeader = "\x96\xc1\x95\xbf\x83\x52\x81\x5b\x83\x68\x2c\x95\xaa\x8a\x84\x82\xcc\x93\xfa\x2c\x95\xaa\x8a\x84\x8c\xe3\x82\xcc\x8a\x94\x90\x94\x0d\x0a"
)

// csvOf は、lines を LF でつなぎ、末尾に改行を付けた CSV の中身にする。
func csvOf(lines ...string) []byte {
	return []byte(strings.Join(lines, "\n") + "\n")
}

// withHeader は、見出しと lines の行を並べた CSV の中身にする。
func withHeader(lines ...string) []byte {
	return csvOf(append([]string{header}, lines...)...)
}

// at は、n行目の行の誤り（Line が n で、文言が「n行目: <msg>」）を作る。
func at(n int, msg string) splitcsv.Problem {
	return splitcsv.Problem{Line: n, Message: fmt.Sprintf("%d行目: %s", n, msg)}
}

// fileProblems は、Parse の err がファイルの段階の誤り（ParseError。ポインタでも値でも受ける）であることを確かめて、その Problems を返す。
func fileProblems(t *testing.T, err error) []splitcsv.Problem {
	t.Helper()
	if err == nil {
		t.Fatal("Parse: got err nil, want ParseError")
	}
	var pe *splitcsv.ParseError
	if errors.As(err, &pe) {
		return pe.Problems
	}
	var anyErr any = err
	if pv, ok := anyErr.(splitcsv.ParseError); ok {
		return pv.Problems
	}
	t.Fatalf("Parse: got err %v (%T), want ParseError", err, err)
	return nil
}

// messagesOf は、problems の Message を並び順のまま返す。
func messagesOf(problems []splitcsv.Problem) []string {
	msgs := make([]string, 0, len(problems))
	for _, p := range problems {
		msgs = append(msgs, p.Message)
	}
	return msgs
}

// assertFileProblem は、Parse がファイルの段階の誤りを、want の1件だけ（文言）で返すことを確かめる。
func assertFileProblem(t *testing.T, content []byte, want string) {
	t.Helper()
	parsed, err := splitcsv.Parse(content, today)
	if err == nil {
		t.Fatalf("Parse: got err nil（行 %+v）, want 誤り %q", parsed.Rows, want)
	}
	if got := messagesOf(fileProblems(t, err)); !slices.Equal(got, []string{want}) {
		t.Errorf("Parse の誤りの文言（ファイルの段階は最初の1つだけ）:\n got %q\nwant %q", got, []string{want})
	}
}

// mustParse は、Parse が成功する（ファイルの段階の誤りがない）ことを確かめて、結果を返す。
func mustParse(t *testing.T, content []byte) splitcsv.Parsed {
	t.Helper()
	parsed, err := splitcsv.Parse(content, today)
	if err != nil {
		t.Fatalf("Parse: got err %v, want nil", err)
	}
	return parsed
}

// wantRow は、Parse の Rows の1行の期待値。
type wantRow struct {
	line  int
	code  string
	date  string // "YYYY-MM-DD"
	ratio int64
}

// assertRows は、Parse の Rows が want とちょうど同じ（Line・Split の各項目。順番も）であることを確かめる。
func assertRows(t *testing.T, got []splitcsv.SplitRow, want []wantRow) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Parse の Rows の件数: got %d (%+v), want %d (%+v)", len(got), got, len(want), want)
	}
	for i, w := range want {
		g := got[i]
		wantSplit := model.StockSplit{StockCode: w.code, SplitDate: w.date, Ratio: w.ratio}
		if g.Line != w.line || g.Split != wantSplit {
			t.Errorf("Rows[%d]: got Line=%d Split=%+v, want Line=%d Split=%+v", i, g.Line, g.Split, w.line, wantSplit)
		}
	}
}

// assertNoProblems は、Problems も Duplicates も空であることを確かめる。
func assertNoProblems(t *testing.T, parsed splitcsv.Parsed) {
	t.Helper()
	if len(parsed.Problems) != 0 || len(parsed.Duplicates) != 0 {
		t.Errorf("got Problems=%+v Duplicates=%+v, want なし", parsed.Problems, parsed.Duplicates)
	}
}

// 基準18（要件16）: 誤りにならない銘柄コード・分割の日・分割後の株数は Rows に入り、Split の各項目が正確な値になる。
// 分割の日は `2026/3/1`・`2026/03/01`・`2026-03-01` のどれも "2026-03-01" に直る。`2026-11-16`（今日）は誤りにならない。`1000` は Ratio 1000。
func Test基準18_Parseは正しい銘柄コードと分割の日と分割後の株数をRowsに入れる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		code  string
		date  string
		ratio string
		want  wantRow
	}{
		{"銘柄コード_4桁の数字", "7203", "2026-03-01", "2", wantRow{2, "7203", "2026-03-01", 2}},
		{"銘柄コード_ピリオド", "BRK.B", "2026-03-01", "2", wantRow{2, "BRK.B", "2026-03-01", 2}},
		{"銘柄コード_ハイフン", "BRK-B", "2026-03-01", "2", wantRow{2, "BRK-B", "2026-03-01", 2}},
		{"銘柄コード_10文字", "ABCDEFGHIJ", "2026-03-01", "2", wantRow{2, "ABCDEFGHIJ", "2026-03-01", 2}},
		{"分割の日_今日", "7203", "2026-11-16", "2", wantRow{2, "7203", "2026-11-16", 2}},
		{"分割の日_スラッシュで0埋めなし", "7203", "2026/3/1", "2", wantRow{2, "7203", "2026-03-01", 2}},
		{"分割の日_スラッシュで0埋めあり", "7203", "2026/03/01", "2", wantRow{2, "7203", "2026-03-01", 2}},
		{"分割の日_ハイフンで0埋めあり", "7203", "2026-03-01", "2", wantRow{2, "7203", "2026-03-01", 2}},
		{"分割の日_スラッシュの月日が2桁", "7203", "2026/10/31", "2", wantRow{2, "7203", "2026-10-31", 2}},
		{"分割の日_うるう日", "7203", "2024-02-29", "2", wantRow{2, "7203", "2024-02-29", 2}},
		{"分割後の株数_最大1000", "7203", "2026-03-01", "1000", wantRow{2, "7203", "2026-03-01", 1000}},
		{"分割後の株数_3", "7203", "2026-03-01", "3", wantRow{2, "7203", "2026-03-01", 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed := mustParse(t, withHeader(tc.code+","+tc.date+","+tc.ratio))

			assertRows(t, parsed.Rows, []wantRow{tc.want})
			assertNoProblems(t, parsed)
		})
	}
}

// 基準18（要件16）: 書き方に合わない銘柄コード・分割の日・分割後の株数は、表の文言の誤りになり、Rows には入らない。誤りは1件だけで、重複の材料も出さない。
// 誤りの値は、1つの列だけを誤りにして、ほかの列は正しい値にする。
func Test基準18_Parseは書き方に合わない値を表の文言の誤りにする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
		want string
	}{
		// 銘柄コード
		{"銘柄コード_空", ",2026-03-01,2", msgCode},
		{"銘柄コード_小文字", "nvda,2026-03-01,2", msgCode},
		{"銘柄コード_11文字", "ABCDEFGHIJK,2026-03-01,2", msgCode},
		{"銘柄コード_空白を含む", "72 03,2026-03-01,2", msgCode},
		// 分割の日（書き方・実在）
		{"分割の日_空", "7203,,2", msgDate},
		{"分割の日_実在しない2月30日", "7203,2026-02-30,2", msgDate},
		{"分割の日_実在しない2月29日_うるう年でない", "7203,2026-02-29,2", msgDate},
		{"分割の日_実在しない4月31日", "7203,2026-04-31,2", msgDate},
		{"分割の日_月が0", "7203,2026-00-10,2", msgDate},
		{"分割の日_月が13", "7203,2026-13-01,2", msgDate},
		{"分割の日_スラッシュで実在しない", "7203,2026/2/30,2", msgDate},
		{"分割の日_ハイフンで0埋めがない", "7203,2026-3-1,2", msgDate},
		{"分割の日_区切りがない", "7203,20260301,2", msgDate},
		{"分割の日_前に空白", "7203, 2026-03-01,2", msgDate},
		{"分割の日_全角数字", "7203,２０２６-03-01,2", msgDate},
		{"分割の日_日付でない文字", "7203,abc,2", msgDate},
		// 分割の日（未来）
		{"分割の日_明日", "7203,2026-11-17,2", msgFuture},
		{"分割の日_スラッシュで明日", "7203,2026/11/17,2", msgFuture},
		{"分割の日_ずっと先", "7203,2099-01-01,2", msgFuture},
		// 分割後の株数
		{"分割後の株数_空", "7203,2026-03-01,", msgRatioSmall},
		{"分割後の株数_0", "7203,2026-03-01,0", msgRatioSmall},
		{"分割後の株数_1", "7203,2026-03-01,1", msgRatioSmall},
		{"分割後の株数_小数", "7203,2026-03-01,1.5", msgRatioSmall},
		{"分割後の株数_小数点の0", "7203,2026-03-01,2.0", msgRatioSmall},
		{"分割後の株数_マイナス", "7203,2026-03-01,-2", msgRatioSmall},
		{"分割後の株数_プラス", "7203,2026-03-01,+3", msgRatioSmall},
		{"分割後の株数_3桁区切り", `7203,2026-03-01,"1,000"`, msgRatioSmall},
		{"分割後の株数_前に空白", "7203,2026-03-01, 3", msgRatioSmall},
		{"分割後の株数_全角数字", "7203,2026-03-01,３", msgRatioSmall},
		{"分割後の株数_英字", "7203,2026-03-01,abc", msgRatioSmall},
		{"分割後の株数_1001", "7203,2026-03-01,1001", msgRatioLarge},
		{"分割後の株数_int64に収まらない長さ", "7203,2026-03-01,99999999999999999999", msgRatioLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed := mustParse(t, withHeader(tc.line))

			if want := []splitcsv.Problem{at(2, tc.want)}; !slices.Equal(parsed.Problems, want) {
				t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
			}
			if len(parsed.Rows) != 0 {
				t.Errorf("書き方に誤りのある行は Rows に入れない: got %+v", parsed.Rows)
			}
			if len(parsed.Duplicates) != 0 {
				t.Errorf("Duplicates: got %+v, want なし", parsed.Duplicates)
			}
		})
	}
}

// 基準18（要件16）: 「今日」は Parse の引数 today で決まる（時計を見ない）。today が 2026-03-01 なら、2026-03-02 は未来、2026-03-01 は誤りにならない。
func Test基準18_Parseの未来の分割の日は引数のtodayで決まる(t *testing.T) {
	t.Parallel()
	parsed, err := splitcsv.Parse(withHeader("7203,2026-03-01,2", "6758,2026-03-02,2"), "2026-03-01")
	if err != nil {
		t.Fatalf("Parse: got err %v, want nil", err)
	}

	assertRows(t, parsed.Rows, []wantRow{{2, "7203", "2026-03-01", 2}})
	if want := []splitcsv.Problem{at(3, msgFuture)}; !slices.Equal(parsed.Problems, want) {
		t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
	}
}

// 基準20（Parse の部分）・5.2: 1行に銘柄コード・分割の日・分割後の株数の3つの誤り → この順に3つ（同じ行番号）。
func Test基準20_Parseは1行の3つの誤りを列の順に全部出す(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("nvda,2026-02-30,1"))

	want := []splitcsv.Problem{at(2, msgCode), at(2, msgDate), at(2, msgRatioSmall)}
	if !slices.Equal(parsed.Problems, want) {
		t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
	}
	if len(parsed.Rows) != 0 {
		t.Errorf("Rows: got %+v, want なし", parsed.Rows)
	}
}

// 基準20（Parse の部分）: 3行目の分割後の株数が `1`、5行目の分割の日が `2026-02-30` で銘柄コードが `nvda`、ほかの行は正しい →
// 3行目の株数、5行目の銘柄コード、5行目の分割の日の順（行番号の昇順。同じ行は列の順）。正しい行は Rows に入る。
func Test基準20_Parseは各行の誤りを行番号の昇順に並べ正しい行をRowsに入れる(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader(
		"7203,2026-03-31,3",
		"6758,2026-05-01,1",
		"9984,2026-06-01,2",
		"nvda,2026-02-30,2",
		"AAPL,2026/4/1,4",
	))

	want := []splitcsv.Problem{at(3, msgRatioSmall), at(5, msgCode), at(5, msgDate)}
	if !slices.Equal(parsed.Problems, want) {
		t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
	}
	assertRows(t, parsed.Rows, []wantRow{
		{2, "7203", "2026-03-31", 3},
		{4, "9984", "2026-06-01", 2},
		{6, "AAPL", "2026-04-01", 4},
	})
}

// 5.2: 4列の行は列の数の誤りだけ（その行の他の列は確かめない）。2列・1列の行も同じ。正しい行は Rows に入る。
func Test要件13_Parseは列の数が3でない行を列の数の誤りだけにする(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("nvda,2026-02-30,1,extra", "7203", "6758,2026-03-01", "9984,2026-03-01,2"))

	want := []splitcsv.Problem{at(2, msgColumns), at(3, msgColumns), at(4, msgColumns)}
	if !slices.Equal(parsed.Problems, want) {
		t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
	}
	assertRows(t, parsed.Rows, []wantRow{{5, "9984", "2026-03-01", 2}})
}

// 要件14: BOM 付き・CRLF・空の行を含み、ダブルクォートで囲んだ CSV を読める。Line はファイルの行番号（空の行も数える）。
func Test要件14_ParseはBOM付きCRLFで空の行とダブルクォートを含むCSVを読める(t *testing.T) {
	t.Parallel()
	content := []byte("\xEF\xBB\xBF" + header + "\r\n\"7203\",\"2026/3/31\",\"3\"\r\n\r\n\"AAPL\",\"2026-04-01\",\"4\"\r\n")

	parsed := mustParse(t, content)

	assertRows(t, parsed.Rows, []wantRow{
		{2, "7203", "2026-03-31", 3},
		{4, "AAPL", "2026-04-01", 4},
	})
	assertNoProblems(t, parsed)
}

// 要件14: 最後の行に改行がなくても読める。
func Test要件14_Parseは最後の行に改行がなくても読める(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, []byte(header+"\n7203,2026-03-31,3"))

	assertRows(t, parsed.Rows, []wantRow{{2, "7203", "2026-03-31", 3}})
}

// 要件14: カンマだけの行（,,）は空の行として読み飛ばさず、3つの列がすべて空の行として、その行（2行目）の3つの列の誤り（銘柄コード・分割の日・分割後の株数）を出す
// （売買履歴の CSV の同じ扱い）。
func Test要件14_Parseはカンマだけの行を読み飛ばさず3つの列の誤りにする(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader(",,"))

	want := []splitcsv.Problem{at(2, msgCode), at(2, msgDate), at(2, msgRatioSmall)}
	if !slices.Equal(parsed.Problems, want) {
		t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
	}
	if len(parsed.Rows) != 0 {
		t.Errorf("Rows: got %+v, want なし", parsed.Rows)
	}
}

// 5.2「行番号は空の行も数える」（書き方の誤り）: 見出し、空の行、「7203,2026-03-31,1」→「3行目: 分割後の株数は2以上の整数にしてください」。
func Test要件14_Parseの行番号は空の行も数える_書き方の誤り(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, csvOf(header, "", "7203,2026-03-31,1"))

	if want := []splitcsv.Problem{at(3, msgRatioSmall)}; !slices.Equal(parsed.Problems, want) {
		t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
	}
}

// 基準17（c）（要件14）: UTF-8 として読めない（Shift_JIS）CSV は、文字コードの誤りだけ。
func Test基準17c_ParseはShift_JISのCSVを文字コードの誤りにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, []byte(shiftJISHeader+"7203,2026-03-31,3\r\n"), msgEncoding)
}

// 基準17（d）（要件15）: 見出しが `銘柄コード,分割後の株数,分割の日`（順番違い）なら、見出しの誤りだけ。
func Test基準17d_Parseは見出しの順番違いを見出しの誤りにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, csvOf("銘柄コード,分割後の株数,分割の日", "7203,3,2026-03-31"), msgHeader)
}

// 5.2: 見出しの前後に空白があると見出しの誤り（完全一致）。
func Test要件15_Parseは見出しの前後の空白を見出しの誤りにする(t *testing.T) {
	t.Parallel()
	for name, h := range map[string]string{
		"前に空白":   " " + header,
		"後ろに空白":  header + " ",
		"カンマの後ろ": "銘柄コード, 分割の日,分割後の株数",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertFileProblem(t, csvOf(h, "7203,2026-03-31,3"), msgHeader)
		})
	}
}

// 基準17（e）（要件13）: 見出しだけの CSV は「取り込む行がありません」。
func Test基準17e_Parseは見出しだけのCSVを取り込む行がない誤りにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, csvOf(header), msgNoRows)
}

// 要件13: 見出しと CRLF の空の行だけでも「取り込む行がありません」（要件14: 空の行は行として数えない）。
func Test要件13_Parseは見出しと空の行だけのCSVを取り込む行がない誤りにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, []byte(header+"\r\n\r\n\r\n"), msgNoRows)
}

// 基準17（f）（要件14）: 2行目の銘柄コードが `"7203` で閉じていない CSV は、「2行目: ダブルクォートの使い方が正しくありません」だけ。
func Test基準17f_Parseは閉じていないダブルクォートを誤りにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, withHeader(`"7203,2026-03-31,3`), "2行目: "+msgQuote)
}

// 「ファイルの段階は最初の1つだけ」: 見出しが違い、行もない → 見出しの誤りだけ（「取り込む行がありません」は出さない）。
func Test要件13_Parseは見出しが違い行もないCSVを見出しの誤りだけにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, csvOf("銘柄コード,分割後の株数,分割の日"), msgHeader)
}

// ダブルクォートの誤りがある CSV では、各行の誤りを出さない（ファイルの段階で止める）。
func Test要件14_Parseはダブルクォートの誤りがあるとき各行の誤りを出さない(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, withHeader("7203,2026-03-31,1", `"6758,2026-03-31,2`, "AAPL,2026-03-31,0"), "3行目: "+msgQuote)
}

// 基準19（要件17）: 2行目「7203,2026-03-31,3」、3行目「6758,2026-03-31,2」、4行目「7203,2026/3/31,2」→ Duplicates が
// 「4行目: 銘柄コード 7203 の 2026-03-31 の分割は 2行目にもあります」だけ（分割の日は日付として比べ、文言は YYYY-MM-DD で出す。3行目は同じ日でも別の銘柄）。
func Test基準19_Parseは同じ銘柄コードと同じ分割の日の2回目の行を誤りにする(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("7203,2026-03-31,3", "6758,2026-03-31,2", "7203,2026/3/31,2"))

	if want := []splitcsv.Problem{at(4, "銘柄コード 7203 の 2026-03-31 の分割は 2行目にもあります")}; !slices.Equal(parsed.Duplicates, want) {
		t.Errorf("Duplicates:\n got %+v\nwant %+v", parsed.Duplicates, want)
	}
	if len(parsed.Problems) != 0 {
		t.Errorf("Problems: got %+v, want なし", parsed.Problems)
	}
}

// 5.2: 同じ銘柄・同じ日が3回 → 2回目と3回目に「… は 2行目にもあります」（どちらも最初の行を指す。直前の行と比べる誤りを見つけるため）。
// 日付の書き方は3回とも違う（`2026-03-31`・`2026/3/31`・`2026/03/31`）。
func Test要件17_Parseは同じ銘柄と同じ日が3回あると最初の行を指す(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("7203,2026-03-31,3", "7203,2026/3/31,2", "7203,2026/03/31,4"))

	want := []splitcsv.Problem{
		at(3, "銘柄コード 7203 の 2026-03-31 の分割は 2行目にもあります"),
		at(4, "銘柄コード 7203 の 2026-03-31 の分割は 2行目にもあります"),
	}
	if !slices.Equal(parsed.Duplicates, want) {
		t.Errorf("Duplicates:\n got %+v\nwant %+v", parsed.Duplicates, want)
	}
}

// 要件17: 同じ銘柄コードでも分割の日が違えば誤りにならない（2回の分割）。同じ分割の日でも銘柄コードが違えば誤りにならない。
func Test要件17_Parseは銘柄コードか分割の日が違えば重ならない(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("7203,2026-03-31,3", "7203,2026-04-01,2", "6758,2026-03-31,2"))

	assertNoProblems(t, parsed)
	assertRows(t, parsed.Rows, []wantRow{
		{2, "7203", "2026-03-31", 3},
		{3, "7203", "2026-04-01", 2},
		{4, "6758", "2026-03-31", 2},
	})
}

// 5.2「行番号は空の行も数える」（行どうし）: 見出し、「7203,2026-03-31,3」、空の行、「7203,2026-03-31,2」→ Duplicates が「4行目: … は 2行目にもあります」。
func Test要件14_Parseの行番号は空の行も数える_重複(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, csvOf(header, "7203,2026-03-31,3", "", "7203,2026-03-31,2"))

	if want := []splitcsv.Problem{at(4, "銘柄コード 7203 の 2026-03-31 の分割は 2行目にもあります")}; !slices.Equal(parsed.Duplicates, want) {
		t.Errorf("Duplicates:\n got %+v\nwant %+v", parsed.Duplicates, want)
	}
	if len(parsed.Problems) != 0 {
		t.Errorf("Problems: got %+v, want なし", parsed.Problems)
	}
}

// FailMissingArgs（4.8）: CSV_FILE がないときの誤り（先頭の行と「CSV_FILE を指定してください」）を stderr に書き、終了コード1を返す（main が DB につなぐ前に使う）。
func Test基準17a_FailMissingArgsは先頭の行とCSV_FILEの誤りを標準エラーに書き終了コード1を返す(t *testing.T) {
	t.Parallel()
	var stderr strings.Builder

	code := splitcsv.FailMissingArgs(&stderr)

	if code != 1 {
		t.Errorf("終了コード: got %d, want 1", code)
	}
	want := splitcsv.FailedHeadline + "\n" + "CSV_FILE を指定してください" + "\n"
	if got := strings.ReplaceAll(stderr.String(), "\r\n", "\n"); got != want {
		t.Errorf("標準エラー:\n got %q\nwant %q", got, want)
	}
	if splitcsv.FailedHeadline != "取り込めませんでした。何も変えていません。" {
		t.Errorf("FailedHeadline: got %q, want %q", splitcsv.FailedHeadline, "取り込めませんでした。何も変えていません。")
	}
}
