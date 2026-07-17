# 仕組みと設計判断

## 処理フロー

```
Slack files.list (page ページング, types=images,videos,zips)
   → mimetype で再フィルタ（image/* video/* application/zip）
   → manifest に記録済みならスキップ
   → url_private_download を Bearer トークン付きで DL（ローカルに一時保存）
   → Drive フォルダへ resumable upload（appProperties に slackFileId を刻印）
   → manifest.json に記録 → 一時ファイル削除
```

1ファイルずつ「DL → アップロード → 削除」を繰り返すため、ディスクには処理中のファイルしか残らない。

## 主要な設計判断

### Slack のページングは page ベース

`files.list` は他の多くの Slack エンドポイントと違い、`response_metadata.next_cursor` が**常に空**で、`paging.pages` による page ベースのページングを使う。cursor だけを見ると先頭 100 件で止まり残りを取りこぼすため、全ページ（1..`paging.pages`）を走査する。

### Drive 認証は gcloud ADC（ユーザー認可）

サービスアカウントは My Drive に storage quota を持たず、個人フォルダへのアップロードは `403 storageQuotaExceeded` で必ず失敗する。そのため**ユーザー本人として認可する ADC** を採用し、個人マイドライブ・共有ドライブのどちらにも入れられるようにした。

### ファイルのダウンロードは Bearer トークン必須

Slack のファイル URL は非公開で、`Authorization: Bearer` を付けないと HTTP 200 のまま HTML ログインページが返る。`Content-Type: text/html` を認証失敗として検出し、無言で壊れたファイルを保存しない。

### 冪等化は Slack file ID キーの manifest

Slack file ID は全チャンネルで一意かつ安定。これをキーにローカル manifest（原子的書き込み）で記録し、中断・再実行で成功済みをスキップする。ファイル名は衝突するので使わない。回復性のため Drive 側にも `appProperties.slackFileId` を刻印する。

### 大容量ファイル対策

動画や zip は数百 MB になり、分単位の転送中に接続が切れることがある。resumable upload を使い、さらにアップロード全体を指数バックオフで最大4回リトライする。失敗した resumable セッションは Drive に可視ファイルを残さないため、再作成しても重複しない。

## パッケージ構成

| パッケージ | 責務 |
|-----------|------|
| `cmd/slacktodrive` | フラグ/設定の解決と全体のオーケストレーション |
| `internal/config` | 環境変数・`.env` の読み込みと検証 |
| `internal/slackfiles` | チャンネル名→ID 解決、files.list 列挙、認証付き DL |
| `internal/drive` | ADC 認証と Drive への resumable upload |
| `internal/manifest` | Slack file ID キーの冪等化記録 |
