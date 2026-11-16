package command

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
)

// ErrUserNotFound は、取り込む先の利用者が DB にいないこと。
var ErrUserNotFound = errors.New("user not found")

// ImportRow は、CSV の1行を読んだ結果。
type ImportRow struct {
	Line  int // CSV のファイルの行番号（見出しが1行目）
	Stock model.Stock
	Trade model.Trade
}

// ImportProblemKind は、行どうしの誤り（段階3）の種類。値の小さい順が、同じ行で誤りを並べる順（要件定義の表の順）。
type ImportProblemKind int

const (
	ProblemStockMismatchInCSV     ImportProblemKind = iota + 1 // 同じ銘柄コードの、銘柄名か通貨が前の行と違う（OtherLine はその銘柄コードが最初に出た行）
	ProblemCurrencyMismatchMaster                              // 銘柄マスタと通貨が違う（Currency は銘柄マスタの通貨）
	ProblemOversell                                            // 多すぎる売り（Held・Sold）
)

// ImportProblem は、行どうしの誤り1件。Kind ごとに意味のある項目だけを入れる。
type ImportProblem struct {
	Line       int
	Kind       ImportProblemKind
	StockCode  string
	OtherLine  int
	Currency   model.Currency
	Held, Sold int64
}

// ImportValidationError は、段階3（行どうし）の誤り。Problems は行番号の昇順で、同じ行では Kind の昇順。
type ImportValidationError struct{ Problems []ImportProblem }

// Error は、誤りの件数を返す。利用者に見せる文言は、interface 層が Problems から作る。
func (e *ImportValidationError) Error() string {
	return fmt.Sprintf("CSV の行どうしの誤りが %d 件あります", len(e.Problems))
}

// ImportResult は、取り込みの結果。
type ImportResult struct {
	Imported      int
	NewStocks     []string // 新しく登録した銘柄コード（昇順）
	RenamedStocks []string // 銘柄名を変えた銘柄コード（昇順）
}

// ImportTrades は、売買履歴の CSV を、利用者の売買履歴として取り込むユースケース（要件19〜25）。
type ImportTrades struct {
	repo repository.TradeImportRepository
}

// NewImportTrades は、repo で取り込む ImportTrades を返す。
func NewImportTrades(repo repository.TradeImportRepository) *ImportTrades {
	return &ImportTrades{repo: repo}
}

// CheckUser は、利用者 userID が DB にいるかを確かめる。いなければ ErrUserNotFound を返す。
func (u *ImportTrades) CheckUser(ctx context.Context, userID uuid.UUID) error {
	exists, err := u.repo.UserExists(ctx, userID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrUserNotFound
	}
	return nil
}

// Execute は、段階3（行どうしの関係）を確かめてから、利用者 userID の売買履歴を rows で入れ替える（要件20・要件21・要件25）。
// 段階3の誤りがあれば *ImportValidationError を返し、何も変えない。rows は CSV の行の順（登録する順）。
// rows が空のときはエラーにする（利用者の売買履歴を全部消すだけの取り込みにしないため。要件21・質問7）。
func (u *ImportTrades) Execute(ctx context.Context, userID uuid.UUID, rows []ImportRow) (ImportResult, error) {
	if len(rows) == 0 {
		return ImportResult{}, errors.New("取り込む行がありません")
	}
	first := firstRowByCode(rows)
	master, err := u.repo.FindStocks(ctx, slices.Collect(maps.Keys(first)))
	if err != nil {
		return ImportResult{}, err
	}

	if problems := findProblems(rows, first, master); len(problems) > 0 {
		return ImportResult{}, &ImportValidationError{Problems: problems}
	}

	newStocks, renamed := classifyStocks(first, master)
	trades := make([]model.Trade, len(rows))
	for i, row := range rows {
		trades[i] = row.Trade
	}
	if err := u.repo.ReplaceTrades(ctx, userID, newStocks, renamed, trades); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{Imported: len(rows), NewStocks: codesOf(newStocks), RenamedStocks: codesOf(renamed)}, nil
}

// firstRowByCode は、銘柄コードごとに、その銘柄コードが最初に出た行を返す。
func firstRowByCode(rows []ImportRow) map[string]ImportRow {
	first := make(map[string]ImportRow)
	for _, row := range rows {
		if _, ok := first[row.Stock.Code]; !ok {
			first[row.Stock.Code] = row
		}
	}
	return first
}

// findProblems は、行どうしの誤り（CSV の中の銘柄名・通貨の食い違い、銘柄マスタとの通貨の食い違い、多すぎる売り）を、
// 行番号の昇順・同じ行では Kind の昇順で返す。
func findProblems(rows []ImportRow, first map[string]ImportRow, master map[string]model.Stock) []ImportProblem {
	var problems []ImportProblem
	for _, row := range rows {
		code := row.Stock.Code
		firstRow := first[code]
		if row.Line == firstRow.Line {
			// その銘柄コードの最初の行だけ、銘柄マスタの通貨と比べる。
			if m, ok := master[code]; ok && m.Currency != row.Stock.Currency {
				problems = append(problems, ImportProblem{Line: row.Line, Kind: ProblemCurrencyMismatchMaster, StockCode: code, Currency: m.Currency})
			}
			continue
		}
		if row.Stock != firstRow.Stock {
			problems = append(problems, ImportProblem{Line: row.Line, Kind: ProblemStockMismatchInCSV, StockCode: code, OtherLine: firstRow.Line})
		}
	}

	trades := make([]model.Trade, len(rows))
	for i, row := range rows {
		trades[i] = row.Trade
	}
	for _, o := range model.FindOversells(trades) {
		problems = append(problems, ImportProblem{Line: rows[o.Index].Line, Kind: ProblemOversell, Held: o.Held, Sold: o.Sold})
	}

	slices.SortStableFunc(problems, func(a, b ImportProblem) int {
		return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Kind, b.Kind))
	})
	return problems
}

// classifyStocks は、銘柄マスタにない銘柄（新しく登録する）と、銘柄マスタにあって銘柄名が CSV と違う銘柄（銘柄名を変える。通貨は銘柄マスタのまま）を、
// 銘柄コードの昇順で返す（要件20）。
func classifyStocks(first map[string]ImportRow, master map[string]model.Stock) (newStocks, renamed []model.Stock) {
	for _, code := range slices.Sorted(maps.Keys(first)) {
		csvStock := first[code].Stock
		m, ok := master[code]
		switch {
		case !ok:
			newStocks = append(newStocks, csvStock)
		case m.Name != csvStock.Name:
			renamed = append(renamed, model.Stock{Code: code, Name: csvStock.Name, Currency: m.Currency})
		}
	}
	return newStocks, renamed
}

// codesOf は、stocks の銘柄コードを、並びのまま返す。
func codesOf(stocks []model.Stock) []string {
	codes := make([]string, 0, len(stocks))
	for _, s := range stocks {
		codes = append(codes, s.Code)
	}
	return codes
}
