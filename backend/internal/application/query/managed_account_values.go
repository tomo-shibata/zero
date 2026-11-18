package query

import (
	"fmt"
	"math/big"
	"slices"
	"strings"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
)

// buildManagedAccounts は、口座ごとの最新の記録 accounts から、カードの値を名前のバイトの昇順に作り、
// 円の途中の値 totals[JPY]（なければ作る）に口座を足す（おまかせ運用のプラン 4.3。判断2）。口座がなければ、空のスライスを返し、totals は変えない。
//
//   - 元本は評価額 − 損益、含み損益は損益そのまま、率は株式と同じ丸め（gainRateTenths。要件12）。
//   - 円の合計には、評価額の和に評価額を、含み損益の和に {損益・損益・元本} を足し、価格のある行がある（hasPriced）ことにする。
//     価格のない行の注記（excludesUnpriced）は変えない（要件15〜要件17）。
//
// 円の合計の評価額が int64 に収まらなければ ErrCalculationOverflow を返す。値そのものは資産の額なので、エラーには入れない。
func buildManagedAccounts(accounts []dto.ManagedAccountRow, totals map[dto.Currency]*currencyTotal) ([]dto.ManagedAccount, error) {
	result := make([]dto.ManagedAccount, 0, len(accounts))
	if len(accounts) == 0 {
		return result, nil
	}
	jpy := totals[dto.CurrencyJPY]
	if jpy == nil {
		jpy = newCurrencyTotal()
		totals[dto.CurrencyJPY] = jpy
	}
	for _, a := range accounts {
		card, err := managedAccountCard(a)
		if err != nil {
			return nil, fmt.Errorf("口座 %q: %w", a.Name, err)
		}
		result = append(result, card)

		gain := big.NewInt(a.GainHundredths)
		jpy.valuation.Add(jpy.valuation, big.NewInt(a.ValuationHundredths))
		jpy.hasPriced = true
		jpy.gain.add(&stockGain{
			gain:      new(big.Rat).SetInt(gain),
			cost:      new(big.Rat).SetInt64(card.PrincipalHundredths),
			displayed: gain,
		})
	}
	// 評価額は負にならないので、最後に確かめれば足す途中でも収まっている。
	if !jpy.valuation.IsInt64() {
		return nil, fmt.Errorf("口座を含めた円の合計評価額: %w", ErrCalculationOverflow)
	}
	// 名前のバイトの昇順（Go の文字列の比べ方。DB の照合順序に頼らない。要件9・判断6）。
	slices.SortFunc(result, func(a, b dto.ManagedAccount) int { return strings.Compare(a.Name, b.Name) })
	return result, nil
}

// managedAccountCard は、口座1つの最新の記録 a から、カードの値を作る（要件12）。
func managedAccountCard(a dto.ManagedAccountRow) (dto.ManagedAccount, error) {
	principal := a.ValuationHundredths - a.GainHundredths
	gain := big.NewInt(a.GainHundredths)
	rate, err := gainRateTenths(new(big.Rat).SetInt(gain), new(big.Rat).SetInt64(principal), gain)
	if err != nil {
		return dto.ManagedAccount{}, err
	}
	return dto.ManagedAccount{
		Name:                     a.Name,
		ValuationDate:            a.ValuationDate,
		ValuationHundredths:      a.ValuationHundredths,
		PrincipalHundredths:      principal,
		UnrealizedGainHundredths: a.GainHundredths,
		UnrealizedGainRateTenths: rate,
	}, nil
}
