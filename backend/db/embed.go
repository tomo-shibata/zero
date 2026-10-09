// Package db は、マイグレーションの SQL をバイナリに埋め込んで公開する。
// 実行時の作業ディレクトリに左右されずに、cmd/migrate とテスト（testdb）が同じスキーマを使えるようにするため。
package db

import "embed"

// Migrations は、migrations/ にある goose 形式の SQL。
//
//go:embed migrations/*.sql
var Migrations embed.FS
