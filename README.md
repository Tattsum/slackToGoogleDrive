# slackToGoogleDrive

Slack チャンネルに投稿された**写真・動画・zip をすべてダウンロードし、Google Drive の指定フォルダにアップロード**する CLI。

- ユーザー本人として認可（gcloud ADC）するので、**個人マイドライブ・共有ドライブのどちらにも**入れられる
- Slack file ID キーの manifest で**冪等**。再実行で新規分だけ取り込み、中断後も再開できる
- 一時的な通信エラーは自動リトライ

## クイックスタート

```sh
# 1. 認証情報を用意（詳細は docs/setup.md）
cp .env.example .env        # SLACK_BOT_TOKEN / SLACK_CHANNEL / DRIVE_FOLDER_ID を設定
gcloud auth application-default login --scopes=openid,https://www.googleapis.com/auth/drive

# 2. 対象を確認（DL・アップロードしない）
go run ./cmd/slacktodrive -dry-run

# 3. 実行
go run ./cmd/slacktodrive
```

## ドキュメント

- [セットアップ](docs/setup.md) — Slack アプリ / Google 認証 / 環境変数
- [実行方法](docs/usage.md) — オプション、別チャンネル運用、テスト、注意点
- [仕組みと設計判断](docs/design.md) — 処理フローと設計の背景
