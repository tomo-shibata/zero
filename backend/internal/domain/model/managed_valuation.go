package model

import (
	"errors"
	"strings"
	"unicode"
)

// おまかせ運用の記録の決まり（おまかせ運用の要件1〜要件3）。

// 評価額と損益の範囲（円）。どちらも 1兆（1,000,000,000,000）未満。DB の CHECK（00010）と同じ値。
const maxManagedAmount int64 = 999_999_999_999

var (
	ErrBlankManagedAccountName   = errors.New("blank managed account name")   // 名前に空白以外の文字がない
	ErrManagedAccountNameControl = errors.New("managed account name control") // 名前に制御文字（改行・タブ・NUL など）がある（要件29）
	ErrFutureValuationDate       = errors.New("future valuation date")        // 日付が今日（日本時間）より後
	ErrValuationNotPositive      = errors.New("valuation not positive")       // 評価額が1未満
	ErrValuationTooLarge         = errors.New("valuation too large")          // 評価額が1兆以上
	ErrGainOutOfRange            = errors.New("gain out of range")            // 損益の絶対値が1兆以上
	ErrPrincipalNotPositive      = errors.New("principal not positive")       // 元本（評価額 − 損益）が1円未満
)

// ManagedValuation は、おまかせ運用の口座の、ある日の記録1件（要件2・要件3）。
type ManagedValuation struct {
	AccountName   string // 口座の名前。書いたとおりに持つ（前後の空白も取り除かない。要件1）
	ValuationDate string // 評価額の基準日（"YYYY-MM-DD"）
	Valuation     int64  // 評価額（円）
	Gain          int64  // 損益（円。マイナスあり）
}

// CheckManagedAccountName は、名前に空白以外の文字が1つ以上あり、制御文字がなければ nil を返す（要件1・要件29）。
// 空白以外の文字がなければ ErrBlankManagedAccountName、あれば次に制御文字（unicode.IsControl。
// U+0000〜U+001F と U+007F〜U+009F）を調べ、含んでいれば ErrManagedAccountNameControl を返す。
// 空白は、全角の空白・タブ・改行も含む（unicode.IsSpace。CheckStockName と同じ）。空白が先なので、タブだけの名前は ErrBlankManagedAccountName になる。
func CheckManagedAccountName(name string) error {
	if strings.TrimFunc(name, unicode.IsSpace) == "" {
		return ErrBlankManagedAccountName
	}
	if strings.ContainsFunc(name, unicode.IsControl) {
		return ErrManagedAccountNameControl
	}
	return nil
}

// CheckValuationDate は、date が today 以前なら nil、後なら ErrFutureValuationDate を返す（要件3）。
// どちらも "YYYY-MM-DD"（年は4桁）。同じ形の文字列は、文字列の比較が日付の比較と一致する。
// 日付として実在するかは確かめない（呼び出し側が確かめてから渡す。CheckSplitDate と同じ）。
func CheckValuationDate(date, today string) error {
	if date > today {
		return ErrFutureValuationDate
	}
	return nil
}

// CheckValuation は、評価額が 1以上 999,999,999,999 以下なら nil。1未満は ErrValuationNotPositive、1兆以上は ErrValuationTooLarge を返す（要件3）。
func CheckValuation(valuation int64) error {
	switch {
	case valuation < 1:
		return ErrValuationNotPositive
	case valuation > maxManagedAmount:
		return ErrValuationTooLarge
	}
	return nil
}

// CheckGain は、損益が -999,999,999,999 以上 999,999,999,999 以下なら nil、そうでなければ ErrGainOutOfRange を返す（要件3）。
func CheckGain(gain int64) error {
	if gain < -maxManagedAmount || gain > maxManagedAmount {
		return ErrGainOutOfRange
	}
	return nil
}

// CheckPrincipal は、元本（評価額 − 損益）が1以上なら nil、そうでなければ ErrPrincipalNotPositive を返す（要件3）。
// どの int64 を渡しても、あふれずに比べる（引き算をせず、評価額 > 損益 で比べる。評価額 − 損益 >= 1 は 評価額 > 損益 と同じ）。
func CheckPrincipal(valuation, gain int64) error {
	if valuation <= gain {
		return ErrPrincipalNotPositive
	}
	return nil
}

// NewManagedValuation は、記録を作る。名前 → 日付 → 評価額 → 損益 → 元本 の順（CSV の列の順）に確かめ、最初に当たった誤りを返す。
// 誤りのときは零値の記録を返す。date と today は "YYYY-MM-DD"。日付として実在するかは、呼び出し側が確かめてから渡す。
func NewManagedValuation(name, date string, valuation, gain int64, today string) (ManagedValuation, error) {
	if err := CheckManagedAccountName(name); err != nil {
		return ManagedValuation{}, err
	}
	if err := CheckValuationDate(date, today); err != nil {
		return ManagedValuation{}, err
	}
	if err := CheckValuation(valuation); err != nil {
		return ManagedValuation{}, err
	}
	if err := CheckGain(gain); err != nil {
		return ManagedValuation{}, err
	}
	if err := CheckPrincipal(valuation, gain); err != nil {
		return ManagedValuation{}, err
	}
	return ManagedValuation{AccountName: name, ValuationDate: date, Valuation: valuation, Gain: gain}, nil
}
