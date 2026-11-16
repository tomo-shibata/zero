package tradecsv

import (
	"errors"
	"fmt"
	"slices"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/interface/csvinput"
)

// Parse は、CSV の中身 content を確かめて ImportRow にする（要件22〜24）。today は "YYYY-MM-DD"（日本時間）。
// 確かめるのは、段階1（ファイル全体）と段階2（各行の書き方）まで。行どうしの関係（段階3）は command.ImportTrades が確かめる。
// 誤りがあれば *ParseError を返す。誤り以外の失敗（CSV の読み取りの想定外のエラーなど）は、ParseError でないエラーで返す。
func Parse(content []byte, today string) ([]command.ImportRow, error) {
	records, err := readRecords(content)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 || !slices.Equal(records[0].Fields, headerColumns) {
		return nil, &ParseError{Problems: []Problem{fileProblem(msgHeader)}}
	}
	if len(records) == 1 {
		return nil, &ParseError{Problems: []Problem{fileProblem(msgNoRows)}}
	}

	var (
		rows     []command.ImportRow
		problems []Problem
	)
	for _, rec := range records[1:] {
		row, rowProblems, err := parseRow(rec, today)
		if err != nil {
			return nil, err
		}
		problems = append(problems, rowProblems...)
		rows = append(rows, row)
	}
	if len(problems) > 0 {
		return nil, &ParseError{Problems: problems}
	}
	return rows, nil
}

// readRecords は、content を csvinput.ReadRecords で読み、その誤りを tradecsv の誤りにする。
// UTF-8 でないものは文字コードの誤り（要件22）、ダブルクォートの誤りは、最初の1か所だけを、そのレコードが始まる行の誤り（質問9）にする。
// それ以外の読み取りの失敗は、ParseError でないエラーで返す。
func readRecords(content []byte) ([]csvinput.Record, error) {
	records, err := csvinput.ReadRecords(content)
	if err == nil {
		return records, nil
	}
	var quoteErr *csvinput.QuoteError
	switch {
	case errors.Is(err, csvinput.ErrNotUTF8):
		return nil, &ParseError{Problems: []Problem{fileProblem(msgEncoding)}}
	case errors.As(err, &quoteErr):
		return nil, &ParseError{Problems: []Problem{lineProblem(quoteErr.Line, msgQuote)}}
	default:
		return nil, fmt.Errorf("CSV を読めません: %w", err)
	}
}
