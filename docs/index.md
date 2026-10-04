---
type: index
title: ドキュメント エントリポイント
description: このプロジェクトのドキュメント（SSOT）の入口。AIエージェントはここから辿ること。
tags: [docs, index, ssot]
timestamp: 2026-10-04T00:00:00Z
---

# ドキュメント（SSOT）

このディレクトリは Single Source of Truth。すべてのドキュメントは **OKF（Open Knowledge Format）** で記述する。

## ルール

- [ドキュメント作成ルール（OKF）](rules/documentation.md) — 全ドキュメント共通
- [バックエンド アーキテクチャルール](rules/backend/architecture.md) — `backend/internal/**` 向け
- [フロントエンド アーキテクチャルール](rules/frontend/architecture.md) — `frontend/**` 向け
- [テストルール](rules/test/architecture.md) — テスト全般向け（`**/test/**`, `e2e/**`, `*_test.go`）
- [AI相互レビュールール](rules/ai-review.md) — レビューの運用・優先度
