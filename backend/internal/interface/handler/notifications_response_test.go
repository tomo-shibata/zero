package handler

import (
	"encoding/json"
	"testing"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 仕様: 価格の変動の通知の要件（docs/requirements/price-alerts.md）の要件22（通知の項目）・要件31・要件32 の表の値と、
// プラン（docs/plans/price-alerts/plan.md）4.5「API の形」（GET /api/notifications）・4.8 の PR②（newNotificationsResponse）・5.2 の
// notifications_response_test.go の行（5種類の通知の JSON を null のキーも含めて形全体で確かめる。ドルの値は小数2桁）。
// 金額は 0.01 単位の整数から、浮動小数点を通さずに文字列にする。単価（終値・取得価格・最高値・前の終値・決めた価格を分割で割った値）は円が小数1桁・ドルが小数2桁、
// 含み損益は円が符号付きの整数・ドルが符号付きの小数2桁（プラスに + は付けない）、率は符号付きの小数1桁、
// 条件の値（threshold）は % が小数1桁・円の金額が整数・ドルの金額が小数2桁・円の価格が小数1桁・ドルの価格が小数2桁。
// id と latestId は整数の文字列、quantity は数、全銘柄の条件の stockCode は null。種類で使わない値は null（キーは常にある）。
// 確かめるのは、newNotificationsResponse の結果を json.Marshal した本文（assertHistoryJSON は stock_history_response_test.go の補助）。

func notifI64(v int64) *int64   { return &v }
func notifStr(v string) *string { return &v }

func marshalNotifications(t *testing.T, list dto.NotificationList) []byte {
	t.Helper()
	resp, err := newNotificationsResponse(list)
	if err != nil {
		t.Fatalf("newNotificationsResponse: got err %v, want nil", err)
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return body
}

// 4.5 の例どおりの形（取得価格から。7203・円）。キーの集まりは、id・stockCode・stockName・currency・tradingDate・isNew・condition・close・rate・basePrice・baseDate・
// unrealizedGain・quantity・adjustedThreshold。latestId は文字列。
func Test補助_4_5の例どおりの本文を作る(t *testing.T) {
	t.Parallel()
	list := dto.NotificationList{
		Items: []dto.NotificationItem{{
			ID: 12, StockCode: "7203", StockName: "トヨタ自動車", Currency: dto.CurrencyJPY, TradingDate: "2026-11-14", IsNew: true,
			Condition:            dto.NotificationCondition{Kind: "cost", Scope: "all", StockCode: "", Direction: "down", ThresholdHundredths: 2000},
			ClosePriceHundredths: 199000,
			Details:              dto.AlertNotificationDetails{RateTenths: notifI64(-204), BasePriceHundredths: notifI64(250000)},
		}},
		LatestID: notifI64(15),
	}

	got := marshalNotifications(t, list)

	assertHistoryJSON(t, got, `{
		"items": [{
			"id": "12", "stockCode": "7203", "stockName": "トヨタ自動車", "currency": "JPY",
			"tradingDate": "2026-11-14", "isNew": true,
			"condition": {"kind": "cost", "scope": "all", "stockCode": null, "direction": "down", "threshold": "20.0"},
			"close": "1990.0", "rate": "-20.4", "basePrice": "2500.0", "baseDate": null,
			"unrealizedGain": null, "quantity": null, "adjustedThreshold": null
		}],
		"latestId": "15"
	}`)
}

// 5種類の通知（要件32 の表）。種類ごとに、使う値だけが入り、ほかは null。ドルの通知は値が小数2桁。見た通知は isNew が false。
func Test補助_5種類の通知の本文は使う値だけを文字列にしほかはnull(t *testing.T) {
	t.Parallel()
	list := dto.NotificationList{
		Items: []dto.NotificationItem{
			{ // 最高値から（円）。最高値とその日。
				ID: 11, StockCode: "6758", StockName: "ソニーグループ", Currency: dto.CurrencyJPY, TradingDate: "2026-11-10", IsNew: false,
				Condition:            dto.NotificationCondition{Kind: "peak", Scope: "all", Direction: "down", ThresholdHundredths: 1500},
				ClosePriceHundredths: 339200,
				Details:              dto.AlertNotificationDetails{RateTenths: notifI64(-152), BasePriceHundredths: notifI64(400000), BaseDate: notifStr("2026-09-30")},
			},
			{ // 前の取引日から（ドル）。動いた（either）・値は 5%。率はプラスなので + を付けない。
				ID: 10, StockCode: "AAPL", StockName: "Apple", Currency: dto.CurrencyUSD, TradingDate: "2026-11-14", IsNew: true,
				Condition:            dto.NotificationCondition{Kind: "prev", Scope: "all", Direction: "either", ThresholdHundredths: 500},
				ClosePriceHundredths: 19124,
				Details:              dto.AlertNotificationDetails{RateTenths: notifI64(61), BasePriceHundredths: notifI64(18025), BaseDate: notifStr("2026-11-13")},
			},
			{ // 含み損益の金額（円）。1銘柄の条件。含み損は符号付きの整数、値は整数。
				ID: 9, StockCode: "6758", StockName: "ソニーグループ", Currency: dto.CurrencyJPY, TradingDate: "2026-11-04", IsNew: false,
				Condition:            dto.NotificationCondition{Kind: "pnl", Scope: "stock", StockCode: "6758", Direction: "loss", ThresholdHundredths: 10000000},
				ClosePriceHundredths: 348000,
				Details: dto.AlertNotificationDetails{
					UnrealizedGainHundredths: notifI64(-10400000), BasePriceHundredths: notifI64(400000), Quantity: notifI64(200),
				},
			},
			{ // 含み損益の金額（ドル）。含み益は + を付けず小数2桁、値は小数2桁。
				ID: 8, StockCode: "AAPL", StockName: "Apple", Currency: dto.CurrencyUSD, TradingDate: "2026-11-10", IsNew: true,
				Condition:            dto.NotificationCondition{Kind: "pnl", Scope: "stock", StockCode: "AAPL", Direction: "gain", ThresholdHundredths: 30000},
				ClosePriceHundredths: 18000,
				Details: dto.AlertNotificationDetails{
					UnrealizedGainHundredths: notifI64(30000), BasePriceHundredths: notifI64(15000), Quantity: notifI64(10),
				},
			},
			{ // 決めた価格（円）。details は空（キーがなく、すべて null）。値は小数1桁。
				ID: 7, StockCode: "9984", StockName: "ソフトバンクグループ", Currency: dto.CurrencyJPY, TradingDate: "2026-11-13", IsNew: true,
				Condition:            dto.NotificationCondition{Kind: "price", Scope: "stock", StockCode: "9984", Direction: "above", ThresholdHundredths: 800000},
				ClosePriceHundredths: 812000,
			},
			{ // 決めた価格（円）を分割で割った値。
				ID: 6, StockCode: "7203", StockName: "トヨタ自動車", Currency: dto.CurrencyJPY, TradingDate: "2026-11-11", IsNew: true,
				Condition:            dto.NotificationCondition{Kind: "price", Scope: "stock", StockCode: "7203", Direction: "above", ThresholdHundredths: 800000},
				ClosePriceHundredths: 410000,
				Details:              dto.AlertNotificationDetails{AdjustedThresholdHundredths: notifI64(400000)},
			},
			{ // 決めた価格（ドル）。以下・値は小数2桁。
				ID: 5, StockCode: "AAPL", StockName: "Apple", Currency: dto.CurrencyUSD, TradingDate: "2026-11-09", IsNew: true,
				Condition:            dto.NotificationCondition{Kind: "price", Scope: "stock", StockCode: "AAPL", Direction: "below", ThresholdHundredths: 15025},
				ClosePriceHundredths: 14900,
			},
		},
		LatestID: notifI64(12),
	}

	got := marshalNotifications(t, list)

	assertHistoryJSON(t, got, `{
		"items": [
			{
				"id": "11", "stockCode": "6758", "stockName": "ソニーグループ", "currency": "JPY", "tradingDate": "2026-11-10", "isNew": false,
				"condition": {"kind": "peak", "scope": "all", "stockCode": null, "direction": "down", "threshold": "15.0"},
				"close": "3392.0", "rate": "-15.2", "basePrice": "4000.0", "baseDate": "2026-09-30",
				"unrealizedGain": null, "quantity": null, "adjustedThreshold": null
			},
			{
				"id": "10", "stockCode": "AAPL", "stockName": "Apple", "currency": "USD", "tradingDate": "2026-11-14", "isNew": true,
				"condition": {"kind": "prev", "scope": "all", "stockCode": null, "direction": "either", "threshold": "5.0"},
				"close": "191.24", "rate": "6.1", "basePrice": "180.25", "baseDate": "2026-11-13",
				"unrealizedGain": null, "quantity": null, "adjustedThreshold": null
			},
			{
				"id": "9", "stockCode": "6758", "stockName": "ソニーグループ", "currency": "JPY", "tradingDate": "2026-11-04", "isNew": false,
				"condition": {"kind": "pnl", "scope": "stock", "stockCode": "6758", "direction": "loss", "threshold": "100000"},
				"close": "3480.0", "rate": null, "basePrice": "4000.0", "baseDate": null,
				"unrealizedGain": "-104000", "quantity": 200, "adjustedThreshold": null
			},
			{
				"id": "8", "stockCode": "AAPL", "stockName": "Apple", "currency": "USD", "tradingDate": "2026-11-10", "isNew": true,
				"condition": {"kind": "pnl", "scope": "stock", "stockCode": "AAPL", "direction": "gain", "threshold": "300.00"},
				"close": "180.00", "rate": null, "basePrice": "150.00", "baseDate": null,
				"unrealizedGain": "300.00", "quantity": 10, "adjustedThreshold": null
			},
			{
				"id": "7", "stockCode": "9984", "stockName": "ソフトバンクグループ", "currency": "JPY", "tradingDate": "2026-11-13", "isNew": true,
				"condition": {"kind": "price", "scope": "stock", "stockCode": "9984", "direction": "above", "threshold": "8000.0"},
				"close": "8120.0", "rate": null, "basePrice": null, "baseDate": null,
				"unrealizedGain": null, "quantity": null, "adjustedThreshold": null
			},
			{
				"id": "6", "stockCode": "7203", "stockName": "トヨタ自動車", "currency": "JPY", "tradingDate": "2026-11-11", "isNew": true,
				"condition": {"kind": "price", "scope": "stock", "stockCode": "7203", "direction": "above", "threshold": "8000.0"},
				"close": "4100.0", "rate": null, "basePrice": null, "baseDate": null,
				"unrealizedGain": null, "quantity": null, "adjustedThreshold": "4000.0"
			},
			{
				"id": "5", "stockCode": "AAPL", "stockName": "Apple", "currency": "USD", "tradingDate": "2026-11-09", "isNew": true,
				"condition": {"kind": "price", "scope": "stock", "stockCode": "AAPL", "direction": "below", "threshold": "150.25"},
				"close": "149.00", "rate": null, "basePrice": null, "baseDate": null,
				"unrealizedGain": null, "quantity": null, "adjustedThreshold": null
			}
		],
		"latestId": "12"
	}`)
}

// 率（0.1% 単位）の文字列: 0 は "0.0"（符号なし）、-5 は "-0.5"、プラスに + は付けない。% の条件の値は小数1桁（250 → "2.5"）。
func Test補助_率は符号付きの小数1桁で0は符号なし(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		rateTenth int64
		want      string
	}{
		{"0は0.0", 0, "0.0"},
		{"-5は-0.5", -5, "-0.5"},
		{"5は0.5", 5, "0.5"},
		{"-204は-20.4", -204, "-20.4"},
		{"1000は100.0", 1000, "100.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			list := dto.NotificationList{
				Items: []dto.NotificationItem{{
					ID: 1, StockCode: "7203", StockName: "トヨタ自動車", Currency: dto.CurrencyJPY, TradingDate: "2026-11-14", IsNew: true,
					Condition:            dto.NotificationCondition{Kind: "prev", Scope: "all", Direction: "either", ThresholdHundredths: 250},
					ClosePriceHundredths: 199000,
					Details:              dto.AlertNotificationDetails{RateTenths: notifI64(tc.rateTenth), BasePriceHundredths: notifI64(250000), BaseDate: notifStr("2026-11-13")},
				}},
				LatestID: notifI64(1),
			}

			got := marshalNotifications(t, list)

			var decoded struct {
				Items []struct {
					Rate      string `json:"rate"`
					Condition struct {
						Threshold string `json:"threshold"`
					} `json:"condition"`
				} `json:"items"`
			}
			if err := json.Unmarshal(got, &decoded); err != nil || len(decoded.Items) != 1 {
				t.Fatalf("本文を読めない: %v\n本文: %s", err, got)
			}
			if decoded.Items[0].Rate != tc.want {
				t.Errorf("rate: got %q, want %q", decoded.Items[0].Rate, tc.want)
			}
			if decoded.Items[0].Condition.Threshold != "2.5" {
				t.Errorf("condition.threshold: got %q, want %q（%% の値は小数1桁）", decoded.Items[0].Condition.Threshold, "2.5")
			}
		})
	}
}

// 通知が0件のとき: items は null でなく空の配列、latestId は null（Items が nil でも空のスライスでも同じ）。
func Test補助_通知が0件なら_itemsは空の配列でlatestIdはnull(t *testing.T) {
	t.Parallel()
	for name, items := range map[string][]dto.NotificationItem{"nil": nil, "空のスライス": {}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := marshalNotifications(t, dto.NotificationList{Items: items, LatestID: nil})

			assertHistoryJSON(t, got, `{"items": [], "latestId": null}`)
		})
	}
}

// 金額を書式にできないとき（知らない通貨）はエラーを返す（黙って誤った文字列にしない）。
func Test補助_知らない通貨の通知はエラーを返す(t *testing.T) {
	t.Parallel()
	list := dto.NotificationList{
		Items: []dto.NotificationItem{{
			ID: 1, StockCode: "7203", StockName: "トヨタ自動車", Currency: dto.Currency("EUR"), TradingDate: "2026-11-14", IsNew: true,
			Condition:            dto.NotificationCondition{Kind: "cost", Scope: "all", Direction: "down", ThresholdHundredths: 2000},
			ClosePriceHundredths: 199000,
			Details:              dto.AlertNotificationDetails{RateTenths: notifI64(-204), BasePriceHundredths: notifI64(250000)},
		}},
		LatestID: notifI64(1),
	}

	if _, err := newNotificationsResponse(list); err == nil {
		t.Errorf("newNotificationsResponse: got nil error, want error（知らない通貨）")
	}
}
