// Package command は、Command 側（状態を変える）のユースケースと、ユースケースが使う外部の IF を持つ。
// 状態は domain のモデルとリポジトリの IF を通して変える（軽量 CQRS）。
package command

import (
	"context"
	"errors"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// ErrPriceUnavailable は、取得元がその銘柄の終値を返せないこと（取得元に定義がない、など）を表す。
// 取得元の実装は、この値を（%w で包んでもよい）返す。
var ErrPriceUnavailable = errors.New("price unavailable")

// PriceSource は、株価の取得元の共通の IF（要件 NFR-1）。
//
// IF をここ（application）に置き、実装（ダミーや、将来の本番の取得元）は infrastructure（pricesource）に置く。
// 取得元を差し替えるときは、新しい実装を足して、組み立てる箇所（cmd/api と cmd/fetch-prices）を変えるだけで済むようにするため
// （価格の取得処理 FetchClosingPrices と、保存の処理は変えない。プラン 6章の判断4）。
//
// 実装する側の約束（取得元を差し替える人へ。プラン 4.4）:
//   - 返す ClosingPrice は model.NewClosingPrice で作り、StockCode は引数 stockCode と同じにする。
//     違えば、その銘柄は Failed になる（別の銘柄の価格を黙って書き換えないため）。
//   - 値がない銘柄は ErrPriceUnavailable（を包んだエラー）を返す。panic しない
//     （FetchClosingPrices は panic をその銘柄の失敗として扱って続けるが、それは約束を破る実装への備えなので、頼らない）。
//   - 返すエラーに、認証情報（トークン・API キー）、それを含む URL、取得元の応答の本文を入れない。
//     エラーは銘柄ごとにログに出るので、認証情報が漏れたり、応答の本文で偽のログ行を作られたりしないため。
//   - stockCode を URL に入れるときは url.PathEscape・url.Values でエンコードし、文字列の連結で作らない。
//     銘柄コードは DB の値で、/ や ? などを含みうるので、別のパスや問い合わせに化けさせないため。
//   - ctx を通信に渡し（http.NewRequestWithContext）、ctx が取り消されたらすぐに戻る。
//     http.Client には Timeout を設定し、http.DefaultClient（Timeout がない）は使わない（PR③ ステップ6 の S-2-4）。
//     FetchClosingPrices が取り消しを見るのは銘柄と銘柄の間だけで、取得元の呼び出しの途中では止められない。
//     取得元が ctx を見ないと、応答しない取得元を待ち続け、定時実行の1時間の上限（プラン 6章の判断18）や
//     API サーバーの停止、Ctrl+C が効かなくなるため。Timeout は、ctx に期限がないとき（手動実行 cmd/fetch-prices）にも、
//     1銘柄の応答を待ち続けて残りの銘柄の取得まで止めないため。
//   - http.Client.Do などが返す *url.Error は、エラーの文字列に要求の URL（問い合わせの部分も含む）を含むので、
//     そのまま包まない（%w・%v で返さない）。net/http が伏せるのは URL のパスワード（user:pass@ の部分）だけなので、
//     URL に認証情報があると、エラーを通して銘柄ごとのログに出てしまうため（PR③ ステップ6 の S-2-5）。
//     同じ理由で、認証情報は URL（問い合わせの部分）ではなく、要求のヘッダーで渡す。
type PriceSource interface {
	// FetchClosingPrice は、銘柄 stockCode の最新の終値と、その取引日を返す。
	// 返せないときは ErrPriceUnavailable か、取得に失敗した理由のエラーを返す。
	FetchClosingPrice(ctx context.Context, stockCode string) (model.ClosingPrice, error)
}
