package managedcsv_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/interface/managedcsv"
)

// 仕様: おまかせ運用の要件（docs/requirements/managed-account.md）の要件1・要件3（名前・評価額・損益・元本・日付の範囲）・要件27（文字コード・
// 区切り・改行・ダブルクォート・空の行は売買履歴の CSV と同じ）・要件28（見出し）・要件29（各列の書き方）・要件30（同じ名前・同じ日付、元本が1円未満）・
// 要件31 と「CSV の誤りの文言」の表（段階はファイル → 各行 → 行どうし。行番号の昇順、同じ行では表の順）・基準23と、
// プラン（docs/plans/managed-account/plan.md）4.5・4.8 の PR① の managedcsv.Parse・Parsed・ParseError・Problem・ValuationRow、5.1 の基準23 の行、
// 5.2 の「Parse（単体）」。
// Parse が確かめるのは、ファイルの段階と各行の段階（Problems）、行どうしの重なり（Duplicates）まで。DB は見ない。今日は 2026-11-16（日本時間）。

const (
	today  = "2026-11-16"
	header = "名前,日付,評価額,損益"
	// bom は、UTF-8 の BOM（U+FEFF）のバイト列。
	bom = "\xef\xbb\xbf"

	// 要件定義の「CSV の誤りの文言」の表（n行目: は at で付ける）。「元本（評価額 − 損益）」の「−」は U+2212（要件定義と同じ）。
	msgColumns          = "列の数を4にしてください"
	msgName             = "名前を入れてください"
	msgNameControl      = "名前に改行やタブなどの制御文字は使えません"
	msgDateFormat       = "日付は YYYY-MM-DD か YYYY/M/D の実在する日付にしてください"
	msgDateFuture       = "未来の日付は取り込めません"
	msgValuationFormat  = "評価額は半角数字だけの整数で書いてください"
	msgValuationZero    = "評価額は1以上にしてください"
	msgValuationTooBig  = "評価額は1兆未満にしてください"
	msgGainFormat       = "損益は半角数字だけの整数で書いてください（マイナスは先頭に -）"
	msgGainOutOfRange   = "損益は -1兆より大きく1兆未満にしてください"
	msgPrincipal        = "元本（評価額 − 損益）が1円未満です"
	msgEncoding         = "CSV の文字コードが UTF-8 ではありません"
	msgHeader           = "1行目の見出しを「名前,日付,評価額,損益」にしてください"
	msgNoRows           = "取り込む行がありません"
	msgQuoteTailMessage = "ダブルクォートの使い方が正しくありません"
)

// 列の位置。
const (
	colName = iota
	colDate
	colValuation
	colGain
)

// fields は、CSV の1行の4つの値。
type fields [4]string

// baseRow は、誤りのない行（元本 3,000,000円）。
var baseRow = fields{"WealthNavi", "2026-11-16", "4000000", "1000000"}

// with は、col の値だけを v に替えた行を返す。
func (f fields) with(col int, v string) fields {
	f[col] = v
	return f
}

// line は、f を CSV の1行にする。カンマを含む値はダブルクォートで囲む。
func (f fields) line() string {
	parts := make([]string, len(f))
	for i, v := range f {
		if strings.Contains(v, ",") {
			v = `"` + v + `"`
		}
		parts[i] = v
	}
	return strings.Join(parts, ",")
}

// csvOf は、lines を LF でつなぎ、末尾に改行を付けた CSV の中身にする。
func csvOf(lines ...string) []byte {
	return []byte(strings.Join(lines, "\n") + "\n")
}

// withHeader は、見出しと rows の行を並べた CSV の中身にする。
func withHeader(rows ...fields) []byte {
	lines := []string{header}
	for _, r := range rows {
		lines = append(lines, r.line())
	}
	return csvOf(lines...)
}

// at は、n行目の誤り（Line が n で、文言が「n行目: <msg>」）を作る。
func at(n int, msg string) managedcsv.Problem {
	return managedcsv.Problem{Line: n, Message: fmt.Sprintf("%d行目: %s", n, msg)}
}

// fileProblem は、行番号のないファイルの段階の誤り（Line は0）を作る。
func fileProblem(msg string) managedcsv.Problem {
	return managedcsv.Problem{Line: 0, Message: msg}
}

// mustParse は、content を today で読み、ファイルの段階の誤りがない（err が nil）ことを確かめて Parsed を返す。
func mustParse(t *testing.T, content []byte) managedcsv.Parsed {
	t.Helper()
	parsed, err := managedcsv.Parse(content, today)
	if err != nil {
		t.Fatalf("Parse: got err %v (%+v), want nil", err, parseProblemsOrNil(err))
	}
	return parsed
}

// parseProblemsOrNil は、失敗メッセージ用に、err が ParseError ならその Problems を返す。
func parseProblemsOrNil(err error) []managedcsv.Problem {
	var pe *managedcsv.ParseError
	if errors.As(err, &pe) {
		return pe.Problems
	}
	return nil
}

// fileStageProblems は、content の Parse がファイルの段階の誤り（ParseError）を返すことを確かめて、その Problems を返す（ポインタでも値でも受ける）。
func fileStageProblems(t *testing.T, content []byte) []managedcsv.Problem {
	t.Helper()
	_, err := managedcsv.Parse(content, today)
	if err == nil {
		t.Fatal("Parse: got err nil, want *ParseError")
	}
	var pe *managedcsv.ParseError
	if errors.As(err, &pe) {
		return pe.Problems
	}
	var anyErr any = err
	if pv, ok := anyErr.(managedcsv.ParseError); ok {
		return pv.Problems
	}
	t.Fatalf("Parse: got err %v (%T), want ParseError", err, err)
	return nil
}

// assertFileProblem は、content の Parse が、ファイルの段階の誤りをちょうど want の1つだけ返すことを確かめる。
func assertFileProblem(t *testing.T, content []byte, want managedcsv.Problem) {
	t.Helper()
	if got := fileStageProblems(t, content); !slices.Equal(got, []managedcsv.Problem{want}) {
		t.Errorf("Parse のファイルの段階の誤り:\n got %+v\nwant %+v", got, []managedcsv.Problem{want})
	}
}

// assertRowProblems は、content の Parse が（ファイルの段階の誤りなしで）各行の段階の誤りをちょうど want（並び順も含めて）返すことを確かめる。
func assertRowProblems(t *testing.T, content []byte, want []managedcsv.Problem) {
	t.Helper()
	parsed := mustParse(t, content)
	if !slices.Equal(parsed.Problems, want) {
		t.Errorf("Parse の各行の段階の誤り:\n got %+v\nwant %+v", parsed.Problems, want)
	}
}

// 基準23: 誤りにならない5行は Rows に入り、Valuation の各項目を比べる（2026/1/6 は "2026-01-06"、2026/01/07 は "2026-01-07"、+999 は 999、
// おまかせ NISA はそのまま）。Line はファイルの行番号。誤りと重なりは出ない。
func Test基準23_Parseは誤りにならない行を記録にする(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		"WealthNavi,2026-11-16,1,0",
		"WealthNavi,2026/1/6,999999999999,999999999998",
		"WealthNavi,2026/01/07,1000,-999999999999",
		"WealthNavi,2026-01-08,1000,+999",
		"おまかせ NISA,2026-01-09,500000,-1",
	)

	parsed := mustParse(t, content)

	want := []managedcsv.ValuationRow{
		{Line: 2, Valuation: model.ManagedValuation{AccountName: "WealthNavi", ValuationDate: "2026-11-16", Valuation: 1, Gain: 0}},
		{Line: 3, Valuation: model.ManagedValuation{AccountName: "WealthNavi", ValuationDate: "2026-01-06", Valuation: 999_999_999_999, Gain: 999_999_999_998}},
		{Line: 4, Valuation: model.ManagedValuation{AccountName: "WealthNavi", ValuationDate: "2026-01-07", Valuation: 1000, Gain: -999_999_999_999}},
		{Line: 5, Valuation: model.ManagedValuation{AccountName: "WealthNavi", ValuationDate: "2026-01-08", Valuation: 1000, Gain: 999}},
		{Line: 6, Valuation: model.ManagedValuation{AccountName: "おまかせ NISA", ValuationDate: "2026-01-09", Valuation: 500_000, Gain: -1}},
	}
	if !slices.Equal(parsed.Rows, want) {
		t.Errorf("Parse の行:\n got %+v\nwant %+v", parsed.Rows, want)
	}
	if len(parsed.Problems) != 0 {
		t.Errorf("Parse の各行の段階の誤り: got %+v, want なし", parsed.Problems)
	}
	if len(parsed.Duplicates) != 0 {
		t.Errorf("Parse の重なり: got %+v, want なし", parsed.Duplicates)
	}
}

// 基準23: 誤りにならない値のそれぞれ（5行を1行ずつ。日付の書き方・境界）。
func Test基準23_Parseは正しい値を1行ずつ受け付ける(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		row  fields
	}{
		{"評価額1・損益0", baseRow.with(colValuation, "1").with(colGain, "0")},
		{"評価額の最大と元本1", baseRow.with(colValuation, "999999999999").with(colGain, "999999999998")},
		{"損益の最小", baseRow.with(colValuation, "1000").with(colGain, "-999999999999")},
		{"損益の先頭に+", baseRow.with(colValuation, "1000").with(colGain, "+999")},
		{"名前に空白を含む", baseRow.with(colName, "おまかせ NISA").with(colValuation, "500000").with(colGain, "-1")},
		{"日付_今日", baseRow.with(colDate, "2026-11-16")},
		{"日付_今日のスラッシュ形", baseRow.with(colDate, "2026/11/16")},
		{"日付_スラッシュで0埋めなし", baseRow.with(colDate, "2026/1/6")},
		{"日付_スラッシュで0埋めあり", baseRow.with(colDate, "2026/01/06")},
		{"日付_うるう日", baseRow.with(colDate, "2024-02-29")},
		{"損益_プラス0", baseRow.with(colGain, "+0")},
		{"損益_マイナス0", baseRow.with(colGain, "-0")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed := mustParse(t, withHeader(tc.row))
			if len(parsed.Problems) != 0 || len(parsed.Rows) != 1 {
				t.Errorf("Parse(%v): got 行 %d・誤り %+v, want 1行・誤りなし", tc.row, len(parsed.Rows), parsed.Problems)
			}
		})
	}
}

// 要件29: 損益の `-0`・`+0` は 0 として読む。日付の書き方 2026/1/6 は "2026-01-06" にそろえる。
func Test要件29_Parseは損益の符号つきの0を0として読み日付をYYYY_MM_DDにそろえる(t *testing.T) {
	t.Parallel()
	for _, gain := range []string{"-0", "+0"} {
		parsed := mustParse(t, withHeader(baseRow.with(colDate, "2026/1/6").with(colGain, gain)))
		if len(parsed.Rows) != 1 {
			t.Fatalf("損益 %q: Parse の行の数: got %d (%+v), want 1", gain, len(parsed.Rows), parsed.Problems)
		}
		got := parsed.Rows[0].Valuation
		if got.Gain != 0 || got.ValuationDate != "2026-01-06" {
			t.Errorf("損益 %q: got 損益 %d・日付 %q, want 0・\"2026-01-06\"", gain, got.Gain, got.ValuationDate)
		}
	}
}

// 基準23: 各列の誤り。表の値を1つずつ書いた行は、「CSV の誤りの文言」の該当する文言が、その行（2行目）の誤りとして1つだけ出る。
// カンマを含む値（`1,000`）は、ダブルクォートで囲んで書く（囲まないと列の数の誤りになるため。fields.line）。
func Test基準23_Parseは各列の誤りを表の文言で返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		row  fields
		msg  string
	}{
		// 名前
		{"名前_空", baseRow.with(colName, ""), msgName},
		{"名前_半角の空白だけ", baseRow.with(colName, "   "), msgName},
		{"名前_全角の空白だけ", baseRow.with(colName, "　"), msgName},
		{"名前_タブだけ", baseRow.with(colName, "\t"), msgName},
		// 日付
		{"日付_空", baseRow.with(colDate, ""), msgDateFormat},
		{"日付_実在しない日", baseRow.with(colDate, "2026-02-30"), msgDateFormat},
		{"日付_スラッシュで実在しない日", baseRow.with(colDate, "2026/2/30"), msgDateFormat},
		{"日付_うるう年でない年の2月29日", baseRow.with(colDate, "2025-02-29"), msgDateFormat},
		{"日付_13月", baseRow.with(colDate, "2026/13/1"), msgDateFormat},
		{"日付_区切りなし", baseRow.with(colDate, "20261116"), msgDateFormat},
		{"日付_ハイフンで0埋めなし", baseRow.with(colDate, "2026-1-6"), msgDateFormat},
		{"日付_ピリオド区切り", baseRow.with(colDate, "2026.01.06"), msgDateFormat},
		{"日付_英字", baseRow.with(colDate, "abc"), msgDateFormat},
		{"日付_年が0000", baseRow.with(colDate, "0000-01-01"), msgDateFormat},
		{"日付_明日", baseRow.with(colDate, "2026-11-17"), msgDateFuture},
		{"日付_明日のスラッシュ形", baseRow.with(colDate, "2026/11/17"), msgDateFuture},
		{"日付_来年", baseRow.with(colDate, "2027-01-01"), msgDateFuture},
		// 評価額の書き方
		{"評価額_空", baseRow.with(colValuation, ""), msgValuationFormat},
		{"評価額_マイナス", baseRow.with(colValuation, "-1"), msgValuationFormat},
		{"評価額_3桁区切り", baseRow.with(colValuation, "1,000"), msgValuationFormat},
		{"評価額_小数", baseRow.with(colValuation, "1.5"), msgValuationFormat},
		{"評価額_円の記号", baseRow.with(colValuation, "¥1000"), msgValuationFormat},
		{"評価額_円の文字", baseRow.with(colValuation, "1000円"), msgValuationFormat},
		{"評価額_英字", baseRow.with(colValuation, "abc"), msgValuationFormat},
		{"評価額_先頭にプラス", baseRow.with(colValuation, "+1000"), msgValuationFormat},
		{"評価額_全角数字", baseRow.with(colValuation, "１０００"), msgValuationFormat},
		{"評価額_前に空白", baseRow.with(colValuation, " 1000"), msgValuationFormat},
		{"評価額_後ろに空白", baseRow.with(colValuation, "1000 "), msgValuationFormat},
		// 評価額の大きさ
		{"評価額_0", baseRow.with(colValuation, "0"), msgValuationZero},
		{"評価額_1兆", baseRow.with(colValuation, "1000000000000"), msgValuationTooBig},
		{"評価額_int64に収まらない", baseRow.with(colValuation, "99999999999999999999"), msgValuationTooBig},
		// 損益の書き方
		{"損益_空", baseRow.with(colGain, ""), msgGainFormat},
		{"損益_プラスだけ", baseRow.with(colGain, "+"), msgGainFormat},
		{"損益_マイナスだけ", baseRow.with(colGain, "-"), msgGainFormat},
		{"損益_3桁区切り", baseRow.with(colGain, "1,000"), msgGainFormat},
		{"損益_小数", baseRow.with(colGain, "1.5"), msgGainFormat},
		{"損益_プラスが2つ", baseRow.with(colGain, "++1"), msgGainFormat},
		{"損益_プラスとマイナス", baseRow.with(colGain, "+-1"), msgGainFormat},
		{"損益_符号の後に空白", baseRow.with(colGain, "- 1"), msgGainFormat},
		{"損益_英字", baseRow.with(colGain, "abc"), msgGainFormat},
		{"損益_全角数字", baseRow.with(colGain, "１０００"), msgGainFormat},
		{"損益_前に空白", baseRow.with(colGain, " 1000"), msgGainFormat},
		// 損益の大きさ
		{"損益_1兆", baseRow.with(colGain, "1000000000000"), msgGainOutOfRange},
		{"損益_プラス1兆", baseRow.with(colGain, "+1000000000000"), msgGainOutOfRange},
		{"損益_マイナス1兆", baseRow.with(colGain, "-1000000000000"), msgGainOutOfRange},
		{"損益_int64に収まらない正の値", baseRow.with(colGain, "99999999999999999999"), msgGainOutOfRange},
		{"損益_int64に収まらない負の値", baseRow.with(colGain, "-99999999999999999999"), msgGainOutOfRange},
		// 元本（評価額と損益がどちらも正しいとき）
		{"元本0（評価額1000・損益1000）", baseRow.with(colValuation, "1000").with(colGain, "1000"), msgPrincipal},
		{"元本が負（評価額1000・損益1500）", baseRow.with(colValuation, "1000").with(colGain, "1500"), msgPrincipal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRowProblems(t, withHeader(tc.row), []managedcsv.Problem{at(2, tc.msg)})
		})
	}
}

// 要件29・「各行」の列の数: 列の数が4でない行は、その行の他の列を確かめない（列の数の誤りだけ）。
func Test要件29_Parseは列の数が4でない行を列の数の誤りだけにする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
	}{
		{"3列", "WealthNavi,2026-11-16,4000000"},
		{"3列で他の列も誤り", ",2026-02-30,abc"},
		{"5列", "WealthNavi,2026-11-16,4000000,1000000,余分"},
		{"5列で他の列も誤り", ",2026-02-30,abc,xyz,余分"},
		{"1列", "WealthNavi"},
		{"末尾のカンマで5列", "WealthNavi,2026-11-16,4000000,1000000,"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRowProblems(t, csvOf(header, tc.line), []managedcsv.Problem{at(2, msgColumns)})
		})
	}
}

// 売買履歴の CSV と同じ（5.2）: カンマだけの行（,,,）は空の行として読み飛ばさず、名前・日付・評価額・損益の4つの誤りにする。
func Test要件27_Parseはカンマだけの行を読み飛ばさず4つの列の誤りにする(t *testing.T) {
	t.Parallel()
	assertRowProblems(t, csvOf(header, ",,,"), []managedcsv.Problem{
		at(2, msgName),
		at(2, msgDateFormat),
		at(2, msgValuationFormat),
		at(2, msgGainFormat),
	})
}

// 「同じ行では表の上の順」: 1つの行のすべての列が誤りのとき、名前・日付・評価額・損益の順に出る（1つの列では最初に当たった1つだけ）。
// 評価額と損益に誤りがあるので、元本は確かめない。
func Test要件31_Parseは1つの行の誤りを表の順に出し元本は確かめない(t *testing.T) {
	t.Parallel()
	row := fields{"", "2026-02-30", "abc", "xyz"}
	assertRowProblems(t, withHeader(row), []managedcsv.Problem{
		at(2, msgName),
		at(2, msgDateFormat),
		at(2, msgValuationFormat),
		at(2, msgGainFormat),
	})
}

// 元本は、評価額と損益がどちらも正しいときだけ確かめる: 評価額 0・損益 5 は評価額の誤りだけ（元本 -5 の誤りは出ない）。
// 評価額が正しく損益が誤りのとき（評価額1000・損益 abc）も、損益の誤りだけ。
func Test要件30_Parseは評価額か損益に誤りがあれば元本を確かめない(t *testing.T) {
	t.Parallel()
	assertRowProblems(t, withHeader(baseRow.with(colValuation, "0").with(colGain, "5")), []managedcsv.Problem{at(2, msgValuationZero)})
	assertRowProblems(t, withHeader(baseRow.with(colValuation, "1000").with(colGain, "abc")), []managedcsv.Problem{at(2, msgGainFormat)})
	assertRowProblems(t, withHeader(baseRow.with(colValuation, "1000000000000").with(colGain, "1000000000000")),
		[]managedcsv.Problem{at(2, msgValuationTooBig), at(2, msgGainOutOfRange)})
}

// 名前や日付が誤りでも、評価額と損益がどちらも正しければ元本は確かめる（元本は名前・日付の誤りに左右されない）。
func Test要件30_Parseは名前や日付の誤りの行でも元本を確かめる(t *testing.T) {
	t.Parallel()
	row := fields{"", "2026-11-17", "1000", "1000"}
	assertRowProblems(t, withHeader(row), []managedcsv.Problem{
		at(2, msgName),
		at(2, msgDateFuture),
		at(2, msgPrincipal),
	})
}

// 基準24（Parse の分）: 3行目は評価額が 0、5行目は日付が 2026-02-30 で損益が abc、ほかの行は正しい
// → 3行目の評価額、5行目の日付、5行目の損益の順に、すべて出る（正しい行は出ない）。
func Test基準24_Parseはすべての行のすべての誤りを行番号の昇順に出す(t *testing.T) {
	t.Parallel()
	content := withHeader(
		baseRow,                             // 2行目
		baseRow.with(colValuation, "0"),     // 3行目
		baseRow.with(colDate, "2026-11-15"), // 4行目
		baseRow.with(colDate, "2026-02-30").with(colGain, "abc"), // 5行目
		baseRow.with(colDate, "2026-11-14"),                      // 6行目
	)
	assertRowProblems(t, content, []managedcsv.Problem{
		at(3, msgValuationZero),
		at(5, msgDateFormat),
		at(5, msgGainFormat),
	})
}

// 基準25（Parse の分）: 2行目「WealthNavi,2026-11-16,…」、4行目「WealthNavi,2026/11/16,…」（書き方が違っても同じ日）、3行目は別の正しい行
// → Duplicates に「4行目: WealthNavi の 2026-11-16 の記録が、2行目と重なっています」だけ。Problems は空、Rows は3行（2回目の行も含む。
// Duplicates は Rows のうちの2回目以降の行）。
func Test基準25_Parseは同じ名前と同じ日付の2回目以降の行を重なりにする(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		"WealthNavi,2026-11-16,4000000,1000000",
		"WealthNavi,2026-10-31,3950000,950000",
		"WealthNavi,2026/11/16,4100000,1100000",
	)

	parsed := mustParse(t, content)

	if len(parsed.Problems) != 0 {
		t.Errorf("Problems: got %+v, want なし", parsed.Problems)
	}
	want := []managedcsv.Problem{at(4, "WealthNavi の 2026-11-16 の記録が、2行目と重なっています")}
	if !slices.Equal(parsed.Duplicates, want) {
		t.Errorf("Duplicates:\n got %+v\nwant %+v", parsed.Duplicates, want)
	}
	if len(parsed.Rows) != 3 {
		t.Errorf("Rows の数: got %d, want 3", len(parsed.Rows))
	}
}

// 要件30・5.2: 同じ名前・同じ日付が3回 → 2回目（3行目）と3回目（4行目）に「…、2行目と重なっています」（m は最初に出た行）。
func Test要件30_Parseは同じ組が3回出ると2回目と3回目をどちらも最初の行と重ねる(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		"WealthNavi,2026-11-16,4000000,1000000",
		"WealthNavi,2026-11-16,4100000,1100000",
		"WealthNavi,2026/11/16,4200000,1200000",
	)

	parsed := mustParse(t, content)

	want := []managedcsv.Problem{
		at(3, "WealthNavi の 2026-11-16 の記録が、2行目と重なっています"),
		at(4, "WealthNavi の 2026-11-16 の記録が、2行目と重なっています"),
	}
	if !slices.Equal(parsed.Duplicates, want) {
		t.Errorf("Duplicates:\n got %+v\nwant %+v", parsed.Duplicates, want)
	}
}

// 要件1・要件30: 大文字と小文字だけが違う名前、前後の空白だけが違う名前（`WealthNavi` と `WealthNavi `）、同じ名前で違う日付は重ならない。
func Test要件30_Parseは名前の大文字小文字と前後の空白が違えば重ならず日付が違っても重ならない(t *testing.T) {
	t.Parallel()
	content := withHeader(
		baseRow, // 2行目
		baseRow.with(colName, "wealthnavi"),
		baseRow.with(colName, "WealthNavi "),
		baseRow.with(colName, " WealthNavi"),
		baseRow.with(colDate, "2026-11-15"),
	)

	parsed := mustParse(t, content)

	if len(parsed.Problems) != 0 || len(parsed.Duplicates) != 0 || len(parsed.Rows) != 5 {
		t.Errorf("Parse: got 行 %d・誤り %+v・重なり %+v, want 5行・誤りなし・重なりなし", len(parsed.Rows), parsed.Problems, parsed.Duplicates)
	}
	// 名前は書いたとおりに持つ（前後の空白も取り除かない）。
	if got := parsed.Rows[2].Valuation.AccountName; got != "WealthNavi " {
		t.Errorf("3つ目の行の名前: got %q, want %q", got, "WealthNavi ")
	}
	if got := parsed.Rows[3].Valuation.AccountName; got != " WealthNavi" {
		t.Errorf("4つ目の行の名前: got %q, want %q", got, " WealthNavi")
	}
}

// 要件27・5.2: BOM 付き・CRLF・ダブルクォートで囲んだ名前（カンマを含む）・空の行・末尾の空行を読める。名前のカンマは列の区切りにならない。
// 空の行は読み飛ばすが、行番号には数える。
func Test要件27_ParseはBOMとCRLFとダブルクォートと空の行を読める(t *testing.T) {
	t.Parallel()
	content := []byte(bom + header + "\r\n" +
		`"Wealth, Navi",2026-11-16,4000000,1000000` + "\r\n" +
		"\r\n" +
		"Alpha,2026/11/1,500000,-20000\r\n" +
		"\r\n")

	parsed := mustParse(t, content)

	want := []managedcsv.ValuationRow{
		{Line: 2, Valuation: model.ManagedValuation{AccountName: "Wealth, Navi", ValuationDate: "2026-11-16", Valuation: 4_000_000, Gain: 1_000_000}},
		{Line: 4, Valuation: model.ManagedValuation{AccountName: "Alpha", ValuationDate: "2026-11-01", Valuation: 500_000, Gain: -20_000}}, // 3行目は空の行
	}
	if !slices.Equal(parsed.Rows, want) {
		t.Errorf("Parse の行:\n got %+v\nwant %+v", parsed.Rows, want)
	}
}

// 要件27: 改行は CRLF と LF のどちらでもよい（混ざっていてもよい）。BOM はなくてもよい。最後の行に改行がなくてもよい。
func Test要件27_Parseは改行の種類とBOMの有無と末尾の改行の有無によらず読める(t *testing.T) {
	t.Parallel()
	row := baseRow.line()
	cases := []struct {
		name    string
		content string
	}{
		{"LF", header + "\n" + row + "\n"},
		{"CRLF", header + "\r\n" + row + "\r\n"},
		{"混在", header + "\r\n" + row + "\n"},
		{"BOMなし・末尾の改行なし", header + "\n" + row},
		{"BOM付き・末尾の改行なし", bom + header + "\n" + row},
		{"末尾に空行が複数", header + "\n" + row + "\n\n\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed := mustParse(t, []byte(tc.content))
			if len(parsed.Rows) != 1 || parsed.Rows[0].Line != 2 || parsed.Rows[0].Valuation.AccountName != "WealthNavi" {
				t.Errorf("Parse の行: got %+v, want 2行目の WealthNavi の1行だけ", parsed.Rows)
			}
		})
	}
}

// 要件27: 見出しや値をダブルクォートで囲んでも、囲まない値と同じに読める。
func Test要件27_Parseは囲んだ値を囲まない値と同じに読む(t *testing.T) {
	t.Parallel()
	content := csvOf(
		`"名前","日付","評価額","損益"`,
		`"WealthNavi","2026-11-16","4000000","1000000"`,
	)
	parsed := mustParse(t, content)
	if len(parsed.Rows) != 1 || parsed.Rows[0].Valuation.Valuation != 4_000_000 || parsed.Rows[0].Valuation.Gain != 1_000_000 {
		t.Errorf("Parse の行: got %+v, want 評価額 4000000・損益 1000000 の1行", parsed.Rows)
	}
}

// 5.2・行番号: 2行目が空の行なら、次の行（3行目）の誤りは「3行目: 」（空の行も行番号に数える）。
func Test要件27_Parseは空の行を数えた行番号で誤りを出す(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		"",                                     // 2行目: 空の行
		baseRow.with(colValuation, "0").line(), // 3行目
	)
	assertRowProblems(t, content, []managedcsv.Problem{at(3, msgValuationZero)})
}

// 5.2: 1行目が空の行で、2行目が見出しの CSV は読める（空の行は読み飛ばす。行番号には数える）。
func Test要件27_Parseは1行目が空の行で2行目が見出しのCSVを読める(t *testing.T) {
	t.Parallel()
	content := []byte("\n" + header + "\n" + baseRow.line() + "\n")
	parsed := mustParse(t, content)
	if len(parsed.Rows) != 1 || parsed.Rows[0].Line != 3 {
		t.Errorf("Parse の行: got %+v, want 3行目の1行だけ（空の1行目も行番号に数える）", parsed.Rows)
	}
}

// 要件27・基準22（d）: UTF-8 として読めない（Shift_JIS）CSV は、ファイルの段階の誤り1つだけ。
// Shift_JIS の見出し「名前,日付,評価額,損益」と、名前に漢字を含む1行（ロボアド,2026-01-10,500000,1000）。
func Test基準22d_Parseは文字コードがUTF8でないCSVをファイルの段階の誤りにする(t *testing.T) {
	t.Parallel()
	shiftJIS := "" +
		"\x96\xbc\x91\x4f\x2c\x93\xfa\x95\x74\x2c\x95\x5d\x89\xbf\x8a\x7a\x2c\x91\xb9\x89\x76\x0d\x0a" +
		"\x83\x8d\x83\x7b\x83\x41\x83\x68\x2c\x32\x30\x32\x36\x2d\x30\x31\x2d\x31\x30\x2c\x35\x30\x30\x30\x30\x30\x2c\x31\x30\x30\x30\x0d\x0a"
	assertFileProblem(t, []byte(shiftJIS), fileProblem(msgEncoding))
}

// 要件28・基準22（e）: 見出しが完全に一致しない CSV は、ファイルの段階の誤り1つだけ（見出しの後の行は確かめない）。
// 見出しの前後の空白は見出しの誤り。
func Test基準22e_Parseは見出しが一致しないCSVをファイルの段階の誤りにする(t *testing.T) {
	t.Parallel()
	row := baseRow.line()
	cases := []struct {
		name   string
		header string
	}{
		{"評価額と日付が逆", "名前,評価額,日付,損益"},
		{"列が足りない", "名前,日付,評価額"},
		{"列が多い", "名前,日付,評価額,損益,備考"},
		{"表記が違う", "口座名,日付,評価額,損益"},
		{"空白が入る", "名前, 日付,評価額,損益"},
		{"見出しの前に空白", " 名前,日付,評価額,損益"},
		{"見出しの後ろに空白", "名前,日付,評価額,損益 "},
		{"英語の見出し", "name,date,valuation,gain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertFileProblem(t, csvOf(tc.header, row), fileProblem(msgHeader))
		})
	}
}

// 5.2: BOM だけのファイルは、見出しがない（見出しの誤り）。BOM 付きの正しい見出しだけの CSV は、「取り込む行がありません」だけ。
func Test要件28_ParseはBOMだけのファイルを見出しの誤りにしBOM付きの見出しは見出しとして読む(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, []byte(bom), fileProblem(msgHeader))
	assertFileProblem(t, []byte(bom+header+"\n"), fileProblem(msgNoRows))
}

// 要件31・基準22（f）: 見出しだけ（空でない行が1つもない）の CSV は「取り込む行がありません」。空の行だけの続きも同じ。
func Test基準22f_Parseは取り込む行がないCSVをファイルの段階の誤りにする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		content string
	}{
		{"見出しだけ", header + "\n"},
		{"見出しだけで末尾の改行なし", header},
		{"見出しと空の行", header + "\n\n\n"},
		{"見出しと CRLF の空の行", header + "\r\n\r\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertFileProblem(t, []byte(tc.content), fileProblem(msgNoRows))
		})
	}
}

// 「ファイルの段階の順番」: 見出しが違い、見出しの後に行がないときは、見出しの誤りだけ（「取り込む行がありません」は出ない）。
func Test要件31_Parseは見出し違いで行がないときは見出しの誤りだけにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, csvOf("名前,評価額,日付,損益"), fileProblem(msgHeader))
}

// 基準22（g）: 3行目の名前が `"Wealth` で閉じていない → 「3行目: ダブルクォートの使い方が正しくありません」だけ
// （他の行の誤りは確かめない。最初の1か所だけ。ファイルの段階の誤り）。
func Test基準22g_Parseは閉じていないダブルクォートを1つの誤りにする(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		baseRow.line(),
		`"Wealth,2026-11-16,4000000,1000000`,
		baseRow.with(colValuation, "0").line(), // 評価額の誤りも出さない
	)
	assertFileProblem(t, content, at(3, msgQuoteTailMessage))
}

// 要件27: 囲んでいない値の中にダブルクォートがある（bare quote）ときも、同じ文言を1つだけ出す。
func Test要件27_Parseは囲んでいない値の中のダブルクォートを誤りにする(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		`Wealth"Navi,2026-11-16,4000000,1000000`,
		baseRow.with(colValuation, "0").line(), // 評価額の誤りは出さない
	)
	assertFileProblem(t, content, at(2, msgQuoteTailMessage))
}

// 要件27: 1行目の見出しで当たったときも、見出しの文言ではなく「1行目: ダブルクォートの使い方が正しくありません」を出す。
// ダブルクォートの誤りは見出しの誤りより先（ファイルの段階の表の順）。
func Test要件27_Parseはダブルクォートの誤りを見出しの誤りより先に出す(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, csvOf(`"名前,日付,評価額,損益`, baseRow.line()), at(1, msgQuoteTailMessage))
	content := csvOf(
		"名前,評価額,日付,損益", // 見出しの誤り
		baseRow.line(),
		`"Wealth,2026-11-16,4000000,1000000`,
	)
	assertFileProblem(t, content, at(3, msgQuoteTailMessage))
}

// 要件27・要件29: 名前が複数行にまたがる（`"Wealth` と改行と `Navi"`）記録は、改行が制御文字なので誤りになり、行番号はその記録が始まる行（2行目）。
// 後ろの行（4行目）の誤りの行番号は、ファイルの行番号のまま（2つの誤りだけ。2〜3行目の記録は1つの行として数える）。
func Test要件27_Parseは複数行にまたがる行の後の行番号をファイルの行番号にする(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		`"Wealth`,
		`Navi",2026-11-16,4000000,1000000`,
		"Alpha,2026-11-16,0,0",
	)
	assertRowProblems(t, content, []managedcsv.Problem{
		at(2, msgNameControl),
		at(4, msgValuationZero),
	})
}

// 要件29・要件31: 名前の列に制御文字（タブ・NUL）がある行は「名前に改行やタブなどの制御文字は使えません」。
// 同じ行の日付の誤りより前に出る（同じ行では表の上の順）。
func Test要件29_Parseは名前に制御文字がある行を制御文字の誤りにする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		row  fields
		want []managedcsv.Problem
	}{
		{"名前にタブ", baseRow.with(colName, "a\tb"), []managedcsv.Problem{at(2, msgNameControl)}},
		{"名前の後ろに NUL", baseRow.with(colName, "a\x00"), []managedcsv.Problem{at(2, msgNameControl)}},
		{"名前に制御文字があり日付も実在しない", baseRow.with(colName, "a\tb").with(colDate, "2026-02-30"),
			[]managedcsv.Problem{at(2, msgNameControl), at(2, msgDateFormat)}},
		{"名前に制御文字があり日付も未来", baseRow.with(colName, "a\x00").with(colDate, "2026-11-17"),
			[]managedcsv.Problem{at(2, msgNameControl), at(2, msgDateFuture)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRowProblems(t, withHeader(tc.row), tc.want)
		})
	}
}

// 要件29・「CSV の誤りの文言」: 名前がタブだけの行（空白以外の文字がない）は、制御文字の誤りは確かめず「名前を入れてください」だけ。
func Test要件29_Parseは名前がタブだけの行を名前の誤りだけにする(t *testing.T) {
	t.Parallel()
	assertRowProblems(t, withHeader(baseRow.with(colName, "\t")), []managedcsv.Problem{at(2, msgName)})
}
