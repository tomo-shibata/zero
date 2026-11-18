package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/application/query/dto"
	"github.com/tomo-shibata/zero/backend/test/testdb"
)

// 仕様: おまかせ運用の要件（docs/requirements/managed-account.md）の基準1・基準3・基準6・基準9・基準12 の API 部分（要件4・要件5・要件9・要件12・
// 要件15〜要件18）と、プラン（docs/plans/managed-account/plan.md）4.4（API の形）・4.8 の PR②の境界（HoldingsReader.ListManagedAccounts・
// GET /api/holdings の managedAccounts）・5.1 の PR② の行・5.2 の PR② の「一覧の API」。
// 組み立ては 4.8 の境界のとおり: read.NewHoldingsReader(pool) → query.NewListHoldings → handler.NewQueryHandler → router.New（getHoldingsFromDB・newHoldingsAPIFor）。
// 記録は insertManagedValuation で SQL から入れる（managed_accounts_fixtures_test.go）。
// 要件定義「成功基準」のダミー価格（7203=3,000円・6758=3,500円・AAPL=180.00ドル、どれも 2026-10-06）と、
// 口座の書き方「<名前> <日付> 評価額 <評価額> 損益 <損益>」を使う。Aは seedUserA、Bは seedUserB。

// assertResponseField は、応答が 200 で、本文の JSON のキー field の値が want（JSON）と同じ（値の種類・配列の順まで）であることを確かめる。
// 本文のほかのキーは見ない。
func assertResponseField(t *testing.T, rec *httptest.ResponseRecorder, field, want string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータス: got %d, want %d\n本文: %s", rec.Code, http.StatusOK, rec.Body.Bytes())
	}
	decoded, err := decodeJSON(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("本文を JSON として読めない: %v\n本文: %s", err, rec.Body.Bytes())
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("本文が JSON のオブジェクトでない\n本文: %s", rec.Body.Bytes())
	}
	got, ok := object[field]
	if !ok {
		t.Fatalf("本文に %q のキーがない\n本文: %s", field, rec.Body.Bytes())
	}
	wantValue, err := decodeJSON([]byte(want))
	if err != nil {
		t.Fatalf("期待値の JSON を読めない（テストの誤り）: %v", err)
	}
	if !reflect.DeepEqual(got, wantValue) {
		gotJSON, _ := json.Marshal(got)
		t.Errorf("%s が違う\n got: %s\nwant: %s", field, gotJSON, compactJSON([]byte(want)))
	}
}

// 基準1 の API 部分（要件2・要件4・要件6・要件12・要件13）: Aの記録が「WealthNavi 2026-11-16 評価額 4,000,000円 損益 +1,000,000円」
// 「WealthNavi 2026-10-31 評価額 3,950,000円 損益 +950,000円」（新しい日付を先に入れる。入れた順の最後を選ぶ誤りを見分けるため）で、売買履歴なし
// → managedAccounts が WealthNavi の1つ（valuation "4000000"・principal "3000000"・unrealizedGain "1000000"・unrealizedGainRate "33.3"・
// valuationDate "2026-11-16"。2026-10-31 の値は出ない）、totals が円の1つ（"4000000"・"1000000"・"33.3"・excludesUnpriced false）、
// holdings が []、combinedTotal が null。
func Test基準1_API_口座の最新の記録からカードの値を作り円の合計にも含める(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-16", 4_000_000, 1_000_000)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-10-31", 3_950_000, 950_000)

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [],
		"totals": [{"currency": "JPY", "valuation": "4000000", "excludesUnpriced": false,
		            "unrealizedGain": "1000000", "unrealizedGainRate": "33.3"}],
		"combinedTotal": null,
		"managedAccounts": [
			{"name": "WealthNavi", "valuation": "4000000", "principal": "3000000",
			 "unrealizedGain": "1000000", "unrealizedGainRate": "33.3", "valuationDate": "2026-11-16"}
		]
	}`)
}

// 基準3 の API 部分（要件9）: SQL で ロボアド B → WealthNavi → Alpha の順に口座を入れる
// → managedAccounts の name が Alpha・WealthNavi・ロボアド B の順（名前の文字コードの昇順。DB から返る順に頼らない）。
func Test基準3_API_口座は名前の昇順に並ぶ(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "ロボアド B", "2026-11-16", 500_000, 0)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-16", 4_000_000, 1_000_000)
	insertManagedValuation(t, pool, seedUserA, "Alpha", "2026-11-16", 300_000, 30_000)

	rec := getHoldingsFromDB(t, pool)

	assertResponseField(t, rec, "managedAccounts", `[
		{"name": "Alpha", "valuation": "300000", "principal": "270000",
		 "unrealizedGain": "30000", "unrealizedGainRate": "11.1", "valuationDate": "2026-11-16"},
		{"name": "WealthNavi", "valuation": "4000000", "principal": "3000000",
		 "unrealizedGain": "1000000", "unrealizedGainRate": "33.3", "valuationDate": "2026-11-16"},
		{"name": "ロボアド B", "valuation": "500000", "principal": "500000",
		 "unrealizedGain": "0", "unrealizedGainRate": "0.0", "valuationDate": "2026-11-16"}
	]`)
}

// 基準6（要件5）: Aが WealthNavi（記録1件。2026-11-15・評価額 4,000,000円・損益 +1,000,000円）を持ち、Bが Alpha（記録1件）と
// WealthNavi（記録1件。2026-11-16・評価額 9,000,000円・損益 +2,000,000円）を持つ
// → Aの一覧の managedAccounts は Aの WealthNavi（"4000000"・"2026-11-15"）の1つだけで、totals の円の評価額は "4000000"。
// 逆向きに、Bの一覧（newHoldingsAPIFor(pool, seedUserB)）は Alpha と Bの WealthNavi（"9000000"）で、Aの値が混ざらない
// （Bの WealthNavi を新しい日付にして、利用者で絞る前に名前ごとの最新を選ぶ誤りを見分ける）。
// Bの Alpha は 2026-11-16・評価額 500,000円・損益 0円（要件定義に指定がないので、結果に関係しにくい値にした）。
// 合計（B）は評価額 9,500,000円・含み損益 +2,000,000円・率 26.7%（2,000,000 ÷（7,000,000 ＋ 500,000）× 100 ＝ 26.66…%）。
func Test基準6_API_一覧にはログイン中の利用者の口座だけが出る(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-15", 4_000_000, 1_000_000)
	insertManagedValuation(t, pool, seedUserB, "Alpha", "2026-11-16", 500_000, 0)
	insertManagedValuation(t, pool, seedUserB, "WealthNavi", "2026-11-16", 9_000_000, 2_000_000)

	t.Run("Aの一覧", func(t *testing.T) {
		t.Parallel()
		rec := getHoldingsFromDB(t, pool)

		assertResponseField(t, rec, "managedAccounts", `[
			{"name": "WealthNavi", "valuation": "4000000", "principal": "3000000",
			 "unrealizedGain": "1000000", "unrealizedGainRate": "33.3", "valuationDate": "2026-11-15"}
		]`)
		assertResponseField(t, rec, "totals", `[
			{"currency": "JPY", "valuation": "4000000", "excludesUnpriced": false,
			 "unrealizedGain": "1000000", "unrealizedGainRate": "33.3"}
		]`)
	})

	t.Run("Bの一覧", func(t *testing.T) {
		t.Parallel()
		rec := serveGet(t, newHoldingsAPIFor(pool, seedUserB), holdingsPath, allowedHost)

		assertResponseField(t, rec, "managedAccounts", `[
			{"name": "Alpha", "valuation": "500000", "principal": "500000",
			 "unrealizedGain": "0", "unrealizedGainRate": "0.0", "valuationDate": "2026-11-16"},
			{"name": "WealthNavi", "valuation": "9000000", "principal": "7000000",
			 "unrealizedGain": "2000000", "unrealizedGainRate": "28.6", "valuationDate": "2026-11-16"}
		]`)
		assertResponseField(t, rec, "totals", `[
			{"currency": "JPY", "valuation": "9500000", "excludesUnpriced": false,
			 "unrealizedGain": "2000000", "unrealizedGainRate": "26.7"}
		]`)
	})
}

// 基準9 の API 部分（要件15・要件16）: Aの売買履歴が 7203「買 2,500円 100株」、6758「買 4,000円 10株」、AAPL「買 150.25ドル 10株」
// （どれも価格保存済み）で、記録が「WealthNavi 2026-11-16 評価額 4,000,000円 損益 +1,000,000円」
// → totals が 円「評価額 4,335,000円・含み損益 +1,045,000円・+31.8%」・ドル「1,800.00ドル・+297.50ドル・+19.8%」の2つ（円、ドルの順）。
func Test基準9_API_円の合計に口座を含めドルの合計は変わらない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserA, "6758", 10, "4000")
	insertBuy(t, pool, seedUserA, "AAPL", 10, "150.25")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	insertClosePrice(t, pool, "6758", "2026-10-06", "3500")
	insertClosePrice(t, pool, "AAPL", "2026-10-06", "180.00")
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-16", 4_000_000, 1_000_000)

	rec := getHoldingsFromDB(t, pool)

	assertResponseField(t, rec, "totals", `[
		{"currency": "JPY", "valuation": "4335000", "excludesUnpriced": false,
		 "unrealizedGain": "1045000", "unrealizedGainRate": "31.8"},
		{"currency": "USD", "valuation": "1800.00", "excludesUnpriced": false,
		 "unrealizedGain": "297.50", "unrealizedGainRate": "19.8"}
	]`)
}

// 基準12 の API 部分（要件18）: Aの売買履歴が MSFT「買 400.00ドル 5株」（価格なし）だけで、記録が「WealthNavi 2026-11-16 評価額 4,000,000円 損益 +1,000,000円」、
// レートは 2026-11-14 150.00円
// → combinedTotal が {"valuation":"4000000","excludesUnpriced":true,"unrealizedGain":"1000000","unrealizedGainRate":"33.3",
// "usdJpyRate":"150.00","usdJpyRateDate":"2026-11-14"}（含み損益は null にならない）。
func Test基準12_API_円とドルの銘柄がすべて価格なしでも口座があれば円にまとめた含み損益を返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "MSFT", "Microsoft", currencyUSD)
	insertBuy(t, pool, seedUserA, "MSFT", 5, "400.00")
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-16", 4_000_000, 1_000_000)
	insertUsdJpyRate(t, pool, "2026-11-14", "150.00")

	rec := getHoldingsFromDB(t, pool)

	assertResponseField(t, rec, "combinedTotal", `{
		"valuation": "4000000", "excludesUnpriced": true,
		"unrealizedGain": "1000000", "unrealizedGainRate": "33.3",
		"usdJpyRate": "150.00", "usdJpyRateDate": "2026-11-14"
	}`)
}

// 補助（要件4・プラン 5.2）: 最新の記録の選び方。WealthNavi の記録を 2026-10-31（評価額 4,100,000円）→ 2026-11-16（4,000,000円）→
// 2026-11-01（3,900,000円）の順に入れる → valuationDate が "2026-11-16"、valuation が "4000000"
// （最初に入れたもの・最後に入れたもの・評価額の最も大きいもの・最も小さいもののどれとも違う）。
// 損益は評価額と元本が 3,000,000円で一定になるよう +1,100,000円・+1,000,000円・+900,000円にした（結果は最新の記録の損益 1,000,000円）。
func Test補助_API_最新の記録は日付がいちばん新しいもの(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-10-31", 4_100_000, 1_100_000)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-16", 4_000_000, 1_000_000)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-01", 3_900_000, 900_000)

	rec := getHoldingsFromDB(t, pool)

	assertResponseField(t, rec, "managedAccounts", `[
		{"name": "WealthNavi", "valuation": "4000000", "principal": "3000000",
		 "unrealizedGain": "1000000", "unrealizedGainRate": "33.3", "valuationDate": "2026-11-16"}
	]`)
}

// 補助（要件4・プラン 5.2）: 口座が2つあり、片方の最新の日付がもう片方より古くても、それぞれの最新が出る
// （全体で最新の1件だけを読む誤りを見分ける）。WealthNavi は 2026-11-16 が最新、Alpha は 2026-10-31 が最新
// （Alpha は古い 2026-10-01 の記録も持つ）。
func Test補助_API_口座ごとにそれぞれの最新の記録が出る(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertManagedValuation(t, pool, seedUserA, "WealthNavi", "2026-11-16", 4_000_000, 1_000_000)
	insertManagedValuation(t, pool, seedUserA, "Alpha", "2026-10-31", 500_000, 0)
	insertManagedValuation(t, pool, seedUserA, "Alpha", "2026-10-01", 480_000, -20_000)

	rec := getHoldingsFromDB(t, pool)

	assertResponseField(t, rec, "managedAccounts", `[
		{"name": "Alpha", "valuation": "500000", "principal": "500000",
		 "unrealizedGain": "0", "unrealizedGainRate": "0.0", "valuationDate": "2026-10-31"},
		{"name": "WealthNavi", "valuation": "4000000", "principal": "3000000",
		 "unrealizedGain": "1000000", "unrealizedGainRate": "33.3", "valuationDate": "2026-11-16"}
	]`)
}

// accountsFailingHoldingsReader は、売買履歴・分割・並び・レートは空で、口座を読むと必ず失敗する HoldingsReader（境界の外側の fake）。
type accountsFailingHoldingsReader struct {
	emptyHoldingsReader
}

// accountsFailingDetail は、fake が返すエラーの内容。応答に出てはいけない（今の一覧の失敗と同じ扱い）。
const accountsFailingDetail = "fake: connection refused to db.internal:5432 (managed_accounts)"

func (accountsFailingHoldingsReader) ListManagedAccounts(context.Context, uuid.UUID) ([]dto.ManagedAccountRow, error) {
	return nil, errors.New(accountsFailingDetail)
}

// 補助（プラン 5.2）: ListManagedAccounts が失敗する HoldingsReader を注入した router.New に GET /api/holdings
// → 500 と {"error":"internal_error"}（今の一覧の失敗と同じ扱い。内部のエラー内容は返さない）。
func Test補助_API_口座を読めないときは500とinternal_errorを返す(t *testing.T) {
	t.Parallel()
	h := newHoldingsAPI(accountsFailingHoldingsReader{})

	rec := serveGet(t, h, holdingsPath, allowedHost)

	assertJSONResponse(t, rec, http.StatusInternalServerError, `{"error":"internal_error"}`)
}
