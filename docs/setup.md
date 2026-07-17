# セットアップ

## 1. Slack アプリ（Bot）

1. https://api.slack.com/apps で **From scratch** でアプリを作成。**社内利用のみ（配布しない）**にする — 配布アプリは 2025/5 以降 `conversations`/`files` 系が 1req/分に制限されるため。
2. **OAuth & Permissions → Bot Token Scopes** に以下を付与:
   - `files:read` — ファイルのメタデータ取得とダウンロード
   - `channels:history` — パブリックチャンネルのメッセージ
   - `channels:read` — チャンネル名 → ID 解決
   - （対象がプライベートチャンネルなら `groups:history` / `groups:read` も追加）
3. ワークスペースにインストールし、**Bot User OAuth Token（`xoxb-...`）** を控える。
4. 対象チャンネルで `/invite @<アプリ名>` を実行し、**bot をチャンネルに招待**する（必須）。

## 2. Google 側（gcloud の Application Default Credentials）

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
4. アップロード先フォルダの ID を控える（URL `https://drive.google.com/drive/folders/<ID>` の `<ID>` 部分）。個人のマイドライブのフォルダで OK。`-folder` / `DRIVE_FOLDER_ID` は URL のまま渡しても ID を自動抽出します。

> ⚠️ **gcloud 内蔵クライアントが Drive スコープを拒否する場合**（`access blocked` / `cloud-platform scope is required`）は、自前の OAuth クライアント（デスクトップアプリ）を作成し、それを使ってログインする:
> ```sh
> gcloud auth application-default login \
>   --client-id-file=oauth-client.json \
>   --scopes=openid,https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/drive
> ```
> OAuth 同意画面を **内部（Internal）** で作成すると、機微スコープを Google 審査なしで使えます。

> ⚠️ ADC のユーザー認証情報は **quota project** が必要です。上の `gcloud config set project` で設定したプロジェクトが使われます。403 quota エラーが出る場合は
> `gcloud auth application-default set-quota-project <YOUR_PROJECT_ID>` を実行してください。

## 3. 環境変数の設定

```sh
cp .env.example .env
# .env を編集して SLACK_BOT_TOKEN / SLACK_CHANNEL / DRIVE_FOLDER_ID を設定
```

| 変数 | 必須 | 説明 |
|------|:---:|------|
| `SLACK_BOT_TOKEN` | ✓ | Bot トークン（`xoxb-...`） |
| `SLACK_CHANNEL` | ✓ | チャンネル名（`#` なし）または ID。`-channel` フラグで上書き可 |
| `DRIVE_FOLDER_ID` | ✓ | アップロード先 Drive フォルダ ID（URL 可）。`-folder` フラグで上書き可 |
| `DOWNLOAD_DIR` | | 一時ダウンロード先（既定 `./downloads`） |
| `MANIFEST_PATH` | | アップロード記録の保存先（既定 `./manifest.json`） |
