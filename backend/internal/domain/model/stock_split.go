package model

import "errors"

// StockSplit は、1件の株式分割（要件1〜3）。全利用者で共有するので、利用者は持たない。
type StockSplit struct {
	StockCode string
	SplitDate string // "YYYY-MM-DD"。分割の後の値段で売買が始まる日（要件3）
	Ratio     int64  // 分割後の株数（2〜1,000。1株が Ratio 株になる。要件2）
}

var (
	// ErrSplitRatioTooSmall は、分割後の株数が2未満であること（要件2）。
	ErrSplitRatioTooSmall = errors.New("split ratio too small")
	// ErrSplitRatioTooLarge は、分割後の株数が1,000より大きいこと（要件2）。
	ErrSplitRatioTooLarge = errors.New("split ratio too large")
	// ErrFutureSplitDate は、分割の日が今日（日本時間）より後であること（要件2）。
	ErrFutureSplitDate = errors.New("future split date")
)

const (
	// minSplitRatio は、分割後の株数の下限（要件2。1株が2株になる分割から）。
	minSplitRatio = 2
	// maxSplitRatio は、分割後の株数の上限（要件2）。
	maxSplitRatio = 1000
)

// CheckSplitRatio は、ratio が2以上1,000以下なら nil、そうでなければ ErrSplitRatioTooSmall か ErrSplitRatioTooLarge を返す（要件2）。
func CheckSplitRatio(ratio int64) error {
	switch {
	case ratio < minSplitRatio:
		return ErrSplitRatioTooSmall
	case ratio > maxSplitRatio:
		return ErrSplitRatioTooLarge
	default:
		return nil
	}
}

// CheckSplitDate は、splitDate が today 以前なら nil、後なら ErrFutureSplitDate を返す（要件2）。
// どちらも "YYYY-MM-DD"（年は4桁）。同じ形の文字列は、文字列の比較が日付の比較と一致する。
// 日付として実在するかは確かめない（呼び出し側が確かめてから渡す。CheckTradeDate と同じ）。
func CheckSplitDate(splitDate, today string) error {
	if splitDate > today {
		return ErrFutureSplitDate
	}
	return nil
}

// NewStockSplit は、分割を作る（要件1〜3）。銘柄コード（CheckStockCode）・分割の日（CheckSplitDate）・比率（CheckSplitRatio）の順
// （CSV の列の順）に確かめ、最初に当たった誤りを返す。誤りのときは零値の分割を返す。
// splitDate と today は "YYYY-MM-DD"。日付として実在するかは、呼び出し側が確かめてから渡す。
func NewStockSplit(code, splitDate string, ratio int64, today string) (StockSplit, error) {
	if err := CheckStockCode(code); err != nil {
		return StockSplit{}, err
	}
	if err := CheckSplitDate(splitDate, today); err != nil {
		return StockSplit{}, err
	}
	if err := CheckSplitRatio(ratio); err != nil {
		return StockSplit{}, err
	}
	return StockSplit{StockCode: code, SplitDate: splitDate, Ratio: ratio}, nil
}
