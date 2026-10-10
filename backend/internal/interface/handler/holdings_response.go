package handler

import (
	"strconv"
	"strings"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// holdingsResponse は、GET /api/holdings の本文（プラン 4.4「API」）。
type holdingsResponse struct {
	Holdings              []holdingResponse `json:"holdings"`
	TotalValuation        int64             `json:"totalValuation"`
	TotalExcludesUnpriced bool              `json:"totalExcludesUnpriced"`
}

// holdingResponse は、一覧の1行。価格のない行は currentPrice・priceDate・valuation を null にする。
// 単価は文字列にする。JSON の数にすると、受け取る側（JavaScript）が浮動小数点に変換して、誤差が入るおそれがあるため（判断1）。
type holdingResponse struct {
	Code             string  `json:"code"`
	Name             string  `json:"name"`
	Quantity         int64   `json:"quantity"`
	AcquisitionPrice string  `json:"acquisitionPrice"`
	CurrentPrice     *string `json:"currentPrice"`
	PriceDate        *string `json:"priceDate"`
	Valuation        *int64  `json:"valuation"`
}

// newHoldingsResponse は、一覧の Read Model を API の本文の形にする。
func newHoldingsResponse(list dto.HoldingsList) holdingsResponse {
	// 0件でも holdings を null ではなく [] にするため、nil ではないスライスから始める。
	holdings := make([]holdingResponse, 0, len(list.Holdings))
	for _, h := range list.Holdings {
		row := holdingResponse{
			Code:             h.Code,
			Name:             h.Name,
			Quantity:         h.Quantity,
			AcquisitionPrice: formatTenths(h.AcquisitionPriceTenths),
			PriceDate:        h.PriceDate,
			Valuation:        h.Valuation,
		}
		if h.CurrentPriceTenths != nil {
			row.CurrentPrice = new(formatTenths(*h.CurrentPriceTenths))
		}
		holdings = append(holdings, row)
	}
	return holdingsResponse{
		Holdings:              holdings,
		TotalValuation:        list.TotalValuation,
		TotalExcludesUnpriced: list.TotalExcludesUnpriced,
	}
}

// formatTenths は、0.1円単位の整数を、小数第1位を必ず1桁付けた円の文字列にする（例: 25000 → "2500.0"、5 → "0.5"）。
// 小数第1位が0でも省かないのは、API の単価の形を1つ（^[0-9]+\.[0-9]$）にして、表示の整え方は画面に任せるため（プラン 4.4）。
// 浮動小数点を通さずに、整数の10進の文字列の最後の桁の前に小数点を入れて作る。
func formatTenths(tenths int64) string {
	digits := strconv.FormatInt(tenths, 10)
	sign := ""
	if rest, ok := strings.CutPrefix(digits, "-"); ok {
		sign, digits = "-", rest
	}
	if len(digits) == 1 {
		digits = "0" + digits
	}
	last := len(digits) - 1
	return sign + digits[:last] + "." + digits[last:]
}
