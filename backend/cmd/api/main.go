// Command api は、HTTP API サーバーを起動する。依存を組み立ててルーターに渡す。
// あわせて、価格の取得処理を毎日 18:00（日本時間）に実行するスケジューラを起動する（PRICE_FETCH_SCHEDULE_ENABLED が false なら起動しない）。
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/config"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/database"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/read"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/pricesource"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/router"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/scheduler"
	"github.com/tomo-shibata/zero/backend/internal/interface/handler"
)

const (
	// listenHost は、待ち受けるアドレス。認証のない API を同じ LAN の他人に公開しないため、
	// このマシンの中からだけ接続できる 127.0.0.1 に限る（プラン 4.3、6章の判断11）。
	listenHost = "127.0.0.1"
	// readHeaderTimeout は、ヘッダーを送らずに接続を占有し続けるクライアント（Slowloris）を切るための上限。
	readHeaderTimeout = 10 * time.Second
	// idleTimeout は、keep-alive の接続が次のリクエストを送らないまま開いていられる上限。
	// 設定しないと、使われない接続をいつまでも持ち続けるため。
	idleTimeout = 2 * time.Minute
	// shutdownTimeout は、停止の合図を受けてから、処理中のリクエストが終わるのを待つ上限。
	shutdownTimeout = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		log.Printf("api: %v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadAPI()
	if err != nil {
		return err
	}

	// Ctrl+C や docker の停止（SIGTERM）で、処理中のリクエストを終えてから止まるようにする。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 待ち受けを始める前に DB につながることを確かめる。DB に接続できないまま起動して、
	// すべてのリクエストに 500 を返し続けるより、起動の時点で失敗した方が原因に気づきやすいため。
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	// サーバーを止めた後（run を抜けるとき）に閉じる。処理中のリクエストが DB を使い終わってから閉じるため。
	defer pool.Close()

	stopSchedule, err := startPriceFetchSchedule(cfg, pool)
	if err != nil {
		return err
	}
	// 接続プールを閉じる前に止める（defer は登録と逆の順に動く）。実行中の取得があれば、DB を使い終わるのを待ってから閉じるため。
	defer stopSchedule()

	// 先に待ち受けを始めてから、実際に待ち受けたアドレスをログに出す。
	// 待ち受けに失敗した（ポートが使用中など）のに、待ち受けているとログに出ないようにするため。
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort(listenHost, strconv.Itoa(cfg.Port)))
	if err != nil {
		return fmt.Errorf("待ち受けを始められません: %w", err)
	}

	srv := &http.Server{
		Handler:           router.New(newDeps(pool, cfg)),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(ln)
	}()
	log.Printf("api: %s で待ち受けています", ln.Addr())

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	// 合図を受けたら、シグナルの扱いを元に戻す。終了を待っている間にもう一度 Ctrl+C を押せば、すぐに止められるように。
	stop()
	log.Printf("api: 停止の合図を受けました。処理中のリクエストが終わるのを待ちます")

	// ctx はもう取り消されているので、待つ上限は別の context で決める。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// 上限までに終わらなかったリクエストの接続は、待たずに閉じる。
		return errors.Join(fmt.Errorf("処理中のリクエストが終わるのを待ちきれません: %w", err), srv.Close())
	}
	return nil
}

// newDeps は、ルーティングが使う依存を組み立てる。
func newDeps(pool *pgxpool.Pool, cfg config.API) router.Deps {
	return router.Deps{
		QueryHandler: handler.NewQueryHandler(query.NewListHoldings(read.NewHoldingsReader(pool))),
		DevUserID:    cfg.DevUserID,
		AllowedHosts: allowedHosts(cfg.Port),
	}
}

// allowedHosts は、/api/ 以下で受け付ける Host ヘッダーを返す（プラン 6章の判断12）。
// このサーバーを指す名前（待ち受けている 127.0.0.1 と、ブラウザで開くときの localhost）に、待ち受けるポートを付けたものだけ。
// ブラウザは既定以外のポートを Host に必ず付けるので、ポートまで含めて比べられる。
func allowedHosts(port int) []string {
	p := strconv.Itoa(port)
	return []string{
		net.JoinHostPort(listenHost, p),
		net.JoinHostPort("localhost", p),
	}
}

// startPriceFetchSchedule は、設定（PRICE_FETCH_SCHEDULE_ENABLED）で有効なら、価格の取得処理を毎日 18:00（日本時間）に
// 実行し始める（要件 FR-15、プラン 2章の T-5）。戻り値の stop で止める。無効なら何もせず、何もしない stop を返す。
func startPriceFetchSchedule(cfg config.API, pool *pgxpool.Pool) (stop func(), err error) {
	if !cfg.PriceFetchScheduleEnabled {
		log.Printf("api: 株価取得の定時実行はしません（PRICE_FETCH_SCHEDULE_ENABLED が false）")
		return func() {}, nil
	}
	fetch := newFetchClosingPrices(pool)
	stop, err = scheduler.Start(func(ctx context.Context) { runScheduledFetch(ctx, fetch) })
	if err != nil {
		return nil, fmt.Errorf("株価取得の定時実行を始められません: %w", err)
	}
	log.Printf("api: 株価取得を定時実行します（%s）", scheduler.FetchClosingPricesSpec)
	return stop, nil
}

// runScheduledFetch は、定時実行で価格の取得処理 fetch を1回実行し、結果をログに出す。
// 定時実行には結果を返す相手がいないので、ログだけが結果を知る手段になる。
func runScheduledFetch(ctx context.Context, fetch *command.FetchClosingPrices) {
	log.Printf("api: 株価取得を始めます")
	result, err := fetch.Execute(ctx)
	if err != nil {
		log.Printf("api: 株価取得に失敗しました: %v", err)
		return
	}
	// 銘柄コードは DB の値で改行などを含みうるので、偽のログ行を作られないよう %q で出す（価格の取得処理のログと同じ）。
	log.Printf("api: 株価取得が終わりました。保存した銘柄: %q", result.Saved)
	if len(result.Failed) > 0 {
		// 失敗の理由は、価格の取得処理が銘柄ごとにログに出している。
		log.Printf("api: 取得・保存できなかった銘柄（保存済みの価格はそのまま）: %q", result.Failed)
	}
	// 時間の上限（プラン 6章の判断18）や API サーバーの停止で取り消されても、Execute はエラーを返さず、
	// 残りの銘柄を Failed に入れて返す（プラン 4.4「株価取得の決まり」4）。途中で打ち切ったことを1行で分かるようにする。
	if err := ctx.Err(); err != nil {
		log.Printf("api: 株価取得を途中で打ち切りました（打ち切った後の銘柄は取得していません）: %v", err)
	}
}

// newFetchClosingPrices は、pool の DB を使う価格の取得処理を組み立てる。
// 取得元を差し替えるときは、ここと cmd/fetch-prices の newFetchClosingPrices の取得元だけを変える（要件 NFR-1、プラン 6章の判断4）。
// 当面の取得元はダミー（要件 NFR-2）。
func newFetchClosingPrices(pool *pgxpool.Pool) *command.FetchClosingPrices {
	return command.NewFetchClosingPrices(
		pricesource.NewDummyPriceSource(),
		write.NewHoldingRepository(pool),
		write.NewClosingPriceRepository(pool),
	)
}
