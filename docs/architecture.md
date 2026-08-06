# XordPress 企画・アーキテクチャ統合設計書

> "The 'X' Generation Engine for the Modern Web."
> WordPress の柔軟性と手軽さを継承しつつ、20年分の負の遺産(重さ・脆弱性・複雑なインフラ管理)を排除した、単一バイナリ型の次世代 CMS。

> 個別の技術選定の背景・却下した代替案は [docs/adr/](adr/README.md) を参照。

## 1. エグゼクティブ・サマリー

- **プロダクト名**: XordPress (ゾードプレス)
- **ビジョン**: 「5秒で起動し、永久に壊れない」Web サイト構築・運用環境の提供
- **ターゲット層**:
  - WP の保守地獄(脆弱性・改ざん・バージョンアップ破壊)に疲弊している Web 制作会社・フリーランス
  - 低コスト(格安 VPS 1 ノード等)で爆速かつ安全な Web サイトを所有したい中小企業・個人ブロガー
  - ノーコードで LP や記事を作りたいマーケター / 1px 単位でこだわりたい Web デザイナー
- **コアコンセプト**:
  - **Zero Infrastructure Overhead**: DB サーバーやミドルウェアのセットアップ不要(Go + 組み込み SQLite による単一バイナリ配布)
  - **Sandboxed Extensibility**: サーバーサイド Wasm(WebAssembly)による、サーバーを破壊できない安全なプラグイン機構
  - **Dual-Engine Theme System**: ノーコード派(D&D ブロックエディタ)とコード派(Liquid テンプレート)の両対応

## 2. システムアーキテクチャ

外部依存(Nginx/Apache、MySQL、PHP-FPM、Node.js 等)を排除し、1 つのコンパイル済みバイナリの中に Web サーバー、データベースエンジン、レンダリングエンジン、Wasm 実行環境をすべて内蔵する。

```text
┌────────────────────────────────────────────────────────────────────────┐
│                        XordPress Single Binary                         │
│                                                                          │
│  ┌──────────────────────┐    ┌──────────────────────────────────────┐  │
│  │  Web / TLS / WAF      │    │  Embedded Management UI (SPA)        │  │
│  │  (Go net/http + ACME) │    │  (Built-in via go:embed)             │  │
│  └──────────┬────────────┘    └──────────────────┬───────────────────┘  │
│             │                                    │                      │
│             ▼                                    ▼                      │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │  Core Engine & Router (Go)                                        │  │
│  └──────────┬───────────────────────────┬───────────────────────────┘  │
│             │                           │                              │
│             ▼                           ▼                              │
│  ┌──────────────────────┐    ┌──────────────────────────────────────┐  │
│  │ Wasm Plugin Runtime   │    │ Dual Theme Engine                     │  │
│  │ (Extism / wazero)     │    │ [Mode A: D&D]  [Mode B: Liquid]       │  │
│  └──────────────────────┘    └──────────────────────────────────────┘  │
│             │                                    │                      │
│             └─────────────────┬──────────────────┘                      │
│                                │                                        │
│                                ▼                                        │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │  Embedded Database: SQLite (modernc.org/sqlite, Pure Go / Cgo-free) │  │
│  │  └─> Local SQLite File + Optional Backup Sync (方式は今後決定)     │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────┘
```

## 3. 技術スタック

| レイヤー | 採用技術 | 選定理由 |
| --- | --- | --- |
| コア言語 | Go (Golang) | 高速な実行速度、低メモリ消費、クロスコンパイルと `go:embed` による単一バイナリ化 |
| データベース | SQLite (`modernc.org/sqlite`, Pure Go) | ゼロ設定・ファイルストレージ動作、CGO 不要でクロスコンパイルと単一バイナリ配布に適合(詳細は [ADR 0001](adr/0001-pure-go-sqlite-over-cgo-libsql.md))。S3/Turso への同期は別手段を今後検討 |
| ネットワーク / Web | Go `net/http` + `golang.org/x/crypto/acme`(Phase 2 で導入) | Caddy 相当の自動 SSL 証明書発行(Let's Encrypt)を目指す。Phase 1 は TLS 終端なしの平文 HTTP(§5.1 の差分注記参照)。「軽量 WAF」は現時点でどの ADR・ロードマップにも対応しない将来的な検討課題であり、Phase 1/2 のスコープではない |
| プラグイン環境 | Extism (`extism/go-sdk`, wazero バックエンド, Pure Go) | サーバーサイド Wasm ランタイム。プラグイン実行をメモリ・I/O レベルでサンドボックス隔離。CGO 不要で [ADR 0001](adr/0001-pure-go-sqlite-over-cgo-libsql.md) の単一バイナリ方針と両立(詳細は [ADR 0002](adr/0002-extism-for-wasm-plugin-runtime.md)) |
| テーマエンジン(コード) | Liquid (`osteele/liquid`) | Shopify 実績のある安全なテンプレート言語。プログラミングロジックの侵入を防ぎ、参入障壁を下げる(詳細は [ADR 0003](adr/0003-existing-go-liquid-library-for-theme-engine.md)) |
| テーマエンジン(ノーコード) | AST JSON Renderer | D&D で生成された JSON 構造を解析し、サーバー側で HTML 化(Phase 2 スコープ) |
| 管理画面フロント | TypeScript + TanStack (React) | Vite でビルドし、生成物を `go:embed` で Go バイナリ内部に全埋め込み(詳細は [ADR 0004](adr/0004-tanstack-react-for-admin-frontend.md)) |

## 4. デュアル・テーマアーキテクチャ

ユーザーのリテラシーと用途に合わせ、「ビジュアル・ブロックモード(Studio 方式)」と「Liquid コードモード(Shopify 方式)」を自由または併用して切り替えられる設計とする。

```text
                               ┌─────────────────────────────────────────┐
                               │       XordPress 配信エンジン (Go)        │
                               └────────────────────┬────────────────────┘
                                                     │
                                ┌───────────────────┴───────────────────┐
                                ▼                                       ▼
                  【モード A: Visual Block Engine】       【モード B: Liquid Code Engine】
                   (Studio / Webflow 方式)                  (Shopify / Ghost 方式)
                                │                                       │
                                ▼                                       ▼
                  管理画面の D&D で配置した                デザイナーが書いた HTML/CSS
                  「JSON データ」を解析                    および .liquid ファイルを解析
                                │                                       │
                                └───────────────────┬───────────────────┘
                                                     │
                                                     ▼
                               ┌─────────────────────────────────────────┐
                               │  数ミリ秒でピュアな HTML をレンダリング  │
                               └─────────────────────────────────────────┘
```

### モード A: ビジュアル・ブロックモード (Studio / Webflow 方式)

- **概要**: コードを一切触りたくない層向けの完全ノーコード環境。
- **仕組み**: 管理画面内の D&D エディタでブロックやレイアウトを配置。データは `theme.json` として SQLite に保存。
- **強み**: コード編集やファイルアップロードが不要で、画面崩れ事故が物理的に起きない。

### モード B: Liquid コードモード (Shopify 方式)

- **概要**: 独自のデザインを 1px 単位で厳密に作り込みたい Web 制作会社・プロデザイナー向けの環境。
- **仕組み**: HTML/CSS/JS に Liquid タグ(`{{ post.title }}` 等)を埋め込んだ `.liquid` テンプレート群を ZIP でアップロード。
- **強み**: 既存の Web 制作フローを崩さず移行可能。PHP のような危険なロジックを排除しているためセキュリティ事故がゼロ。

### ハイブリッド運用

全体構造はモード B(Liquid テーマ)で固定しつつ、記事や LP の本文エリア内のみモード A(ビジュアルブロック)で自由にレイアウトを組む運用をサポートする。

## 5. 主要機能とユーザー体験

### 5.1 5 秒セットアップ (Zero-Config Installation)

1. サーバー上でバイナリを 1 個配置して実行する。

   ```bash
   chmod +x xordpress && ./xordpress
   ```

2. ブラウザでアクセスする(ポート 80/443 を自動バインドし、SSL 証明書も自動取得)。
3. 初回画面で「サイト名」と「管理者パスワード」を入力するだけでセットアップ完了。

> **Phase 1 での差分**: 自動 SSL 取得(ACME)は Phase 2 スコープ([docs/architecture.md](#8-ロードマップ-product-roadmap) §8)。Phase 1 では既定ポート `:8080` で `http://` のみ待ち受ける簡略版から始める([Phase 1 設計書](plans/phase1-mvp/design.md) §10)。また、初回セットアップの乗っ取り(誰でも最初に到達して管理者になれてしまう問題)を防ぐため、起動時に標準出力へ一度だけ出力される setup token をセットアップ画面に入力する一手間が Phase 1 では必要になる(詳細は [Phase 1 設計書](plans/phase1-mvp/design.md) §4)。「5秒セットアップ」は Phase 2 以降、この手順を安全に省略・自動化する形を再検討する。

### 5.2 Wasm プラグイン・エコシステム (Sandboxed Plugins)

- **配布形式**: `.wasm` ファイル単体、または公式ストアからの 1 クリックインストール。
- **安全性**: プラグインごとに「特定テーブルの読み書き許可」「特定ドメインへの HTTP リクエスト許可」等のパーミッションを管理画面から付与。悪質・不具合コードによるサーバーダウンやデータ漏洩が構造的に不可能。

### 5.3 WP ワンクリックインポーター (WP Migration)

- WordPress 公式の「XML エクスポートファイル」または「REST API URL」を入力するだけで、記事・固定ページ・画像アセット・カテゴリー構造を全自動で SQLite 内へ変換・引き継ぎ。

## 6. ディレクトリ構成

本番運用時、サーバー上に展開される構成:

```text
/var/www/my-site/
├── xordpress              # Go の単一バイナリファイル (20MB〜30MB程度)
├── data/
│   ├── xordpress.db       # SQLite データベースファイル
│   └── xordpress.db-wal   # Write-Ahead Logging
├── uploads/               # ユーザーがアップロードした画像・メディア
├── themes/                # 導入済みテーマフォルダ (Liquid モード用)
│   └── default/
└── plugins/               # 導入済み Wasm プラグイン (.wasm)
```

## 7. プラグインアーキテクチャ詳細 (Server-Side Wasm)

プラグイン開発者は Go, Rust, TypeScript (AssemblyScript), C/C++ など任意の言語でコードを記述し、Wasm にコンパイルして配布する。

### プラグインフックの例 (Rust)

```rust
// 記事公開時に自動で外部へ通知を飛ばすプラグイン例
#[plugin_fn]
pub fn on_post_publish(post_json: String) -> FnResult<String> {
    // 許可された HTTP 通信のみ実行可能
    let res = http::send_request("POST", "https://api.example.com/webhook", post_json);
    Ok(res)
}
```

## 8. ロードマップ

### Phase 1: Minimum Viable Product (MVP)

- Go + 組み込み SQLite (modernc.org/sqlite) による単一バイナリコアの構築
- Liquid テンプレートエンジンの統合と基本テーマの提供
- CLI でのポータブル起動と基本管理画面(記事作成・一覧)の実装

### Phase 2: Dual Theme Engine & Migration

- D&D ビジュアルブロックエディタの実装
- WP インポーターの実装
- Extism を用いた Wasm プラグインランタイムの組み込み
- 自動 SSL 取得機能(ACME)の実装

### Phase 3: Ecosystem & Marketplace

- 公式プラグイン・テーマストアの提供(Web/管理画面内統合)
- Litestream 等による DB ワンクリックバックアップ機能の提供(具体的な同期方式は [ADR 0001](adr/0001-pure-go-sqlite-over-cgo-libsql.md) の Consequences で今後決定するとした通り、別途 ADR 化する)
