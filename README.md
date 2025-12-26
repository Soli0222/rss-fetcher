# RSS Fetcher

RSSフィードを定期的に監視し、新しい更新があった場合にWebhook（Slack, Discord, 等）に通知を送るGo製のアプリケーションです。

## 特徴

- **複数フィード対応**: 複数のRSSフィードを監視できます。
- **Webhook通知**: 更新検知時に指定したエンドポイントにPOSTリクエストを送信します。
- **レートリミット**: WebhookへのPOST間隔を制御し、レートリミット超過を防ぎます。
- **状態管理 (State Persistence)**:
  - **Valkey (Redis)**: 再起動しても過去の通知済みアイテムを記憶します。
  - **In-Memory**: 簡易的な利用のためにオンメモリ動作も可能です。
- **Prometheusメトリクス**: `/metrics` エンドポイントで監視用メトリクスを提供します。

## 使い方 (Docker Compose)

このプロジェクトには `compose.yaml` が含まれており、以下のコマンドですぐに実行可能です。

```bash
docker compose up --build -d
```

### 設定ファイル

設定は `config/` ディレクトリ内のYAMLファイルで行います。

#### 1. `config/feeds.yaml`

監視するRSSフィードと、チェック間隔、状態保存先を設定します。

```yaml
feeds:
  - https://rss.nytimes.com/services/xml/rss/nyt/Technology.xml
  - https://www.youtube.com/feeds/videos.xml?channel_id=UCRcLAVTbmx2-iNcXSsupdNA

# フィードをチェックする間隔
interval: 10m

store:
  # 永続化にValkey (Redis) を使用する場合
  type: 'valkey'
  address: valkey:6379
  
  # オンメモリで使用する場合（再起動で履歴が消えます）
  # type: 'memory'
```

#### 2. `config/webhooks.yaml`

通知先のWebhookを設定します。

```yaml
webhooks:
  - name: "my-webhook"
    url: "https://your-webhook-url.com/entrypoint"
    # POSTリクエスト間の待機時間（レートリミット対策）
    post_interval: 2s
    # ペイロード形式を指定: 'generic' (デフォルト) または 'discord'
    provider: generic

  - name: "discord-channel"
    url: "https://discord.com/api/webhooks/..."
    provider: discord
    post_interval: 2s
```

## 開発・ビルド

### 必要要件
- Docker / Docker Compose
- (ローカル開発の場合) Go 1.25+

### ローカルでの実行

```bash
# ビルド
go build -o rss-fetcher ./cmd/server

# 実行 (設定ファイルを指定)
./rss-fetcher -feeds config/feeds.yaml -webhooks config/webhooks.yaml
```

## メトリクス

アプリケーションはポート `:9090` でPrometheusメトリクスを公開しています。

- `http://localhost:9090/metrics`

主なメトリクス:
- `rss_fetch_count_total`: RSS取得回数 (status=success/error)
- `rss_new_items_total`: 新規検出アイテム数
