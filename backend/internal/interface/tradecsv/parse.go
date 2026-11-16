package tradecsv

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"unicode/utf8"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
)

// utf8BOM は、UTF-8 の BOM（要件22: あってもなくてもよい）。
const utf8BOM = "\xef\xbb\xbf"

// record は、CSV の売買の行（レコード）1つ。Line はそのレコードが始まるファイルの行番号（値の中の改行も行に数える）。
type record struct {
	Line   int
	Fields []string
}

// Parse は、CSV の中身 content を確かめて ImportRow にする（要件22〜24）。today は "YYYY-MM-DD"（日本時間）。
// 確かめるのは、段階1（ファイル全体）と段階2（各行の書き方）まで。行どうしの関係（段階3）は command.ImportTrades が確かめる。
// 誤りがあれば *ParseError を返す。誤り以外の失敗（CSV の読み取りの想定外のエラーなど）は、ParseError でないエラーで返す。
func Parse(content []byte, today string) ([]command.ImportRow, error) {
	if !utf8.Valid(content) {
		return nil, &ParseError{Problems: []Problem{fileProblem(msgEncoding)}}
	}
	records, err := readRecords(bytes.TrimPrefix(content, []byte(utf8BOM)))
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

// readRecords は、content（BOM を除いたもの）を csv として読み、見出しを含むすべてのレコードを返す。
// 空の行は csv.Reader が読み飛ばす（行番号には数える。要件22）。
// ダブルクォートの使い方の誤り（csv.ErrQuote・csv.ErrBareQuote）は、最初の1か所だけを、そのレコードが始まる行の誤りとして返す（質問9）。
// 列の数は確かめない（FieldsPerRecord = -1）。各行の段階で確かめる。
func readRecords(content []byte) ([]record, error) {
	r := csv.NewReader(bytes.NewReader(content))
	r.FieldsPerRecord = -1

	var records []record
	for {
		fields, err := r.Read()
		if errors.Is(err, io.EOF) {
			return records, nil
		}
		if err != nil {
			var parseErr *csv.ParseError
			if errors.As(err, &parseErr) && (errors.Is(parseErr.Err, csv.ErrQuote) || errors.Is(parseErr.Err, csv.ErrBareQuote)) {
				return nil, &ParseError{Problems: []Problem{lineProblem(parseErr.StartLine, msgQuote)}}
			}
			return nil, fmt.Errorf("CSV を読めません: %w", err)
		}
		line, _ := r.FieldPos(0)
		records = append(records, record{Line: line, Fields: fields})
	}
}
