package repository

import "github.com/tomo-shibata/zero/backend/internal/domain/model"

// HeldStock は、保有されている銘柄（保有数量が1以上の銘柄）の銘柄コードと通貨。
// 価格の取得処理が、取得した終値の精度を通貨で確かめるために、通貨も一緒に読む（要件5）。
type HeldStock struct {
	Code     string
	Currency model.Currency
}
