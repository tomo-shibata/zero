---
type: rule
title: ドキュメント作成ルール（OKF）
description: このプロジェクトの全ドキュメントを OKF（Open Knowledge Format）で記述するためのルールとテンプレート。
tags: [docs, okf, rule]
timestamp: 2026-10-04T00:00:00Z
---

# ドキュメント作成ルール（OKF）

**このプロジェクトのすべてのドキュメントは OKF（Open Knowledge Format）で記述すること。**

OKF は Google Cloud が策定した、AI/AIエージェントにドキュメントや知識を効率的に理解させるためのMarkdownベースのオープン仕様（v0.1, 2026年6月公開）。

## ルール

- **YAMLフロントマター**を各ドキュメント先頭に付ける。
  - `type`（**必須**）: その文書が表す概念の種別（例: `rule`, `index`, `spec`, `runbook`）。
  - `title`（任意）: 人間可読な名称。
  - `description`（任意）: 何を表し、どう使うかの説明（AIが用途を把握するための1文）。
  - `resource`（任意）: 実リソースへのURL。
  - `tags`（任意）: 分類用ラベルの配列。
  - `timestamp`（任意）: 最終更新のISO 8601時刻。
- **1ファイル＝1概念**で書く。
- ドキュメント同士は**通常のMarkdownリンク**（相対パス）で相互リンクし、知識グラフとして辿れるようにする。
- `docs/index.md` をエントリポイント、`log.md` を変更履歴に用いる。
- ドキュメントの正（SSOT）は `docs/` に集約する。入口は [docs/index.md](../index.md)。

## フロントマターのテンプレート

```yaml
---
type: rule
title: <人間可読なタイトル>
description: <何を表し、どう使うかの1文>
tags: [<label1>, <label2>]
timestamp: <ISO 8601>
---
```
