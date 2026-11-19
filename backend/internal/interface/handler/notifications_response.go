package handler

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 通知の条件の種類（価格の変動の通知の要件1。model.AlertKind と同じ値）。値の書式が種類で変わるので、ここで見分ける。
const (
	notificationKindCost  = "cost"
	notificationKindPeak  = "peak"
	notificationKindPrev  = "prev"
	notificationKindPnl   = "pnl"
	notificationKindPrice = "price"
)

// percentThresholdStepHundredths は、% の条件の値の最小の単位 0.1%（0.01 単位で10）。
const percentThresholdStepHundredths = 10

var errPercentThresholdNotTenth = errors.New("% の条件の値が 0.1% で割り切れません")

// notificationsResponse は、GET /api/notifications の本文（価格の変動の通知のプラン 4.5）。
// 金額は文字列にする（受け取る側の JavaScript の浮動小数点で誤差が入らないように。形は /api/holdings と同じ）。
// id と latestId は整数の文字列。種類で使わない値は null（キーは常にある）。items が0件でも [] で、latestId は null。
type notificationsResponse struct {
	Items    []notificationResponse `json:"items"`
	LatestID *string                `json:"latestId"`
}

// notificationResponse は、通知1件。close は単価の形、rate は符号付きの小数1桁、basePrice・adjustedThreshold は単価の形、
// unrealizedGain は含み損益の形（円は符号付きの整数、ドルは符号付きの小数2桁）、quantity は数。
type notificationResponse struct {
	ID                string                        `json:"id"`
	StockCode         string                        `json:"stockCode"`
	StockName         string                        `json:"stockName"`
	Currency          string                        `json:"currency"`
	TradingDate       string                        `json:"tradingDate"`
	IsNew             bool                          `json:"isNew"`
	Condition         notificationConditionResponse `json:"condition"`
	Close             string                        `json:"close"`
	Rate              *string                       `json:"rate"`
	BasePrice         *string                       `json:"basePrice"`
	BaseDate          *string                       `json:"baseDate"`
	UnrealizedGain    *string                       `json:"unrealizedGain"`
	Quantity          *int64                        `json:"quantity"`
	AdjustedThreshold *string                       `json:"adjustedThreshold"`
}

// notificationConditionResponse は、通知を作ったときの条件。stockCode は全銘柄なら null。
// threshold は、% は小数1桁、円の金額は整数、ドルの金額は小数2桁、価格は単価の形（円は小数1桁、ドルは小数2桁）。
type notificationConditionResponse struct {
	Kind      string  `json:"kind"`
	Scope     string  `json:"scope"`
	StockCode *string `json:"stockCode"`
	Direction string  `json:"direction"`
	Threshold string  `json:"threshold"`
}

// newNotificationsResponse は、通知のリストの Read Model を API の本文の形にする。
// 金額を書式にできないとき（円の値が 0.1円の単位でない、知らない通貨）は、黙って切り捨てずにエラーにする（newHoldingsResponse と同じ）。
func newNotificationsResponse(list dto.NotificationList) (notificationsResponse, error) {
	items := make([]notificationResponse, 0, len(list.Items))
	for _, n := range list.Items {
		item, err := newNotificationResponse(n)
		if err != nil {
			return notificationsResponse{}, fmt.Errorf("通知（id %d）: %w", n.ID, err)
		}
		items = append(items, item)
	}
	var latest *string
	if list.LatestID != nil {
		text := strconv.FormatInt(*list.LatestID, 10)
		latest = &text
	}
	return notificationsResponse{Items: items, LatestID: latest}, nil
}

// newNotificationResponse は、通知1件 n を API の本文の形にする。取得した値の順に変換し、最初のエラーを返す。
func newNotificationResponse(n dto.NotificationItem) (notificationResponse, error) {
	closePrice, err := formatUnitPrice(n.Currency, n.ClosePriceHundredths)
	if err != nil {
		return notificationResponse{}, fmt.Errorf("終値: %w", err)
	}
	threshold, err := formatNotificationThreshold(n.Currency, n.Condition)
	if err != nil {
		return notificationResponse{}, fmt.Errorf("条件の値: %w", err)
	}
	item := notificationResponse{
		ID:          strconv.FormatInt(n.ID, 10),
		StockCode:   n.StockCode,
		StockName:   n.StockName,
		Currency:    string(n.Currency),
		TradingDate: n.TradingDate,
		IsNew:       n.IsNew,
		Condition: notificationConditionResponse{
			Kind: n.Condition.Kind, Scope: n.Condition.Scope, Direction: n.Condition.Direction, Threshold: threshold,
		},
		Close:    closePrice,
		BaseDate: n.Details.BaseDate,
		Quantity: n.Details.Quantity,
	}
	if n.Condition.StockCode != "" {
		code := n.Condition.StockCode
		item.Condition.StockCode = &code
	}
	if n.Details.RateTenths != nil {
		rate := formatGainRate(*n.Details.RateTenths)
		item.Rate = &rate
	}
	if n.Details.BasePriceHundredths != nil {
		base, err := formatUnitPrice(n.Currency, *n.Details.BasePriceHundredths)
		if err != nil {
			return notificationResponse{}, fmt.Errorf("基準の価格: %w", err)
		}
		item.BasePrice = &base
	}
	if n.Details.UnrealizedGainHundredths != nil {
		gain, err := formatUnrealizedGain(n.Currency, *n.Details.UnrealizedGainHundredths)
		if err != nil {
			return notificationResponse{}, fmt.Errorf("含み損益: %w", err)
		}
		item.UnrealizedGain = &gain
	}
	if n.Details.AdjustedThresholdHundredths != nil {
		adjusted, err := formatUnitPrice(n.Currency, *n.Details.AdjustedThresholdHundredths)
		if err != nil {
			return notificationResponse{}, fmt.Errorf("分割で割った値: %w", err)
		}
		item.AdjustedThreshold = &adjusted
	}
	return item, nil
}

// formatNotificationThreshold は、条件の値（0.01 単位）を、種類ごとの文字列にする。
// % の種類は小数1桁（2000 → "20.0"。0.1% の倍数でなければエラー）、含み損益の金額の種類は通貨の金額の形（円は整数、ドルは小数2桁）、
// 決めた価格の種類は単価の形（円は小数1桁、ドルは小数2桁）。
func formatNotificationThreshold(currency dto.Currency, c dto.NotificationCondition) (string, error) {
	switch c.Kind {
	case notificationKindCost, notificationKindPeak, notificationKindPrev:
		if c.ThresholdHundredths%percentThresholdStepHundredths != 0 {
			return "", errPercentThresholdNotTenth
		}
		return formatDecimal(c.ThresholdHundredths/percentThresholdStepHundredths, rateDecimals), nil
	case notificationKindPnl:
		return formatValuation(currency, c.ThresholdHundredths)
	case notificationKindPrice:
		return formatUnitPrice(currency, c.ThresholdHundredths)
	default:
		return "", fmt.Errorf("知らない種類 %q です", c.Kind)
	}
}
