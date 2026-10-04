# エージェント向け README

## プロジェクト概要

個人向けのお金の管理アプリケーション。

## 技術スタック

- **フロントエンド**: React + TypeScript
- **サーバーサイド**: Go
- **データベース**: PostgreSQL
- **ルーティング**: [chi](https://github.com/go-chi/chi)
- **DBアクセス**: [sqlc](https://sqlc.dev/)（SQLから型安全なGoコードを生成）

## アーキテクチャ方針

- **クリーンアーキテクチャ** をベースに **軽量CQRS** を採用する。
- 読み書きで同じPostgreSQLを使い、コード上で Command（書き込み）と Query（読み込み）を分離する。
- **Command側**: domainの集約・整合性ルールを通して状態を変更する。
- **Query側**: domainを経由せず、sqlcで生成したコードでSQLを直接実行し、読み取り専用のDTO（Read Model）を返す。月次集計・ダッシュボードなどを高速・簡潔に書くため。
- 依存の向きはすべて内側（domain）へ向かう。domainは何にも依存しない。

## ディレクトリ構成

```
zero/
├── backend/                          # Go サーバーサイド
│   ├── cmd/
│   │   └── api/
│   │       └── main.go               # エントリーポイント（DI組み立て・サーバー起動）
│   ├── internal/                     # 外部からimport不可（アプリ本体）
│   │   ├── domain/                   # 【最内層】書き込み側のビジネスルール
│   │   │   ├── model/                #   Account, Transaction, Category など（集約・エンティティ）
│   │   │   └── repository/           #   書き込み用リポジトリIF（interfaceのみ定義）
│   │   │
│   │   ├── application/              # 【アプリ層】CQRSの中核
│   │   │   ├── command/              #   ■ Command側（状態を変える）
│   │   │   └── query/                #   ■ Query側（状態を読むだけ）
│   │   │       └── dto/              #     読み取り専用モデル（Read Model）
│   │   │
│   │   ├── interface/                # 【IF層】外界との境界
│   │   │   └── handler/
│   │   │       ├── command_handler.go  #   POST/PUT/DELETE → Command呼び出し
│   │   │       └── query_handler.go     #   GET → Query呼び出し
│   │   │
│   │   └── infrastructure/           # 【最外層】技術的詳細の実装
│   │       ├── persistence/
│   │       │   ├── write/            #   書き込み実装（domain.repositoryの実装）
│   │       │   └── read/             #   読み取り実装（sqlc生成コードでDTOを返す）
│   │       ├── database/             #   DB接続
│   │       └── router/               #   chi によるルーティング設定
│   ├── db/
│   │   ├── migrations/               # SQLマイグレーションファイル
│   │   └── query/                    # sqlc 用のクエリSQL
│   ├── test/                         # バックエンドのテスト（統合テスト等）
│   │                                 #   ※ 単体テストは *_test.go で各パッケージに同居
│   ├── sqlc.yaml                     # sqlc 設定
│   ├── go.mod
│   └── go.sum
├── frontend/                         # React + TypeScript
│   ├── src/
│   └── test/                         # フロントエンドのテスト
├── e2e/                              # E2Eテスト（フロント↔バック通し）
├── docs/                             # SSOT（Single Source of Truth）となるドキュメント群
├── docker-compose.yml                # Postgres等のローカル環境
└── エージェント向けREADME.md
```

### テスト方針

- **backend**: 単体テストは Go 慣習に従い各パッケージ内に `*_test.go` で同居。統合テスト等は `backend/test/` にまとめる。
- **frontend**: `frontend/test/` にテストを配置。
- **E2E**: フロント↔バックを通したE2Eテストを `e2e/` に配置。
- **docs（SSOT）**: 仕様・設計・意思決定などの正となるドキュメントを `docs/` に集約し、Single Source of Truth とする。

## 主要コマンド

タスクランナーには [Task（go-task）](https://taskfile.dev/) を使用する。ルートの `Taskfile.yml` で定義する想定。
※ 以下はすべて**仮（暫定）**。実装が固まり次第、正式なものに更新すること。

```bash
# --- セットアップ ---
task setup              # 依存インストール（backend / frontend 両方）

# --- 起動 ---
task dev                # 開発環境を一括起動（backend + frontend + DB）
task backend:dev        # バックエンドのみ起動
task frontend:dev       # フロントエンドのみ起動

# --- ビルド ---
task build              # backend / frontend を両方ビルド
task backend:build      # Go サーバーをビルド
task frontend:build     # フロントをビルド

# --- テスト ---
task test               # 全テスト（backend + frontend + e2e）
task backend:test       # Go の単体・統合テスト
task frontend:test      # フロントのテスト
task e2e                # E2E テスト（Playwright: cd e2e && npx playwright test）

# --- リント / フォーマット ---
task lint               # backend / frontend をまとめてリント
task backend:lint       # golangci-lint
task frontend:lint      # ESLint
task fmt                # フォーマット（gofmt / Prettier）

# --- データベース ---
task db:up              # DB コンテナ起動
task db:migrate         # マイグレーション適用
task sqlc               # sqlc でコード生成
```

## 開発ルール

ファイルを **読む・変更する・レビューする** ときは、**対象パスに一致するルールを先に読むこと**。
複数のルールが一致した場合は、**一致したルールをすべて適用する**。

| 対象パス | 先に読むルール |
|---|---|
| `backend/internal/**` | `docs/rules/backend/architecture.md` |
| `frontend/**` | `docs/rules/frontend/architecture.md` |
| テスト（`**/test/**`, `e2e/**`, `*_test.go` など） | `docs/rules/test/architecture.md` |

- ルールと仕様が矛盾した場合は、**推測で進めず、作業を止めて矛盾を報告すること**。

## ドキュメント作成ルール

- このプロジェクトのすべてのドキュメントは **OKF（Open Knowledge Format）** で記述する（Google Cloud策定、v0.1）。
- 各ドキュメント先頭に YAMLフロントマター（`type` 必須、`title` / `description` / `resource` / `tags` / `timestamp` 任意）を付ける。
- 1ファイル1概念。相互リンクは通常のMarkdownリンクで辿れるようにする。
- ドキュメントのSSOTは `docs/` に集約し、入口は [docs/index.md](docs/index.md)。
- 詳細・テンプレートは [CLAUDE.md](CLAUDE.md) を参照。


### 依存の向き

```
infrastructure ──→ interface ──→ application ──→ domain
                                                    ↑
              （依存はすべて内側＝domainへ向かう。domainは何にも依存しない）
```
