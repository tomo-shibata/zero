package pricecsv_test

import (
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/interface/pricecsv"
)

// 仕様: 要件（docs/requirements/price-import.md）の要件4（BOM・CRLF・ダブルクォート・空の行）・要件5（見出し）・要件7（銘柄コードと終値の書き方）・
// 要件8（同じ銘柄コード）と「CSV の誤りの文言」の表（ファイル・各行・行どうしの3段階。行番号はファイルの行番号）・基準7・基準8・基準9（書き方）と、
// プラン（docs/plans/price-import/plan.md）4.5 の pricecsv.Parse・Parsed・ParseError・Problem、5.1 の基準9 の行、5.3 の「行番号は空の行も数える」「Parse（単体）」。
// Parse は DB を見ない。銘柄マスタにあるかと、通貨ごとの桁（円は小数第1位、ドルは小数第2位）は確かめない（command.ImportClosingPrices.Classify の役目）。

const (
	header = "銘柄コード,終値"

	// 要件定義の「CSV の誤りの文言」の表。
	msgCode      = "銘柄コードは半角の英大文字・数字・「.」・「-」で1〜10文字にしてください"
	msgPrice     = "終値は半角数字と小数点で書いてください"
	msgPriceZero = "終値は0より大きくしてください"
	msgPriceMax  = "終値は1億未満にしてください"
	msgColumns   = "列の数を2にしてください"

	msgEncoding = "CSV の文字コードが UTF-8 ではありません"
	msgHeader   = "1行目の見出しを「銘柄コード,終値」にしてください"
	msgNoRows   = "登録する行がありません"
	msgQuote    = "ダブルクォートの使い方が正しくありません"

	// Shift_JIS で書いた見出し「銘柄コード,終値」。実際の Shift_JIS のバイト列。
	shiftJISHeader = "\x96\xc1\x95\xbf\x83\x52\x81\x5b\x83\x68\x2c\x8f\x49\x92\x6c\x0d\x0a"
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
func at(n int, msg string) pricecsv.Problem {
	return pricecsv.Problem{Line: n, Message: fmt.Sprintf("%d行目: %s", n, msg)}
}

// fileProblems は、Parse の err がファイルの段階の誤り（ParseError。ポインタでも値でも受ける）であることを確かめて、その Problems を返す。
func fileProblems(t *testing.T, err error) []pricecsv.Problem {
	t.Helper()
	if err == nil {
		t.Fatal("Parse: got err nil, want ParseError")
	}
	var pe *pricecsv.ParseError
	if errors.As(err, &pe) {
		return pe.Problems
	}
	var anyErr any = err
	if pv, ok := anyErr.(pricecsv.ParseError); ok {
		return pv.Problems
	}
	t.Fatalf("Parse: got err %v (%T), want ParseError", err, err)
	return nil
}

// messagesOf は、problems の Message を並び順のまま返す。
func messagesOf(problems []pricecsv.Problem) []string {
	msgs := make([]string, 0, len(problems))
	for _, p := range problems {
		msgs = append(msgs, p.Message)
	}
	return msgs
}

// assertFileProblem は、Parse がファイルの段階の誤りを、want の1件だけ（文言）で返すことを確かめる。
func assertFileProblem(t *testing.T, content []byte, want string) {
	t.Helper()
	parsed, err := pricecsv.Parse(content)
	if err == nil {
		t.Fatalf("Parse: got err nil（行 %+v）, want 誤り %q", parsed.Rows, want)
	}
	if got := messagesOf(fileProblems(t, err)); !slices.Equal(got, []string{want}) {
		t.Errorf("Parse の誤りの文言（ファイルの段階は最初の1つだけ）:\n got %q\nwant %q", got, []string{want})
	}
}

// mustParse は、Parse が成功する（ファイルの段階の誤りがない）ことを確かめて、結果を返す。
func mustParse(t *testing.T, content []byte) pricecsv.Parsed {
	t.Helper()
	parsed, err := pricecsv.Parse(content)
	if err != nil {
		t.Fatalf("Parse: got err %v, want nil", err)
	}
	return parsed
}

// wantRow は、Parse の Rows の1行の期待値。
type wantRow struct {
	line  int
	code  string
	price *big.Rat
}

// assertRows は、Parse の Rows が want とちょうど同じ（Line・StockCode・Price の値。順番も）であることを確かめる。
// Price は *big.Rat の値で比べる（float で読む誤りを見つけるため）。
func assertRows(t *testing.T, got []command.PriceImportRow, want []wantRow) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Parse の Rows の件数: got %d (%+v), want %d (%+v)", len(got), got, len(want), want)
	}
	for i, w := range want {
		g := got[i]
		if g.Line != w.line || g.StockCode != w.code {
			t.Errorf("Rows[%d]: got Line=%d StockCode=%q, want Line=%d StockCode=%q", i, g.Line, g.StockCode, w.line, w.code)
		}
		if g.Price == nil || g.Price.Cmp(w.price) != 0 {
			t.Errorf("Rows[%d] の Price（%s）: got %v, want %v", i, w.code, g.Price, w.price)
		}
	}
}

// 基準9（要件7）: 誤りにならない銘柄コードと終値は Rows に入り、Line・StockCode・Price が正確な値になる。
// 終値の通貨ごとの桁（円は小数第1位、ドルは小数第2位）は Parse では確かめない（Classify の役目）ので、どちらの桁も Rows に入る。
func Test基準9_Parseは正しい銘柄コードと終値をRowsに入れる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		code  string
		price string
		want  *big.Rat
	}{
		{"銘柄コード_4桁の数字", "7203", "2850", big.NewRat(2850, 1)},
		{"銘柄コード_数字と英字", "130A", "2850", big.NewRat(2850, 1)},
		{"銘柄コード_ピリオド", "BRK.B", "2850", big.NewRat(2850, 1)},
		{"銘柄コード_ハイフン", "BRK-B", "2850", big.NewRat(2850, 1)},
		{"銘柄コード_10文字", "ABCDEFGHIJ", "2850", big.NewRat(2850, 1)},
		{"終値_円の最小0.1", "7203", "0.1", big.NewRat(1, 10)},
		{"終値_円の最大99999999.9", "7203", "99999999.9", big.NewRat(999999999, 10)},
		{"終値_整数", "7203", "2850", big.NewRat(2850, 1)},
		{"終値_ドルの最小0.01", "AAPL", "0.01", big.NewRat(1, 100)},
		{"終値_ドルの最大99999999.99", "AAPL", "99999999.99", big.NewRat(9999999999, 100)},
		{"終値_ドル182.3", "AAPL", "182.3", big.NewRat(1823, 10)},
		{"終値_ドル182.35", "AAPL", "182.35", big.NewRat(18235, 100)},
		{"終値_桁は確かめない_小数第2位", "7203", "2850.25", big.NewRat(285025, 100)},
		{"終値_桁は確かめない_小数第3位", "AAPL", "182.355", big.NewRat(182355, 1000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed := mustParse(t, withHeader(tc.code+","+tc.price))

			assertRows(t, parsed.Rows, []wantRow{{line: 2, code: tc.code, price: tc.want}})
			if len(parsed.Problems) != 0 || len(parsed.Duplicates) != 0 {
				t.Errorf("誤りにならない値: got Problems=%+v Duplicates=%+v, want なし", parsed.Problems, parsed.Duplicates)
			}
		})
	}
}

// 基準9（要件7）: 書き方に合わない銘柄コード・終値は、表の文言の誤りになり、Rows には入らない。誤りは1件だけで、重複の材料も出さない。
// 空・-1・"1,500"（ダブルクォートで囲んだ3桁区切り）・abc・1.・.5 は「半角数字と小数点で書いて」、0 は「0より大きく」、100000000 は「1億未満に」。
// 1e3・+5・全角数字も半角数字と小数点だけではないので書き方の誤り（プラン 4.2）。
func Test基準9_Parseは書き方に合わない銘柄コードと終値を誤りにする(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		line string
		want string
	}{
		{"銘柄コード_空", ",2850", msgCode},
		{"銘柄コード_小文字", "aapl,2850", msgCode},
		{"銘柄コード_11文字", "ABCDEFGHIJK,2850", msgCode},
		{"銘柄コード_空白を含む", "72 03,2850", msgCode},
		{"銘柄コード_前に空白", " 7203,2850", msgCode},
		{"終値_空", "7203,", msgPrice},
		{"終値_マイナス", "7203,-1", msgPrice},
		{"終値_3桁区切り", `7203,"1,500"`, msgPrice},
		{"終値_英字", "7203,abc", msgPrice},
		{"終値_小数点の後に数字がない", "7203,1.", msgPrice},
		{"終値_小数点の前に数字がない", "7203,.5", msgPrice},
		{"終値_指数", "7203,1e3", msgPrice},
		{"終値_プラス", "7203,+5", msgPrice},
		{"終値_全角数字", "7203,１５００", msgPrice},
		{"終値_小数点が2つ", "7203,1.2.3", msgPrice},
		{"終値_0", "7203,0", msgPriceZero},
		{"終値_0.0", "7203,0.0", msgPriceZero},
		{"終値_1億", "7203,100000000", msgPriceMax},
		{"終値_1億ちょうどの小数", "7203,100000000.0", msgPriceMax},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed := mustParse(t, withHeader(tc.line))

			if want := []pricecsv.Problem{at(2, tc.want)}; !slices.Equal(parsed.Problems, want) {
				t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
			}
			if len(parsed.Rows) != 0 {
				t.Errorf("書き方に誤りのある行は Rows に入れない: got %+v", parsed.Rows)
			}
		})
	}
}

// 基準9: 境界の1つ内側は誤りにならない（0より大きい最小・1億未満の最大）。
func Test基準9_Parseは0より大きい最小と1億未満の最大を受け付ける(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("7203,0.01", "6758,99999999.99"))

	assertRows(t, parsed.Rows, []wantRow{
		{line: 2, code: "7203", price: big.NewRat(1, 100)},
		{line: 3, code: "6758", price: big.NewRat(9999999999, 100)},
	})
	if len(parsed.Problems) != 0 {
		t.Errorf("Problems: got %+v, want なし", parsed.Problems)
	}
}

// 基準8（要件4）: BOM 付き・CRLF・空の行を含み、ダブルクォートで囲んだ CSV を読める。Line はファイルの行番号（空の行も数える）。
func Test基準8_ParseはBOM付きCRLFで空の行とダブルクォートを含むCSVを読める(t *testing.T) {
	t.Parallel()
	content := []byte("\xEF\xBB\xBF" + header + "\r\n\"7203\",\"2850\"\r\n\r\n\"6758\",\"3600\"\r\n")

	parsed := mustParse(t, content)

	assertRows(t, parsed.Rows, []wantRow{
		{line: 2, code: "7203", price: big.NewRat(2850, 1)},
		{line: 4, code: "6758", price: big.NewRat(3600, 1)},
	})
	if len(parsed.Problems) != 0 || len(parsed.Duplicates) != 0 {
		t.Errorf("got Problems=%+v Duplicates=%+v, want なし", parsed.Problems, parsed.Duplicates)
	}
}

// 基準8: 最後の行に改行がなくても読める。
func Test基準8_Parseは最後の行に改行がなくても読める(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, []byte(header+"\n7203,2850"))

	assertRows(t, parsed.Rows, []wantRow{{line: 2, code: "7203", price: big.NewRat(2850, 1)}})
}

// 基準7（c）（要件3・要件4）: UTF-8 として読めない（Shift_JIS）CSV は、文字コードの誤りだけ。
func Test基準7c_ParseはShift_JISのCSVを文字コードの誤りにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, []byte(shiftJISHeader+"7203,2850\r\n"), msgEncoding)
}

// 基準7（d）（要件5）: 見出しが「終値,銘柄コード」（順番違い）なら、見出しの誤りだけ。
func Test基準7d_Parseは見出しの順番違いを見出しの誤りにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, csvOf("終値,銘柄コード", "2850,7203"), msgHeader)
}

// 5.3: 見出しの前後に空白があると見出しの誤り（完全一致）。
func Test補助_Parseは見出しの前後の空白を見出しの誤りにする(t *testing.T) {
	t.Parallel()
	for name, h := range map[string]string{
		"前に空白":   " " + header,
		"後ろに空白":  header + " ",
		"カンマの後ろ": "銘柄コード, 終値",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertFileProblem(t, csvOf(h, "7203,2850"), msgHeader)
		})
	}
}

// 基準7（e）（要件3）: 見出しだけの CSV は「登録する行がありません」。
func Test基準7e_Parseは見出しだけのCSVを登録する行がない誤りにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, csvOf(header), msgNoRows)
}

// 基準7（e'）: 見出しと CRLF の空の行だけでも「登録する行がありません」。
func Test基準7e2_Parseは見出しと空の行だけのCSVを登録する行がない誤りにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, []byte(header+"\r\n\r\n\r\n"), msgNoRows)
}

// 基準7（f）（要件3・要件4）: 2行目の銘柄コードが `"7203` で閉じていない CSV は、「2行目: ダブルクォートの使い方が正しくありません」だけ。
func Test基準7f_Parseは閉じていないダブルクォートを誤りにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, withHeader(`"7203,2850`), "2行目: "+msgQuote)
}

// 5.3「ファイルの段階は最初の1つだけ」: 見出しが違い、行もない → 見出しの誤りだけ（「登録する行がありません」は出さない）。
func Test補助_Parseは見出しが違い行もないCSVを見出しの誤りだけにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, csvOf("終値,銘柄コード"), msgHeader)
}

// 5.3: 見出しの行にダブルクォートの誤り → 「1行目: ダブルクォートの使い方が正しくありません」だけ。
func Test補助_Parseは見出しの行のダブルクォートの誤りを1行目の誤りにする(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, csvOf(`"銘柄コード,終値`, "7203,2850"), "1行目: "+msgQuote)
}

// 5.3: ダブルクォートの誤りがある CSV では、各行の誤りを出さない（ファイルの段階で止める）。
func Test補助_Parseはダブルクォートの誤りがあるとき各行の誤りを出さない(t *testing.T) {
	t.Parallel()
	assertFileProblem(t, withHeader("7203,abc", `"6758,3600`, "AAPL,0"), "3行目: "+msgQuote)
}

// 5.3: 3列の行は列の数の誤りだけ（その行の他の列は確かめない）。1列の行も同じ。
func Test補助_Parseは列の数が2でない行を列の数の誤りだけにする(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("aapl,abc,1", "7203", "6758,3600"))

	want := []pricecsv.Problem{at(2, msgColumns), at(3, msgColumns)}
	if !slices.Equal(parsed.Problems, want) {
		t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
	}
	assertRows(t, parsed.Rows, []wantRow{{line: 4, code: "6758", price: big.NewRat(3600, 1)}})
}

// 5.3: 1つの行に両方の誤り（「aapl,abc」）→ 銘柄コードの文言、終値の文言の順。
func Test補助_Parseは1つの行の銘柄コードと終値の誤りをこの順に出す(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("aapl,abc"))

	want := []pricecsv.Problem{at(2, msgCode), at(2, msgPrice)}
	if !slices.Equal(parsed.Problems, want) {
		t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
	}
}

// 5.3: 各行の誤りは行番号の昇順。
func Test補助_Parseは各行の誤りを行番号の昇順に並べる(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("7203,abc", "6758,3600", "AAPL,0", "9984,100000000"))

	want := []pricecsv.Problem{at(2, msgPrice), at(4, msgPriceZero), at(5, msgPriceMax)}
	if !slices.Equal(parsed.Problems, want) {
		t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
	}
	assertRows(t, parsed.Rows, []wantRow{{line: 3, code: "6758", price: big.NewRat(3600, 1)}})
}

// 5.3「行番号は空の行も数える」（Parse）: 見出し、空の行、「7203,abc」→「3行目: 終値は半角数字と小数点で書いてください」。
func Test補助_Parseの行番号は空の行も数える_書き方の誤り(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, csvOf(header, "", "7203,abc"))

	if want := []pricecsv.Problem{at(3, msgPrice)}; !slices.Equal(parsed.Problems, want) {
		t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
	}
}

// 5.3「行番号は空の行も数える」（Parse）: 見出し、「7203,2850」、空の行、「7203,2900」→ Duplicates が「4行目: 銘柄コード 7203 は 2行目にもあります」。
func Test補助_Parseの行番号は空の行も数える_同じ銘柄コード(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, csvOf(header, "7203,2850", "", "7203,2900"))

	if want := []pricecsv.Problem{at(4, "銘柄コード 7203 は 2行目にもあります")}; !slices.Equal(parsed.Duplicates, want) {
		t.Errorf("Duplicates:\n got %+v\nwant %+v", parsed.Duplicates, want)
	}
	if len(parsed.Problems) != 0 {
		t.Errorf("Problems: got %+v, want なし", parsed.Problems)
	}
}

// 5.3: 同じ銘柄コードが3回 → Duplicates は2回目・3回目の行に、どちらも最初の行（2行目）を指す（直前の行と比べる誤りを見つけるため）。
func Test補助_Parseは同じ銘柄コードが3回あると最初の行を指す(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("7203,2850", "7203,2900", "7203,2950"))

	want := []pricecsv.Problem{
		at(3, "銘柄コード 7203 は 2行目にもあります"),
		at(4, "銘柄コード 7203 は 2行目にもあります"),
	}
	if !slices.Equal(parsed.Duplicates, want) {
		t.Errorf("Duplicates:\n got %+v\nwant %+v", parsed.Duplicates, want)
	}
}

// 基準11（要件8）: 2行目と4行目が同じ銘柄コード → Duplicates が「4行目: 銘柄コード 7203 は 2行目にもあります」だけ。間の行は別の銘柄。
func Test基準11_Parseは同じ銘柄コードの2回目の行を誤りにする(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("7203,2850", "6758,3600", "7203,2900"))

	if want := []pricecsv.Problem{at(4, "銘柄コード 7203 は 2行目にもあります")}; !slices.Equal(parsed.Duplicates, want) {
		t.Errorf("Duplicates:\n got %+v\nwant %+v", parsed.Duplicates, want)
	}
	if len(parsed.Problems) != 0 {
		t.Errorf("Problems: got %+v, want なし", parsed.Problems)
	}
}

// 5.3（要件8）: 銘柄コードや終値の書き方に誤りのある行も、列の数が2なら銘柄コードの文字列で比べる。
// 書き方の誤りは Problems に、同じ銘柄コードは Duplicates に、それぞれ出る（Parse は両方返す。どちらを使うかは Run が決める）。
func Test補助_Parseは書き方の誤りのある行も同じ銘柄コードの比べに含める(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("7203,abc", "7203,2850", "6758,3600"))

	if want := []pricecsv.Problem{at(2, msgPrice)}; !slices.Equal(parsed.Problems, want) {
		t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
	}
	if want := []pricecsv.Problem{at(3, "銘柄コード 7203 は 2行目にもあります")}; !slices.Equal(parsed.Duplicates, want) {
		t.Errorf("Duplicates:\n got %+v\nwant %+v", parsed.Duplicates, want)
	}
}

// 5.3（要件8）: 列の数が2でない行は、同じ銘柄コードの比べに含めない。
func Test補助_Parseは列の数が2でない行を同じ銘柄コードの比べに含めない(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("7203,2850,1", "7203,2900"))

	if len(parsed.Duplicates) != 0 {
		t.Errorf("Duplicates: got %+v, want なし（列の数が2でない行は比べない）", parsed.Duplicates)
	}
	if want := []pricecsv.Problem{at(2, msgColumns)}; !slices.Equal(parsed.Problems, want) {
		t.Errorf("Problems:\n got %+v\nwant %+v", parsed.Problems, want)
	}
}

// 5.3（要件8）: 同じ銘柄コードの比べは文字列どおり（大文字小文字などの違う文字列は別の銘柄コード）。
// 「7203」と「7203」だけが重なり、「130A」と「130a」は重ならない（後者は書き方の誤りで Problems に出る）。
func Test補助_Parseは銘柄コードの文字列が違えば同じ銘柄コードとしない(t *testing.T) {
	t.Parallel()
	parsed := mustParse(t, withHeader("130A,100", "130a,100"))

	if len(parsed.Duplicates) != 0 {
		t.Errorf("Duplicates: got %+v, want なし", parsed.Duplicates)
	}
}
