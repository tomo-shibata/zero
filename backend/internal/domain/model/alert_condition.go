package model

import (
	"errors"
	"fmt"
)

// AlertKind は、価格の変動の通知の条件の種類（要件2）。
// 値は、DB の alert_conditions.kind の CHECK（00011）と同じ文字列。
type AlertKind string

// AlertScope は、条件の対象（全銘柄か、1銘柄か。要件2）。値は alert_conditions.scope の CHECK と同じ。
type AlertScope string

// AlertDirection は、条件の向き（要件2）。値は alert_conditions.direction の CHECK と同じ。
type AlertDirection string

// 定数は1つずつ型を書く（値を書いた定数は前の行の型を引き継がず、型のない文字列になるため）。
const (
	AlertKindCost  AlertKind = "cost"  // 取得価格から
	AlertKindPeak  AlertKind = "peak"  // 最高値から
	AlertKindPrev  AlertKind = "prev"  // 前の取引日から
	AlertKindPnl   AlertKind = "pnl"   // 含み損益の金額
	AlertKindPrice AlertKind = "price" // 決めた価格
)

const (
	AlertScopeAll   AlertScope = "all"   // 全銘柄
	AlertScopeStock AlertScope = "stock" // 1銘柄
)

const (
	AlertDirectionUp     AlertDirection = "up"
	AlertDirectionDown   AlertDirection = "down"
	AlertDirectionEither AlertDirection = "either"
	AlertDirectionLoss   AlertDirection = "loss"
	AlertDirectionGain   AlertDirection = "gain"
	AlertDirectionAbove  AlertDirection = "above"
	AlertDirectionBelow  AlertDirection = "below"
)

// ErrInvalidAlertCondition は、条件の種類・対象・向き・値の組み合わせや範囲が正しくないこと（要件2・要件4）。errors.Is で見分ける。
var ErrInvalidAlertCondition = errors.New("条件の種類・対象・向き・値が正しくありません")

// 値の範囲（要件4）。すべて 0.01 単位。% の種類と、円・ドルの金額と価格で変わる。
const (
	percentUnit      int64 = 10 // % は小数第1位（0.1%）まで
	percentDownMax   int64 = 9990
	percentUpMax     int64 = 100000
	yenPnlUnit       int64 = 100 // 円の含み損益は整数
	yenPnlMax        int64 = 100000000000
	usdPnlMax        int64 = 1000000000
	yenAlertPriceMax int64 = 9999999990
	usdAlertPriceMax int64 = 9999999999
	usdCentUnit      int64 = 1
)

// AlertCondition は、通知の条件の中身（種類・対象・向き・値）。NewAlertCondition で作ると、組み合わせと値の範囲が確かめてある。
type AlertCondition struct {
	Kind                AlertKind
	Scope               AlertScope
	StockCode           string // 全銘柄は ""
	Direction           AlertDirection
	ThresholdHundredths int64 // 値（0.01 単位。20% は 2000、100,000円は 10000000、150.25ドルは 15025）
}

// NewAlertCondition は、条件の組み合わせ（要件2）と値の範囲・精度（要件4）を確かめて作る。
// currency は1銘柄の条件の銘柄の通貨で、% の種類と全銘柄では見ない。違えば ErrInvalidAlertCondition を返す。
func NewAlertCondition(kind AlertKind, scope AlertScope, stockCode string, direction AlertDirection, thresholdHundredths int64, currency Currency) (AlertCondition, error) {
	if err := checkAlertScope(kind, scope, stockCode); err != nil {
		return AlertCondition{}, err
	}
	if err := checkAlertThreshold(kind, direction, thresholdHundredths, currency); err != nil {
		return AlertCondition{}, err
	}
	return AlertCondition{Kind: kind, Scope: scope, StockCode: stockCode, Direction: direction, ThresholdHundredths: thresholdHundredths}, nil
}

// checkAlertScope は、対象と銘柄コードの組み合わせと、種類ごとに選べる対象を確かめる。
func checkAlertScope(kind AlertKind, scope AlertScope, stockCode string) error {
	switch scope {
	case AlertScopeAll:
		if stockCode != "" {
			return fmt.Errorf("%w: 全銘柄に銘柄コードは付けられません", ErrInvalidAlertCondition)
		}
		if kind == AlertKindPnl || kind == AlertKindPrice {
			return fmt.Errorf("%w: この種類は1銘柄だけです", ErrInvalidAlertCondition)
		}
	case AlertScopeStock:
		if stockCode == "" {
			return fmt.Errorf("%w: 1銘柄には銘柄コードが要ります", ErrInvalidAlertCondition)
		}
	default:
		return fmt.Errorf("%w: 知らない対象 %q です", ErrInvalidAlertCondition, string(scope))
	}
	return nil
}

// checkAlertThreshold は、種類と向きの組み合わせと、値の範囲・精度を確かめる。
func checkAlertThreshold(kind AlertKind, direction AlertDirection, value int64, currency Currency) error {
	lo, hi, unit, ok := alertThresholdRange(kind, direction, currency)
	if !ok {
		return fmt.Errorf("%w: 種類 %q と向き %q の組み合わせは選べません", ErrInvalidAlertCondition, string(kind), string(direction))
	}
	if value < lo || value > hi || value%unit != 0 {
		return fmt.Errorf("%w: 値が範囲や精度に合いません", ErrInvalidAlertCondition)
	}
	return nil
}

// alertThresholdRange は、種類・向き・通貨ごとの値の下限・上限・単位（0.01 単位）を返す。選べない組み合わせは ok=false。
func alertThresholdRange(kind AlertKind, direction AlertDirection, currency Currency) (lo, hi, unit int64, ok bool) {
	switch kind {
	case AlertKindCost:
		switch direction {
		case AlertDirectionUp:
			return percentUnit, percentUpMax, percentUnit, true
		case AlertDirectionDown:
			return percentUnit, percentDownMax, percentUnit, true
		}
	case AlertKindPeak:
		if direction == AlertDirectionDown {
			return percentUnit, percentDownMax, percentUnit, true
		}
	case AlertKindPrev:
		switch direction {
		case AlertDirectionUp, AlertDirectionEither:
			return percentUnit, percentUpMax, percentUnit, true
		case AlertDirectionDown:
			return percentUnit, percentDownMax, percentUnit, true
		}
	case AlertKindPnl:
		if direction == AlertDirectionLoss || direction == AlertDirectionGain {
			switch currency {
			case CurrencyJPY:
				return yenPnlUnit, yenPnlMax, yenPnlUnit, true
			case CurrencyUSD:
				return usdCentUnit, usdPnlMax, usdCentUnit, true
			}
		}
	case AlertKindPrice:
		if direction == AlertDirectionAbove || direction == AlertDirectionBelow {
			switch currency {
			case CurrencyJPY:
				return yenPriceUnitHundredths, yenAlertPriceMax, yenPriceUnitHundredths, true
			case CurrencyUSD:
				return usdCentUnit, usdAlertPriceMax, usdCentUnit, true
			}
		}
	}
	return 0, 0, 0, false
}
