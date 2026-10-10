// Package scheduler は、価格の取得処理を毎日決まった時刻に実行する（定時実行。要件 FR-15、プラン 2章の T-5）。
package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	// タイムゾーンのデータベースを実行ファイルに埋め込む。Windows など、OS にデータベースがない環境でも
	// Asia/Tokyo を読めるようにするため（読めないと FetchClosingPricesSpec を解釈できない。プラン 4.3）。
	_ "time/tzdata"

	"github.com/robfig/cron/v3"
)

// FetchClosingPricesSpec は、価格の取得処理を実行する時刻（毎日 18:00（日本時間）。要件 FR-15）。
// 書式は cron の5項目（分 時 日 月 曜日。cron.ParseStandard で解釈する）。
// CRON_TZ で日本時間を指定するのは、サーバーのタイムゾーンの設定に左右されずに日本時間の 18:00 に実行するため。
const FetchClosingPricesSpec = "CRON_TZ=Asia/Tokyo 0 18 * * *"

// jobTimeout は、定時実行の1回の実行にかける時間の上限（プラン 6章の判断18）。
// 本番の取得元が応答しないときに、実行が終わらないまま次の実行と重なったり、API サーバーの資源を持ち続けたりしないため。
// 上限に達すると job の ctx が取り消されるので、価格の取得処理は残りの銘柄を取得せずに終わる（プラン 4.4「株価取得の決まり」4）。
const jobTimeout = time.Hour

// Start は、FetchClosingPricesSpec の時刻ごとに job を実行し始める。
// job に渡す ctx は、実行を始めてから jobTimeout たつと取り消される。前回の job がまだ終わっていなければ、その回は実行しない。
// 戻り値の stop は、定時実行をやめ、実行中の job があれば終わるまで待つ。待つ前に job に渡した ctx を取り消すので、
// job は ctx を見て早めに切り上げられる（API サーバーを止めるときに、実行中の取得を最後まで待ち続けないため）。
// stop は複数回呼んでもよい。
func Start(job func(context.Context)) (stop func(), err error) {
	// テスト（AC-14a）が確かめるのと同じ解釈（cron.ParseStandard）で読み、実行にもその結果を使う。
	schedule, err := cron.ParseStandard(FetchClosingPricesSpec)
	if err != nil {
		return nil, fmt.Errorf("定時実行の時刻 %q を解釈できません: %w", FetchClosingPricesSpec, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// job が panic しても、API サーバーのプロセスごと落ちないよう、ログに出して次の実行を待つ
	// （robfig/cron は既定では panic を回復しない。net/http がハンドラーの panic を回復するのと揃える）。
	logger := cron.PrintfLogger(log.New(log.Writer(), "scheduler: ", log.Flags()|log.Lmsgprefix))
	// SkipIfStillRunning は、実行を飛ばしたことを Info の "skip" だけで知らせる。PrintfLogger は Info を出さないので、
	// Info も出す logger に、何を飛ばしたのかが分かる前置きを付けて渡す（時間の上限を過ぎても ctx を見ずに終わらない job に気づけるように）。
	skipLogger := cron.VerbosePrintfLogger(log.New(log.Writer(), "scheduler: 前回の実行が終わっていないので、今回は実行しません: ", log.Flags()|log.Lmsgprefix))
	// 前回の実行と重ねない（判断18）。チェーンは先に書いたものが外側になる。
	// SkipIfStillRunning を Recover の外側に置くのは、robfig/cron v3.0.1 の SkipIfStillRunning が、job の panic を
	// 通り抜けさせると「実行中」の印を戻さず、その後の定時実行をすべて飛ばしてしまうため（内側の Recover で panic を止める）。
	c := cron.New(cron.WithChain(cron.SkipIfStillRunning(skipLogger), cron.Recover(logger)))
	c.Schedule(schedule, cron.FuncJob(func() {
		jobCtx, cancelJob := context.WithTimeout(ctx, jobTimeout)
		defer cancelJob()
		job(jobCtx)
	}))
	c.Start()

	return func() {
		cancel()
		<-c.Stop().Done()
	}, nil
}
