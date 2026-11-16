package pricecsv

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 終了コード（要件3）。
const (
	exitSuccess = 0
	exitFailure = 1
)

// RunInput は、Run の入力。
type RunInput struct {
	CSVPath  string // 空なら「CSV_FILE を指定してください」
	Today    string // "YYYY-MM-DD"（日本時間。csvinput.TodayInJapan で作る）。登録する終値の取引日
	Importer *command.ImportClosingPrices
}

// Run は、登録の全体（プラン 4.1 の 1〜6）を行い、終了コード（成功0、誤り1）を返す。
// 成功の表示（「n件の価格を登録しました（取引日 …）。」と飛ばした行）は stdout、誤りの表示（先頭の行と誤り）は stderr に1行ずつ書く。誤りのときは、何も変えていない。
//
// ファイルの段階の誤りは、要件定義の表の順に確かめ、最初に当たった1つだけを出す: 引数 → ファイルを開けない → 文字コード・ダブルクォート・見出し・登録する行がない（Parse）。
// その後、各行の誤り（Parse の書き方、Classify の桁を行の順に並べる）、行どうしの誤り（Parse の Duplicates）の順に、前の段階に誤りがあれば次の段階は確かめない。
// 文言の表にない失敗（DB の障害など）は、先頭の行の後に理由を1行出して、終了コード1にする。
func Run(ctx context.Context, in RunInput, stdout, stderr io.Writer) int {
	if in.CSVPath == "" {
		return FailMissingArgs(stderr)
	}
	content, err := os.ReadFile(in.CSVPath)
	if err != nil {
		return fail(stderr, fmt.Sprintf("ファイル %s を開けません", in.CSVPath))
	}

	parsed, err := Parse(content)
	if err != nil {
		return failParse(stderr, err)
	}

	plan, err := in.Importer.Classify(ctx, parsed.Rows, in.Today)
	if err != nil {
		return fail(stderr, reasonOf(err))
	}

	if rowProblems := rowStageProblems(parsed.Problems, plan.TooFine); len(rowProblems) > 0 {
		return fail(stderr, messagesOf(rowProblems)...)
	}
	if len(parsed.Duplicates) > 0 {
		return fail(stderr, messagesOf(parsed.Duplicates)...)
	}

	if err := in.Importer.Save(ctx, plan.Prices); err != nil {
		return fail(stderr, reasonOf(err))
	}
	writeSuccess(stdout, len(plan.Prices), in.Today, plan.Skipped)
	return exitSuccess
}

// FailMissingArgs は、CSV_FILE が空のときの誤り（先頭の行と「CSV_FILE を指定してください」）を stderr に書き、終了コード1を返す。
// Run が使う。main も、DB につなぐ前に同じ形で止めるために使う。
func FailMissingArgs(stderr io.Writer) int {
	return fail(stderr, msgMissingArgs)
}

// failParse は、Parse のエラー err を出力して終了コードを返す。*ParseError なら誤りを1行ずつ、それ以外は理由を1行出す。
func failParse(stderr io.Writer, err error) int {
	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		return fail(stderr, messagesOf(parseErr.Problems)...)
	}
	return fail(stderr, reasonOf(err))
}

// rowStageProblems は、各行の段階の誤り（Parse の書き方の誤りと、Classify の桁の誤り）を、行番号の昇順に並べて返す。
// 書き方の誤りのある行は Classify に渡していないので、同じ行に両方が出ることはない。
func rowStageProblems(written []Problem, tooFine []command.PriceTooFine) []Problem {
	problems := slices.Clone(written)
	for _, t := range tooFine {
		problems = append(problems, lineProblem(t.Line, tooFineMessage(t.Currency)))
	}
	slices.SortStableFunc(problems, func(a, b Problem) int { return cmp.Compare(a.Line, b.Line) })
	return problems
}

// tooFineMessage は、通貨 currency の精度に合わない終値の文言（「n行目: 」なし）。
func tooFineMessage(currency model.Currency) string {
	if currency == model.CurrencyJPY {
		return msgPriceYen
	}
	return msgPriceDollar
}

// messagesOf は、problems の Message を並び順のまま返す。
func messagesOf(problems []Problem) []string {
	messages := make([]string, 0, len(problems))
	for _, p := range problems {
		messages = append(messages, p.Message)
	}
	return messages
}

// reasonOf は、文言の表にない失敗 err の理由を、1行の文字列にする（改行は空白にする）。
func reasonOf(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}

// fail は、「登録できませんでした。何も変えていません。」と messages を stderr に1行ずつ書き、終了コード1を返す。
func fail(stderr io.Writer, messages ...string) int {
	writeLines(stderr, append([]string{FailedHeadline}, messages...))
	return exitFailure
}

// writeSuccess は、成功の表示（登録した件数と取引日、飛ばした行）を stdout に1行ずつ書く（要件3・要件6・要件9）。
func writeSuccess(stdout io.Writer, registered int, tradingDate string, skipped []command.PriceImportSkip) {
	lines := []string{fmt.Sprintf("%d件の価格を登録しました（取引日 %s）。", registered, tradingDate)}
	for _, s := range skipped {
		lines = append(lines, fmt.Sprintf("%d行目: 銘柄コード %s は銘柄マスタにないので飛ばしました", s.Line, s.StockCode))
	}
	writeLines(stdout, lines)
}

// writeLines は、lines を1行ずつ w に書く。書き込みの失敗（端末が閉じているなど）は、登録の結果に関係しないので見ない。
func writeLines(w io.Writer, lines []string) {
	_, _ = io.WriteString(w, strings.Join(lines, "\n")+"\n")
}
