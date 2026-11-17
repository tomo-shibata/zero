package integration

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
	"github.com/tomo-shibata/zero/backend/internal/domain/repository"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/write"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/price-update.md）の要件16（ログイン中の利用者が保有する全銘柄を取得する。他の利用者だけが保有する銘柄は含めない）と、
// プラン（docs/plans/price-update/plan.md）5.2 の PR① の ListHeldStockCodesOf の行
// 「Aの 7203 が2件でも1つ、130A と 1301 は 1301・130A の順（COLLATE "C"）、保有のない利用者は空のスライス（nil でない）」。
// 境界は 4.4: write.NewHoldingRepository(pool) の ListHeldStockCodesOf(ctx, userID)（重複なし・昇順（COLLATE "C"）。0件なら空のスライス）。
// 売買履歴は SQL の INSERT で入れ（holdings_fixtures_test.go）、結果は戻り値で確かめる。
//
// 売買履歴のプラン（docs/plans/trade-history/plan.md）4.6（PR②）・5.5: 保有している銘柄は通貨と一緒に読む。
// ListHeldStockCodesOf は ListHeldStocksOf(ctx, userID) ([]repository.HeldStock, error) に、
// ListHeldStockCodes は ListHeldStocks(ctx) ([]repository.HeldStock, error) になる（並びと条件は PR① と同じ）。
// 統合テストの補助 insertStock は insertStock(t, pool, code, name, currency string)（4.7）。

// 銘柄の通貨（stocks.currency の値。プラン 4.2）。
const (
	currencyJPY = "JPY"
	currencyUSD = "USD"
)

// insertStock は、銘柄マスタ（stocks）に、test.sql にない銘柄を1件足す（4.7）。通貨 currency は "JPY" か "USD"。
func insertStock(t *testing.T, pool *pgxpool.Pool, code, name, currency string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO stocks (code, name, currency) VALUES ($1::text, $2::text, $3::text)",
		code, name, currency,
	); err != nil {
		t.Fatalf("銘柄マスタに %q（%s・%s）を足せない: %v", code, name, currency, err)
	}
}

// insertYenStock は、円の銘柄を、テスト用の仮の名前で足す。
func insertYenStock(t *testing.T, pool *pgxpool.Pool, code string) {
	t.Helper()
	insertStock(t, pool, code, "テスト銘柄"+code, currencyJPY)
}

// heldYen は、円の銘柄 codes を、保有している銘柄（通貨は JPY）にする。
func heldYen(codes ...string) []repository.HeldStock {
	stocks := make([]repository.HeldStock, 0, len(codes))
	for _, code := range codes {
		stocks = append(stocks, repository.HeldStock{Code: code, Currency: model.CurrencyJPY})
	}
	return stocks
}

// listHeldStocksOf は、pool の DB で、利用者 userID の保有銘柄を ListHeldStocksOf で読む。
func listHeldStocksOf(t *testing.T, pool *pgxpool.Pool, userID string) []repository.HeldStock {
	t.Helper()
	got, err := write.NewHoldingRepository(pool).ListHeldStocksOf(t.Context(), uuid.MustParse(userID))
	if err != nil {
		t.Fatalf("ListHeldStocksOf(%s): got err %v, want nil", userID, err)
	}
	return got
}

// 補助 要件16（5.2）: ListHeldStockCodesOf の戻り値。
func Test補助_要件16_ListHeldStockCodesOfはその利用者の保有銘柄を重複なしCOLLATE_Cの昇順で返し保有がなければ空のスライスを返す(t *testing.T) {
	t.Parallel()

	// Aの 7203 が2件（数量違い）でも1つ。Bだけが保有する 6758 は含めない。
	t.Run("Aの7203が2件でも1つで他の利用者の銘柄は含めない", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 100, "2000")
		insertBuy(t, pool, seedUserA, "7203", 300, "3000")
		insertBuy(t, pool, seedUserB, "6758", 10, "3200")

		got := listHeldStocksOf(t, pool, seedUserA)

		if want := heldYen("7203"); !slices.Equal(got, want) {
			t.Errorf("ListHeldStocksOf(A): got %v, want %v", got, want)
		}
	})

	// 130A と 1301（どちらも銘柄マスタにない銘柄なので足す）。1301 より先に 130A の買いを入れる（入れた順が昇順と逆）。
	t.Run("130Aと1301は1301_130Aの順", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertYenStock(t, pool, "130A")
		insertYenStock(t, pool, "1301")
		insertBuy(t, pool, seedUserA, "130A", 10, "1000")
		insertBuy(t, pool, seedUserA, "1301", 10, "1000")

		got := listHeldStocksOf(t, pool, seedUserA)

		if want := heldYen("1301", "130A"); !slices.Equal(got, want) {
			t.Errorf("ListHeldStocksOf(A): got %v, want %v", got, want)
		}
	})

	// 補助（プランの 130A・1301 に足したデータ）: COLLATE "C"（バイト順）では大文字 130A（0x41）が小文字 130a（0x61）より前。
	// DB の既定の照合順序（en_US.utf8）のままなら 130a が 130A より前に来る（persistence/write/holding_repository.go の heldStocksOf。Go の文字列比較で並べる）。
	// プランの 130A と 1301 の組は、どちらの照合順序でも同じ順になるので、これを足して COLLATE "C" を見分ける。
	t.Run("大文字の130Aは小文字の130aより前（バイト順）", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertYenStock(t, pool, "130a")
		insertYenStock(t, pool, "130A")
		insertBuy(t, pool, seedUserA, "130a", 10, "1000")
		insertBuy(t, pool, seedUserA, "130A", 10, "1000")

		got := listHeldStocksOf(t, pool, seedUserA)

		if want := heldYen("130A", "130a"); !slices.Equal(got, want) {
			t.Errorf("ListHeldStocksOf(A): got %v, want %v", got, want)
		}
	})

	// 保有のない利用者（B）は空のスライス。nil ではない（0件のとき nil を返す実装を見分ける）。Aは保有している。
	t.Run("保有のない利用者は空のスライスでnilではない", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 100, "2000")

		got := listHeldStocksOf(t, pool, seedUserB)

		if got == nil {
			t.Errorf("ListHeldStocksOf(B): got nil, want 空のスライス")
		}
		if len(got) != 0 {
			t.Errorf("ListHeldStocksOf(B): got %v, want 空", got)
		}
	})
}

// ---- 売買履歴への作り変え（docs/plans/trade-history/plan.md 5.3。PR①）----
// 「保有している」は、利用者ごとの保有数量（買いの株数の合計 − 売りの株数の合計）が1以上のこと（4.6）。
// 売り切った銘柄（保有数量が0）は含まない。

// 補助 5.3: ListHeldStockCodesOf は、保有数量が1以上の銘柄だけを返す。
//   - Aが 7203 を買って売り切る → 含まない。6758 は保有 → 含む
//   - 境界: 100株を買って99株売る（保有数量1）→ 含む。100株を買って100株売る（保有数量0）→ 含まない
//   - Bが売り切った銘柄は、Aが保有していれば Aには含む（利用者ごとに数える）
func Test補助_5_3_ListHeldStockCodesOfは保有数量が1以上の銘柄だけを返す(t *testing.T) {
	t.Parallel()

	t.Run("売り切った銘柄は含まず保有している銘柄は含む", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 100, "2000")
		insertTrade(t, pool, seedUserA, "7203", "sell", 100, "2500", "2026-02-10")
		insertBuy(t, pool, seedUserA, "6758", 10, "3200")

		got := listHeldStocksOf(t, pool, seedUserA)

		if want := heldYen("6758"); !slices.Equal(got, want) {
			t.Errorf("ListHeldStocksOf(A): got %v, want %v", got, want)
		}
	})

	t.Run("保有数量1は含み保有数量0は含まない", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 100, "2000")
		insertTrade(t, pool, seedUserA, "7203", "sell", 99, "2500", "2026-02-10")
		insertBuy(t, pool, seedUserA, "6758", 100, "3200")
		insertTrade(t, pool, seedUserA, "6758", "sell", 100, "3500", "2026-02-10")

		got := listHeldStocksOf(t, pool, seedUserA)

		if want := heldYen("7203"); !slices.Equal(got, want) {
			t.Errorf("ListHeldStocksOf(A): got %v, want %v（7203 は1株残っている。6758 は売り切り）", got, want)
		}
	})

	t.Run("Bが売り切った銘柄もAが保有していればAには含む", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 100, "2000")
		insertBuy(t, pool, seedUserB, "7203", 50, "2000")
		insertTrade(t, pool, seedUserB, "7203", "sell", 50, "2500", "2026-02-10")

		got := listHeldStocksOf(t, pool, seedUserA)

		if want := heldYen("7203"); !slices.Equal(got, want) {
			t.Errorf("ListHeldStocksOf(A): got %v, want %v", got, want)
		}
	})

	t.Run("売り切っただけの利用者は空のスライスでnilではない", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 100, "2000")
		insertTrade(t, pool, seedUserA, "7203", "sell", 100, "2500", "2026-02-10")

		got := listHeldStocksOf(t, pool, seedUserA)

		if got == nil {
			t.Errorf("ListHeldStocksOf(A): got nil, want 空のスライス")
		}
		if len(got) != 0 {
			t.Errorf("ListHeldStocksOf(A): got %v, want 空", got)
		}
	})
}

// listAllHeldStocks は、pool の DB で、誰かが保有している銘柄を ListHeldStocks（全利用者）で読む。
func listAllHeldStocks(t *testing.T, pool *pgxpool.Pool) []repository.HeldStock {
	t.Helper()
	got, err := write.NewHoldingRepository(pool).ListHeldStocks(t.Context())
	if err != nil {
		t.Fatalf("ListHeldStocks: got err %v, want nil", err)
	}
	return got
}

// 補助 5.3（全利用者の分）: ListHeldStocks は、誰かの保有数量が1以上の銘柄を、重複なし・昇順（COLLATE "C"）で返す。
//   - Aが売り切り・Bが保有 → 含む
//   - AもBも売り切り → 含まない
//   - AもBも保有していても1つ
//   - 昇順は COLLATE "C"（バイト順）: 1301、130A、130a の順（大文字 130A が小文字 130a より前）
func Test補助_5_3_ListHeldStocksは誰かの保有数量が1以上の銘柄を重複なしCOLLATE_Cの昇順で返す(t *testing.T) {
	t.Parallel()

	t.Run("Aが売り切りBが保有する銘柄は含む", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 100, "2000")
		insertTrade(t, pool, seedUserA, "7203", "sell", 100, "2500", "2026-02-10")
		insertBuy(t, pool, seedUserB, "7203", 10, "2100")

		got := listAllHeldStocks(t, pool)

		if want := heldYen("7203"); !slices.Equal(got, want) {
			t.Errorf("ListHeldStocks: got %v, want %v", got, want)
		}
	})

	t.Run("AもBも売り切った銘柄は含まない", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "6758", 10, "3200")
		insertTrade(t, pool, seedUserA, "6758", "sell", 10, "3500", "2026-02-10")
		insertBuy(t, pool, seedUserB, "6758", 20, "3300")
		insertTrade(t, pool, seedUserB, "6758", "sell", 20, "3400", "2026-02-11")
		insertBuy(t, pool, seedUserA, "7203", 1, "2000")

		got := listAllHeldStocks(t, pool)

		if want := heldYen("7203"); !slices.Equal(got, want) {
			t.Errorf("ListHeldStocks: got %v, want %v（6758 は2人とも売り切り）", got, want)
		}
	})

	t.Run("AもBも保有していても重複なしで1つ", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 100, "2000")
		insertBuy(t, pool, seedUserA, "7203", 50, "2100")
		insertBuy(t, pool, seedUserB, "7203", 10, "2200")

		got := listAllHeldStocks(t, pool)

		if want := heldYen("7203"); !slices.Equal(got, want) {
			t.Errorf("ListHeldStocks: got %v, want %v", got, want)
		}
	})

	t.Run("昇順はバイト順で入れた順に左右されない", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertYenStock(t, pool, "130a")
		insertYenStock(t, pool, "130A")
		insertYenStock(t, pool, "1301")
		insertBuy(t, pool, seedUserA, "130a", 10, "1000")
		insertBuy(t, pool, seedUserB, "130A", 10, "1000")
		insertBuy(t, pool, seedUserA, "1301", 10, "1000")

		got := listAllHeldStocks(t, pool)

		if want := heldYen("1301", "130A", "130a"); !slices.Equal(got, want) {
			t.Errorf("ListHeldStocks: got %v, want %v", got, want)
		}
	})
}

// ---- ドルの銘柄（docs/plans/trade-history/plan.md 4.6・5.5。PR②）----
// 保有している銘柄は、銘柄マスタの通貨と一緒に返す（取得の処理が、終値の精度を通貨で確かめるため）。
// ドルの銘柄 AAPL・MSFT は test.sql にない（各テストで足す。4.7）。

// 補助 4.6（PR②）: ListHeldStocksOf は、保有数量が1以上の銘柄を、銘柄マスタの通貨と一緒に、銘柄コードの昇順（COLLATE "C"）で返す。
// Aが 7203（円）と AAPL（ドル）を保有し、MSFT（ドル）は売り切り → [7203（JPY）, AAPL（USD）]。数字で始まるコードが英字で始まるコードより先。
// Bだけが保有する 6758（円）は含めない。
func Test補助_4_6_ListHeldStocksOfは保有している銘柄を通貨と一緒にコードの昇順で返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertStock(t, pool, "MSFT", "Microsoft", currencyUSD)
	insertBuy(t, pool, seedUserA, "AAPL", 10, "150.25")
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserA, "MSFT", 5, "400.00")
	insertTrade(t, pool, seedUserA, "MSFT", "sell", 5, "410.00", "2026-02-10")
	insertBuy(t, pool, seedUserB, "6758", 10, "3200")

	got := listHeldStocksOf(t, pool, seedUserA)

	want := []repository.HeldStock{
		{Code: "7203", Currency: model.CurrencyJPY},
		{Code: "AAPL", Currency: model.CurrencyUSD},
	}
	if !slices.Equal(got, want) {
		t.Errorf("ListHeldStocksOf(A): got %v, want %v", got, want)
	}
}

// 補助 4.6（PR②）: ListHeldStocks（全利用者）も、通貨と一緒に返す。Aが 7203（円）、Bが AAPL（ドル）を保有 → [7203（JPY）, AAPL（USD）]。
// 同じ銘柄を2人が保有しても1つ。
func Test補助_4_6_ListHeldStocksは誰かが保有している銘柄を通貨と一緒に重複なしで返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertBuy(t, pool, seedUserB, "AAPL", 10, "150.25")
	insertBuy(t, pool, seedUserA, "AAPL", 3, "151.00")
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")

	got := listAllHeldStocks(t, pool)

	want := []repository.HeldStock{
		{Code: "7203", Currency: model.CurrencyJPY},
		{Code: "AAPL", Currency: model.CurrencyUSD},
	}
	if !slices.Equal(got, want) {
		t.Errorf("ListHeldStocks: got %v, want %v", got, want)
	}
}
