// Command import-trades は、売買履歴の CSV を、指定した利用者の売買履歴として DATABASE_URL の DB に取り込む
// （要件19。task trades:import で実行する。マイグレーションの適用は task が先に行う）。
//
// 取り込みの中身（CSV の確かめ・取り込み・文言・終了コード）は interface/tradecsv の Run にあり、この main は、
// 引数と DB への接続を用意して Run を呼ぶだけにする（統合テストが Run を直接呼べるように。プラン 6章の判断12）。
// 終了コードは、成功が 0、取り込めなかったときが 1。Run を呼ぶ前の失敗（設定の読み込み・DB への接続）も 1 で、理由を標準エラーに出す。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tomo-shibata/zero/backend/internal/application/command"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/config"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/database"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/internal/interface/tradecsv"
)

func main() {
	os.Exit(run())
}

// run は、引数を読み、DB につないで tradecsv.Run を呼び、終了コードを返す。
func run() int {
	csvPath := flag.String("csv", "", "取り込む CSV のパス（task の CSV_FILE）")
	userID := flag.String("user", "", "取り込む先の利用者の ID（task の USER_ID）")
	workdir := flag.String("workdir", "", "task を実行したディレクトリ（-csv が相対パスのとき、ここからのパスとして読む）")
	flag.Parse()

	// Ctrl+C で止めたときは、取り込みの途中の処理を取り消す（トランザクションは元に戻り、何も変わらない）。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 引数が足りないときは、DB につなぐ前に、Run と同じ形で止める。
	if *csvPath == "" || *userID == "" {
		return tradecsv.FailMissingArgs(os.Stderr)
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

	return tradecsv.Run(ctx, tradecsv.RunInput{
		CSVPath:  resolveCSVPath(*workdir, *csvPath),
		UserID:   *userID,
		Today:    tradecsv.TodayInJapan(time.Now()),
		Importer: command.NewImportTrades(write.NewTradeImportRepository(pool)),
	}, os.Stdout, os.Stderr)
}

// resolveCSVPath は、-csv の値 csv を、ファイルを開くパスにする。
// 空は空のまま返す（「CSV_FILE と USER_ID を指定してください」にするため）。絶対パスはそのまま返す。
// 相対パスは、task を実行したディレクトリ workdir からのパスとして workdir につなぐ
// （task は backend/ で動くので、そのままだと backend/ からのパスに読まれてしまう。プラン 6章の判断10）。
func resolveCSVPath(workdir, csv string) string {
	// Windows の「\data\t.csv」（ドライブの根からのパス）と「C:t.csv」（ドライブだけが付いた相対パス）は、
	// filepath.IsAbs が false を返すが、workdir につなぐと別の場所になるので、そのまま返す。
	if csv == "" || filepath.IsAbs(csv) || filepath.VolumeName(csv) != "" || csv[0] == '\\' || csv[0] == '/' {
		return csv
	}
	return filepath.Join(workdir, csv)
}

// failBeforeRun は、Run を呼ぶ前の失敗 err を、取り込みの誤りと同じ形（先頭の行と理由の1行）で標準エラーに出し、終了コード 1 を返す。
func failBeforeRun(err error) int {
	_, _ = fmt.Fprintln(os.Stderr, tradecsv.FailedHeadline)
	_, _ = fmt.Fprintln(os.Stderr, err)
	return 1
}
