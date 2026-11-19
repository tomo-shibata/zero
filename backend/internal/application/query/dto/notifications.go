package dto

// NotificationCondition は、通知を作ったときの条件の中身（通知に写して持っている。価格の変動の通知の要件22）。
type NotificationCondition struct {
	Kind, Scope string
	StockCode   string // 全銘柄の条件は ""
	Direction   string
	// ThresholdHundredths は、条件の値（0.01 単位。20% は 2000、100,000円は 10000000、150.25ドルは 15025）。
	ThresholdHundredths int64
}

// NotificationItem は、通知のリストの1件（要件31）。
type NotificationItem struct {
	ID                   int64
	StockCode, StockName string
	Currency             Currency
	TradingDate          string // "YYYY-MM-DD"
	IsNew                bool   // 取得した時点で見ていなかったか
	Condition            NotificationCondition
	ClosePriceHundredths int64
	Details              AlertNotificationDetails
}

// NotificationList は、通知のリスト。Items は取引日の新しい順（同じ取引日は銘柄コードの昇順、全銘柄の条件が先、条件 ID の昇順）で100件まで。
// LatestID は、その利用者の通知の最大の ID（100件より古いものを含む）で、0件なら nil。
type NotificationList struct {
	Items    []NotificationItem
	LatestID *int64
}
