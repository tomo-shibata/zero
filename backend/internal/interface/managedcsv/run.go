package managedcsv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// 終了コード（要件31）。
const (
	exitSuccess = 0
	exitFailure = 1
)

// RunInput は、Run の入力。
type RunInput struct {
	CSVPath  string // 空なら「CSV_FILE と USER_ID を指定してください」
	UserID   string // 空なら同上。UUID として読めない値は「利用者 <ID> が見つかりません」
	Today    string // "YYYY-MM-DD"（日本時間。csvinput.TodayInJapan で作る）。これより後の日付は誤り
	Importer *command.ImportManagedAccounts
}

// Run は、取り込みの全体（プラン 4.5）を行い、終了コード（成功0、誤り1）を返す。
// 成功の表示は stdout、誤りの表示（先頭の行と誤り）は stderr に1行ずつ書く。誤りのときは、何も変えていない。
//
// 誤りは、要件定義の表の順に確かめ、前の段階に誤りがあれば次の段階は確かめない:
// ファイル（引数 → ファイルを開けない → 利用者がいない → Parse のファイルの段階）→ 各行（Parse の Problems）→ 行どうし（Parse の Duplicates）。
// 文言の表にない失敗（DB の障害など）は、先頭の行の後に理由を1行出して、終了コード1にする。
func Run(ctx context.Context, in RunInput, stdout, stderr io.Writer) int {
	if in.CSVPath == "" || in.UserID == "" {
		return FailMissingArgs(stderr)
	}
	content, err := os.ReadFile(in.CSVPath)
	if err != nil {
		return fail(stderr, fmt.Sprintf("ファイル %s を開けません", in.CSVPath))
	}

	userID, err := uuid.Parse(in.UserID)
	if err != nil {
		return fail(stderr, userNotFoundMessage(in.UserID))
	}
	if err := in.Importer.CheckUser(ctx, userID); err != nil {
		if errors.Is(err, command.ErrUserNotFound) {
			return fail(stderr, userNotFoundMessage(in.UserID))
		}
		return fail(stderr, reasonOf(err))
	}

	parsed, err := Parse(content, in.Today)
	if err != nil {
		return failParse(stderr, err)
	}
	if len(parsed.Problems) > 0 {
		return fail(stderr, messagesOf(parsed.Problems)...)
	}
	if len(parsed.Duplicates) > 0 {
		return fail(stderr, messagesOf(parsed.Duplicates)...)
	}

	valuations := make([]model.ManagedValuation, 0, len(parsed.Rows))
	for _, row := range parsed.Rows {
		valuations = append(valuations, row.Valuation)
	}
	imported, err := in.Importer.Execute(ctx, userID, valuations)
	if err != nil {
		return fail(stderr, reasonOf(err))
	}
	writeLines(stdout, []string{fmt.Sprintf("%d件の記録を取り込みました。", imported)})
	return exitSuccess
}

// FailMissingArgs は、CSV_FILE か USER_ID が空のときの誤り（先頭の行と「CSV_FILE と USER_ID を指定してください」）を stderr に書き、終了コード1を返す。
// Run が使う。main も、DB につなぐ前に同じ形で止めるために使う。
func FailMissingArgs(stderr io.Writer) int {
	return fail(stderr, msgMissingArgs)
}

// userNotFoundMessage は、利用者 id が見つからないときの文言（利用者 <ID> が見つかりません）。
func userNotFoundMessage(id string) string {
	return fmt.Sprintf("利用者 %s が見つかりません", id)
}

// failParse は、Parse のエラー err を出力して終了コードを返す。*ParseError なら誤りを1行ずつ、それ以外は理由を1行出す。
func failParse(stderr io.Writer, err error) int {
	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		return fail(stderr, messagesOf(parseErr.Problems)...)
	}
	return fail(stderr, reasonOf(err))
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

// fail は、「取り込めませんでした。何も変えていません。」と messages を stderr に1行ずつ書き、終了コード1を返す。
func fail(stderr io.Writer, messages ...string) int {
	writeLines(stderr, append([]string{FailedHeadline}, messages...))
	return exitFailure
}

// writeLines は、lines を1行ずつ w に書く。書き込みの失敗（端末が閉じているなど）は、取り込みの結果に関係しないので見ない。
func writeLines(w io.Writer, lines []string) {
	_, _ = io.WriteString(w, strings.Join(lines, "\n")+"\n")
}
