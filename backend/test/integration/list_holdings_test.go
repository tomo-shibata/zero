package integration

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tomo-shibata/zero/backend/internal/application/query"
	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/internal/infrastructure/persistence/read"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: 要件（docs/requirements/trade-history.md）の基準6（要件10・要件12）・基準8（要件4）と、
// プラン（docs/plans/trade-history/plan.md）5.1 の基準6・基準8 の行（PR①）、5.5 の「基準6」「基準8 の補助」。
// 組み立ては 4.6 の境界のとおり: read.NewHoldingsReader(pool)（ListTradeRows）→ query.NewListHoldings → Execute。
// 売買履歴は SQL の INSERT で入れる（insertTrade・insertBuy）。
// 元の保有株式一覧の AC-2（他の利用者の保有が含まれない）は、基準8 のテストで兼ねる（5.2）。

// listHoldingsOf は、pool の DB で、利用者 userID の一覧を取得する。取得できなければテストを止める。
func listHoldingsOf(t *testing.T, pool *pgxpool.Pool, userID string) dto.HoldingsList {
	t.Helper()
	got, err := query.NewListHoldings(read.NewHoldingsReader(pool)).Execute(t.Context(), uuid.MustParse(userID))
	if err != nil {
		t.Fatalf("利用者 %s の一覧を取得できない: %v", userID, err)
	}
	return got
}

// 基準6（要件10・要件12）: Aの売買履歴が、7203「買 2,000円 100株 2026-01-10」「売 2,500円 100株 2026-02-10」と、
// 6758「買 3,200円 10株 2026-01-10」→ Aの一覧は 6758 の1行だけで、合計評価額は 35,000円（売り切った 7203 は一覧にも合計にも入らない）。
// 6758 の価格（3,500円・2026-10-06）は保存済み。売り切った 7203 の価格は入れない（5.5: 価格のない売り切りの銘柄が
// 「価格を取得できない銘柄を含みません」を出さないことも確かめる。Totals の円の行の ExcludesUnpriced が false）。
// PR②（5.1）で、合計は通貨ごとの Totals にする（円の1件だけ。35,000円 = 3500000（0.01 単位））。金額は 0.01 単位（Hundredths）。
func Test基準6_売り切った銘柄は一覧に出さず合計評価額に含めない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTrade(t, pool, seedUserA, "7203", "buy", 100, "2000", "2026-01-10")
	insertTrade(t, pool, seedUserA, "7203", "sell", 100, "2500", "2026-02-10")
	insertTrade(t, pool, seedUserA, "6758", "buy", 10, "3200", "2026-01-10")
	insertClosePrice(t, pool, "6758", "2026-10-06", "3500")

	got := listHoldingsOf(t, pool, seedUserA)

	if codes := holdingCodes(got.Holdings); !slices.Equal(codes, []string{"6758"}) {
		t.Fatalf("Aの一覧の銘柄コード: got %v, want [6758]（売り切った 7203 は出さない）", codes)
	}
	h := got.Holdings[0]
	if h.Quantity != 10 {
		t.Errorf("6758 の保有数量: got %d, want 10", h.Quantity)
	}
	if h.ValuationHundredths == nil || *h.ValuationHundredths != 3500000 {
		t.Errorf("6758 の評価額（0.01 単位）: got %v, want 3500000（35,000円）", h.ValuationHundredths)
	}
	// 合計は円の1件だけ（ドルの行はない）。価格のない 7203 は売り切っていて、注記（ExcludesUnpriced）の対象にならない。
	// 含み損益と率（ポインタ）は値で比べる（slices.Equal はアドレスで比べるので使わない。カード表示のプラン 5.2）。
	// 6758: 取得額 32,000円・評価額 35,000円 → +3,000円（300000）・9.375% を四捨五入して 9.4%（94）。
	if len(got.Totals) != 1 {
		t.Fatalf("Totals の件数: got %d, want 1（円だけ）", len(got.Totals))
	}
	total := got.Totals[0]
	if total.Currency != dto.CurrencyJPY || total.ValuationHundredths != 3500000 || total.ExcludesUnpriced {
		t.Errorf("Totals[0]: got %+v, want 円・評価額 3500000・注記なし", total)
	}
	if total.UnrealizedGainHundredths == nil || *total.UnrealizedGainHundredths != 300000 {
		t.Errorf("Totals[0] の含み損益（0.01 単位）: got %v, want 300000（+3,000円）", total.UnrealizedGainHundredths)
	}
	if total.UnrealizedGainRateTenths == nil || *total.UnrealizedGainRateTenths != 94 {
		t.Errorf("Totals[0] の率（0.1%% 単位）: got %v, want 94（9.4%%）", total.UnrealizedGainRateTenths)
	}
	if h.UnrealizedGainHundredths == nil || *h.UnrealizedGainHundredths != 300000 ||
		h.UnrealizedGainRateTenths == nil || *h.UnrealizedGainRateTenths != 94 {
		t.Errorf("6758 の含み損益・率: got %v・%v, want 300000・94", h.UnrealizedGainHundredths, h.UnrealizedGainRateTenths)
	}
}

// 基準8（要件4）: Aが 7203「買 2,500円 100株 2026-01-10」、Bが 6758「買 3,200円 10株 2026-01-10」で持つ
// → Aの一覧は 7203 の1行だけで、6758 は含まれない（保有数量と取得価格は、ログイン中の利用者の売買履歴から計算する）。
// 対照: 同じ DB でBの一覧には 6758 が出る（6758 がAの一覧にないのは、利用者で絞っているからであって、6758 の行が読めないからではない）。
func Test基準8_AC2_利用者Aの一覧に利用者Bの売買履歴は含まれない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserB, "6758", 10, "3200")

	got := listHoldingsOf(t, pool, seedUserA)
	if codes := holdingCodes(got.Holdings); !slices.Equal(codes, []string{"7203"}) {
		t.Errorf("利用者Aの一覧の銘柄コード: got %v, want [7203]（利用者Bの 6758 を含めない）", codes)
	}

	gotB := listHoldingsOf(t, pool, seedUserB)
	if codes := holdingCodes(gotB.Holdings); !slices.Equal(codes, []string{"6758"}) {
		t.Errorf("利用者Bの一覧の銘柄コード: got %v, want [6758]", codes)
	}
}

// 基準8 の補助（5.5）: AとBが同じ 7203 を持つとき、Aの保有数量・取得価格はAの売買履歴の分だけ
// （Aが 100株・2,500円、Bが 30株・3,000円 → Aは 100株・2,500円。Bの分を足すと 130株・2,615.4円 になって誤り）。
// Bの一覧は 30株・3,000円。取得価格は 0.01 単位（250000・300000）。
func Test基準8補助_同じ銘柄を持つ利用者の保有数量と取得価格は自分の売買履歴の分だけになる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserB, "7203", 30, "3000")

	gotA := listHoldingsOf(t, pool, seedUserA)
	hA := onlyHoldingOf(t, gotA.Holdings, "7203")
	if hA.Quantity != 100 || hA.AcquisitionPriceHundredths != 250000 {
		t.Errorf("Aの 7203: got %d株・%d（0.01 単位）, want 100株・250000（2,500円）", hA.Quantity, hA.AcquisitionPriceHundredths)
	}

	gotB := listHoldingsOf(t, pool, seedUserB)
	hB := onlyHoldingOf(t, gotB.Holdings, "7203")
	if hB.Quantity != 30 || hB.AcquisitionPriceHundredths != 300000 {
		t.Errorf("Bの 7203: got %d株・%d（0.01 単位）, want 30株・300000（3,000円）", hB.Quantity, hB.AcquisitionPriceHundredths)
	}
}

// insertTradeWithID は、trades.id を id に指定して、利用者 userID の売買履歴1件を入れる（id は登録した順。要件8）。
// DB から読むときに id を渡し忘れると、同じ取引日の並びが入れた順になることを見分けるため、入れる順と id の順を変えるのに使う。
func insertTradeWithID(t *testing.T, pool *pgxpool.Pool, id int64, userID, stockCode, side string, quantity int64, price, tradeDate string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO trades (id, user_id, stock_code, side, quantity, price, trade_date) VALUES ($1::bigint, $2::uuid, $3::text, $4::text, $5::bigint, $6::text::numeric, $7::text::date)",
		id, userID, stockCode, side, quantity, price, tradeDate,
	); err != nil {
		t.Fatalf("売買履歴（id %d・利用者 %s・%s・%s・%d株・%s円・%s）を入れられない: %v", id, userID, stockCode, side, quantity, price, tradeDate, err)
	}
}

// 基準4（要件8）の統合: 単体（calculate_holdings_test.go の基準4）と同じ4件の売買履歴を、id を指定して id が 3 → 2 → 1 → 4 の順に入れる
// （id 1「買 2,000円 100株 2026-03-02」、id 2「売 2,000円 100株 2026-03-02」、id 3「買 3,000円 100株 2026-03-02」、id 4「買 1,000円 10株 2026-03-01」）
// → Aの一覧の 7203 は 110株・2,900.8円（Hundredths で 290080）。
// 同じ取引日の順は DB に入れた順ではなく id の順（登録した順）で決まる。読み取りが id を渡し忘れると、入れた順（3 → 2 → 1）にたどって別の値になる。
func Test基準4_同じ取引日の売買履歴はDBに入れた順ではなくidの順にたどる(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertTradeWithID(t, pool, 3, seedUserA, "7203", "buy", 100, "3000", "2026-03-02")
	insertTradeWithID(t, pool, 2, seedUserA, "7203", "sell", 100, "2000", "2026-03-02")
	insertTradeWithID(t, pool, 1, seedUserA, "7203", "buy", 100, "2000", "2026-03-02")
	insertTradeWithID(t, pool, 4, seedUserA, "7203", "buy", 10, "1000", "2026-03-01")

	got := listHoldingsOf(t, pool, seedUserA)

	h := onlyHoldingOf(t, got.Holdings, "7203")
	if h.Quantity != 110 || h.AcquisitionPriceHundredths != 290080 {
		t.Errorf("Aの 7203: got %d株・%d（0.01 単位）, want 110株・290080（2,900.8円）", h.Quantity, h.AcquisitionPriceHundredths)
	}
}

// onlyHoldingOf は、holdings がちょうど1行で、その銘柄コードが code であることを確かめて、その行を返す。
func onlyHoldingOf(t *testing.T, holdings []dto.Holding, code string) dto.Holding {
	t.Helper()
	if codes := holdingCodes(holdings); !slices.Equal(codes, []string{code}) {
		t.Fatalf("一覧の銘柄コード: got %v, want [%s]", codes, code)
	}
	return holdings[0]
}

// holdingCodes は、一覧の行の銘柄コードを並び順のまま返す。
func holdingCodes(holdings []dto.Holding) []string {
	codes := make([]string, 0, len(holdings))
	for _, h := range holdings {
		codes = append(codes, h.Code)
	}
	return codes
}
