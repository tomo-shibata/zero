package query

import "github.com/tomo-shibata/zero/backend/internal/application/query/dto"

// ApplyHoldingOrder は、holdings（銘柄コードの昇順）を、保存した並び order の順に並べ直した新しいスライスを返す（要件26・要件27。プラン 4.3「読む」）。
//  1. order にある銘柄を、その順に先に置く。
//  2. order にない銘柄（新しく出てきた銘柄。一度も保存していなければ全部）を、その後ろに、holdings の順（銘柄コードの昇順）で置く。
//  3. order にあって holdings にない銘柄は無視する。
//
// holdings は変えない（行の中身はそのまま運ぶ）。0件のときも nil ではない空のスライスを返す（API で null にしないため）。
func ApplyHoldingOrder(holdings []dto.Holding, order []string) []dto.Holding {
	byCode := make(map[string]dto.Holding, len(holdings))
	for _, h := range holdings {
		byCode[h.Code] = h
	}
	ordered := make([]dto.Holding, 0, len(holdings))
	placed := make(map[string]bool, len(holdings))
	for _, code := range order {
		h, ok := byCode[code]
		if !ok || placed[code] {
			continue
		}
		placed[code] = true
		ordered = append(ordered, h)
	}
	for _, h := range holdings {
		if !placed[h.Code] {
			ordered = append(ordered, h)
		}
	}
	return ordered
}
