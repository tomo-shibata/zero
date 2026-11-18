package dto

import "github.com/google/uuid"

// AlertConditionRow は、DB から読んだ通知の条件1件（価格の変動の通知の要件1〜要件6。判定する前）。
type AlertConditionRow struct {
	ID                  int64
	UserID              uuid.UUID
	Kind, Scope         string // model.AlertKind・AlertScope と同じ値
	StockCode           string // 全銘柄は ""
	Direction           string
	ThresholdHundredths int64  // 値（0.01 単位。20% は 2000、100,000円は 10000000、150.25ドルは 15025）
	AddedOn             string // 足した日（created_at の日本時間の日付。"YYYY-MM-DD"）
}

// AlertJudgmentRow は、DB から読んだ判定の結果1件（条件と銘柄の組の、ある取引日の判定。要件16）。
type AlertJudgmentRow struct {
	ConditionID int64
	TradingDate string // "YYYY-MM-DD"
	Satisfied   bool
}

// AlertEvaluationInput は、1銘柄・1取引日の判定の入力（プラン 4.3）。
type AlertEvaluationInput struct {
	StockCode            string
	Currency             Currency
	TradingDate          string // 判定する終値の取引日 D
	ClosePriceHundredths int64
	LatestTradingDate    string                     // その銘柄の最新の取引日（D の保存の後の値）
	Splits               []StockSplitRow            // その銘柄の分割
	Conditions           []AlertConditionRow        // 全利用者の、全銘柄の条件とこの銘柄の条件
	TradesByUser         map[uuid.UUID][]TradeRow   // 条件を持つ利用者ごとの、この銘柄の売買（ClosePriceHundredths・PriceDate は nil でよい）
	PreviousClose        *ClosingPriceRow           // D より前で最も新しい終値。なければ nil
	Closes               []ClosingPriceRow          // 最高値の条件があるときだけ。D 以前を取引日の昇順に（D の行があっても判定では使わず、D の値は ClosePriceHundredths を使う）
	PreviousJudgments    map[int64]AlertJudgmentRow // 条件 ID ごとの、D より前で最も新しい判定
}

// AlertNotificationDetails は、通知の、種類で変わる値。JSON に入れるときのキーは各フィールドの json タグで、nil は書かない（プラン 判断4）。
type AlertNotificationDetails struct {
	RateTenths                  *int64  `json:"rateTenths,omitempty"`                  // 率（0.1% 単位）。取得価格から・最高値から・前の取引日から
	BasePriceHundredths         *int64  `json:"basePriceHundredths,omitempty"`         // 取得価格・最高値・前の終値（表示の丸めの後）
	BaseDate                    *string `json:"baseDate,omitempty"`                    // 最高値・前の終値の取引日
	UnrealizedGainHundredths    *int64  `json:"unrealizedGainHundredths,omitempty"`    // 含み損益の金額
	Quantity                    *int64  `json:"quantity,omitempty"`                    // 含み損益の金額
	AdjustedThresholdHundredths *int64  `json:"adjustedThresholdHundredths,omitempty"` // 決めた価格を分割で割った値（割る数が1より大きいときだけ）
}

// AlertJudgment は、書く判定の結果1件。
type AlertJudgment struct {
	ConditionID int64
	StockCode   string
	TradingDate string
	Satisfied   bool
}

// AlertNotification は、作る通知1件。
type AlertNotification struct {
	UserID               uuid.UUID
	Condition            AlertConditionRow // 作ったときの条件（通知に写す）
	StockCode            string
	TradingDate          string
	ClosePriceHundredths int64
	Details              AlertNotificationDetails
}

// AlertEvaluationResult は、判定の結果と、作る通知。どちらも条件 ID の昇順で、0件は空のスライス（nil ではない）。
type AlertEvaluationResult struct {
	Judgments     []AlertJudgment
	Notifications []AlertNotification
}
