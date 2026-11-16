// Command import-prices は、価格の CSV の終値を、実行した日（日本時間）の終値として DATABASE_URL の DB に登録する
// （要件1。task prices:import で実行する。マイグレーションの適用は task が先に行う）。
//
// 登録の中身（CSV の確かめ・登録・文言・終了コード）は interface/pricecsv の Run にあり、この main は、
// 引数と DB への接続を用意して Run を呼ぶだけにする（統合テストが Run を直接呼べるように。プラン 6章の判断5）。
// 終了コードは、成功が 0、登録できなかったときが 1。Run を呼ぶ前の失敗（設定の読み込み・DB への接続）も 1 で、理由を標準エラーに出す。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/config"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/database"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/interface/csvinput"
	"github.com/tomo-shibata/zero/backend/internal/interface/pricecsv"
)

func main() {
	os.Exit(run())
}

// run は、引数を読み、DB につないで pricecsv.Run を呼び、終了コードを返す。
func run() int {
	csvPath := flag.String("csv", "", "登録する CSV のパス（task の CSV_FILE）")
	workdir := flag.String("workdir", "", "task を実行したディレクトリ（-csv が相対パスのとき、ここからのパスとして読む）")
	flag.Parse()

	// Ctrl+C で止めたときは、登録の途中の処理を取り消す（トランザクションは元に戻り、何も変わらない）。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 引数が足りないときは、DB につなぐ前に、Run と同じ形で止める。
	if *csvPath == "" {
		return pricecsv.FailMissingArgs(os.Stderr)
	}

	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		return failBeforeRun(err)
	}
	pool, err := database.Connect(ctx, databaseURL)
	if err != nil {
		return failBeforeRun(err)
	}
	defer pool.Close()

	return pricecsv.Run(ctx, pricecsv.RunInput{
		CSVPath:  csvinput.ResolvePath(*workdir, *csvPath),
		Today:    csvinput.TodayInJapan(time.Now()),
		Importer: command.NewImportClosingPrices(write.NewClosingPriceImportRepository(pool)),
	}, os.Stdout, os.Stderr)
}

// failBeforeRun は、Run を呼ぶ前の失敗 err を、登録の誤りと同じ形（先頭の行と理由の1行）で標準エラーに出し、終了コード 1 を返す。
func failBeforeRun(err error) int {
	_, _ = fmt.Fprintln(os.Stderr, pricecsv.FailedHeadline)
	_, _ = fmt.Fprintln(os.Stderr, err)
	return 1
}
