// Command migrate は、DATABASE_URL の DB にマイグレーションを適用する。
//
//	-seed <file>  マイグレーションの後に、seed（DML）のファイルを流す
//	-recreate     先に DB を消して作り直す（DB 名が zero_e2e か zero_test_ で始まるときだけ）
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/tomo-shibata/zero/backend/internal/infrastructure/config"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/database"
)

const (
	// maintenanceDatabase は、-recreate で DB を消す・作るときに接続する DB。
	// 消す DB そのものには接続したまま DROP できないので、PostgreSQL が必ず持っている postgres を使う。
	maintenanceDatabase = "postgres"
	// e2eDatabase は、E2E 用の DB（プラン 4.5）。-recreate で消してよい。
	e2eDatabase = "zero_e2e"
	// recreatableRule は、-recreate で消してよい DB 名の決まり。フラグの説明とエラーの文言で使う。
	recreatableRule = "DB 名が " + e2eDatabase + " か " + database.TestDatabasePrefix + " で始まるときだけ"
)

func main() {
	if err := run(); err != nil {
		log.Printf("migrate: %v", err)
		os.Exit(1)
	}
}

func run() error {
	seedFile := flag.String("seed", "", "マイグレーションの後に流す seed（DML）のファイル")
	recreate := flag.Bool("recreate", false, "先に DB を消して作り直す（"+recreatableRule+"）")
	flag.Parse()
	if flag.NArg() > 0 {
		return fmt.Errorf("不明な引数があります: %v", flag.Args())
	}

	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		return err
	}
	// 接続先は、この後実際に接続するときと同じ解釈（pgx）で決める。
	// URL の path だけを見ると、?dbname=… などで別の DB を指していても見落とすため。
	target, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("DATABASE_URL を読めません: %w", err)
	}
	// Taskfile は DATABASE_URL（パスワードを含む）を端末に出さないので、どの DB に流すかはここで示す。
	// パスワードを出さないよう、接続文字列ではなくホスト・ポート・DB 名だけを出す。
	log.Printf("migrate: 接続先: ホスト %s、ポート %d、DB %q", target.Host, target.Port, target.Database)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// seed は DB に手を付ける前に読む。ファイルの指定を誤ったときに、DB を作り直してから失敗しないように。
	var seedSQL string
	if *seedFile != "" {
		b, err := os.ReadFile(*seedFile)
		if err != nil {
			return fmt.Errorf("seed のファイルを読めません: %w", err)
		}
		seedSQL = string(b)
	}

	if *recreate {
		if err := recreateDatabase(ctx, target); err != nil {
			return err
		}
	}

	pool, err := database.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	results, err := database.Migrate(ctx, pool)
	// 途中で失敗しても、そこまでに適用できたものは残るので出しておく。
	for _, r := range results {
		log.Printf("migrate: 適用しました: %s", r)
	}
	if err != nil {
		return err
	}
	if len(results) == 0 {
		log.Printf("migrate: 適用するマイグレーションはありません")
	}

	if *seedFile != "" {
		if err := database.Seed(ctx, pool, seedSQL); err != nil {
			return err
		}
		log.Printf("migrate: seed を流しました: %s", *seedFile)
	}
	return nil
}

// recreateDatabase は、target（DATABASE_URL を pgx で読んだもの）の DB を消してから空の DB として作り直す。
func recreateDatabase(ctx context.Context, target *pgx.ConnConfig) error {
	name := target.Database
	if !isRecreatable(name) {
		return fmt.Errorf("-recreate は%s使えます（DB 名: %q）", recreatableRule, name)
	}

	admin := target.Copy()
	admin.Database = maintenanceDatabase
	if err := database.DropDatabase(ctx, admin, name); err != nil {
		return err
	}
	if err := database.CreateDatabase(ctx, admin, name); err != nil {
		return err
	}
	log.Printf("migrate: DB %q を作り直しました", name)
	return nil
}

// isRecreatable は、-recreate で消してよい DB 名かを返す。
// 開発用 DB（zero）を誤って消さないため、E2E 用とテスト用の名前だけを許す（プラン 6章の判断10）。
func isRecreatable(name string) bool {
	return name == e2eDatabase || strings.HasPrefix(name, database.TestDatabasePrefix)
}
