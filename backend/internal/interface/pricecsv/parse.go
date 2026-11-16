package pricecsv

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"slices"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/interface/csvinput"
)

// CSV の列の位置（見出しの順。要件5）。
const (
	colStockCode = iota
	colPrice
	columnCount
)

// priceFormat は、終値の書き方（要件7）。半角数字と小数点だけで、小数点の前にも後にも数字がある。
var priceFormat = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// Parsed は、Parse の結果。どれも行の順。
type Parsed struct {
	Rows []command.PriceImportRow // 書き方に誤りのない行
	// Problems は、各行の段階のうち、書き方の誤り（列の数・銘柄コード・終値の書き方・0以下・1億以上）。
	// 通貨ごとの桁の誤りは、銘柄マスタの通貨が要るので含まない（command.ImportClosingPrices.Classify が見つける）。
	Problems []Problem
	// Duplicates は、行どうしの段階の誤り（要件8）。列の数が2の行（銘柄コードや終値の書き方の誤りがある行も含む）を銘柄コードの文字列で比べ、
	// 2回目以降の行に「n行目: 銘柄コード <コード> は m行目にもあります」を入れる。m はその銘柄コードが最初に出た行。
	// Problems が空でないとき、Run は Duplicates を使わない（前の段階の誤りがあれば次の段階は確かめないため）。
	Duplicates []Problem
}

// Parse は、CSV の中身 content を読み、ファイル全体（要件4・要件5）と各行の書き方（要件7）、同じ銘柄コードの行（要件8）を確かめる。DB は見ない。
// ファイルの段階の誤りは、最初に当たった1つだけを *ParseError で返す。誤り以外の失敗（CSV の読み取りの想定外のエラーなど）は、ParseError でないエラーで返す。
func Parse(content []byte) (Parsed, error) {
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
	firstLine := make(map[string]int) // 銘柄コード → その銘柄コードが最初に出た行
	for _, rec := range records[1:] {
		if len(rec.Fields) != columnCount {
			parsed.Problems = append(parsed.Problems, lineProblem(rec.Line, msgColumns))
			continue
		}
		code := rec.Fields[colStockCode]
		if first, ok := firstLine[code]; ok {
			parsed.Duplicates = append(parsed.Duplicates, lineProblem(rec.Line, fmt.Sprintf("銘柄コード %s は %d行目にもあります", code, first)))
		} else {
			firstLine[code] = rec.Line
		}

		row, texts := parseRow(rec.Line, code, rec.Fields[colPrice])
		if len(texts) > 0 {
			for _, text := range texts {
				parsed.Problems = append(parsed.Problems, lineProblem(rec.Line, text))
			}
			continue
		}
		parsed.Rows = append(parsed.Rows, row)
	}
	return parsed, nil
}

// parseRow は、列の数が2の行の銘柄コード code と終値 price の書き方を確かめる。
// 誤りの文言（「n行目: 」なし）を、銘柄コード、終値の順に返す（1つの列では最初に当たった1つだけ）。誤りがなければ文言は空で、行を返す。
func parseRow(line int, code, price string) (command.PriceImportRow, []string) {
	var texts []string
	if err := model.CheckStockCode(code); err != nil {
		texts = append(texts, msgStockCode)
	}
	value, text := parsePrice(price)
	if text != "" {
		texts = append(texts, text)
	}
	if len(texts) > 0 {
		return command.PriceImportRow{}, texts
	}
	return command.PriceImportRow{Line: line, StockCode: code, Price: value}, nil
}

// parsePrice は、終値の列を読む。誤りなら、2つ目の戻り値に文言を返す。書き方 → 0以下 → 1億以上の順に確かめる。
// 通貨ごとの桁は確かめない（Classify の役目）。
func parsePrice(s string) (*big.Rat, string) {
	if !priceFormat.MatchString(s) {
		return nil, msgPriceFormat
	}
	price, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, msgPriceFormat
	}
	if err := model.CheckPrice(price); err != nil {
		if errors.Is(err, model.ErrPriceNotPositive) {
			return nil, msgPriceNotPos
		}
		return nil, msgPriceTooLarge
	}
	return price, ""
}

// readRecords は、content を csvinput.ReadRecords で読み、その誤りを pricecsv の誤りにする。
// UTF-8 でないものは文字コードの誤り、ダブルクォートの誤りは、最初の1か所だけを、そのレコードが始まる行の誤りにする。
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
