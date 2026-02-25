# AGENTS.md

このファイルは、このリポジトリで作業する人間/エージェント向けの共通ガイドです。

## Project Overview
- Module: `github.com/ryotarai/hayai`
- Go version: `1.22`
- Backend: Go (`cmd/server`, `internal/...`)
- Frontend: Vite + React (`web/`)
- DB access: `sqlc` 生成コードを利用

## Repository Layout
- `cmd/server/`: サーバー起動エントリポイント
- `internal/server/`: ルーター・配信処理
- `internal/db/`: DB接続・設定・マイグレーション・ストア
- `internal/db/sqlc/`: SQL定義と生成コード
- `web/`: フロントエンドソース
- `internal/server/ui/dist/`: フロントエンドビルド成果物

## Setup
1. Go `1.22` を用意
2. Node.js + npm を用意
3. `sqlc` をインストール
4. 依存解決
   - Go: `go mod download`
   - Web: `npm --prefix web ci`

## Build / Generate
- 全体ビルド: `make build`
- フロントエンドのみ: `make build-frontend`
- サーバーのみ: `make build-server`
- sqlcコード生成: `make sqlc`

`make build-server` は内部で `make sqlc` を実行します。

## Run / Test
- サーバー起動（ビルド済みバイナリ）: `./bin/server`
- 単体テスト: `go test ./...`

## Coding Guidelines (Go)
- フォーマット: `gofmt`（必要なら `go fmt ./...`）
- 静的解析: `go vet ./...`
- コンテキスト付きI/O処理を優先し、`context.Context` を適切に受け渡す
- `internal/` 配下の既存責務を尊重し、境界をまたぐ変更は最小化

## Generated Files
以下は生成物のため、手編集を避ける。
- `internal/db/sqlc/postgresql/*.go`
- `internal/db/sqlc/sqlite/*.go`
- `internal/server/ui/dist/*`

変更が必要な場合はソースを編集して再生成する。
- sqlc: `internal/db/sqlc/schema.sql`, `internal/db/sqlc/query.sql` を編集して `make sqlc`
- UI: `web/src/*` を編集して `make build-frontend`

## Change Checklist
- 関連する生成処理を再実行した
- `go test ./...` が通ることを確認した
- 不要な差分（キャッシュ、ローカル設定）を含めていない
- 変更内容に応じてドキュメントを更新した
- 適宜、適切な単位で `git commit` した
