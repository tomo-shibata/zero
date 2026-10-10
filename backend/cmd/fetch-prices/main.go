// Command fetch-prices は、保有されている全銘柄の終値を取得元から取得して DATABASE_URL の DB に保存する処理を、1回だけ実行する
// （要件 FR-15a。task prices:fetch で実行する）。cmd/api が毎日 18:00（日本時間）に実行するのと同じ処理（command.FetchClosingPrices）。
//
// 終了コードが 1 になるのは、保有されている銘柄を読めなかったとき（価格の取得処理がエラーを返したとき）、
// その前の設定の読み込みや DB への接続に失敗したとき、Ctrl+C などで途中で止めたときだけ。
// 取得・保存できなかった銘柄があっても、その銘柄をログに出して 0 で終わる
// （1銘柄の失敗で他の銘柄の取得を止めない要件 FR-16 の結果として、正常な終わり方なので。プラン 4.4「株価取得の決まり」5）。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/config"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/database"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/pricesource"
)

func main() {
	if err := run(); err != nil {
		log.Printf("fetch-prices: %v", err)
		os.Exit(1)
	}
}

func run() error {
	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		return err
	}
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("DATABASE_URL を読めません: %w", err)
	}
	// Taskfile は DATABASE_URL（パスワードを含む）を端末に出さないので、どの DB に保存するかはここで示す（cmd/migrate と同じ）。
	// パスワードを出さないよう、接続文字列ではなくホスト・ポート・DB 名だけを出す。
	cc := poolConfig.ConnConfig
	log.Printf("fetch-prices: 接続先: ホスト %s、ポート %d、DB %q", cc.Host, cc.Port, cc.Database)

	// Ctrl+C で止めたときは、取得中・保存中の銘柄の処理を取り消して早めに終わる（それまでに保存した銘柄は残る）。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.ConnectConfig(ctx, poolConfig)
	if err != nil {
		return err
	}
	defer pool.Close()

	result, err := newFetchClosingPrices(pool).Execute(ctx)
	if err != nil {
		return err
	}
	// 銘柄コードは DB の値で改行などを含みうるので、偽のログ行を作られないよう %q で出す（価格の取得処理のログと同じ）。
	log.Printf("fetch-prices: 保存した銘柄: %q", result.Saved)
	if len(result.Failed) > 0 {
		// 失敗の理由は、価格の取得処理が銘柄ごとにログに出している。
		log.Printf("fetch-prices: 取得・保存できなかった銘柄（保存済みの価格はそのまま）: %q", result.Failed)
	}
	// 取り消されても Execute はエラーを返さず、残りの銘柄を Failed に入れて返す（プラン 4.4「株価取得の決まり」4）。
	// 途中で止めた実行を、すべての銘柄を試し終えた実行と終了コードで見分けられるよう、ここで失敗にする（決まり5）。
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("途中で止めました（止めた後の銘柄は取得していません）: %w", err)
	}
	return nil
}

// newFetchClosingPrices は、pool の DB を使う価格の取得処理を組み立てる。
// 取得元を差し替えるときは、ここと cmd/api の newFetchClosingPrices の取得元だけを変える（要件 NFR-1、プラン 6章の判断4）。
// 当面の取得元はダミー（要件 NFR-2）。
func newFetchClosingPrices(pool *pgxpool.Pool) *command.FetchClosingPrices {
	return command.NewFetchClosingPrices(
		pricesource.NewDummyPriceSource(),
		write.NewHoldingRepository(pool),
		write.NewClosingPriceRepository(pool),
	)
}
