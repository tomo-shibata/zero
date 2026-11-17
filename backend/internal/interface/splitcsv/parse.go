package splitcsv

import (
	"errors"
	"fmt"
	"slices"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/interface/csvinput"
)

// SplitRow は、書き方に誤りのない行。
type SplitRow struct {
	Line  int
	Split model.StockSplit // SplitDate は "YYYY-MM-DD" に直した値（2026/3/1 → 2026-03-01）
}

// Parsed は、Parse の結果。どれも行の順。
type Parsed struct {
	Rows []SplitRow // 書き方に誤りのない行
	// Problems は、各行の段階の誤り（列の数・銘柄コード・分割の日・未来の日・分割後の株数）。1行の誤りは列の順にすべて入る。
	Problems []Problem
	// Duplicates は、行どうしの段階の誤り（要件17）。Rows のうち、同じ銘柄コード・同じ分割の日の2回目以降の行に
	// 「n行目: 銘柄コード <コード> の <YYYY-MM-DD> の分割は m行目にもあります」を入れる。m はその組が最初に出た行。
	// Problems が空でないとき、Run は Duplicates を使わない（前の段階の誤りがあれば次の段階は確かめないため）。
	Duplicates []Problem
}

// splitKey は、同じ分割（銘柄コードと分割の日）かを比べるキー。
type splitKey struct{ code, date string }

// Parse は、CSV の中身 content を、今日 today（"YYYY-MM-DD"）で読み、ファイル全体（要件14・要件15）・各行の書き方（要件16）・
// 同じ銘柄コードと分割の日の行（要件17）を確かめる。DB は見ない。
// ファイルの段階の誤りは、最初に当たった1つだけを *ParseError で返す。誤り以外の失敗（CSV の読み取りの想定外のエラーなど）は、ParseError でないエラーで返す。
func Parse(content []byte, today string) (Parsed, error) {
	records, err := readRecords(content)
	if err != nil {
		return Parsed{}, err
	}
	if len(records) == 0 || !slices.Equal(records[0].Fields, headerColumns) {
		return Parsed{}, &ParseError{Problems: []Problem{fileProblem(msgHeader)}}
	}
	if len(records) == 1 {
		return Parsed{}, &ParseError{Problems: []Problem{fileProblem(msgNoRows)}}
	}

	var parsed Parsed
	firstLine := make(map[splitKey]int) // 分割 → その分割が最初に出た行
	for _, rec := range records[1:] {
		split, texts, err := parseRow(rec, today)
		if err != nil {
			return Parsed{}, err
		}
		if len(texts) > 0 {
			for _, text := range texts {
				parsed.Problems = append(parsed.Problems, lineProblem(rec.Line, text))
			}
			continue
		}
		parsed.Rows = append(parsed.Rows, SplitRow{Line: rec.Line, Split: split})

		key := splitKey{code: split.StockCode, date: split.SplitDate}
		if first, ok := firstLine[key]; ok {
			parsed.Duplicates = append(parsed.Duplicates, lineProblem(rec.Line,
				fmt.Sprintf("銘柄コード %s の %s の分割は %d行目にもあります", key.code, key.date, first)))
		} else {
			firstLine[key] = rec.Line
		}
	}
	return parsed, nil
}

// readRecords は、content を csvinput.ReadRecords で読み、その誤りを splitcsv の誤りにする。
// UTF-8 でないものは文字コードの誤り、ダブルクォートの誤りは、最初の1か所を、そのレコードが始まる行の誤りにする。
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
