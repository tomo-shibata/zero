// Package config は、コマンドが使う設定を環境変数から読む。
// 必要な環境変数はコマンドごとに違う（プラン 4.3「コマンドごとの環境変数」）ので、コマンドごとに読む関数を分ける。
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// defaultPort は、PORT がないときに cmd/api が待ち受けるポート。
const defaultPort = 8080

// API は、cmd/api の設定。
type API struct {
	Port int // 待ち受けるポート。PORT。既定は 8080。1〜65535
}

// LoadAPI は、cmd/api の設定を読む。
// PORT が 1〜65535 の整数でなければエラーにする。確かめないと、誤った値でも起動してしまい
// （0 なら空いている適当なポートで待ち受ける）、どこで待ち受けているのか分からなくなるため。
func LoadAPI() (API, error) {
	s := os.Getenv("PORT")
	if s == "" {
		return API{Port: defaultPort}, nil
	}
	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return API{}, fmt.Errorf("環境変数 PORT は 1〜65535 の整数にしてください（PORT=%q）", s)
	}
	return API{Port: port}, nil
}

// LoadDatabaseURL は、DATABASE_URL（接続先の DB）を読む。cmd/migrate が使う。
// 既定値を持たないのは、意図しない DB にマイグレーションや seed を流さないため。
func LoadDatabaseURL() (string, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return "", errors.New("環境変数 DATABASE_URL が設定されていません")
	}
	return databaseURL, nil
}
