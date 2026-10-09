// Package database は、PostgreSQL への接続と、スキーマ・データ・DB そのものの準備を受け持つ。
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect は、connString の DB への接続プールを作り、実際につながることを確かめてから返す。
func Connect(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("DB の接続設定を読めません: %w", err)
	}
	return ConnectConfig(ctx, cfg)
}

// ConnectConfig は、cfg の設定で接続プールを作り、Connect と同じく実際につながることを確かめてから返す。
// 接続文字列を変えずに、プールの設定（接続数の上限など）だけを変えたいときに使う。
func ConnectConfig(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("DB の接続プールを作れません: %w", err)
	}
	// pgxpool は接続を遅延して作るため、Ping しないと設定の誤りや DB の停止に最初のクエリまで気づけない。
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("DB に接続できません: %w", err)
	}
	return pool, nil
}
