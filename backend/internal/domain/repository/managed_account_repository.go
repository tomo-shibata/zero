package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/tomo-shibata/zero/backend/internal/domain/model"
)

// ManagedAccountImportRepository は、おまかせ運用の CSV の取り込み（要件25〜要件31）が使う、DB の読み書き。
// 利用者 ID の型に uuid.UUID を使う理由は TradeImportRepository と同じ。
type ManagedAccountImportRepository interface {
	// UserExists は、利用者 userID が DB にいるかを返す。
	UserExists(ctx context.Context, userID uuid.UUID) (bool, error)
	// ReplaceManagedAccounts は、1つのトランザクションで、users の利用者 userID の行をロックし（判断7）、その利用者の口座をすべて消し
	// （記録も消える。判断4）、valuations に出てくる名前ごとに口座を1つ作り、valuations をすべて入れる。途中で失敗したら何も変えない（要件26）。
	// 他の利用者の口座と記録、売買履歴・銘柄マスタ・終値・並び・分割・レートは変えない。
	ReplaceManagedAccounts(ctx context.Context, userID uuid.UUID, valuations []model.ManagedValuation) error
}

// ErrManagedAccountNotFound は、対象の口座が（その利用者に）ないこと。
var ErrManagedAccountNotFound = errors.New("managed account not found")

// ManagedAccountDeleteRepository は、おまかせ運用の口座の削除（要件24）が使う、DB の書き込み。
type ManagedAccountDeleteRepository interface {
	// DeleteManagedAccount は、1つのトランザクションで users の行をロックし（判断7）、利用者 userID の名前 name の口座を消す（記録も消える。判断4）。
	// 名前は書いたとおりに比べる（要件1）。他の利用者の口座と記録、売買履歴などは消さない。1つも消えなければ ErrManagedAccountNotFound を返す。
	DeleteManagedAccount(ctx context.Context, userID uuid.UUID, name string) error
}
