package tradecsv

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"time"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// CSV の列の位置（見出しの順。要件23）。
const (
	colStockCode = iota
	colStockName
	colCurrency
	colSide
	colPrice
	colQuantity
	colTradeDate
	columnCount
)

var (
	// priceFormat は、価格の書き方（要件24）。半角数字と小数点だけで、小数点の前にも後にも数字がある。
	priceFormat = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
	// quantityFormat は、株数の書き方（要件24）。半角数字だけ。
	quantityFormat = regexp.MustCompile(`^[0-9]+$`)
	// hyphenDateFormat・slashDateFormat は、取引日の書き方（要件24）。ハイフンの形は月と日の0埋めが要り、スラッシュの形は0埋めが任意。
	hyphenDateFormat = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})$`)
	slashDateFormat  = regexp.MustCompile(`^([0-9]{4})/([0-9]{1,2})/([0-9]{1,2})$`)
)

// parseRow は、売買の行 rec を確かめて ImportRow にする（段階2）。
// 列の数が7でなければ、列の数の誤りだけを返す。そうでなければ、すべての列を確かめ、誤りを列の順に（1つの列では最初に当たった1つだけ）返す。
// 誤りがなければ、ImportRow（Line は rec.Line）を返す。誤りがあるときの ImportRow は使えない。
// 3つ目の戻り値は、確かめを通ったのに ImportRow を組み立てられないという、起こらないはずの失敗。
func parseRow(rec record, today string) (command.ImportRow, []Problem, error) {
	if len(rec.Fields) != columnCount {
		return command.ImportRow{}, []Problem{lineProblem(rec.Line, msgColumns)}, nil
	}
	f := rec.Fields

	var texts []string // 誤りの文言（「n行目: 」なし）
	add := func(text string) { texts = append(texts, text) }

	if err := model.CheckStockCode(f[colStockCode]); err != nil {
		add(msgStockCode)
	}
	if err := model.CheckStockName(f[colStockName]); err != nil {
		add(msgStockName)
	}
	currency, currencyOK := parseCurrency(f[colCurrency])
	if !currencyOK {
		add(msgCurrency)
	}
	side, sideOK := parseSide(f[colSide])
	if !sideOK {
		add(msgSide)
	}
	priceHundredths, priceText := parsePrice(f[colPrice], currency, currencyOK)
	if priceText != "" {
		add(priceText)
	}
	quantity, quantityText := parseQuantity(f[colQuantity])
	if quantityText != "" {
		add(quantityText)
	}
	tradeDate, dateText := parseTradeDate(f[colTradeDate], today)
	if dateText != "" {
		add(dateText)
	}

	if len(texts) > 0 {
		problems := make([]Problem, 0, len(texts))
		for _, text := range texts {
			problems = append(problems, lineProblem(rec.Line, text))
		}
		return command.ImportRow{}, problems, nil
	}

	stock, err := model.NewStock(f[colStockCode], f[colStockName], currency)
	if err != nil {
		return command.ImportRow{}, nil, fmt.Errorf("%d行目の銘柄を作れません: %w", rec.Line, err)
	}
	trade, err := model.NewTrade(stock, side, quantity, priceHundredths, tradeDate, today)
	if err != nil {
		return command.ImportRow{}, nil, fmt.Errorf("%d行目の売買を作れません: %w", rec.Line, err)
	}
	return command.ImportRow{Line: rec.Line, Stock: stock, Trade: trade}, nil, nil
}

// parseCurrency は、通貨の列を読む。JPY か USD だけ（小文字や空は誤り）。
func parseCurrency(s string) (model.Currency, bool) {
	switch c := model.Currency(s); c {
	case model.CurrencyJPY, model.CurrencyUSD:
		return c, true
	default:
		return "", false
	}
}

// parseSide は、売買の列を読む。「買」か「売」だけ。
func parseSide(s string) (model.TradeSide, bool) {
	switch s {
	case sideBuyText:
		return model.TradeSideBuy, true
	case sideSellText:
		return model.TradeSideSell, true
	default:
		return "", false
	}
}

// parsePrice は、価格の列を読んで 0.01 単位の整数にする。誤りなら、2つ目の戻り値に文言を返す（1つの列では最初に当たった1つだけ）。
// 書き方 → 0以下 → 1億以上 → 通貨の精度の順に確かめる。精度は通貨が正しいとき（currencyOK）だけ確かめる。
func parsePrice(s string, currency model.Currency, currencyOK bool) (int64, string) {
	if !priceFormat.MatchString(s) {
		return 0, msgPriceFormat
	}
	price, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, msgPriceFormat
	}
	if err := model.CheckPrice(price); err != nil {
		if errors.Is(err, model.ErrPriceNotPositive) {
			return 0, msgPriceNotPos
		}
		return 0, msgPriceTooLarge
	}
	if !currencyOK {
		return 0, ""
	}
	hundredths, err := currency.PriceHundredths(price)
	if err != nil {
		if currency == model.CurrencyJPY {
			return 0, msgPriceYen
		}
		return 0, msgPriceDollar
	}
	return hundredths, ""
}

// parseQuantity は、株数の列を読む。誤りなら、2つ目の戻り値に文言を返す。
// 半角数字だけで int64 に収まらない値は、「1,000万株未満」の誤りにする。
func parseQuantity(s string) (int64, string) {
	if !quantityFormat.MatchString(s) {
		return 0, msgQtyFormat
	}
	q, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, msgQtyTooLarge
		}
		return 0, msgQtyFormat
	}
	if err := model.CheckQuantity(q); err != nil {
		if errors.Is(err, model.ErrQuantityNotPositive) {
			return 0, msgQtyFormat
		}
		return 0, msgQtyTooLarge
	}
	return q, ""
}

// parseTradeDate は、取引日の列を読んで "YYYY-MM-DD" にそろえる。誤りなら、2つ目の戻り値に文言を返す。
// 書き方と実在する日付かを確かめてから、今日（today）より後でないかを確かめる。
func parseTradeDate(s, today string) (string, string) {
	m := hyphenDateFormat.FindStringSubmatch(s)
	if m == nil {
		m = slashDateFormat.FindStringSubmatch(s)
	}
	if m == nil {
		return "", msgDateFormat
	}
	// 桁は正規表現が4桁・1〜2桁に限っているので、Atoi は失敗しない。
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	day, _ := strconv.Atoi(m[3])
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	// time.Date は 2月30日を3月2日などに直すので、直されていない（実在する日付）ことを確かめる。年は1以上（DB の取引日の範囲）。
	if year < 1 || date.Year() != year || int(date.Month()) != month || date.Day() != day {
		return "", msgDateFormat
	}
	normalized := date.Format(time.DateOnly)
	if err := model.CheckTradeDate(normalized, today); err != nil {
		return "", msgDateFuture
	}
	return normalized, ""
}
