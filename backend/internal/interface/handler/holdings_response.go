package handler

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// 金額の整数（0.01 単位）を、通貨ごとの文字列にするときの単位と小数の桁数。
const (
	// yenUnitPriceStepHundredths は、円の単価の最小の単位 0.1円（0.01 単位で10）。円の単価はこの倍数でなければならない。
	yenUnitPriceStepHundredths = 10
	// yenValuationStepHundredths は、円の評価額の最小の単位 1円（0.01 単位で100）。円の評価額はこの倍数でなければならない。
	yenValuationStepHundredths = 100
	// yenUnitPriceDecimals・usdDecimals は、単価の文字列の小数の桁数（円は 0.1円まで、ドルは 0.01ドルまで）。
	yenUnitPriceDecimals = 1
	usdDecimals          = 2
	// rateDecimals は、含み損益の率の文字列の小数の桁数（0.1% まで）。
	rateDecimals = 1
)

// 値そのものは資産の額なので、エラー（ログに出る）には入れない。
var (
	errYenUnitPriceNotTenth = errors.New("円の単価が 0.1円で割り切れません")
	errYenValuationNotWhole = errors.New("円の評価額が1円で割り切れません")
	errYenGainNotWhole      = errors.New("円の含み損益が1円で割り切れません")
)

// holdingsResponse は、GET /api/holdings の本文（売買履歴のプラン 4.3「一覧の API の形」）。
type holdingsResponse struct {
	Holdings []holdingResponse       `json:"holdings"`
	Totals   []currencyTotalResponse `json:"totals"`
	// CombinedTotal は、円にまとめた合計。切り替えを出さないとき（ドルの保有がない・レートがない。ドルを円にまとめる合計の要件2）は null。
	// キーは常にある。
	CombinedTotal *combinedTotalResponse `json:"combinedTotal"`
	// ManagedAccounts は、おまかせ運用の口座のカードの値（名前の順）。キーは常にあり、口座がなければ [] （null にしない）。
	ManagedAccounts []managedAccountResponse `json:"managedAccounts"`
}

// managedAccountResponse は、おまかせ運用の口座1つ（おまかせ運用のプラン 4.4）。
// valuation・principal・unrealizedGain・unrealizedGainRate の形は、円の通貨ごとの合計と同じ（円の整数・符号付きの整数・符号付きの小数1桁）。
type managedAccountResponse struct {
	Name               string `json:"name"`
	Valuation          string `json:"valuation"`
	Principal          string `json:"principal"`
	UnrealizedGain     string `json:"unrealizedGain"`
	UnrealizedGainRate string `json:"unrealizedGainRate"`
	ValuationDate      string `json:"valuationDate"`
}

// combinedTotalResponse は、円にまとめた合計（ドルを円にまとめる合計のプラン 4.4）。
// valuation・unrealizedGain・unrealizedGainRate の形は、円の通貨ごとの合計と同じ（円の整数・符号付きの整数・符号付きの小数1桁）。
// unrealizedGain と unrealizedGainRate は、両方とも null か両方とも文字列。usdJpyRate は小数2桁の文字列（3桁区切りは画面が付ける）。
type combinedTotalResponse struct {
	Valuation          string  `json:"valuation"`
	ExcludesUnpriced   bool    `json:"excludesUnpriced"`
	UnrealizedGain     *string `json:"unrealizedGain"`
	UnrealizedGainRate *string `json:"unrealizedGainRate"`
	UsdJpyRate         string  `json:"usdJpyRate"`
	UsdJpyRateDate     string  `json:"usdJpyRateDate"`
}

// holdingResponse は、一覧の1行。価格のない行は currentPrice・priceDate・valuation・unrealizedGain・unrealizedGainRate を null にする。
// 金額（単価・評価額・含み損益）は文字列にする。JSON の数にすると、受け取る側（JavaScript）が浮動小数点に変換して、誤差が入るおそれがあるため
// （売買履歴のプラン 6章の判断7）。単価は円が小数1桁（"2500.0"）・ドルが小数2桁（"150.25"）、
// 評価額は円が整数（"450000"）・ドルが小数2桁（"1800.00"）、
// 含み損益は円が符号付きの整数（"-501"）・ドルが符号付きの小数2桁（"-0.01"）、率は符号付きの小数1桁（"-12.5"）
// （カード表示のプラン 4.2。プラスに "+" は付けない）。
type holdingResponse struct {
	Code               string  `json:"code"`
	Name               string  `json:"name"`
	Currency           string  `json:"currency"`
	Quantity           int64   `json:"quantity"`
	AcquisitionPrice   string  `json:"acquisitionPrice"`
	CurrentPrice       *string `json:"currentPrice"`
	PriceDate          *string `json:"priceDate"`
	Valuation          *string `json:"valuation"`
	UnrealizedGain     *string `json:"unrealizedGain"`
	UnrealizedGainRate *string `json:"unrealizedGainRate"`
}

// currencyTotalResponse は、1つの通貨の合計評価額と含み損益の合計。valuation・unrealizedGain・unrealizedGainRate の形は一覧の行と同じ。
// 価格のある銘柄がない通貨は、unrealizedGain・unrealizedGainRate を null にする。
type currencyTotalResponse struct {
	Currency           string  `json:"currency"`
	Valuation          string  `json:"valuation"`
	ExcludesUnpriced   bool    `json:"excludesUnpriced"`
	UnrealizedGain     *string `json:"unrealizedGain"`
	UnrealizedGainRate *string `json:"unrealizedGainRate"`
}

// newHoldingsResponse は、一覧の Read Model を API の本文の形にする。
// 円の単価（取得価格・現在の価格）が 0.1円で割り切れない（0.01 単位で10の倍数でない）ときは、黙って切り捨てずにエラーにする
// （SQL で円の終値 3000.25 を入れた、など。DB の CHECK は円の精度を確かめられない。売買履歴のプラン 4.7）。
func newHoldingsResponse(list dto.HoldingsList) (holdingsResponse, error) {
	// 0件でも holdings・totals を null ではなく [] にするため、nil ではないスライスから始める。
	holdings := make([]holdingResponse, 0, len(list.Holdings))
	for _, h := range list.Holdings {
		row, err := newHoldingResponse(h)
		if err != nil {
			return holdingsResponse{}, err
		}
		holdings = append(holdings, row)
	}

	totals := make([]currencyTotalResponse, 0, len(list.Totals))
	for _, t := range list.Totals {
		valuation, err := formatValuation(t.Currency, t.ValuationHundredths)
		if err != nil {
			return holdingsResponse{}, fmt.Errorf("%s の合計評価額: %w", t.Currency, err)
		}
		gain, rate, err := formatGainAndRate(t.Currency, t.UnrealizedGainHundredths, t.UnrealizedGainRateTenths)
		if err != nil {
			return holdingsResponse{}, fmt.Errorf("%s の含み損益の合計: %w", t.Currency, err)
		}
		totals = append(totals, currencyTotalResponse{
			Currency:           string(t.Currency),
			Valuation:          valuation,
			ExcludesUnpriced:   t.ExcludesUnpriced,
			UnrealizedGain:     gain,
			UnrealizedGainRate: rate,
		})
	}
	combined, err := newCombinedTotalResponse(list.CombinedTotal)
	if err != nil {
		return holdingsResponse{}, err
	}
	accounts, err := newManagedAccountResponses(list.ManagedAccounts)
	if err != nil {
		return holdingsResponse{}, err
	}
	return holdingsResponse{Holdings: holdings, Totals: totals, CombinedTotal: combined, ManagedAccounts: accounts}, nil
}

// newManagedAccountResponses は、おまかせ運用の口座のカードの値 accounts を API の本文の形にする。0件でも nil ではなく空のスライスを返す。
// 金額は円の形にする（100の倍数でなければ、黙って切り捨てずにエラー）。
func newManagedAccountResponses(accounts []dto.ManagedAccount) ([]managedAccountResponse, error) {
	result := make([]managedAccountResponse, 0, len(accounts))
	for _, a := range accounts {
		valuation, err := formatValuation(dto.CurrencyJPY, a.ValuationHundredths)
		if err != nil {
			return nil, fmt.Errorf("口座 %q の評価額: %w", a.Name, err)
		}
		principal, err := formatValuation(dto.CurrencyJPY, a.PrincipalHundredths)
		if err != nil {
			return nil, fmt.Errorf("口座 %q の元本: %w", a.Name, err)
		}
		gain, err := formatUnrealizedGain(dto.CurrencyJPY, a.UnrealizedGainHundredths)
		if err != nil {
			return nil, fmt.Errorf("口座 %q の含み損益: %w", a.Name, err)
		}
		result = append(result, managedAccountResponse{
			Name:               a.Name,
			Valuation:          valuation,
			Principal:          principal,
			UnrealizedGain:     gain,
			UnrealizedGainRate: formatGainRate(a.UnrealizedGainRateTenths),
			ValuationDate:      a.ValuationDate,
		})
	}
	return result, nil
}

// newCombinedTotalResponse は、円にまとめた合計 c を API の本文の形にする。c が nil なら nil（本文では null）。
// 評価額と含み損益は円の形にする（100の倍数でなければエラー）。レートは小数2桁の文字列にする。
func newCombinedTotalResponse(c *dto.CombinedTotal) (*combinedTotalResponse, error) {
	if c == nil {
		return nil, nil
	}
	valuation, err := formatValuation(dto.CurrencyJPY, c.ValuationHundredths)
	if err != nil {
		return nil, fmt.Errorf("円にまとめた評価額: %w", err)
	}
	gain, rate, err := formatGainAndRate(dto.CurrencyJPY, c.UnrealizedGainHundredths, c.UnrealizedGainRateTenths)
	if err != nil {
		return nil, fmt.Errorf("円にまとめた含み損益: %w", err)
	}
	return &combinedTotalResponse{
		Valuation:          valuation,
		ExcludesUnpriced:   c.ExcludesUnpriced,
		UnrealizedGain:     gain,
		UnrealizedGainRate: rate,
		UsdJpyRate:         formatDecimal(c.Rate.RateHundredths, usdDecimals),
		UsdJpyRateDate:     c.Rate.RateDate,
	}, nil
}

// newHoldingResponse は、一覧の1行 h を API の本文の形にする。取得価格、現在の価格、評価額の順に変換し、最初のエラーを返す。
func newHoldingResponse(h dto.Holding) (holdingResponse, error) {
	acquisition, err := formatUnitPrice(h.Currency, h.AcquisitionPriceHundredths)
	if err != nil {
		return holdingResponse{}, fmt.Errorf("銘柄 %q の取得価格: %w", h.Code, err)
	}
	row := holdingResponse{
		Code:             h.Code,
		Name:             h.Name,
		Currency:         string(h.Currency),
		Quantity:         h.Quantity,
		AcquisitionPrice: acquisition,
		PriceDate:        h.PriceDate,
	}
	if h.CurrentPriceHundredths != nil {
		current, err := formatUnitPrice(h.Currency, *h.CurrentPriceHundredths)
		if err != nil {
			return holdingResponse{}, fmt.Errorf("銘柄 %q の現在の価格: %w", h.Code, err)
		}
		row.CurrentPrice = &current
	}
	if h.ValuationHundredths != nil {
		valuation, err := formatValuation(h.Currency, *h.ValuationHundredths)
		if err != nil {
			return holdingResponse{}, fmt.Errorf("銘柄 %q の評価額: %w", h.Code, err)
		}
		row.Valuation = &valuation
	}
	gain, rate, err := formatGainAndRate(h.Currency, h.UnrealizedGainHundredths, h.UnrealizedGainRateTenths)
	if err != nil {
		return holdingResponse{}, fmt.Errorf("銘柄 %q の含み損益: %w", h.Code, err)
	}
	row.UnrealizedGain, row.UnrealizedGainRate = gain, rate
	return row, nil
}

// formatGainAndRate は、含み損益 gain（0.01 単位）と率 rate（0.1% 単位）を、API の文字列にする。
// どちらも nil（価格がない）なら、どちらも nil を返す。
func formatGainAndRate(currency dto.Currency, gain, rate *int64) (gainText, rateText *string, err error) {
	if gain == nil || rate == nil {
		return nil, nil, nil
	}
	g, err := formatUnrealizedGain(currency, *gain)
	if err != nil {
		return nil, nil, err
	}
	r := formatGainRate(*rate)
	return &g, &r, nil
}

// formatUnitPrice は、0.01 単位の単価を、通貨の単価の形の文字列にする。
// 円は小数第1位を必ず1桁付ける（例: 250000 → "2500.0"、5 は 0.05円なのでエラー）。ドルは小数第2位を必ず2桁付ける（例: 15025 → "150.25"）。
// 小数の末尾が0でも省かないのは、API の単価の形を通貨ごとに1つ（円 ^[0-9]+\.[0-9]$・ドル ^[0-9]+\.[0-9]{2}$）にして、
// 表示の整え方は画面に任せるため。
func formatUnitPrice(currency dto.Currency, hundredths int64) (string, error) {
	switch currency {
	case dto.CurrencyJPY:
		if hundredths%yenUnitPriceStepHundredths != 0 {
			return "", errYenUnitPriceNotTenth
		}
		return formatDecimal(hundredths/yenUnitPriceStepHundredths, yenUnitPriceDecimals), nil
	case dto.CurrencyUSD:
		return formatDecimal(hundredths, usdDecimals), nil
	default:
		return "", fmt.Errorf("知らない通貨 %q です", currency)
	}
}

// formatValuation は、0.01 単位の評価額を、通貨の評価額の形の文字列にする。
// 円は整数（例: 45000000 → "450000"。評価額は1円未満を切り捨てた値なので100の倍数。そうでなければエラー）、
// ドルは小数第2位を必ず2桁付ける（例: 180000 → "1800.00"）。
func formatValuation(currency dto.Currency, hundredths int64) (string, error) {
	switch currency {
	case dto.CurrencyJPY:
		if hundredths%yenValuationStepHundredths != 0 {
			return "", errYenValuationNotWhole
		}
		return strconv.FormatInt(hundredths/yenValuationStepHundredths, 10), nil
	case dto.CurrencyUSD:
		return formatDecimal(hundredths, usdDecimals), nil
	default:
		return "", fmt.Errorf("知らない通貨 %q です", currency)
	}
}

// formatUnrealizedGain は、0.01 単位の含み損益を、通貨の含み損益の形の文字列にする（カード表示のプラン 4.2）。
// 円は符号付きの整数（例: -50100 → "-501"。表示する含み損益は1円未満を四捨五入した値なので100の倍数。そうでなければエラー）、
// ドルは符号付きで小数第2位を必ず2桁付ける（例: -1 → "-0.01"）。プラスに "+" は付けない（符号は画面が付ける）。
func formatUnrealizedGain(currency dto.Currency, hundredths int64) (string, error) {
	switch currency {
	case dto.CurrencyJPY:
		if hundredths%yenValuationStepHundredths != 0 {
			return "", errYenGainNotWhole
		}
		return strconv.FormatInt(hundredths/yenValuationStepHundredths, 10), nil
	case dto.CurrencyUSD:
		return formatDecimal(hundredths, usdDecimals), nil
	default:
		return "", fmt.Errorf("知らない通貨 %q です", currency)
	}
}

// formatGainRate は、0.1% 単位の率を、符号付きの小数1桁の文字列にする（例: -125 → "-12.5"、-5 → "-0.5"、0 → "0.0"）。
// 0 は符号なしの "0.0" になる（"-0.0" は出ない）。
func formatGainRate(tenths int64) string {
	return formatDecimal(tenths, rateDecimals)
}

// formatDecimal は、整数 units を、末尾から decimals 桁を小数部にした文字列にする（例: units 25000・decimals 1 → "2500.0"、5・1 → "0.5"、5・2 → "0.05"）。
// 浮動小数点を通さずに、整数の10進の文字列の途中に小数点を入れて作る。
func formatDecimal(units int64, decimals int) string {
	digits := strconv.FormatInt(units, 10)
	sign := ""
	if rest, ok := strings.CutPrefix(digits, "-"); ok {
		sign, digits = "-", rest
	}
	// 整数部が少なくとも1桁（0）になるまで、先頭に0を足す。
	if len(digits) <= decimals {
		digits = strings.Repeat("0", decimals+1-len(digits)) + digits
	}
	point := len(digits) - decimals
	return sign + digits[:point] + "." + digits[point:]
}
