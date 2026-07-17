# slackToGoogleDrive

Slack チャンネル（例: `#general`）に投稿された**写真・動画・zip をすべてダウンロードし、Google Drive の指定フォルダにアップロード**する CLI。認可したユーザー本人としてアップロードするため、個人のマイドライブのフォルダでも共有ドライブでも入れられます。

再実行しても安全（冪等）です。アップロード済みファイルは Slack file ID をキーにローカル manifest で記録し、次回以降スキップします。新しく投稿されたファイルの取り込みや、中断後の再開に使えます。

## 仕組み

```
Slack files.list (cursor ページング, images+videos)
   → url_private_download を Bearer トークン付きで DL（ローカルに一時保存）
   → Drive フォルダへ resumable upload（appProperties に slackFileId を刻印）
   → manifest.json に記録 → 一時ファイル削除
```

## 事前準備

### 1. Slack アプリ（Bot）

1. https://api.slack.com/apps で **From scratch** でアプリを作成。**社内利用のみ（配布しない）**にする — 配布アプリは 2025/5 以降 `conversations`/`files` 系が 1req/分に制限されるため。
2. **OAuth & Permissions → Bot Token Scopes** に以下を付与:
   - `files:read` — ファイルのメタデータ取得とダウンロード
   - `channels:history` — パブリックチャンネルのメッセージ
   - `channels:read` — チャンネル名 → ID 解決
   - （対象がプライベートチャンネルなら `groups:history` / `groups:read` も追加）
3. ワークスペースにインストールし、**Bot User OAuth Token（`xoxb-...`）** を控える。
4. 対象チャンネルで `/invite @<アプリ名>` を実行し、**bot をチャンネルに招待**する（必須）。

### 2. Google 側（gcloud の Application Default Credentials）

認可したユーザー本人としてアップロードするため、**個人のマイドライブのフォルダにそのまま入れられます**（共有ドライブも可）。キー JSON のダウンロードは不要で、`gcloud` の認証を使います。

1. **[gcloud CLI](https://cloud.google.com/sdk/docs/install) をインストール**（未導入の場合）。
2. GCP プロジェクトで **Drive API** を有効化（既存プロジェクトの流用で可・無料）:
   ```sh
   gcloud config set project <YOUR_PROJECT_ID>
   gcloud services enable drive.googleapis.com
   ```
3. **Drive スコープ付きで ADC ログイン**（初回だけブラウザで同意）:
   ```sh
   gcloud auth application-default login \
     --scopes=openid,https://www.googleapis.com/auth/drive
   ```
   認証情報は `~/.config/gcloud/application_default_credentials.json` に保存され、以降は非対話で使われます。
4. アップロード先フォルダの ID を控える（URL `https://drive.google.com/drive/folders/<ID>` の `<ID>` 部分）。個人のマイドライブのフォルダで OK。

> ⚠️ ADC のユーザー認証情報は **quota project** が必要です。上の `gcloud config set project` で設定したプロジェクトが使われます。403 quota エラーが出る場合は
> `gcloud auth application-default set-quota-project <YOUR_PROJECT_ID>` を実行してください。

### 3. 設定

```sh
cp .env.example .env
# .env を編集して SLACK_BOT_TOKEN / SLACK_CHANNEL / DRIVE_FOLDER_ID を設定
```

## 実行

```sh
# 何がアップロードされるか確認（DL・アップロードしない）
go run ./cmd/slacktodrive -dry-run

# .env の設定で実行
go run ./cmd/slacktodrive

# 別チャンネルを別フォルダへ（.env を編集せずフラグで上書き）
go run ./cmd/slacktodrive -channel other-channel -folder <別フォルダID>

# バイナリをビルドして実行
go build -o slacktodrive ./cmd/slacktodrive
./slacktodrive
```

オプション:

| フラグ | 既定 | 説明 |
|--------|------|------|
| `-env` | `.env` | 読み込む .env ファイルのパス |
| `-channel` | （空） | Slack チャンネル名 or ID。指定すると `SLACK_CHANNEL` を上書き |
| `-folder` | （空） | アップロード先 Drive フォルダ ID。指定すると `DRIVE_FOLDER_ID` を上書き |
| `-dry-run` | `false` | 対象一覧を表示するだけで DL・アップロードしない |
| `-limit` | `0` | この実行でアップロードする新規ファイルを最大 N 件に制限（0=無制限）。セットアップ確認用に `-limit 1` で1件だけ試すのに便利 |

> 別チャンネルで使うときの注意:
> - そのチャンネルにも **bot を招待**する（未招待だと `not_in_channel` エラー）。
> - `manifest.json` は Slack file ID（全チャンネルで一意）で重複管理するため共有で問題ないが、チャンネルごとに記録を分けたい場合は `MANIFEST_PATH=./manifest-<channel>.json` を指定する。

## テスト

```sh
go test ./...
```

## 注意

- `.env` は**秘密情報**（Slack トークンを含む）。`.gitignore` 済みだがコミットしないこと。Google 認証は gcloud ADC（`~/.config/gcloud`）を使うのでリポジトリ内に鍵ファイルは置かない。
- `manifest.json` を消すと全ファイルを再アップロードします（Drive 側に重複が作られます）。保持してください。
- 大量ファイルの場合、Slack の `Retry-After` を尊重して自動で待機します。
