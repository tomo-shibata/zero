// Package config は、コマンドが使う設定を環境変数から読む。
// 必要な環境変数はコマンドごとに違う（プラン 4.3「コマンドごとの環境変数」）ので、コマンドごとに読む関数を分ける。
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/google/uuid"
)

const (
	// defaultPort は、PORT がないときに cmd/api が待ち受けるポート。
	defaultPort = 8080
	// minPort・maxPort は、PORT に指定できるポート番号の範囲。
	minPort = 1
	maxPort = 65535
)

// API は、cmd/api の設定。
type API struct {
	Port        int       // 待ち受けるポート。PORT。既定は 8080。1〜65535
	DatabaseURL string    // 接続先の DB。DATABASE_URL。必須
	DevUserID   uuid.UUID // ログイン中の利用者として扱う開発用利用者。DEV_USER_ID。必須（プラン 6章の判断5）
}

// LoadAPI は、cmd/api の設定を読む。
func LoadAPI() (API, error) {
	port, err := loadPort()
	if err != nil {
		return API{}, err
	}
	databaseURL, err := LoadDatabaseURL()
	if err != nil {
		return API{}, err
	}
	devUserID, err := loadDevUserID()
	if err != nil {
		return API{}, err
	}
	return API{Port: port, DatabaseURL: databaseURL, DevUserID: devUserID}, nil
}

// loadPort は、PORT（cmd/api が待ち受けるポート）を読む。なければ既定の 8080。
// PORT が 1〜65535 の整数でなければエラーにする。確かめないと、誤った値でも起動してしまい
// （0 なら空いている適当なポートで待ち受ける）、どこで待ち受けているのか分からなくなるため。
//
// 制約: PORT=80 では /api/ に届かない（すべて 403）。ブラウザは HTTP の既定のポート 80 を Host ヘッダーに付けない
// （"127.0.0.1" だけを送る）が、/api/ の Host の許可リストは "127.0.0.1:80" のようにポート付きで比べるため
// （cmd/api の allowedHosts、プラン 6章の判断12）。80 は拒まずに起動する（/healthz は応答する）ので、80 以外を使う。
func loadPort() (int, error) {
	s := os.Getenv("PORT")
	if s == "" {
		return defaultPort, nil
	}
	port, err := strconv.Atoi(s)
	if err != nil || port < minPort || port > maxPort {
		return 0, fmt.Errorf("環境変数 PORT は %d〜%d の整数にしてください（PORT=%q）", minPort, maxPort, s)
	}
	return port, nil
}

// LoadDatabaseURL は、DATABASE_URL（接続先の DB）を読む。cmd/migrate と cmd/api が使う。
// 既定値を持たないのは、意図しない DB にマイグレーションや seed を流したり、意図しない DB のデータを返したりしないため。
func LoadDatabaseURL() (string, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return "", errors.New("環境変数 DATABASE_URL が設定されていません")
	}
	return databaseURL, nil
}

// loadDevUserID は、DEV_USER_ID（ログイン中の利用者として扱う開発用利用者の UUID）を読む。
// 既定値を持たないのは、どの利用者のデータを返すかを、設定した人が必ず決めるようにするため。
func loadDevUserID() (uuid.UUID, error) {
	s := os.Getenv("DEV_USER_ID")
	if s == "" {
		return uuid.UUID{}, errors.New("環境変数 DEV_USER_ID が設定されていません")
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("環境変数 DEV_USER_ID は UUID にしてください（DEV_USER_ID=%q）", s)
	}
	return id, nil
}
