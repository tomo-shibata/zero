package tradecsv_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/interface/tradecsv"
)

// 仕様: 要件（docs/requirements/trade-history.md）の要件22（UTF-8・BOM・カンマ区切り・CRLF と LF・ダブルクォート・空の行）・
// 要件23（見出し）・要件24（各列の書き方）・要件21 と「CSV の誤りの文言」の表（段階1: ファイル全体、段階2: 各行の書き方。
// 行番号はファイルの行番号。誤りは行番号の昇順、同じ行では表の順）・基準25・基準24（d）（e）（f）（g）・基準26 と、
// プラン（docs/plans/trade-history/plan.md）4.6（PR③）の tradecsv.Parse・ParseError・Problem、4.7（ダブルクォートの誤り・行番号・
// 株数の書き方）、5.3・5.5（PR③）の「Parse（単体）」。
// Parse は段階1・段階2だけを確かめる（行どうしの関係の段階3は command.ImportTrades の役目で、Parse は確かめない）。
// 今日は 2026-11-16（日本時間）。

const (
	today  = "2026-11-16"
	header = "銘柄コード,銘柄名,通貨,売買,価格,株数,取引日"

	// 要件定義の「CSV の誤りの文言」の表（n行目: は prefix で付ける）。
	msgCode          = "銘柄コードは半角の英大文字・数字・「.」・「-」で1〜10文字にしてください"
	msgName          = "銘柄名を入れてください"
	msgCurrency      = "通貨は JPY か USD にしてください"
	msgSide          = "売買は 買 か 売 にしてください"
	msgPriceFormat   = "価格は半角数字と小数点で書いてください"
	msgPriceNotPos   = "価格は0より大きくしてください"
	msgPriceTooLarge = "価格は1億未満にしてください"
	msgPriceYen      = "円の価格は小数第1位までにしてください"
	msgPriceDollar   = "ドルの価格は小数第2位までにしてください"
	msgQtyFormat     = "株数は1以上の整数にしてください"
	msgQtyTooLarge   = "株数は1,000万株未満にしてください"
	msgDateFormat    = "取引日は YYYY-MM-DD か YYYY/M/D の実在する日付にしてください"
	msgDateFuture    = "未来の取引日は取り込めません"
	msgColumns       = "列の数を7にしてください"

	msgEncoding  = "CSV の文字コードが UTF-8 ではありません"
	msgHeader    = "1行目の見出しを「銘柄コード,銘柄名,通貨,売買,価格,株数,取引日」にしてください"
	msgNoRows    = "取り込む行がありません"
	msgQuoteTail = "行目: ダブルクォートの使い方が正しくありません"
)

// 列の位置。
const (
	colCode = iota
	colName
	colCurrency
	colSide
	colPrice
	colQuantity
	colDate
)

// fields は、CSV の1行の7つの値。
type fields [7]string

// baseRow は、誤りのない円の買いの行。
var baseRow = fields{"7203", "トヨタ自動車", "JPY", "買", "2500", "100", "2026-01-10"}

// usdRow は、誤りのないドルの買いの行。
var usdRow = fields{"AAPL", "Apple", "USD", "買", "150.25", "10", "2026-01-10"}

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

// parseProblems は、err が ParseError であることを確かめて、その Problems を返す（ポインタでも値でも受ける）。
func parseProblems(t *testing.T, err error) []tradecsv.Problem {
	t.Helper()
	if err == nil {
		t.Fatal("Parse: got err nil, want *ParseError")
	}
	var pe *tradecsv.ParseError
	if errors.As(err, &pe) {
		return pe.Problems
	}
	var anyErr any = err
	if pv, ok := anyErr.(tradecsv.ParseError); ok {
		return pv.Problems
	}
	t.Fatalf("Parse: got err %v (%T), want ParseError", err, err)
	return nil
}

// messagesOf は、problems の Message を並び順のまま返す。
func messagesOf(problems []tradecsv.Problem) []string {
	msgs := make([]string, 0, len(problems))
	for _, p := range problems {
		msgs = append(msgs, p.Message)
	}
	return msgs
}

// assertProblems は、Parse の誤りが want（Line と Message。並び順も含めて）とちょうど同じであることを確かめる。
func assertProblems(t *testing.T, content []byte, want []tradecsv.Problem) {
	t.Helper()
	rows, err := tradecsv.Parse(content, today)
	if err == nil {
		t.Fatalf("Parse: got err nil（行 %+v）, want 誤り %+v", rows, want)
	}
	if got := parseProblems(t, err); !slices.Equal(got, want) {
		t.Errorf("Parse の誤り:\n got %+v\nwant %+v", got, want)
	}
}

// assertMessages は、Parse の誤りの文言が want とちょうど同じ（並び順も含めて）であることを確かめる（Line は見ない）。
func assertMessages(t *testing.T, content []byte, want ...string) {
	t.Helper()
	_, err := tradecsv.Parse(content, today)
	if got := messagesOf(parseProblems(t, err)); !slices.Equal(got, want) {
		t.Errorf("Parse の誤りの文言:\n got %q\nwant %q", got, want)
	}
}

// at は、n行目の誤り（Line が n で、文言が「n行目: <msg>」）を作る。
func at(n int, msg string) tradecsv.Problem {
	return tradecsv.Problem{Line: n, Message: fmt.Sprintf("%d行目: %s", n, msg)}
}

// fileProblem は、行番号のないファイルの段階の誤り（Line は0）を作る。
func fileProblem(msg string) tradecsv.Problem {
	return tradecsv.Problem{Line: 0, Message: msg}
}

// 成功: 見出しと2行を読み、ImportRow にする。Line はファイルの行番号、価格は 0.01 単位、取引日は YYYY-MM-DD にそろえる。
func Test基準25_Parseは正しいCSVをImportRowにする(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10",
		"AAPL,Apple,USD,売,150.25,10,2026/1/6",
	)

	got, err := tradecsv.Parse(content, today)
	if err != nil {
		t.Fatalf("Parse: got err %v, want nil", err)
	}

	want := []command.ImportRow{
		{
			Line:  2,
			Stock: model.Stock{Code: "7203", Name: "トヨタ自動車", Currency: model.CurrencyJPY},
			Trade: model.Trade{StockCode: "7203", Side: model.TradeSideBuy, Quantity: 100, PriceHundredths: 250000, TradeDate: "2026-01-10"},
		},
		{
			Line:  3,
			Stock: model.Stock{Code: "AAPL", Name: "Apple", Currency: model.CurrencyUSD},
			Trade: model.Trade{StockCode: "AAPL", Side: model.TradeSideSell, Quantity: 10, PriceHundredths: 15025, TradeDate: "2026-01-06"},
		},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Parse の行:\n got %+v\nwant %+v", got, want)
	}
}

// 基準25: 誤りにならない値（銘柄コード・価格・株数・取引日の境界）。
func Test基準25_Parseは正しい値を受け付ける(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		row  fields
	}{
		{"銘柄コード_4桁の数字", baseRow.with(colCode, "7203")},
		{"銘柄コード_数字と英字", baseRow.with(colCode, "130A")},
		{"銘柄コード_ピリオド", baseRow.with(colCode, "BRK.B")},
		{"銘柄コード_ハイフン", baseRow.with(colCode, "BRK-B")},
		{"銘柄コード_10文字", baseRow.with(colCode, "ABCDEFGHIJ")},
		{"円の価格_0.1", baseRow.with(colPrice, "0.1")},
		{"円の価格_99999999.9", baseRow.with(colPrice, "99999999.9")},
		{"円の価格_小数第1位が0", baseRow.with(colPrice, "2000.0")},
		{"ドルの価格_0.01", usdRow.with(colPrice, "0.01")},
		{"ドルの価格_99999999.99", usdRow.with(colPrice, "99999999.99")},
		{"ドルの価格_整数", usdRow.with(colPrice, "400")},
		{"株数_1", baseRow.with(colQuantity, "1")},
		{"株数_9999999", baseRow.with(colQuantity, "9999999")},
		{"取引日_今日", baseRow.with(colDate, "2026-11-16")},
		{"取引日_スラッシュで0埋めなし", baseRow.with(colDate, "2026/1/6")},
		{"取引日_スラッシュで0埋めあり", baseRow.with(colDate, "2026/01/06")},
		{"取引日_スラッシュで月だけ0埋め", baseRow.with(colDate, "2026/01/6")},
		{"取引日_スラッシュで日だけ0埋め", baseRow.with(colDate, "2026/1/06")},
		{"取引日_ハイフンで0埋め", baseRow.with(colDate, "2026-01-06")},
		{"取引日_うるう日", baseRow.with(colDate, "2024-02-29")},
		{"取引日_今日のスラッシュ形", baseRow.with(colDate, "2026/11/16")},
		{"売買_売", baseRow.with(colSide, "売")},
		{"銘柄名_前後に空白", baseRow.with(colName, " Apple ")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows, err := tradecsv.Parse(withHeader(tc.row), today)
			if err != nil {
				t.Fatalf("Parse(%v): got err %v (%+v), want nil", tc.row, err, parseProblemsOrNil(err))
			}
			if len(rows) != 1 {
				t.Errorf("Parse(%v) の行の数: got %d, want 1", tc.row, len(rows))
			}
		})
	}
}

// parseProblemsOrNil は、失敗メッセージ用に、err が ParseError ならその Problems を返す。
func parseProblemsOrNil(err error) []tradecsv.Problem {
	var pe *tradecsv.ParseError
	if errors.As(err, &pe) {
		return pe.Problems
	}
	return nil
}

// 基準25: 価格・取引日の値は、0.01 単位の整数と YYYY-MM-DD にそろえて ImportRow に入る。
func Test基準25_Parseは価格を0_01単位に取引日をYYYY_MM_DDにそろえる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		row       fields
		wantPrice int64
		wantDate  string
	}{
		{"円の最小", baseRow.with(colPrice, "0.1"), 10, "2026-01-10"},
		{"円の最大", baseRow.with(colPrice, "99999999.9"), 9_999_999_990, "2026-01-10"},
		{"ドルの最小", usdRow.with(colPrice, "0.01"), 1, "2026-01-10"},
		{"ドルの最大", usdRow.with(colPrice, "99999999.99"), 9_999_999_999, "2026-01-10"},
		{"スラッシュの0埋めなし", baseRow.with(colDate, "2026/1/6"), 250000, "2026-01-06"},
		{"スラッシュの0埋めあり", baseRow.with(colDate, "2026/01/06"), 250000, "2026-01-06"},
		{"ハイフン", baseRow.with(colDate, "2026-01-06"), 250000, "2026-01-06"},
		{"スラッシュで月と日が2桁", baseRow.with(colDate, "2026/11/16"), 250000, "2026-11-16"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows, err := tradecsv.Parse(withHeader(tc.row), today)
			if err != nil || len(rows) != 1 {
				t.Fatalf("Parse: got (%d行, %v), want (1行, nil)", len(rows), err)
			}
			if got := rows[0].Trade.PriceHundredths; got != tc.wantPrice {
				t.Errorf("価格（0.01 単位）: got %d, want %d", got, tc.wantPrice)
			}
			if got := rows[0].Trade.TradeDate; got != tc.wantDate {
				t.Errorf("取引日: got %q, want %q", got, tc.wantDate)
			}
		})
	}
}

// 基準25: 各列の誤り。表の値を1つずつ書いた行は、「CSV の誤りの文言」の該当する文言が、その行（2行目）の誤りとして1つだけ出る。
func Test基準25_Parseは各列の誤りを表の文言で返す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		row  fields
		msg  string
	}{
		// 銘柄コード
		{"銘柄コード_空", baseRow.with(colCode, ""), msgCode},
		{"銘柄コード_小文字", baseRow.with(colCode, "aapl"), msgCode},
		{"銘柄コード_11文字", baseRow.with(colCode, "ABCDEFGHIJK"), msgCode},
		{"銘柄コード_途中に空白", baseRow.with(colCode, "72 03"), msgCode},
		{"銘柄コード_前に空白", baseRow.with(colCode, " 7203"), msgCode},
		// 銘柄名
		{"銘柄名_空", baseRow.with(colName, ""), msgName},
		{"銘柄名_半角の空白だけ", baseRow.with(colName, "   "), msgName},
		{"銘柄名_全角の空白だけ", baseRow.with(colName, "　"), msgName},
		// 通貨
		{"通貨_EUR", baseRow.with(colCurrency, "EUR"), msgCurrency},
		{"通貨_小文字", baseRow.with(colCurrency, "jpy"), msgCurrency},
		{"通貨_空", baseRow.with(colCurrency, ""), msgCurrency},
		// 売買
		{"売買_買い", baseRow.with(colSide, "買い"), msgSide},
		{"売買_BUY", baseRow.with(colSide, "BUY"), msgSide},
		{"売買_空", baseRow.with(colSide, ""), msgSide},
		// 価格の書き方
		{"価格_空", baseRow.with(colPrice, ""), msgPriceFormat},
		{"価格_マイナス", baseRow.with(colPrice, "-1"), msgPriceFormat},
		{"価格_3桁区切り", baseRow.with(colPrice, "1,500"), msgPriceFormat},
		{"価格_英字", baseRow.with(colPrice, "abc"), msgPriceFormat},
		{"価格_小数点の後に数字がない", baseRow.with(colPrice, "1."), msgPriceFormat},
		{"価格_小数点の前に数字がない", baseRow.with(colPrice, ".5"), msgPriceFormat},
		{"価格_小数点が2つ", baseRow.with(colPrice, "1.2.3"), msgPriceFormat},
		{"価格_全角数字", baseRow.with(colPrice, "１００"), msgPriceFormat},
		// 価格の大きさ
		{"価格_0", baseRow.with(colPrice, "0"), msgPriceNotPos},
		{"価格_0.0", baseRow.with(colPrice, "0.0"), msgPriceNotPos},
		{"価格_1億", baseRow.with(colPrice, "100000000"), msgPriceTooLarge},
		{"価格_1億を超える小数", baseRow.with(colPrice, "100000000.5"), msgPriceTooLarge},
		{"円の価格_1億以上は精度より先に大きさ", baseRow.with(colPrice, "100000000.55"), msgPriceTooLarge},
		// 価格の精度（通貨が正しいときだけ）
		{"円の価格_小数第2位", baseRow.with(colPrice, "2000.25"), msgPriceYen},
		{"円の価格_0.05", baseRow.with(colPrice, "0.05"), msgPriceYen},
		{"円の価格_0.001", baseRow.with(colPrice, "0.001"), msgPriceYen},
		{"ドルの価格_小数第3位", usdRow.with(colPrice, "150.255"), msgPriceDollar},
		{"ドルの価格_0.001", usdRow.with(colPrice, "0.001"), msgPriceDollar},
		// 通貨が誤りの行では、価格の精度は確かめない（通貨の誤りだけ）
		{"通貨が誤りの行の価格_2000.255", baseRow.with(colCurrency, "EUR").with(colPrice, "2000.255"), msgCurrency},
		{"通貨が空の行の価格_150.255", baseRow.with(colCurrency, "").with(colPrice, "150.255"), msgCurrency},
		// 株数
		{"株数_空", baseRow.with(colQuantity, ""), msgQtyFormat},
		{"株数_0", baseRow.with(colQuantity, "0"), msgQtyFormat},
		{"株数_00", baseRow.with(colQuantity, "00"), msgQtyFormat},
		{"株数_マイナス", baseRow.with(colQuantity, "-1"), msgQtyFormat},
		{"株数_小数", baseRow.with(colQuantity, "1.5"), msgQtyFormat},
		{"株数_3桁区切り", baseRow.with(colQuantity, "1,000"), msgQtyFormat},
		{"株数_英字", baseRow.with(colQuantity, "abc"), msgQtyFormat},
		{"株数_1000万", baseRow.with(colQuantity, "10000000"), msgQtyTooLarge},
		{"株数_int64に収まらない", baseRow.with(colQuantity, "99999999999999999999"), msgQtyTooLarge},
		// 取引日
		{"取引日_空", baseRow.with(colDate, ""), msgDateFormat},
		{"取引日_実在しない日", baseRow.with(colDate, "2026-02-30"), msgDateFormat},
		{"取引日_スラッシュで実在しない日", baseRow.with(colDate, "2026/2/30"), msgDateFormat},
		{"取引日_うるう年でない年の2月29日", baseRow.with(colDate, "2025-02-29"), msgDateFormat},
		{"取引日_13月", baseRow.with(colDate, "2026/13/1"), msgDateFormat},
		{"取引日_区切りなし", baseRow.with(colDate, "20260110"), msgDateFormat},
		{"取引日_ピリオド区切り", baseRow.with(colDate, "2026.01.10"), msgDateFormat},
		{"取引日_ハイフンで月と日の0埋めなし", baseRow.with(colDate, "2026-1-6"), msgDateFormat},
		{"取引日_ハイフンで日の0埋めなし", baseRow.with(colDate, "2026-01-6"), msgDateFormat},
		{"取引日_英字", baseRow.with(colDate, "abc"), msgDateFormat},
		{"取引日_明日", baseRow.with(colDate, "2026-11-17"), msgDateFuture},
		{"取引日_明日のスラッシュ形", baseRow.with(colDate, "2026/11/17"), msgDateFuture},
		{"取引日_来年", baseRow.with(colDate, "2027-01-01"), msgDateFuture},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertProblems(t, withHeader(tc.row), []tradecsv.Problem{at(2, tc.msg)})
		})
	}
}

// 今の動きを止めておく境界（要件24）: 年が 0000 の取引日は、実在する日付として受け付けず、取引日の誤りにする。
func Test基準25_Parseは年が0000の取引日を誤りにする(t *testing.T) {
	t.Parallel()
	assertProblems(t, withHeader(baseRow.with(colDate, "0000-01-01")), []tradecsv.Problem{at(2, msgDateFormat)})
}

// 今の動きを止めておく境界（要件24）: 円の価格の小数第2位が 0（2500.00）は受け付け、価格は 250000（0.01 単位）になる。
func Test基準25_Parseは円の価格の小数第2位が0なら受け付ける(t *testing.T) {
	t.Parallel()
	rows, err := tradecsv.Parse(withHeader(baseRow.with(colPrice, "2500.00")), today)
	if err != nil || len(rows) != 1 {
		t.Fatalf("Parse: got (%d行, %v), want (1行, nil)", len(rows), err)
	}
	if got := rows[0].Trade.PriceHundredths; got != 250000 {
		t.Errorf("価格（0.01 単位）: got %d, want 250000", got)
	}
}

// 今の動きを止めておく境界（要件21・要件22）: カンマだけの行（,,,,,,）は空の行として読み飛ばさず、7つの列がすべて空の行として、
// その行（2行目）の7つの列の誤りを表の順に出す。
func Test基準25_Parseはカンマだけの行を読み飛ばさず7つの列の誤りにする(t *testing.T) {
	t.Parallel()
	assertProblems(t, csvOf(header, ",,,,,,"), []tradecsv.Problem{
		at(2, msgCode),
		at(2, msgName),
		at(2, msgCurrency),
		at(2, msgSide),
		at(2, msgPriceFormat),
		at(2, msgQtyFormat),
		at(2, msgDateFormat),
	})
}

// 今の動きを止めておく境界（要件22）: 1行目が空の行で、2行目が見出しの CSV は受け付ける（空の行は読み飛ばす。行番号には数える）。
func Test要件22_Parseは1行目が空の行で2行目が見出しのCSVを読める(t *testing.T) {
	t.Parallel()
	content := []byte("\n" + header + "\n" + "7203,トヨタ自動車,JPY,買,2500,100,2026-01-10\n")

	rows, err := tradecsv.Parse(content, today)
	if err != nil {
		t.Fatalf("Parse: got err %v (%+v), want nil", err, parseProblemsOrNil(err))
	}
	if len(rows) != 1 || rows[0].Stock.Code != "7203" || rows[0].Line != 3 {
		t.Errorf("Parse の行: got %+v, want 3行目の 7203 の1行だけ（空の1行目も行番号に数える）", rows)
	}
}

// 今の動きを止めておく境界（要件24）: 銘柄名の前後の空白は取り除かず、そのまま銘柄名になる。
func Test要件24_Parseは銘柄名の前後の空白を取り除かない(t *testing.T) {
	t.Parallel()
	rows, err := tradecsv.Parse(withHeader(baseRow.with(colName, " Apple ")), today)
	if err != nil || len(rows) != 1 {
		t.Fatalf("Parse: got (%d行, %v), want (1行, nil)", len(rows), err)
	}
	if got := rows[0].Stock.Name; got != " Apple " {
		t.Errorf("銘柄名: got %q, want %q（前後の空白をそのまま残す）", got, " Apple ")
	}
}

// 要件21・「各行」の列の数: 列の数が7でない行は、その行の他の列を確かめない（列の数の誤りだけ）。
// 5.5: 8列の行は、ダブルクォートの誤りではなく「n行目: 列の数を7にしてください」になる。
func Test基準25_Parseは列の数が7でない行を列の数の誤りだけにする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
	}{
		{"8列", "7203,トヨタ自動車,JPY,買,2500,100,2026-01-10,余分"},
		{"8列で他の列も誤り", "aapl,,EUR,BUY,abc,0,20260110,余分"},
		{"6列", "7203,トヨタ自動車,JPY,買,2500,100"},
		{"6列で他の列も誤り", "aapl,,EUR,BUY,abc,0"},
		{"1列", "7203"},
		{"末尾のカンマで8列", "7203,トヨタ自動車,JPY,買,2500,100,2026-01-10,"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertProblems(t, csvOf(header, tc.line), []tradecsv.Problem{at(2, msgColumns)})
		})
	}
}

// 要件21・「誤りは行番号の昇順に並べ、同じ行では表の上の行の順」: 1つの行のすべての列が誤りのとき、
// 列の数を除く7つの文言が、銘柄コード・銘柄名・通貨・売買・価格・株数・取引日の順に出る（1つの列では最初に当たった1つだけ）。
func Test基準26_Parseは1つの行の誤りを表の順に出し1つの列では最初の1つだけにする(t *testing.T) {
	t.Parallel()
	row := fields{"aapl", "", "EUR", "BUY", "abc", "0", "20260110"}
	assertProblems(t, withHeader(row), []tradecsv.Problem{
		at(2, msgCode),
		at(2, msgName),
		at(2, msgCurrency),
		at(2, msgSide),
		at(2, msgPriceFormat),
		at(2, msgQtyFormat),
		at(2, msgDateFormat),
	})
}

// 基準26（Parse の分）: 3行目は株数が 0、5行目は取引日が 2026-02-30 で価格が abc、ほかの行は正しい。
// → 3行目の株数、5行目の価格、5行目の取引日の順に、すべて出る（正しい行は出ない）。
func Test基準26_Parseはすべての行のすべての誤りを行番号の昇順に出す(t *testing.T) {
	t.Parallel()
	content := withHeader(
		baseRow,                        // 2行目
		baseRow.with(colQuantity, "0"), // 3行目
		usdRow,                         // 4行目
		baseRow.with(colPrice, "abc").with(colDate, "2026-02-30"), // 5行目
		baseRow, // 6行目
	)
	assertProblems(t, content, []tradecsv.Problem{
		at(3, msgQtyFormat),
		at(5, msgPriceFormat),
		at(5, msgDateFormat),
	})
}

// Parse は段階1・段階2だけを確かめる。行どうしの食い違い（銘柄名・通貨・多すぎる売り）は誤りにしない（段階3は ImportTrades.Execute）。
func Test補助_Parseは行どうしの食い違いを確かめない(t *testing.T) {
	t.Parallel()
	content := withHeader(
		usdRow,                             // 2行目
		usdRow.with(colName, "Apple Inc."), // 3行目: 銘柄名が2行目と違う
		baseRow.with(colSide, "売").with(colQuantity, "999"), // 4行目: 持っていない銘柄の売り
	)
	rows, err := tradecsv.Parse(content, today)
	if err != nil {
		t.Fatalf("Parse: got err %v, want nil（行どうしの誤りは段階3）", err)
	}
	if len(rows) != 3 {
		t.Errorf("Parse の行の数: got %d, want 3", len(rows))
	}
}

// 要件22: BOM 付き・CRLF・ダブルクォートで囲んだ値（カンマを含む）・空の行・末尾の空行を読める。
// 銘柄名のカンマ（"Alphabet Inc., Class A"）は列の区切りにならない。空の行は読み飛ばすが、行番号には数える。
func Test基準23_ParseはBOMとCRLFとダブルクォートと空の行を読める(t *testing.T) {
	t.Parallel()
	content := []byte("\ufeff" + header + "\r\n" +
		`GOOGL,"Alphabet Inc., Class A",USD,買,150.5,10,2026-01-10` + "\r\n" +
		"\r\n" +
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10\r\n" +
		"\r\n")

	got, err := tradecsv.Parse(content, today)
	if err != nil {
		t.Fatalf("Parse: got err %v (%+v), want nil", err, parseProblemsOrNil(err))
	}

	want := []command.ImportRow{
		{
			Line:  2,
			Stock: model.Stock{Code: "GOOGL", Name: "Alphabet Inc., Class A", Currency: model.CurrencyUSD},
			Trade: model.Trade{StockCode: "GOOGL", Side: model.TradeSideBuy, Quantity: 10, PriceHundredths: 15050, TradeDate: "2026-01-10"},
		},
		{
			Line:  4, // 3行目は空の行
			Stock: model.Stock{Code: "7203", Name: "トヨタ自動車", Currency: model.CurrencyJPY},
			Trade: model.Trade{StockCode: "7203", Side: model.TradeSideBuy, Quantity: 100, PriceHundredths: 250000, TradeDate: "2026-01-10"},
		},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Parse の行:\n got %+v\nwant %+v", got, want)
	}
}

// 要件22: 改行は CRLF と LF のどちらでもよい（混ざっていてもよい）。BOM はなくてもよい。最後の行に改行がなくてもよい。
func Test要件22_Parseは改行の種類とBOMの有無と末尾の改行の有無によらず読める(t *testing.T) {
	t.Parallel()
	row := "7203,トヨタ自動車,JPY,買,2500,100,2026-01-10"
	cases := []struct {
		name    string
		content string
	}{
		{"LF", header + "\n" + row + "\n"},
		{"CRLF", header + "\r\n" + row + "\r\n"},
		{"混在", header + "\r\n" + row + "\n"},
		{"BOMなし・末尾の改行なし", header + "\n" + row},
		{"BOM付き・末尾の改行なし", "\ufeff" + header + "\n" + row},
		{"末尾に空行が複数", header + "\n" + row + "\n\n\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows, err := tradecsv.Parse([]byte(tc.content), today)
			if err != nil {
				t.Fatalf("Parse: got err %v (%+v), want nil", err, parseProblemsOrNil(err))
			}
			if len(rows) != 1 || rows[0].Line != 2 || rows[0].Stock.Code != "7203" {
				t.Errorf("Parse の行: got %+v, want 2行目の 7203 の1行だけ", rows)
			}
		})
	}
}

// 要件22: 見出しや値をダブルクォートで囲んでも、囲まない値と同じに読める。
func Test要件22_Parseは囲んだ値を囲まない値と同じに読む(t *testing.T) {
	t.Parallel()
	content := csvOf(
		`"銘柄コード","銘柄名","通貨","売買","価格","株数","取引日"`,
		`"7203","トヨタ自動車","JPY","買","2500","100","2026-01-10"`,
	)
	rows, err := tradecsv.Parse(content, today)
	if err != nil {
		t.Fatalf("Parse: got err %v (%+v), want nil", err, parseProblemsOrNil(err))
	}
	if len(rows) != 1 || rows[0].Stock.Code != "7203" || rows[0].Trade.PriceHundredths != 250000 {
		t.Errorf("Parse の行: got %+v, want 7203・250000 の1行", rows)
	}
}

// 要件24・基準24（d）: UTF-8 として読めない（Shift_JIS）CSV は、ファイルの段階の誤り1つだけ。
// Shift_JIS の見出しと1行（銘柄名「トヨタ自動車」を含む）。
func Test基準24d_Parseは文字コードがUTF8でないCSVをファイルの段階の誤りにする(t *testing.T) {
	t.Parallel()
	shiftJIS := "" +
		"\x96\xc1\x95\xbf\x83\x52\x81\x5b\x83\x68\x2c\x96\xc1\x95\xbf\x96\xbc\x2c\x92\xca\x89\xdd\x2c\x94\x84\x94\x83\x2c\x89\xbf\x8a\x69\x2c\x8a\x94\x90\x94\x2c\x8e\xe6\x88\xf8\x93\xfa\x0d\x0a" +
		"\x37\x32\x30\x33\x2c\x83\x67\x83\x88\x83\x5e\x8e\xa9\x93\xae\x8e\xd4\x2c\x4a\x50\x59\x2c\x94\x83\x2c\x32\x35\x30\x30\x2c\x31\x30\x30\x2c\x32\x30\x32\x36\x2d\x30\x31\x2d\x31\x30\x0d\x0a"
	assertProblems(t, []byte(shiftJIS), []tradecsv.Problem{fileProblem(msgEncoding)})
}

// 要件23・基準24（e）: 見出しが完全に一致しない CSV は、ファイルの段階の誤り1つだけ（見出しの後の行は確かめない）。
func Test基準24e_Parseは見出しが一致しないCSVをファイルの段階の誤りにする(t *testing.T) {
	t.Parallel()
	row := "7203,トヨタ自動車,JPY,買,2500,100,2026-01-10"
	cases := []struct {
		name   string
		header string
	}{
		{"通貨と売買が逆", "銘柄コード,銘柄名,売買,通貨,価格,株数,取引日"},
		{"列が足りない", "銘柄コード,銘柄名,通貨,売買,価格,株数"},
		{"列が多い", "銘柄コード,銘柄名,通貨,売買,価格,株数,取引日,備考"},
		{"表記が違う", "コード,銘柄名,通貨,売買,価格,株数,取引日"},
		{"空白が入る", "銘柄コード, 銘柄名,通貨,売買,価格,株数,取引日"},
		{"英語の見出し", "code,name,currency,side,price,quantity,date"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertProblems(t, csvOf(tc.header, row), []tradecsv.Problem{fileProblem(msgHeader)})
		})
	}
}

// 5.3: BOM だけのファイルは、見出しがない（1行目が見出しと一致しない）。
func Test補助_ParseはBOMだけのファイルを見出しの誤りにする(t *testing.T) {
	t.Parallel()
	assertProblems(t, []byte("\ufeff"), []tradecsv.Problem{fileProblem(msgHeader)})
}

// 5.3: BOM 付きの見出し（正しい）の CSV は、見出しとして読める（BOM が見出しの先頭の文字に残らない）。
func Test補助_ParseはBOM付きの見出しを正しい見出しとして読む(t *testing.T) {
	t.Parallel()
	_, err := tradecsv.Parse([]byte("\ufeff"+header+"\n"), today)
	// 見出しだけなので、誤りは「取り込む行がありません」だけ（見出しの誤りにならない）。
	if got := messagesOf(parseProblems(t, err)); !slices.Equal(got, []string{msgNoRows}) {
		t.Errorf("Parse(BOM 付きの見出しだけ): got %q, want [%q]", got, msgNoRows)
	}
}

// 要件21・基準24（f）・質問7: 見出しだけ（空でない行が1つもない）の CSV は「取り込む行がありません」。空の行だけの続きも同じ。
func Test基準24f_Parseは取り込む行がないCSVをファイルの段階の誤りにする(t *testing.T) {
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
			assertProblems(t, []byte(tc.content), []tradecsv.Problem{fileProblem(msgNoRows)})
		})
	}
}

// 5.5・「ファイルの段階の順番」: 見出しが違い、見出しの後に行がないときは、見出しの誤りだけ（「取り込む行がありません」は出ない）。
func Test基準24_Parseは見出し違いで行がないときは見出しの誤りだけにする(t *testing.T) {
	t.Parallel()
	assertProblems(t, csvOf("銘柄コード,銘柄名,売買,通貨,価格,株数,取引日"), []tradecsv.Problem{fileProblem(msgHeader)})
}

// 要件21・質問9・基準24（g）: 3行目の銘柄名が `"Apple` で閉じていない → 「3行目: ダブルクォートの使い方が正しくありません」だけ
// （他の行の誤りは確かめない。最初の1か所だけ）。
func Test基準24g_Parseは閉じていないダブルクォートを1つの誤りにする(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10",
		`AAPL,"Apple,USD,買,150.25,10,2026-01-10`,
		"MSFT,Microsoft,USD,買,400,0,2026-02-01", // 株数の誤りも出さない
	)
	assertMessages(t, content, "3"+msgQuoteTail)
}

// 質問9: 囲んでいない値の中にダブルクォートがある（bare quote）ときも、同じ文言を1つだけ出す。
func Test質問9_Parseは囲んでいない値の中のダブルクォートを誤りにする(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		`7203,トヨ"タ自動車,JPY,買,2500,100,2026-01-10`,
		"AAPL,Apple,USD,買,150.25,0,2026-01-10", // 株数の誤りは出さない
	)
	assertMessages(t, content, "2"+msgQuoteTail)
}

// 質問9・5.5: 1行目の見出しで当たったときも、見出しの文言ではなく「1行目: ダブルクォートの使い方が正しくありません」を出す。
func Test質問9_Parseは見出しのダブルクォートの誤りを見出しの文言にしない(t *testing.T) {
	t.Parallel()
	content := csvOf(
		`"銘柄コード,銘柄名,通貨,売買,価格,株数,取引日`,
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10",
	)
	assertMessages(t, content, "1"+msgQuoteTail)
}

// 5.5: ダブルクォートの誤りは見出しの誤りより先（ファイルの段階の表の順）。見出しが違っても、3行目の閉じていない囲みを出す。
func Test質問9_Parseはダブルクォートの誤りを見出しの誤りより先に出す(t *testing.T) {
	t.Parallel()
	content := csvOf(
		"銘柄コード,銘柄名,売買,通貨,価格,株数,取引日",
		"7203,トヨタ自動車,買,JPY,2500,100,2026-01-10",
		`AAPL,"Apple,買,USD,150.25,10,2026-01-10`,
	)
	assertMessages(t, content, "3"+msgQuoteTail)
}

// 5.5: 2行目の銘柄名が `"Apple` と改行と `Inc."` の2行にまたがり、3行目の後ろの列に囲んでいない `"` がある
// → 売買の行が始まる行（2行目）で「2行目: ダブルクォートの使い方が正しくありません」だけ。
func Test質問9_Parseはダブルクォートの誤りを売買の行が始まる行で出す(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		`AAPL,"Apple`,
		`Inc.",USD,買,150.25,10,20"26-01-10`,
	)
	assertMessages(t, content, "2"+msgQuoteTail)
}

// 5.5・行番号: 3行目が空の行で、ファイルの4行目の株数が 0 → 「4行目: 株数は1以上の整数にしてください」（空の行も行番号に数える）。
func Test基準26_Parseは空の行を数えた行番号で誤りを出す(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10", // 2行目
		"", // 3行目: 空の行
		"6758,ソニーグループ,JPY,買,3200,0,2026-01-10", // 4行目
	)
	assertProblems(t, content, []tradecsv.Problem{at(4, msgQtyFormat)})
}

// 5.5・行番号: 2行目の銘柄名が `"Apple` と改行と `Inc."` の2行にまたがり、ファイルの4行目の株数が 0 → 「4行目: …」
// （値の中の改行も行番号に数える）。またがった行自体は誤りではなく、その行の売買は 2行目の行として読める。
func Test基準26_Parseは値の中の改行を数えた行番号で誤りを出す(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		`AAPL,"Apple`,                      // 2行目（売買の行はここから始まる）
		`Inc.",USD,買,150.25,10,2026-01-10`, // 3行目
		"6758,ソニーグループ,JPY,買,3200,0,2026-01-10", // 4行目
	)
	assertProblems(t, content, []tradecsv.Problem{at(4, msgQtyFormat)})
}

// 5.5・行番号: 値の中に改行を含む売買の行の誤りは、その売買の行が始まる行（2行目）で出す。
func Test基準26_Parseは複数行にまたがる売買の誤りを始まる行で出す(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		`AAPL,"Apple`,                     // 2行目（売買の行はここから始まる）
		`Inc.",USD,買,150.25,0,2026-01-10`, // 3行目: 株数 0
	)
	assertProblems(t, content, []tradecsv.Problem{at(2, msgQtyFormat)})
}

// 5.5: 正しい行の Line は、複数行にまたがる行の後ろでも、ファイルの行番号のまま。
func Test補助_Parseは複数行にまたがる行の後の行番号をファイルの行番号にする(t *testing.T) {
	t.Parallel()
	content := csvOf(
		header,
		`AAPL,"Apple`,
		`Inc.",USD,買,150.25,10,2026-01-10`,
		"7203,トヨタ自動車,JPY,買,2500,100,2026-01-10",
	)
	rows, err := tradecsv.Parse(content, today)
	if err != nil {
		t.Fatalf("Parse: got err %v (%+v), want nil", err, parseProblemsOrNil(err))
	}
	var lines []int
	for _, r := range rows {
		lines = append(lines, r.Line)
	}
	if !slices.Equal(lines, []int{2, 4}) {
		t.Errorf("Parse の行の Line: got %v, want [2 4]", lines)
	}
}
