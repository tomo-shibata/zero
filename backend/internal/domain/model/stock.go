package model

import (
	"errors"
	"strings"
	"unicode"
)

// Stock は、銘柄マスタの1銘柄（要件1）。銘柄コード・銘柄名・通貨を持つ。
type Stock struct {
	Code, Name string
	Currency   Currency
}

var (
	// ErrInvalidStockCode は、銘柄コードが半角の英大文字・数字・「.」・「-」だけの1〜10文字でないこと（要件24）。
	ErrInvalidStockCode = errors.New("invalid stock code")
	// ErrBlankStockName は、銘柄名に空白以外の文字がないこと（要件24）。
	ErrBlankStockName = errors.New("blank stock name")
	// ErrInvalidCurrency は、銘柄の通貨が円（JPY）・ドル（USD）以外であること（要件1）。
	ErrInvalidCurrency = errors.New("invalid currency")
)

const (
	minStockCodeLength = 1
	maxStockCodeLength = 10
)

// CheckStockCode は、銘柄コードが半角の英大文字・数字・「.」・「-」だけの1〜10文字なら nil、そうでなければ ErrInvalidStockCode を返す（要件24）。
// 前後の空白や改行も誤りにする（取り除かない）。
func CheckStockCode(code string) error {
	if len(code) < minStockCodeLength || len(code) > maxStockCodeLength {
		return ErrInvalidStockCode
	}
	for i := range len(code) {
		c := code[i]
		isUpper := c >= 'A' && c <= 'Z'
		isDigit := c >= '0' && c <= '9'
		if !isUpper && !isDigit && c != '.' && c != '-' {
			return ErrInvalidStockCode
		}
	}
	return nil
}

// CheckStockName は、銘柄名に空白以外の文字が1つ以上あれば nil、なければ ErrBlankStockName を返す（要件24）。
// 空白は、全角の空白・タブ・改行も含む（unicode.IsSpace）。
func CheckStockName(name string) error {
	if strings.TrimFunc(name, unicode.IsSpace) == "" {
		return ErrBlankStockName
	}
	return nil
}

// NewStock は、銘柄を作る。銘柄コードか銘柄名が書き方に合わなければ、CheckStockCode・CheckStockName のエラーを返す。
// 通貨が円（JPY）・ドル（USD）以外なら ErrInvalidCurrency を返す（要件1）。
func NewStock(code, name string, currency Currency) (Stock, error) {
	if err := CheckStockCode(code); err != nil {
		return Stock{}, err
	}
	if err := CheckStockName(name); err != nil {
		return Stock{}, err
	}
	if currency != CurrencyJPY && currency != CurrencyUSD {
		return Stock{}, ErrInvalidCurrency
	}
	return Stock{Code: code, Name: name, Currency: currency}, nil
}
