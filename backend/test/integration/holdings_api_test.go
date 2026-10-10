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
// 数は書かれた字面（json.Number）のまま比べるので、300000 と 300000.0、"2500.0" と 2500 と "2500" は別のものになる
// （4.4: 単価は小数第1位を1桁付けた文字列、評価額と合計は円の整数、価格がなければ null）。
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
	insertHolding(t, pool, seedUserA, "7203", 100, "2500")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "7203", "name": "トヨタ自動車", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": 300000}
		],
		"totalValuation": 300000,
		"totalExcludesUnpriced": false
	}`)
}

// AC-7・AC-8a（FR-6）の API 部分:
// 利用者Aが 7203 を100株、6758 を10株保有し、両方の価格（3,000円・3,500円）が保存済み
// → 合計評価額は 335,000円（AC-7）。全銘柄に価格があるので、注記のフラグ totalExcludesUnpriced は false（AC-8a）。
// 6758 の取得価格は AC に指定がないので 3,200円 とした。並び順は銘柄コードの昇順（FR-5）。
func TestAC7_AC8a_API_全銘柄に価格があれば合計評価額は評価額の合計で注記のフラグはfalse(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertHolding(t, pool, seedUserA, "7203", 100, "2500")
	insertHolding(t, pool, seedUserA, "6758", 10, "3200")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	insertClosePrice(t, pool, "6758", "2026-10-06", "3500")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "6758", "name": "ソニーグループ", "quantity": 10,
			 "acquisitionPrice": "3200.0", "currentPrice": "3500.0",
			 "priceDate": "2026-10-06", "valuation": 35000},
			{"code": "7203", "name": "トヨタ自動車", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": 300000}
		],
		"totalValuation": 335000,
		"totalExcludesUnpriced": false
	}`)
}

// AC-8（FR-6）・AC-11（FR-10）の API 部分:
// 利用者Aが 7203 を100株（価格保存済み）と 9984（価格は一度も保存されていない）を保有している
// → 9984 の行の currentPrice・priceDate・valuation は null（AC-11。画面では「取得できません」）、
// 7203 の行は AC-1 のとおり。合計評価額は 7203 の分だけの 300,000円で、totalExcludesUnpriced は true
// （AC-8。画面では「価格を取得できない銘柄を含みません」）。
// 9984 の保有数量と取得価格は AC に指定がないので 10株・9,000円 とした。
func TestAC8_AC11_API_価格のない銘柄はnullで返し合計評価額に含めず注記のフラグはtrue(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertHolding(t, pool, seedUserA, "7203", 100, "2500")
	insertHolding(t, pool, seedUserA, "9984", 10, "9000")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "7203", "name": "トヨタ自動車", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": 300000},
			{"code": "9984", "name": "ソフトバンクグループ", "quantity": 10,
			 "acquisitionPrice": "9000.0", "currentPrice": null,
			 "priceDate": null, "valuation": null}
		],
		"totalValuation": 300000,
		"totalExcludesUnpriced": true
	}`)
}

// AC-9（FR-7）の API 部分と、補助 FR-7（SR-5）の API、補助 FR-1（PR② の TC-05）:
// 利用者Aが株式を1件も保有していない（DB には利用者Bの 6758（10株・3,200円）の保有がある）
// → {"holdings":[],"totalValuation":0,"totalExcludesUnpriced":false}（holdings は null ではなく空の配列。4.4）。
// 利用者Bの保有は A の応答に出ない（API の経路でも、ログイン中の利用者の保有データだけを読む。FR-1）。
func TestAC9_FR7_SR5_FR1_PR2_TC05_API_保有株式が0件なら他の利用者の保有があっても空の配列と合計0を返す(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertHolding(t, pool, seedUserB, "6758", 10, "3200")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{"holdings":[],"totalValuation":0,"totalExcludesUnpriced":false}`)
}

// 補助 FR-8（TC-11）: 現在の価格は、保存済みの価格のうち最新の取引日の終値で、価格の基準日はその取引日。
// 7203（100株・2,500円）の価格を 2026-10-05 の 2,900円 → 2026-10-06 の 3,000円 → 2026-10-02 の 3,100円 の順に入れる
// （最新の行が、最初に入れた行・最後に入れた行・最も古い行・終値の最も高い行のどれとも違うようにして、
// 「入れた順の最初／最後を読む」「取引日の古い行を選ぶ」「終値の最大を選ぶ」誤りを見分けるため。
// PR② ステップ6 の TC-01、2ラウンド目の PR2_TC08 で 10-02 の終値を 2,800円 から 3,100円 に変えた）。
// → 1行・quantity 100・currentPrice "3000.0"・priceDate "2026-10-06"・valuation 300000。
func TestFR8_TC11_PR2_TC08_API_現在の価格は最新の取引日の終値で行は増えない(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertHolding(t, pool, seedUserA, "7203", 100, "2500")
	insertClosePrice(t, pool, "7203", "2026-10-05", "2900")
	insertClosePrice(t, pool, "7203", "2026-10-06", "3000")
	insertClosePrice(t, pool, "7203", "2026-10-02", "3100")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "7203", "name": "トヨタ自動車", "quantity": 100,
			 "acquisitionPrice": "2500.0", "currentPrice": "3000.0",
			 "priceDate": "2026-10-06", "valuation": 300000}
		],
		"totalValuation": 300000,
		"totalExcludesUnpriced": false
	}`)
}

// 補助 FR-4・FR-12・FR-19（TC-14）: 桁数を指定しない NUMERIC は書いた桁（scale）のまま保存されるので、
// DB から読むときに scale が 0 でも 1 でも正しく 0.1円単位にする（プラン 4.2）。
// 7203 を 1株・取得価格 2000（scale 0）と 2株・2001.0（scale 1）で保有し、終値は 1234.5
// → quantity 3・acquisitionPrice "2000.7"・currentPrice "1234.5"・valuation 3703・totalValuation 3703。
func TestFR4_FR12_FR19_TC14_API_scaleの違う単価も正しく読んで集約する(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	insertHolding(t, pool, seedUserA, "7203", 1, "2000")
	insertHolding(t, pool, seedUserA, "7203", 2, "2001.0")
	insertClosePrice(t, pool, "7203", "2026-10-06", "1234.5")

	rec := getHoldingsFromDB(t, pool)

	assertJSONResponse(t, rec, http.StatusOK, `{
		"holdings": [
			{"code": "7203", "name": "トヨタ自動車", "quantity": 3,
			 "acquisitionPrice": "2000.7", "currentPrice": "1234.5",
			 "priceDate": "2026-10-06", "valuation": 3703}
		],
		"totalValuation": 3703,
		"totalExcludesUnpriced": false
	}`)
}

// 補助 FR-12・FR-19・4.2（PR② の EC-3、TC-03）: 単価が scale 2 以上で末尾が 0 の NUMERIC（例 3000.00・1234.50）や、
// 最小の単価 0.1円でも、正しく 0.1円単位にして、小数第1位を1桁付けた文字列で返す（4.2、4.4「API」）。
// 3000.00・2001.00・1234.50 は「= round(…, 1)」を満たすので保存できる値（4.2）。
func TestFR12_FR19_PR2_EC3_TC03_API_scale2の単価や最小の単価0_1円も小数第1位付きの文字列で返す(t *testing.T) {
	t.Parallel()

	// 7203 を1株・取得価格 0.1、終値 3000.00（scale 2、2026-10-06）
	// → acquisitionPrice "0.1"・currentPrice "3000.0"・valuation 3000。
	t.Run("取得価格0.1と終値3000.00", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertHolding(t, pool, seedUserA, "7203", 1, "0.1")
		insertClosePrice(t, pool, "7203", "2026-10-06", "3000.00")

		rec := getHoldingsFromDB(t, pool)

		assertJSONResponse(t, rec, http.StatusOK, `{
			"holdings": [
				{"code": "7203", "name": "トヨタ自動車", "quantity": 1,
				 "acquisitionPrice": "0.1", "currentPrice": "3000.0",
				 "priceDate": "2026-10-06", "valuation": 3000}
			],
			"totalValuation": 3000,
			"totalExcludesUnpriced": false
		}`)
	})

	// 7203 を1株・取得価格 2001.00、終値 1234.50（どちらも scale 2、2026-10-06）
	// → acquisitionPrice "2001.0"・currentPrice "1234.5"・valuation 1234。
	t.Run("取得価格2001.00と終値1234.50", func(t *testing.T) {
		t.Parallel()
		pool := testdb.New(t)
		insertHolding(t, pool, seedUserA, "7203", 1, "2001.00")
		insertClosePrice(t, pool, "7203", "2026-10-06", "1234.50")

		rec := getHoldingsFromDB(t, pool)

		assertJSONResponse(t, rec, http.StatusOK, `{
			"holdings": [
				{"code": "7203", "name": "トヨタ自動車", "quantity": 1,
				 "acquisitionPrice": "2001.0", "currentPrice": "1234.5",
				 "priceDate": "2026-10-06", "valuation": 1234}
			],
			"totalValuation": 1234,
			"totalExcludesUnpriced": false
		}`)
	})
}

// failingHoldingsReader は、保有データを読むと必ず失敗する HoldingsReader（境界の外側の fake。FR-22 用）。
type failingHoldingsReader struct{}

// failingReaderDetail は、fake が返すエラーの内容。応答に出てはいけない（4.4: 内部のエラー内容は返さない）。
const failingReaderDetail = "fake: connection refused to db.internal:5432"

func (failingHoldingsReader) ListHoldingRows(context.Context, uuid.UUID) ([]dto.HoldingRow, error) {
	return nil, errors.New(failingReaderDetail)
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

func (emptyHoldingsReader) ListHoldingRows(context.Context, uuid.UUID) ([]dto.HoldingRow, error) {
	return []dto.HoldingRow{}, nil
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
		assertJSONResponse(t, rec, http.StatusOK, `{"holdings":[],"totalValuation":0,"totalExcludesUnpriced":false}`)
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
