# CLAUDE.md

このファイルは入口です。詳細は各参照先を読むこと（ここには内容を書かない）。

## まず読む

- プロジェクト概要・技術スタック・構成・コマンド・開発ルール: [エージェント向けREADME.md](エージェント向けREADME.md)
- ドキュメントのSSOT入口: [docs/index.md](docs/index.md)

## ドキュメント作成

- すべてのドキュメントは OKF 形式で書く: [docs/rules/documentation.md](docs/rules/documentation.md)

## 作業前に読むルール（対象パスに一致するものを先に読む）

- backend/internal/**: [docs/rules/backend/architecture.md](docs/rules/backend/architecture.md)
- frontend/**: [docs/rules/frontend/architecture.md](docs/rules/frontend/architecture.md)
- テスト全般: [docs/rules/test/architecture.md](docs/rules/test/architecture.md)
- AI相互レビュー: [docs/rules/ai-review.md](docs/rules/ai-review.md)

## サブエージェント

- [design-review](.claude/agents/design-review.md) / [edge-case-review](.claude/agents/edge-case-review.md) / [security-review](.claude/agents/security-review.md)

## スキル

- [tdd](.claude/skills/tdd/SKILL.md) / [pull-request](.claude/skills/pull-request/SKILL.md) / [self-review](.claude/skills/self-review/SKILL.md)
