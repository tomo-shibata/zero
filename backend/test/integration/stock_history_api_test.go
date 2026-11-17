package integration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/read"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/router"
	"github.com/tomo-shibata/zero/backend/internal/interface/handler"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 銘柄ごとの売買履歴の画面の要件（docs/requirements/stock-history.md）の基準7・12・14・22・24（要件6・要件7・要件9・要件20・要件21）と、
// プラン（docs/plans/stock-history/plan.md）4.2〜4.4（読み方・組み立て・API の形）・4.8 の PR①・5.1 の PR① の統合の行・5.2 の stock_history_api_test.go の行。
// 確かめるのは GET /api/holdings/{code}/history の応答。
// 組み立ては 4.8 の境界のとおり: read.NewStockHistoryReader(pool) → query.NewGetStockHistory → handler.NewStockHistoryHandler →
// router.New(router.Deps{StockHistoryHandler: …})。ログイン中の利用者（DevUserID）は利用者A。Host は allowedHost。
// 売買履歴・終値・分割は SQL で入れる（insertTrade・insertBuy・insertClosePrice・insertStockSplit）。銘柄マスタの 7203・6758・9984 は test.sql から。

const (
	notFoundBodyJSON = `{"error":"not_found"}`
	internalErrJSON  = `{"error":"internal_error"}`
)

// stockHistoryPath は、銘柄 encodedCode（パスにそのまま入れる形）の売買履歴の API のパス。
func stockHistoryPath(encodedCode string) string {
	return "/api/holdings/" + encodedCode + "/history"
}

// newStockHistoryAPI は、reader から読む売買履歴の API を、4.8 の境界のとおりに組み立てる。ログイン中の利用者は利用者A。
func newStockHistoryAPI(reader query.StockHistoryReader) http.Handler {
	return router.New(router.Deps{
		StockHistoryHandler: handler.NewStockHistoryHandler(query.NewGetStockHistory(reader)),
		DevUserID:           uuid.MustParse(seedUserA),
		AllowedHosts:        []string{allowedHost},
	})
}

// getStockHistoryFromDB は、pool の DB を読む API で、利用者Aとして銘柄 encodedCode の売買履歴を取得する。
func getStockHistoryFromDB(t *testing.T, pool *pgxpool.Pool, encodedCode string) *httptest.ResponseRecorder {
	t.Helper()
	return serveGet(t, newStockHistoryAPI(read.NewStockHistoryReader(pool)), stockHistoryPath(encodedCode), allowedHost)
}

// insertOtherStockData は、基準7 の前提「別の銘柄のデータ」を入れる（7203 の応答に混ざらないことを確かめるため）:
// 6758 の終値 2026-03-02 3,500円、6758 の分割 1→2 2026-02-01、Aの 6758「買 3,200円 10株 2026-01-10」。
func insertOtherStockData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	insertClosePrice(t, pool, "6758", "2026-03-02", "3500")
	insertStockSplit(t, pool, "6758", "2026-02-01", 2)
	insertTrade(t, pool, seedUserA, "6758", "buy", 10, "3200", "2026-01-10")
}

// 基準7（要件6・要件7）: Aの 7203 の売買履歴が「買 2,500円 100株 2026-01-10」「売 3,050円 50株 2026-03-02」で、
// 7203 の終値が 2026-01-09 2,400円、2026-01-10 2,450円、2026-03-02 3,000円、2026-10-06 3,000円。
// 終値は 2026-10-06、2026-01-09、2026-03-02、2026-01-10 の順に入れる（ORDER BY を忘れる誤りを見分けるため）。
// 別の銘柄（6758）のデータがあっても混ざらない。
// 入れた後に ANALYZE stock_prices を実行し、表を頭から読む計画（登録した順に返る）を選ばせる（新しい DB の小さな表では、
// 主キーの索引で読んで偶然に昇順で返ることがあり、ORDER BY を外しても落ちないため）。
// → closes の日付の列は ["2026-01-10","2026-03-02","2026-10-06"]（2026-01-09 を含まない）、entries は 2026-03-02 の売、2026-01-10 の買の順。
func Test基準7_終値は最初の売買の取引日からを取引日の昇順で返し別の銘柄は混ざらない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2500", "2026-01-10")
	insertTrade(t, pool, seedUserA, "7203", "sell", 50, "3050", "2026-03-02")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	insertClosePrice(t, pool, "7203", "2026-01-09", "2400")
	insertClosePrice(t, pool, "7203", "2026-03-02", "3000")
	insertClosePrice(t, pool, "7203", "2026-01-10", "2450")
	insertOtherStockData(t, pool)
	analyzeStockPrices(t, pool)

	rec := getStockHistoryFromDB(t, pool, "7203")

	assertJSONResponse(t, rec, http.StatusOK, `{
		"stock": {"code": "7203", "name": "トヨタ自動車", "currency": "JPY"},
		"entries": [
			{"kind": "trade", "date": "2026-03-02", "side": "sell", "quantity": 50,
			 "price": "3050.0", "amount": "152500.0", "splitDivisor": "1", "adjustedPrice": "3050.0"},
			{"kind": "trade", "date": "2026-01-10", "side": "buy", "quantity": 100,
			 "price": "2500.0", "amount": "250000.0", "splitDivisor": "1", "adjustedPrice": "2500.0"}
		],
		"closes": [
			{"date": "2026-01-10", "price": "2450.0", "splitDivisor": "1", "adjustedPrice": "2450.0"},
			{"date": "2026-03-02", "price": "3000.0", "splitDivisor": "1", "adjustedPrice": "3000.0"},
			{"date": "2026-10-06", "price": "3000.0", "splitDivisor": "1", "adjustedPrice": "3000.0"}
		]
	}`)
}

// 基準12（要件9〜11）: 7203 の分割が「1→3 2026-03-31」、Aの売買履歴が「買 2,500円 100株 2026-01-10」「買 1,000円 30株 2026-03-31」、
// 終値が 2026-03-30 3,100円・2026-03-31 1,050円。基準7 と同じ別の銘柄のデータ（6758 の分割 1→2）を入れる。
// → 応答は プラン 4.4 の例のとおり: 割る数は 2026-03-30 の終値と 2026-01-10 の売買が "3"、2026-03-31 の終値と売買が "1"。
// 割った価格は "1033.3"・"833.3"。6758 の分割の 2 は掛からず、entries に 6758 の分割の行は増えない（分割の行は 7203 の 2026-03-31 の1つだけ）。
func Test基準12_分割の前の終値と売買を割る数で割り別の銘柄の分割は混ざらない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStockSplit(t, pool, "7203", "2026-03-31", 3)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2500", "2026-01-10")
	insertTrade(t, pool, seedUserA, "7203", "buy", 30, "1000", "2026-03-31")
	insertClosePrice(t, pool, "7203", "2026-03-30", "3100")
	insertClosePrice(t, pool, "7203", "2026-03-31", "1050")
	insertOtherStockData(t, pool)

	rec := getStockHistoryFromDB(t, pool, "7203")

	assertJSONResponse(t, rec, http.StatusOK, `{
		"stock": {"code": "7203", "name": "トヨタ自動車", "currency": "JPY"},
		"entries": [
			{"kind": "trade", "date": "2026-03-31", "side": "buy", "quantity": 30,
			 "price": "1000.0", "amount": "30000.0", "splitDivisor": "1", "adjustedPrice": "1000.0"},
			{"kind": "split", "date": "2026-03-31", "ratio": 3},
			{"kind": "trade", "date": "2026-01-10", "side": "buy", "quantity": 100,
			 "price": "2500.0", "amount": "250000.0", "splitDivisor": "3", "adjustedPrice": "833.3"}
		],
		"closes": [
			{"date": "2026-03-30", "price": "3100.0", "splitDivisor": "3", "adjustedPrice": "1033.3"},
			{"date": "2026-03-31", "price": "1050.0", "splitDivisor": "1", "adjustedPrice": "1050.0"}
		]
	}`)
}

// 基準14（要件12）: Aの 9984 の売買履歴が「買 6,000円 100株 2026-01-10」「売 7,000円 50株 2026-02-10」で、9984 の終値は1件もない
// → 200 で、closes は空の配列 []（null にしない）。応答の JSON 全体を比べる。
func Test基準14_終値が1件もなければclosesは空の配列(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserA, "9984", "buy", 100, "6000", "2026-01-10")
	insertTrade(t, pool, seedUserA, "9984", "sell", 50, "7000", "2026-02-10")

	rec := getStockHistoryFromDB(t, pool, "9984")

	assertJSONResponse(t, rec, http.StatusOK, `{
		"stock": {"code": "9984", "name": "ソフトバンクグループ", "currency": "JPY"},
		"entries": [
			{"kind": "trade", "date": "2026-02-10", "side": "sell", "quantity": 50,
			 "price": "7000.0", "amount": "350000.0", "splitDivisor": "1", "adjustedPrice": "7000.0"},
			{"kind": "trade", "date": "2026-01-10", "side": "buy", "quantity": 100,
			 "price": "6000.0", "amount": "600000.0", "splitDivisor": "1", "adjustedPrice": "6000.0"}
		],
		"closes": []
	}`)
}

// 基準22（要件20・要件21）: Aは 7203 だけを売買し、Bが 6758「買 3,200円 10株 2026-01-10」を持つ。
// Aが 6758、9984（誰も売買していない）、XXXX（銘柄マスタにない）の売買履歴を取得する
// → どれも 404 と {"error":"not_found"}（本文が同じ。他の利用者が売買しているかが分からない）。Bの売買履歴は返らない。
func Test基準22_売買履歴のない銘柄は3つとも同じ404で他の利用者の売買履歴は返らない(t *testing.T) {
	t.Parallel()
	for _, code := range []string{"6758", "9984", "XXXX"} {
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			pool := testdb.New(t)
			insertBuy(t, pool, seedUserA, "7203", 100, "2500")
			insertTrade(t, pool, seedUserB, "6758", "buy", 10, "3200", "2026-01-10")

			rec := getStockHistoryFromDB(t, pool, code)

			assertJSONResponse(t, rec, http.StatusNotFound, notFoundBodyJSON)
		})
	}
}

// 基準24（要件21）: Aの 7203 の売買履歴が「買 2,500円 100株 2026-01-10」、Bの 7203 の売買履歴が「買 2,700円 20株 2026-02-10」。
// 7203 の終値は 2026-01-10 2,450円、2026-01-20 2,500円、2026-02-10 2,600円。
// → 売買はAの1件だけで、closes は 2026-01-10 から（Bの 2026-02-10 を最初の日にしない）の3件。
func Test基準24_他の利用者の売買履歴は含めず終値はAの最初の売買の日から返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2500", "2026-01-10")
	insertTrade(t, pool, seedUserB, "7203", "buy", 20, "2700", "2026-02-10")
	insertClosePrice(t, pool, "7203", "2026-01-10", "2450")
	insertClosePrice(t, pool, "7203", "2026-01-20", "2500")
	insertClosePrice(t, pool, "7203", "2026-02-10", "2600")

	rec := getStockHistoryFromDB(t, pool, "7203")

	assertJSONResponse(t, rec, http.StatusOK, `{
		"stock": {"code": "7203", "name": "トヨタ自動車", "currency": "JPY"},
		"entries": [
			{"kind": "trade", "date": "2026-01-10", "side": "buy", "quantity": 100,
			 "price": "2500.0", "amount": "250000.0", "splitDivisor": "1", "adjustedPrice": "2500.0"}
		],
		"closes": [
			{"date": "2026-01-10", "price": "2450.0", "splitDivisor": "1", "adjustedPrice": "2450.0"},
			{"date": "2026-01-20", "price": "2500.0", "splitDivisor": "1", "adjustedPrice": "2500.0"},
			{"date": "2026-02-10", "price": "2600.0", "splitDivisor": "1", "adjustedPrice": "2600.0"}
		]
	}`)
}

// 補助（5.2。要件6・要件21）: Bの 7203「買 2,700円 20株 2025-12-01」（Aより前）、Aの 7203「買 2,500円 100株 2026-01-10」、
// 終値 2025-12-15 2,900円・2026-01-10 2,450円 → closes は 2026-01-10 の1件だけ
// （利用者で絞らずに最初の日を出す誤りを見分ける。基準24 はBの売買が後なので見分けられない）。
func Test補助_要件6_要件21_終値の最初の日はBの売買ではなくAの最初の売買の日(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserB, "7203", "buy", 20, "2700", "2025-12-01")
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2500", "2026-01-10")
	insertClosePrice(t, pool, "7203", "2025-12-15", "2900")
	insertClosePrice(t, pool, "7203", "2026-01-10", "2450")

	rec := getStockHistoryFromDB(t, pool, "7203")

	assertJSONResponse(t, rec, http.StatusOK, `{
		"stock": {"code": "7203", "name": "トヨタ自動車", "currency": "JPY"},
		"entries": [
			{"kind": "trade", "date": "2026-01-10", "side": "buy", "quantity": 100,
			 "price": "2500.0", "amount": "250000.0", "splitDivisor": "1", "adjustedPrice": "2500.0"}
		],
		"closes": [
			{"date": "2026-01-10", "price": "2450.0", "splitDivisor": "1", "adjustedPrice": "2450.0"}
		]
	}`)
}

// fakeStockHistoryReader は、決めた値を返す StockHistoryReader（境界の外側の fake。失敗の経路と router の確認用）。
// tradesErr・splitsErr・closesErr が nil でなければ、その読み取りがそのエラーを返す。
type fakeStockHistoryReader struct {
	trades                          []dto.TradeRow
	tradesErr, splitsErr, closesErr error
}

// fakeReaderErrorDetail は、fake が返すエラーの内容。応答に出てはいけない（4.4: 内部のエラー内容は返さない）。
const fakeReaderErrorDetail = "fake: connection refused to db.internal:5432"

func (f fakeStockHistoryReader) ListTradesOfStock(context.Context, uuid.UUID, string) ([]dto.TradeRow, error) {
	if f.tradesErr != nil {
		return nil, f.tradesErr
	}
	return f.trades, nil
}

func (f fakeStockHistoryReader) ListStockSplitsOfStock(context.Context, string) ([]dto.StockSplitRow, error) {
	if f.splitsErr != nil {
		return nil, f.splitsErr
	}
	return []dto.StockSplitRow{}, nil
}

func (f fakeStockHistoryReader) ListClosingPricesSince(context.Context, string, string) ([]dto.ClosingPriceRow, error) {
	if f.closesErr != nil {
		return nil, f.closesErr
	}
	return []dto.ClosingPriceRow{}, nil
}

// oneTrade は、fake が返す売買履歴（7203「買 2,500円 100株 2026-01-10」）。
func oneTrade() []dto.TradeRow {
	return []dto.TradeRow{{
		ID: 1, Code: "7203", Name: "トヨタ自動車", Currency: dto.CurrencyJPY, Side: dto.TradeSideBuy,
		Quantity: 100, PriceHundredths: 250000, TradeDate: "2026-01-10",
	}}
}

// oneTradeJSON は、oneTrade を読めて、分割も終値もないときの 200 の本文。
const oneTradeJSON = `{
	"stock": {"code": "7203", "name": "トヨタ自動車", "currency": "JPY"},
	"entries": [
		{"kind": "trade", "date": "2026-01-10", "side": "buy", "quantity": 100,
		 "price": "2500.0", "amount": "250000.0", "splitDivisor": "1", "adjustedPrice": "2500.0"}
	],
	"closes": []
}`

// 補助（5.2。4.4 の「そのほかの失敗」）: 売買履歴・分割・終値のどれかの読み取りが失敗する reader を注入した router に取得
// → 500 と {"error":"internal_error"}（内部のエラー内容は本文に出さない）。対照: 失敗しない同じ fake なら 200。
func Test補助_読み取りが失敗したときは500とinternal_errorを返す(t *testing.T) {
	t.Parallel()
	failure := errors.New(fakeReaderErrorDetail)
	cases := []struct {
		name   string
		reader fakeStockHistoryReader
	}{
		{"売買履歴の読み取りが失敗", fakeStockHistoryReader{trades: oneTrade(), tradesErr: failure}},
		{"分割の読み取りが失敗", fakeStockHistoryReader{trades: oneTrade(), splitsErr: failure}},
		{"終値の読み取りが失敗", fakeStockHistoryReader{trades: oneTrade(), closesErr: failure}},
		// 読み取りはすべて成功するが、応答の本文を作れない（円の価格 0.05円 は 0.1円の単位でなく、黙って切り捨てずに 500 にする）。
		{"応答の本文を作れない_円の価格が0_05円", fakeStockHistoryReader{trades: []dto.TradeRow{{
			ID: 1, Code: "7203", Name: "トヨタ自動車", Currency: dto.CurrencyJPY, Side: dto.TradeSideBuy,
			Quantity: 100, PriceHundredths: 5, TradeDate: "2026-01-10",
		}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newStockHistoryAPI(tc.reader)

			rec := serveGet(t, h, stockHistoryPath("7203"), allowedHost)

			assertJSONResponse(t, rec, http.StatusInternalServerError, internalErrJSON)
		})
	}

	t.Run("対照_失敗しない読み取りなら200", func(t *testing.T) {
		t.Parallel()
		h := newStockHistoryAPI(fakeStockHistoryReader{trades: oneTrade()})

		rec := serveGet(t, h, stockHistoryPath("7203"), allowedHost)

		assertJSONResponse(t, rec, http.StatusOK, oneTradeJSON)
	})
}

// 補助（5.2。pathStockCode を同じに使っているか）: パスの銘柄コードが「/」（%2F）・NUL（%00）・正しくない UTF-8（%FF）を含むなら、
// 500 にならず 404 と {"error":"not_found"}。Aは 7203 の売買履歴を持つ（その履歴が返らないことも確かめる）。
// %00（NUL）・%FF（正しくない UTF-8）を含むコードは、pathStockCode が戻せずに、DB に問い合わせずに 404 にする
// （NUL は PostgreSQL の TEXT に入らず、問い合わせると 500 になるため）。
// %2F を含むコードは、戻した値（例: "A/B"・"7203/"）でふつうに問い合わせるが、その銘柄の売買履歴がないので 404 になる。
func Test補助_パスの銘柄コードに2Fか00か不正なUTF8を含むと404(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		encodedCode string
	}{
		{"スラッシュを含むA_2FB", "A%2FB"},
		{"7203の後ろにスラッシュ", "7203%2F"},
		{"NULの_00", "%00"},
		{"正しくないUTF8の_FF", "%FF"},
		{"RawPathあり_2F_のあとにNULの_00", "A%2F%00"},
		{"RawPathあり_2F_のあとに正しくないUTF8の_FF", "%2F%FF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := testdb.New(t)
			insertBuy(t, pool, seedUserA, "7203", 100, "2500")

			rec := getStockHistoryFromDB(t, pool, tc.encodedCode)

			assertJSONResponse(t, rec, http.StatusNotFound, notFoundBodyJSON)
		})
	}
}

// 補助（5.2。今のミドルウェア allowed_hosts・cross_origin を同じに通るか。一覧の GET /api/holdings と同じ扱い）:
//   - 許可リストにない Host（example.com）は 403 と {"error":"forbidden"}。許可した Host なら 200。
//   - Origin: http://evil.example の GET は通る（読み取りは対象外。CSRF の対策は書き込みだけ）。
//   - 200・404・403 の応答に Cache-Control: no-store と X-Content-Type-Options: nosniff が付く（資産のデータをキャッシュに残さないため）。
func Test補助_ホストの許可リストと別のオリジンとヘッダーは一覧と同じ扱い(t *testing.T) {
	t.Parallel()
	h := newStockHistoryAPI(fakeStockHistoryReader{trades: oneTrade()})

	t.Run("許可リストにないHostは403", func(t *testing.T) {
		t.Parallel()
		rec := serveGet(t, h, stockHistoryPath("7203"), "example.com")
		assertJSONResponse(t, rec, http.StatusForbidden, forbiddenJSON)
	})

	t.Run("対照_許可したHostは200", func(t *testing.T) {
		t.Parallel()
		rec := serveGet(t, h, stockHistoryPath("7203"), allowedHost)
		assertJSONResponse(t, rec, http.StatusOK, oneTradeJSON)
	})

	t.Run("Originがevil_exampleのGETは通る", func(t *testing.T) {
		t.Parallel()
		rec := serveGetWithHeader(t, h, stockHistoryPath("7203"), allowedHost, "Origin", evilOrigin)
		assertJSONResponse(t, rec, http.StatusOK, oneTradeJSON)
	})

	notFound := newStockHistoryAPI(fakeStockHistoryReader{trades: []dto.TradeRow{}})
	for _, tc := range []struct {
		name string
		h    http.Handler
		host string
		want int
	}{
		{"200", h, allowedHost, http.StatusOK},
		{"404", notFound, allowedHost, http.StatusNotFound},
		{"403", h, "example.com", http.StatusForbidden},
	} {
		t.Run("応答"+tc.name+"にno_storeとnosniffが付く", func(t *testing.T) {
			t.Parallel()
			rec := serveGet(t, tc.h, stockHistoryPath("7203"), tc.host)

			if rec.Code != tc.want {
				t.Fatalf("前提のステータス: got %d, want %d", rec.Code, tc.want)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control: got %q, want %q", got, "no-store")
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options: got %q, want %q", got, "nosniff")
			}
		})
	}
}

// analyzeStockPrices は、stock_prices の統計を更新する（ANALYZE）。小さな表を主キーの索引でなく頭から読む計画を選ばせ、
// ORDER BY のない問い合わせが登録した順に返るようにする（並びの保証を ORDER BY に頼っているかを確かめるため）。
func analyzeStockPrices(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), "ANALYZE stock_prices"); err != nil {
		t.Fatalf("ANALYZE stock_prices: %v", err)
	}
}
