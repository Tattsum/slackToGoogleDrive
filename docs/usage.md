# 実行方法

事前に [セットアップ](./setup.md) を済ませておく。

```sh
# 何がアップロードされるか確認（DL・アップロードしない）
go run ./cmd/slacktodrive -dry-run

# .env の設定で実行
go run ./cmd/slacktodrive

# 別チャンネルを別フォルダへ（.env を編集せずフラグで上書き）
go run ./cmd/slacktodrive -channel other-channel -folder <別フォルダID or URL>

# バイナリをビルドして実行
go build -o slacktodrive ./cmd/slacktodrive
./slacktodrive
```

## オプション

| フラグ | 既定 | 説明 |
|--------|------|------|
| `-env` | `.env` | 読み込む .env ファイルのパス |
| `-channel` | （空） | Slack チャンネル名 or ID。指定すると `SLACK_CHANNEL` を上書き |
| `-folder` | （空） | アップロード先 Drive フォルダ ID（URL 可）。指定すると `DRIVE_FOLDER_ID` を上書き |
| `-dry-run` | `false` | 対象一覧を表示するだけで DL・アップロードしない |
| `-limit` | `0` | この実行でアップロードする新規ファイルを最大 N 件に制限（0=無制限）。セットアップ確認用に `-limit 1` で1件だけ試すのに便利 |

## 別チャンネルで使うときの注意

- そのチャンネルにも **bot を招待**する（未招待だと `not_in_channel` エラー）。
- `manifest.json` は Slack file ID（全チャンネルで一意）で重複管理するため共有で問題ない。チャンネルごとに記録を分けたい場合は `MANIFEST_PATH=./manifest-<channel>.json` を指定する。

## テスト

```sh
go test ./...
```

## 運用上の注意

- `.env` は**秘密情報**（Slack トークンを含む）。`.gitignore` 済みだがコミットしないこと。Google 認証は gcloud ADC（`~/.config/gcloud`）を使うのでリポジトリ内に鍵ファイルは置かない。
- `manifest.json` を消すと全ファイルを再アップロードする（Drive 側に重複が作られる）。保持すること。
- 一時的な通信エラーは自動リトライする（アップロードは指数バックオフで最大4回、Slack の `Retry-After` も尊重）。失敗が残っても、再実行すれば未完了分だけを拾う。
