package command

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
)

// FetchResult は、価格の取得処理の結果。
type FetchResult struct {
	Saved  []string // 終値を保存できた銘柄コード（昇順）
	Failed []string // 終値を取得できなかった、または保存できなかった銘柄コード（昇順）
}

// FetchClosingPrices は、保有されている全銘柄の終値を取得元から取得して保存するユースケース（要件 FR-15、FR-16）。
// 18:00 の定時実行（cmd/api のスケジューラ）と、手動実行（cmd/fetch-prices。task prices:fetch）が同じこれを使う（FR-15a）。
type FetchClosingPrices struct {
	source   PriceSource
	holdings repository.HoldingRepository
	prices   repository.ClosingPriceRepository
}

// NewFetchClosingPrices は、src から終値を取得し、holdings の保有銘柄について、prices に保存する FetchClosingPrices を返す。
// 取得元は引数で受け取る。取得元を差し替えても、このユースケースのコードは変えずに済むようにするため（要件 NFR-1）。
func NewFetchClosingPrices(src PriceSource, holdings repository.HoldingRepository, prices repository.ClosingPriceRepository) *FetchClosingPrices {
	return &FetchClosingPrices{source: src, holdings: holdings, prices: prices}
}

// Execute は、保有されている銘柄ごとに、取得元から終値を取得して保存する（プラン 4.4「株価取得の決まり」）。
//   - 1銘柄の取得や保存に失敗しても止めずに、その銘柄を Failed に入れて次の銘柄へ進む（FR-16）。
//     取得元の panic も、その銘柄の失敗として扱う。失敗した銘柄の保存済みの価格には手を付けないので、そのまま残る。
//   - ctx が取り消されたら、残りの銘柄は取得せずに Failed に入れて返す（エラーは返さない。4.4 決まり4）。
//     取り消されたかどうかは、呼び出し側が ctx で分かるため。
//   - エラーを返すのは、保有されている銘柄を読めなかったときだけ（どの銘柄を取得すればよいか分からないため）。
//   - Saved・Failed は ListHeldStockCodes の順（銘柄コードの昇順）に並ぶ。
//
// 失敗の理由は FetchResult に入らないので、銘柄ごとにログに出す（取得元にない銘柄なのか、DB の障害なのかを後から見分けるため）。
// 取得元が panic したときは、そのスタックも別の行でログに出す（fetchFromSource）。
func (c *FetchClosingPrices) Execute(ctx context.Context) (FetchResult, error) {
	codes, err := c.holdings.ListHeldStockCodes(ctx)
	if err != nil {
		return FetchResult{}, fmt.Errorf("保有されている銘柄を読めません: %w", err)
	}

	var result FetchResult
	for _, code := range codes {
		if err := c.fetchAndSave(ctx, code); err != nil {
			// 銘柄コード（DB の値）とエラーの文字列（取得元が作る）は、どちらも改行などを含みうるので %q で出す。
			// そのまま出すと、偽のログ行を作られたり、1件の失敗が複数行に割れて読み違えたりするため（PR③ ステップ6 の S-1-2・S-2-2）。
			log.Printf("株価取得: 銘柄 %q: %q", code, err)
			result.Failed = append(result.Failed, code)
			continue
		}
		result.Saved = append(result.Saved, code)
	}
	return result, nil
}

// fetchAndSave は、銘柄 code の終値を取得元から取得して保存する。
func (c *FetchClosingPrices) fetchAndSave(ctx context.Context, code string) error {
	// 取り消し（Ctrl+C、API サーバーの停止、定時実行の時間の上限）の後は、取得元を呼ばない。
	// 取得元が ctx を見ない実装（ダミーなど）でも、取り消しの後に残りの銘柄の取得・保存を続けないため（4.4 決まり4）。
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("取り消されたので取得しません: %w", err)
	}
	fetched, err := c.fetchFromSource(ctx, code)
	if err != nil {
		return fmt.Errorf("終値を取得できません: %w", err)
	}
	// 取得元が別の銘柄の終値を返したら、保存しない。保存すると、その別の銘柄の価格を黙って書き換えてしまうため。
	if fetched.StockCode != code {
		return fmt.Errorf("取得元が別の銘柄（%q）の終値を返しました", fetched.StockCode)
	}
	// ClosingPrice のフィールドは外から書けるので、取得元の実装が model.NewClosingPrice を通さずに作った値かもしれない。
	// 保存する前に domain の決まり（終値は0より大きい、など）で確かめ直す（Command 側は domain の決まりを通して状態を変える）。
	p, err := model.NewClosingPrice(fetched.StockCode, fetched.TradingDate, fetched.PriceTenths)
	if err != nil {
		return fmt.Errorf("取得元が返した終値が正しくありません: %w", err)
	}
	if err := c.prices.Save(ctx, p); err != nil {
		return fmt.Errorf("終値を保存できません: %w", err)
	}
	return nil
}

// fetchFromSource は、取得元から銘柄 code の終値を取得する。取得元が panic したら、その panic をエラーにして返す。
// 取得元は差し替えられる外の実装なので、その誤り（panic）で残りの銘柄の取得まで止めないため（FR-16、4.4 決まり2）。
// panic しないことは PriceSource の約束だが、約束を破る実装でも、ほかの銘柄は続ける。
func (c *FetchClosingPrices) fetchFromSource(ctx context.Context, code string) (p model.ClosingPrice, err error) {
	defer func() {
		if r := recover(); r != nil {
			// 取得元のどこで panic したかを後から直せるよう、スタックをログに残す（PR③ ステップ6 の EC-P2-8）。
			// recover した場所（panic している途中の deferred 関数の中）で取らないと、panic した箇所がスタックに残らない。
			// スタックは複数行なので、銘柄の失敗のログ（Execute が1行で出す）とは別の行に出す。
			// 銘柄コードは DB の値なので %q で出す。スタックは関数名・ファイルの位置と、引数を16進の数で表したものだけで、
			// 文字列の中身（DB の値や取得元の応答）を含まないので、読めるようにそのまま出す。
			log.Printf("株価取得: 銘柄 %q: 取得元が panic しました。スタック:\n%s", code, debug.Stack())
			err = fmt.Errorf("取得元が panic しました: %v", r)
		}
	}()
	return c.source.FetchClosingPrice(ctx, code)
}
