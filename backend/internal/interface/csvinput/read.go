// Package csvinput は、売買履歴の CSV（tradecsv）と価格の CSV（pricecsv）で共通の、CSV を読む部分
// （UTF-8 の確かめ・レコードの読み出し・日本時間の今日・ファイルのパスの解き方）を持つ。
// 誤りの文言や、列の意味の確かめは持たない（それぞれの CSV のパッケージが付ける）。
package csvinput

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

// utf8BOM は、UTF-8 の BOM。あってもなくてもよい。
const utf8BOM = "\xef\xbb\xbf"

// Record は、CSV のレコード1つ。Line はそのレコードが始まるファイルの行番号（見出しが1行目。空の行と、値の中の改行も行に数える）。
type Record struct {
	Line   int
	Fields []string
}

// ErrNotUTF8 は、content が UTF-8 として読めないこと。
var ErrNotUTF8 = errors.New("csv is not utf-8")

// QuoteError は、ダブルクォートの使い方の誤り。Line はそのレコードが始まる行。
type QuoteError struct{ Line int }

func (e *QuoteError) Error() string {
	return fmt.Sprintf("csv: invalid double quote in record starting at line %d", e.Line)
}

// ReadRecords は、content を UTF-8 として確かめ、先頭の BOM を除いて、見出しを含むすべてのレコードを返す。
// 空の行は読み飛ばす（行番号には数える）。列の数は確かめない。
// UTF-8 でなければ ErrNotUTF8、ダブルクォートの誤りは最初の1か所を *QuoteError で返す。それ以外の読み取りの失敗は、そのほかのエラーで返す。
func ReadRecords(content []byte) ([]Record, error) {
	if !utf8.Valid(content) {
		return nil, ErrNotUTF8
	}
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(content, []byte(utf8BOM))))
	r.FieldsPerRecord = -1

	var records []Record
	for {
		fields, err := r.Read()
		if errors.Is(err, io.EOF) {
			return records, nil
		}
		if err != nil {
			var parseErr *csv.ParseError
			if errors.As(err, &parseErr) && (errors.Is(parseErr.Err, csv.ErrQuote) || errors.Is(parseErr.Err, csv.ErrBareQuote)) {
				return nil, &QuoteError{Line: parseErr.StartLine}
			}
			return nil, err
		}
		line, _ := r.FieldPos(0)
		records = append(records, Record{Line: line, Fields: fields})
	}
}
