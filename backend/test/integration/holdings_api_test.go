package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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

// 売買履歴への作り変え（docs/plans/trade-history/plan.md 5.2・5.3。PR①）: 前提の保有データ（holdings）は売買履歴（買い）で作る
// （insertBuy）。偽の読み取りは ListHoldingRows → ListTradeRows。
// PR②（4.3・4.6・5.3・5.5）: 行に currency を足し、金額（単価・評価額・合計評価額）はすべて文字列にし、合計評価額は通貨ごとの配列 totals にする。
// 単価は円が小数1桁（"2500.0"）・ドルが小数2桁（"150.25"）、評価額は円が整数（"300000"）・ドルが小数2桁（"1800.00"）。
// totals は保有のある通貨だけを円、ドルの順に入れ、0件なら空の配列。円の終値に 0.1円より細かい値があれば 500（黙って切り捨てない）。
// 保有数量が途中で負になる売買履歴を SQL で入れると、API は 500 と internal_error（5.3、判断6）。
//
// 仕様: GET /api/holdings の応答（プラン docs/plans/stock-holdings/plan.md 4.4「API」）。
// 確かめるのは、プラン 5.1 の次の行（PR②）。
//   - 「AC-1、7、8、8a、9、11 の API 部分」: 要件（docs/requirements/stock-holdings.md）の各 AC と同じデータで
//     GET /api/holdings を呼び、JSON を比べる（画面の確認は PR④ の E2E）。
//   - 補助 FR-7（SR-5）の API の0件の JSON、FR-8（TC-11）の最新の価格、FR-4・FR-12・FR-19（TC-14）の scale の違い、
//     FR-22（TC-24）の 500、判断12（SR-6、S-1）の 403 と /healthz。
//   - PR② ステップ6 の指摘で足した行: FR-12・FR-19・4.2（PR② の EC-3、TC-03）の scale 2 と 0.1円、
//     FR-1（PR② の TC-05）の他の利用者の保有がある DB。
//     判断12（PR② の S-1、EC-5）と 4.4 API のヘッダー（PR② の S-2）は holdings_api_hosts_headers_test.go、
//     4.4 API のあふれ（PR② の EC-1）は holdings_api_overflow_test.go。
//
// 組み立ては 4.4 の境界のとおり: read.NewHoldingsReader(pool) → query.NewListHoldings → handler.NewQueryHandler →
// router.New(router.Deps{…})。ログイン中の利用者（DevUserID）は利用者A。
// router は /api/ 以下を Host ヘッダーの許可リスト（Deps.AllowedHosts）に限るので（判断12）、
// リクエストの Host には許可リストに入れた allowedHost を必ず付ける（付けないと 403 になる）。
//
// 価格の基準日は、要件「成功基準」のダミー価格の取引日 2026-10-06 を使う。

// allowedHost は、テストで Deps.AllowedHosts に入れる Host（cmd/api が作る "127.0.0.1:<PORT>" の形。判断12）。
const allowedHost = "127.0.0.1:8080"

// holdingsPath・healthzPath は、router.New が持つパス（4.4）。
const (
	holdingsPath = "/api/holdings"
	healthzPath  = "/healthz"
)

// newHoldingsAPI は、reader から保有データを読む API を、4.4 の境界のとおりに組み立てる。ログイン中の利用者は利用者A。
func newHoldingsAPI(reader query.HoldingsReader) http.Handler {
	return router.New(router.Deps{
		QueryHandler: handler.NewQueryHandler(query.NewListHoldings(reader)),
		DevUserID:    uuid.MustParse(seedUserA),
		AllowedHosts: []string{allowedHost},
	})
}

// serveGet は、Host ヘッダーを host にした GET path を h に送り、応答を返す。
func serveGet(t *testing.T, h http.Handler, path, host string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// getHoldingsFromDB は、pool の DB を読む API で、利用者Aとして GET /api/holdings を呼ぶ。
func getHoldingsFromDB(t *testing.T, pool *pgxpool.Pool) *httptest.ResponseRecorder {
	t.Helper()
	return serveGet(t, newHoldingsAPI(read.NewHoldingsReader(pool)), holdingsPath, allowedHost)
}

// assertJSONResponse は、応答のステータスが wantStatus で、本文が JSON として want と同じであることを確かめる。
// 同じとは、キーの集まり・値・値の種類（文字列／数／真偽値／null）・配列の順がすべて同じこと。
// 数は書かれた字面（json.Number）のまま比べるので、300000 と "300000"、"2500.0" と 2500 と "2500" は別のものになる
// （4.3: 単価は円が小数1桁・ドルが小数2桁の文字列、評価額と合計は円が整数・ドルが小数2桁の文字列、価格がなければ null）。
func assertJSONResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, want string) {
	t.Helper()
	body := rec.Body.Bytes()
	if rec.Code != wantStatus {
		t.Errorf("ステータス: got %d, want %d\n本文: %s", rec.Code, wantStatus, body)
	}
	wantValue, err := decodeJSON([]byte(want))
	if err != nil {
		t.Fatalf("期待値の JSON を読めない（テストの誤り）: %v", err)
	}
	got, err := decodeJSON(body)
	if err != nil {
		t.Fatalf("本文を JSON として読めない: %v\n本文: %s", err, body)
	}
	if !reflect.DeepEqual(got, wantValue) {
		t.Errorf("本文の JSON が違う\n got: %s\nwant: %s", compactJSON(body), compactJSON([]byte(want)))
	}
}

// decodeJSON は、data を1つの JSON の値として読む。数は json.Number のまま残す。
// 値の後に空白以外のものが続くときはエラーにする。
func decodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON の値の後に余分なものがある")
	}
	return v, nil
}

// compactJSON は、失敗のメッセージ用に data の空白を詰めて返す。JSON として読めなければそのまま返す。
func compactJSON(data []byte) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return string(data)
	}
	return buf.String()
}

// AC-1（FR-1、FR-3、FR-8、FR-9、FR-11〜14、FR-20）の API 部分:
// 利用者Aが 7203 を100株、取得価格2,500円で保有し、7203 の価格（3,000円・2026-10-06）が保存済み
// → 7203 の行は「7203 / トヨタ自動車 / 100株 / 2,500円 / 3,000円 / 2026-10-06 / 300,000円」。
// 銘柄名は銘柄マスタから取る（FR-20）。単価は scale 0 で保存した値（"2500"・"3000"）でも小数第1位付きの文字列で返す。
func TestAC1_API_価格のある銘柄の行を単価は小数第1位付きの文字列で評価額は整数で返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": "300000",
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"}
		],
		"totals": [{"currency": "JPY", "valuation": "300000", "excludesUnpriced": false,
		            "unrealizedGain": "50000", "unrealizedGainRate": "20.0"}]
	}`)
}

// AC-7・AC-8a（FR-6）の API 部分:
// 利用者Aが 7203 を100株、6758 を10株保有し、両方の価格（3,000円・3,500円）が保存済み
// → 合計評価額は 335,000円（AC-7）。全銘柄に価格があるので、totals の円の行の excludesUnpriced は false（AC-8a）。
// 6758 の取得価格は AC に指定がないので 3,200円 とした。並び順は銘柄コードの昇順（FR-5）。
func TestAC7_AC8a_API_全銘柄に価格があれば合計評価額は評価額の合計で注記のフラグはfalse(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserA, "6758", 10, "3200")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	insertClosePrice(t, pool, "6758", "2026-10-06", "3500")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "6758", "name": "ソニーグループ", "currency": "JPY", "quantity": 10,
			 "acquisitionPrice": "3200.0", "currentPrice": "3500.0",
			 "priceDate": "2026-10-06", "valuation": "35000",
			 "unrealizedGain": "3000", "unrealizedGainRate": "9.4"},
			{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": "300000",
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"}
		],
		"totals": [{"currency": "JPY", "valuation": "335000", "excludesUnpriced": false,
		            "unrealizedGain": "53000", "unrealizedGainRate": "18.8"}]
	}`)
}

// AC-8（FR-6）・AC-11（FR-10）の API 部分:
// 利用者Aが 7203 を100株（価格保存済み）と 9984（価格は一度も保存されていない）を保有している
// → 9984 の行の currentPrice・priceDate・valuation は null（AC-11。画面では「取得できません」）、
// 7203 の行は AC-1 のとおり。合計評価額は 7203 の分だけの 300,000円で、totals の円の行の excludesUnpriced は true
// （AC-8。画面では「価格を取得できない銘柄を含みません」）。
// 9984 の保有数量と取得価格は AC に指定がないので 10株・9,000円 とした。
func TestAC8_AC11_API_価格のない銘柄はnullで返し合計評価額に含めず注記のフラグはtrue(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserA, "9984", 10, "9000")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": "300000",
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"},
			{"code": "9984", "name": "ソフトバンクグループ", "currency": "JPY", "quantity": 10,
			 "acquisitionPrice": "9000.0", "currentPrice": null,
			 "priceDate": null, "valuation": null,
			 "unrealizedGain": null, "unrealizedGainRate": null}
		],
		"totals": [{"currency": "JPY", "valuation": "300000", "excludesUnpriced": true,
		            "unrealizedGain": "50000", "unrealizedGainRate": "20.0"}]
	}`)
}

// AC-9（FR-7）の API 部分と、補助 FR-7（SR-5）の API、補助 FR-1（PR② の TC-05）:
// 利用者Aが株式を1件も保有していない（DB には利用者Bの 6758（10株・3,200円）の保有がある）
// → {"holdings":[],"totals":[]}（holdings は null ではなく空の配列。4.4）。
// 利用者Bの保有は A の応答に出ない（API の経路でも、ログイン中の利用者の保有データだけを読む。FR-1）。
func TestAC9_FR7_SR5_FR1_PR2_TC05_API_保有株式が0件なら他の利用者の保有があっても空の配列と合計0を返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserB, "6758", 10, "3200")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{"holdings":[],"totals":[]}`)
}

// 補助 FR-8（TC-11）: 現在の価格は、保存済みの価格のうち最新の取引日の終値で、価格の基準日はその取引日。
// 7203（100株・2,500円）の価格を 2026-10-05 の 2,900円 → 2026-10-06 の 3,000円 → 2026-10-02 の 3,100円 の順に入れる
// （最新の行が、最初に入れた行・最後に入れた行・最も古い行・終値の最も高い行のどれとも違うようにして、
// 「入れた順の最初／最後を読む」「取引日の古い行を選ぶ」「終値の最大を選ぶ」誤りを見分けるため。
// PR② ステップ6 の TC-01、2ラウンド目の PR2_TC08 で 10-02 の終値を 2,800円 から 3,100円 に変えた）。
// → 1行・quantity 100・currentPrice "3000.0"・priceDate "2026-10-06"・valuation "300000"（文字列）。
func TestFR8_TC11_PR2_TC08_API_現在の価格は最新の取引日の終値で行は増えない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertClosePrice(t, pool, "7203", "2026-10-05", "2900")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	insertClosePrice(t, pool, "7203", "2026-10-02", "3100")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": "300000",
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"}
		],
		"totals": [{"currency": "JPY", "valuation": "300000", "excludesUnpriced": false,
		            "unrealizedGain": "50000", "unrealizedGainRate": "20.0"}]
	}`)
}

// 補助 FR-4・FR-12・FR-19（TC-14）: 桁数を指定しない NUMERIC は書いた桁（scale）のまま保存されるので、
// DB から読むときに scale が 0 でも 1 でも正しく 0.1円単位にする（プラン 4.2）。
// 7203 を 1株・価格 2000（scale 0）と 2株・2001.0（scale 1）で買い、終値は 1234.5
// → quantity 3・acquisitionPrice "2000.7"・currentPrice "1234.5"・valuation "3703"・totals の円の行の valuation "3703"（どちらも文字列）。
func TestFR4_FR12_FR19_TC14_API_scaleの違う単価も正しく読んで集約する(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertBuy(t, pool, seedUserA, "7203", 1, "2000")
	insertBuy(t, pool, seedUserA, "7203", 2, "2001.0")
	insertClosePrice(t, pool, "7203", "2026-10-06", "1234.5")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 3,
			 "acquisitionPrice": "2000.7", "currentPrice": "1234.5",
			 "priceDate": "2026-10-06", "valuation": "3703",
			 "unrealizedGain": "-2299", "unrealizedGainRate": "-38.3"}
		],
		"totals": [{"currency": "JPY", "valuation": "3703", "excludesUnpriced": false,
		            "unrealizedGain": "-2299", "unrealizedGainRate": "-38.3"}]
	}`)
}

// 補助 FR-12・FR-19・4.2（PR② の EC-3、TC-03）: 単価が scale 2 以上で末尾が 0 の NUMERIC（例 3000.00・1234.50）や、
// 最小の単価 0.1円でも、正しく 0.1円単位にして、小数第1位を1桁付けた文字列で返す（4.2、4.4「API」）。
// 3000.00・2001.00・1234.50 は「= round(…, 1)」を満たすので保存できる値（4.2）。
func TestFR12_FR19_PR2_EC3_TC03_API_scale2の単価や最小の単価0_1円も小数第1位付きの文字列で返す(t *testing.T) {
	t.Parallel()

	// 7203 を1株・取得価格 0.1、終値 3000.00（scale 2、2026-10-06）
	// → acquisitionPrice "0.1"・currentPrice "3000.0"・valuation "3000"（文字列）。
	t.Run("取得価格0.1と終値3000.00", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 1, "0.1")
		insertClosePrice(t, pool, "7203", "2026-10-06", "3000.00")

		rec := getHoldingsFromDB(t, pool)

		assertJSONResponse(t, rec, http.StatusOK, `{
			"holdings": [
				{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 1,
				 "acquisitionPrice": "0.1", "currentPrice": "3000.0",
				 "priceDate": "2026-10-06", "valuation": "3000",
				 "unrealizedGain": "3000", "unrealizedGainRate": "2999900.0"}
			],
			"totals": [{"currency": "JPY", "valuation": "3000", "excludesUnpriced": false,
			            "unrealizedGain": "3000", "unrealizedGainRate": "2999900.0"}]
		}`)
	})

	// 7203 を1株・取得価格 2001.00、終値 1234.50（どちらも scale 2、2026-10-06）
	// → acquisitionPrice "2001.0"・currentPrice "1234.5"・valuation "1234"（文字列）。
	t.Run("取得価格2001.00と終値1234.50", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 1, "2001.00")
		insertClosePrice(t, pool, "7203", "2026-10-06", "1234.50")

		rec := getHoldingsFromDB(t, pool)

		assertJSONResponse(t, rec, http.StatusOK, `{
			"holdings": [
				{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 1,
				 "acquisitionPrice": "2001.0", "currentPrice": "1234.5",
				 "priceDate": "2026-10-06", "valuation": "1234",
				 "unrealizedGain": "-767", "unrealizedGainRate": "-38.3"}
			],
			"totals": [{"currency": "JPY", "valuation": "1234", "excludesUnpriced": false,
			            "unrealizedGain": "-767", "unrealizedGainRate": "-38.3"}]
		}`)
	})
}

// failingHoldingsReader は、保有データを読むと必ず失敗する HoldingsReader（境界の外側の fake。FR-22 用）。
type failingHoldingsReader struct{}

// failingReaderDetail は、fake が返すエラーの内容。応答に出てはいけない（4.4: 内部のエラー内容は返さない）。
const failingReaderDetail = "fake: connection refused to db.internal:5432"

func (failingHoldingsReader) ListTradeRows(context.Context, uuid.UUID) ([]dto.TradeRow, error) {
	return nil, errors.New(failingReaderDetail)
}

// ListStockSplits は空を返す（株式分割のプラン 4.8 PR①。売買履歴の読み取りが失敗すれば、この結果は使われない）。
func (failingHoldingsReader) ListStockSplits(context.Context, uuid.UUID) ([]dto.StockSplitRow, error) {
	return []dto.StockSplitRow{}, nil
}

// ListHoldingOrder は空の並びを返す（カード表示のプラン 4.6 PR②。売買履歴の読み取りが失敗すれば、この結果は使われない）。
func (failingHoldingsReader) ListHoldingOrder(context.Context, uuid.UUID) ([]string, error) {
	return []string{}, nil
}

// 補助 FR-22（TC-24）: エラーを返す HoldingsReader を注入した router.New に GET /api/holdings
// → 500 と {"error":"internal_error"}（4.4。内部のエラー内容は返さない）。
func TestFR22_TC24_API_保有データを読めないときは500とinternal_errorを返す(t *testing.T) {
	t.Parallel()
	h := newHoldingsAPI(failingHoldingsReader{})

	rec := serveGet(t, h, holdingsPath, allowedHost)

	assertJSONResponse(t, rec, http.StatusInternalServerError, `{"error":"internal_error"}`)
}

// emptyHoldingsReader は、保有データが0件の HoldingsReader（境界の外側の fake。判断12 用。DB を使わずに router を組み立てるため）。
type emptyHoldingsReader struct{}

func (emptyHoldingsReader) ListTradeRows(context.Context, uuid.UUID) ([]dto.TradeRow, error) {
	return []dto.TradeRow{}, nil
}

// ListStockSplits は空を返す（株式分割のプラン 4.8 PR①）。
func (emptyHoldingsReader) ListStockSplits(context.Context, uuid.UUID) ([]dto.StockSplitRow, error) {
	return []dto.StockSplitRow{}, nil
}

// ListHoldingOrder は空の並びを返す（カード表示のプラン 4.6 PR②）。
func (emptyHoldingsReader) ListHoldingOrder(context.Context, uuid.UUID) ([]string, error) {
	return []string{}, nil
}

// 補助 判断12（SR-6、S-1）: Deps.AllowedHosts にない Host（例 example.com）で /api/holdings を呼ぶと
// 403 と {"error":"forbidden"}。/healthz は Host に関係なく 200・"ok"（許可リストは /api/ 以下だけにかける。4.4）。
func Test判断12_SR6_S1_許可リストにないHostのapiは403でhealthzはHostに関係なく200(t *testing.T) {
	t.Parallel()
	h := newHoldingsAPI(emptyHoldingsReader{})

	t.Run("許可リストにないHostのapi_holdingsは403とforbidden", func(t *testing.T) {
		t.Parallel()
		rec := serveGet(t, h, holdingsPath, "example.com")
		assertJSONResponse(t, rec, http.StatusForbidden, `{"error":"forbidden"}`)
	})

	// 対照: 同じ router で、許可リストにある Host なら 200（403 が Host 以外の理由でないことを確かめる）。
	t.Run("対照_許可リストにあるHostのapi_holdingsは200", func(t *testing.T) {
		t.Parallel()
		rec := serveGet(t, h, holdingsPath, allowedHost)
		assertJSONResponse(t, rec, http.StatusOK, `{"holdings":[],"totals":[]}`)
	})

	for _, host := range []string{"example.com", allowedHost} {
		t.Run("healthzはHostが"+host+"でも200とok", func(t *testing.T) {
			t.Parallel()
			rec := serveGet(t, h, healthzPath, host)
			if rec.Code != http.StatusOK {
				t.Errorf("ステータス: got %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Body.String(); got != "ok" {
				t.Errorf("本文: got %q, want %q", got, "ok")
			}
		})
	}
}

// 5.3・判断6: 保有数量が途中で負になる売買履歴（取り込みを通さずに SQL で入れた壊れたデータ。Aが持っていない 7203 を売る）が
// あると、GET /api/holdings は 500 と {"error":"internal_error"}（誤った一覧を 200 で返さない。本文に内容を出さない）。
// 対照: 同じ DB で売りを買いの後ろにした正しい売買履歴なら 200。
func TestPR1_5_3_API_保有数量が負になる売買履歴なら500とinternal_errorを返す(t *testing.T) {
	t.Parallel()

	t.Run("持っていない株の売りがあると500", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "6758", 10, "3200")
		insertTrade(t, pool, seedUserA, "7203", "sell", 10, "2500", "2026-02-10")

		rec := getHoldingsFromDB(t, pool)

		assertJSONResponse(t, rec, http.StatusInternalServerError, `{"error":"internal_error"}`)
	})

	t.Run("対照_買いの後の売りなら200", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 10, "2500")
		insertTrade(t, pool, seedUserA, "7203", "sell", 10, "2600", "2026-02-10")

		rec := getHoldingsFromDB(t, pool)

		assertJSONResponse(t, rec, http.StatusOK, emptyListJSON)
	})
}

// ---- ドルの銘柄と通貨ごとの合計（docs/plans/trade-history/plan.md 4.3・5.3。PR②） ----

// 基準9 の API 部分（要件1・要件11・要件14）: Aの売買履歴が AAPL「買 150.25ドル 10株 2026-01-10」で、AAPL の価格（180.00ドル・2026-10-06）が保存済み
// → AAPL の行は「AAPL / Apple / USD / 10株 / "150.25" / "180.00" / 2026-10-06 / "1800.00"」。通貨は銘柄マスタから取る。
// 合計は ドルの1件だけ（"1800.00"）。単価はドルが小数2桁、評価額はドルが小数2桁の文字列。
func Test基準9_API_ドルの銘柄の行は通貨USDで単価と評価額を小数2桁の文字列で返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertBuy(t, pool, seedUserA, "AAPL", 10, "150.25")
	insertClosePrice(t, pool, "AAPL", "2026-10-06", "180.00")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "AAPL", "name": "Apple", "currency": "USD", "quantity": 10,
			 "acquisitionPrice": "150.25", "currentPrice": "180.00",
			 "priceDate": "2026-10-06", "valuation": "1800.00",
			 "unrealizedGain": "297.50", "unrealizedGainRate": "19.8"}
		],
		"totals": [{"currency": "USD", "valuation": "1800.00", "excludesUnpriced": false,
		            "unrealizedGain": "297.50", "unrealizedGainRate": "19.8"}]
	}`)
}

// 基準10 の API 部分（要件12）: Aの売買履歴が 7203「買 2,500円 100株」と AAPL「買 150.25ドル 10株」で、どちらも価格が保存済み
// → totals は円、ドルの順に1件ずつ（円 "300000"・ドル "1800.00"、どちらも excludesUnpriced は false）。
// 銘柄の並びは銘柄コードの昇順（数字で始まる 7203 が先）。ドルの銘柄を先に入れても、totals は円、ドルの順。
func Test基準10_API_合計は通貨ごとに円ドルの順で返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertBuy(t, pool, seedUserA, "AAPL", 10, "150.25")
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertClosePrice(t, pool, "AAPL", "2026-10-06", "180.00")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": "300000",
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"},
			{"code": "AAPL", "name": "Apple", "currency": "USD", "quantity": 10,
			 "acquisitionPrice": "150.25", "currentPrice": "180.00",
			 "priceDate": "2026-10-06", "valuation": "1800.00",
			 "unrealizedGain": "297.50", "unrealizedGainRate": "19.8"}
		],
		"totals": [
			{"currency": "JPY", "valuation": "300000", "excludesUnpriced": false,
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"},
			{"currency": "USD", "valuation": "1800.00", "excludesUnpriced": false,
			 "unrealizedGain": "297.50", "unrealizedGainRate": "19.8"}
		]
	}`)
}

// 基準11 の API 部分（要件12・要件13）: Aの売買履歴が 7203「買 2,500円 100株」（価格保存済み）と MSFT「買 400.00ドル 5株」（価格なし）
// → 円の合計は "300000"（excludesUnpriced false）、ドルの合計は "0.00"（excludesUnpriced true）。MSFT の行は価格の3項目が null。
func Test基準11_API_ドルの銘柄がすべて価格なしならドルの合計は0_00で注記のフラグはtrue(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "MSFT", "Microsoft", currencyUSD)
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertBuy(t, pool, seedUserA, "MSFT", 5, "400.00")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": "300000",
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"},
			{"code": "MSFT", "name": "Microsoft", "currency": "USD", "quantity": 5,
			 "acquisitionPrice": "400.00", "currentPrice": null,
			 "priceDate": null, "valuation": null,
			 "unrealizedGain": null, "unrealizedGainRate": null}
		],
		"totals": [
			{"currency": "JPY", "valuation": "300000", "excludesUnpriced": false,
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"},
			{"currency": "USD", "valuation": "0.00", "excludesUnpriced": true,
			 "unrealizedGain": null, "unrealizedGainRate": null}
		]
	}`)
}

// 5.5: ドルを切り捨てない。AAPL を3株（買 150.25ドル）、終値 180.25ドル → 評価額 "540.75"（円のように1円未満を切り捨てない。要件11）、合計も "540.75"。
// 終値 0.01ドルの AAPL（1株）→ 単価・評価額・合計は "0.01"（整数部の 0 を落とさない）。
func Test補助_API_ドルの評価額は切り捨てずに小数2桁の文字列で返す(t *testing.T) {
	t.Parallel()

	t.Run("終値180.25ドルの3株は540.75", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertStock(t, pool, "AAPL", "Apple", currencyUSD)
		insertBuy(t, pool, seedUserA, "AAPL", 3, "150.25")
		insertClosePrice(t, pool, "AAPL", "2026-10-06", "180.25")

		rec := getHoldingsFromDB(t, pool)

		assertJSONResponse(t, rec, http.StatusOK, `{
			"holdings": [
				{"code": "AAPL", "name": "Apple", "currency": "USD", "quantity": 3,
				 "acquisitionPrice": "150.25", "currentPrice": "180.25",
				 "priceDate": "2026-10-06", "valuation": "540.75",
				 "unrealizedGain": "90.00", "unrealizedGainRate": "20.0"}
			],
			"totals": [{"currency": "USD", "valuation": "540.75", "excludesUnpriced": false,
			            "unrealizedGain": "90.00", "unrealizedGainRate": "20.0"}]
		}`)
	})

	t.Run("終値0.01ドルの1株は0.01", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertStock(t, pool, "AAPL", "Apple", currencyUSD)
		insertBuy(t, pool, seedUserA, "AAPL", 1, "0.01")
		insertClosePrice(t, pool, "AAPL", "2026-10-06", "0.01")

		rec := getHoldingsFromDB(t, pool)

		assertJSONResponse(t, rec, http.StatusOK, `{
			"holdings": [
				{"code": "AAPL", "name": "Apple", "currency": "USD", "quantity": 1,
				 "acquisitionPrice": "0.01", "currentPrice": "0.01",
				 "priceDate": "2026-10-06", "valuation": "0.01",
				 "unrealizedGain": "0.00", "unrealizedGainRate": "0.0"}
			],
			"totals": [{"currency": "USD", "valuation": "0.01", "excludesUnpriced": false,
			            "unrealizedGain": "0.00", "unrealizedGainRate": "0.0"}]
		}`)
	})
}

// 5.5・4.7: 円の終値に 0.1円より細かい値（3000.25）を SQL で入れる（DB の CHECK は小数第2位まで許す）と、
// 一覧の API は黙って切り捨てずに 500 と {"error":"internal_error"}。
// 対照: 円の終値が 3000.20（0.1円単位）なら 200。
func Test補助_API_円の終値が0_1円より細かいと500とinternal_errorを返す(t *testing.T) {
	t.Parallel()

	t.Run("円の終値3000.25は500", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 100, "2500")
		insertClosePrice(t, pool, "7203", "2026-10-06", "3000.25")

		rec := getHoldingsFromDB(t, pool)

		assertJSONResponse(t, rec, http.StatusInternalServerError, `{"error":"internal_error"}`)
	})

	t.Run("対照_円の終値3000.20は200", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertBuy(t, pool, seedUserA, "7203", 100, "2500")
		insertClosePrice(t, pool, "7203", "2026-10-06", "3000.20")

		rec := getHoldingsFromDB(t, pool)

		assertJSONResponse(t, rec, http.StatusOK, `{
			"holdings": [
				{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 100,
				 "acquisitionPrice": "2500.0", "currentPrice": "3000.2",
				 "priceDate": "2026-10-06", "valuation": "300020",
				 "unrealizedGain": "50020", "unrealizedGainRate": "20.0"}
			],
			"totals": [{"currency": "JPY", "valuation": "300020", "excludesUnpriced": false,
			            "unrealizedGain": "50020", "unrealizedGainRate": "20.0"}]
		}`)
	})
}

// 5.5: 売り切ったドルの銘柄は、一覧にも totals にも出ない。Aが AAPL を買って売り切り（価格なし）、7203 を保有（価格あり）
// → holdings は 7203 の1行だけで、totals は円の1件だけ（ドルの行も、注記もない）。
func Test補助_API_売り切ったドルの銘柄はtotalsにも出ない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertStock(t, pool, "AAPL", "Apple", currencyUSD)
	insertBuy(t, pool, seedUserA, "AAPL", 10, "150.25")
	insertTrade(t, pool, seedUserA, "AAPL", "sell", 10, "170.00", "2026-02-10")
	insertBuy(t, pool, seedUserA, "7203", 100, "2500")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "7203", "name": "トヨタ自動車", "currency": "JPY", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": "300000",
			 "unrealizedGain": "50000", "unrealizedGainRate": "20.0"}
		],
		"totals": [{"currency": "JPY", "valuation": "300000", "excludesUnpriced": false,
		            "unrealizedGain": "50000", "unrealizedGainRate": "20.0"}]
	}`)
}
