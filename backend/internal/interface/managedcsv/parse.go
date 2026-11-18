package managedcsv

import (
	"errors"
	"fmt"
	"slices"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/interface/csvinput"
)

// ValuationRow は、書き方に誤りのない行。
type ValuationRow struct {
	Line      int
	Valuation model.ManagedValuation // ValuationDate は "YYYY-MM-DD" に直した値（2026/1/6 → 2026-01-06）
}

// Parsed は、Parse の結果。どれも行の順。
type Parsed struct {
	Rows []ValuationRow // 書き方に誤りのない行
	// Problems は、各行の段階の誤り（列の数・名前・日付・評価額・損益・元本）。1行の誤りは列の順にすべて入る
	// （列の数の誤りなら、その1つだけ。元本は、評価額と損益がどちらも正しいときだけ確かめる）。
	Problems []Problem
	// Duplicates は、行どうしの段階の誤り（要件30）。Rows のうち、同じ名前・同じ日付の2回目以降の行に
	// 「n行目: <名前> の <YYYY-MM-DD> の記録が、m行目と重なっています」を入れる。m はその組が最初に出た行。
	// Problems が空でないとき、Run は Duplicates を使わない（前の段階の誤りがあれば次の段階は確かめないため）。
	Duplicates []Problem
}

// valuationKey は、同じ記録（名前と日付）かを比べるキー。名前は書いたとおりの文字列で比べる（要件1）。
type valuationKey struct{ name, date string }

// Parse は、CSV の中身 content を、今日 today（"YYYY-MM-DD"）で読み、ファイル全体（要件27・要件28）・各行の書き方（要件29）・
// 同じ名前と日付の行（要件30）を確かめる。DB は見ない。
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
	firstLine := make(map[valuationKey]int) // 名前と日付 → その組が最初に出た行
	for _, rec := range records[1:] {
		valuation, texts, err := parseRow(rec, today)
		if err != nil {
			return Parsed{}, err
		}
		if len(texts) > 0 {
			for _, text := range texts {
				parsed.Problems = append(parsed.Problems, lineProblem(rec.Line, text))
			}
			continue
		}
		parsed.Rows = append(parsed.Rows, ValuationRow{Line: rec.Line, Valuation: valuation})

		key := valuationKey{name: valuation.AccountName, date: valuation.ValuationDate}
		if first, ok := firstLine[key]; ok {
			parsed.Duplicates = append(parsed.Duplicates, lineProblem(rec.Line,
				fmt.Sprintf("%s の %s の記録が、%d行目と重なっています", key.name, key.date, first)))
		} else {
			firstLine[key] = rec.Line
		}
	}
	return parsed, nil
}

// readRecords は、content を csvinput.ReadRecords で読み、その誤りを managedcsv の誤りにする。
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
