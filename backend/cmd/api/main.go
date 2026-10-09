// Command api は、HTTP API サーバーを起動する。依存を組み立ててルーターに渡す。
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

	"github.com/tomo-shibata/zero/backend/internal/infrastructure/config"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/router"
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

	// 先に待ち受けを始めてから、実際に待ち受けたアドレスをログに出す。
	// 待ち受けに失敗した（ポートが使用中など）のに、待ち受けているとログに出ないようにするため。
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort(listenHost, strconv.Itoa(cfg.Port)))
	if err != nil {
		return fmt.Errorf("待ち受けを始められません: %w", err)
	}

	srv := &http.Server{
		Handler:           router.New(router.Deps{}),
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
