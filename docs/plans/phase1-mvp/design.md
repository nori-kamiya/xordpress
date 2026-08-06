# XordPress Phase 1 (MVP) — 詳細設計

> 前提: [docs/architecture.md](../../architecture.md) §8 の Phase 1 スコープ、および [docs/adr/](../../adr/README.md) の決定(0001-0007)。
> 本書は3案(simplest-first / scale-perf-first / migration-risk-first)のブラインド設計パネル+判定を経て、simplest-first案を採用し、他案から要素を接合したもの。その後 Opus 5 / Fable による独立した敵対的レビューを**2ラウンド**実施し、都度発見された欠陥を反映済み(1ラウンド目: §4のルーティング矛盾、draft漏洩、schema_migrationsの循環参照、go:embedパス誤りなど。2ラウンド目: 実コードを書いて実行検証した上での、`settings`テーブルの書き手不在の再発、path-traversalテストの無効化、Secure cookie の欠落、setup処理のアトミック性の担保先不在など)。§11 に対応履歴を記録。

## 1. Overview

Phase 1 は「Go 単一バイナリ + SQLite + Liquid + 最小限の管理画面(投稿の作成・一覧のみ)」を実装する。パッケージ境界は `content`(ドメイン)/`theme`(レンダリング)/`storage`(永続化)/`httpapi`(公開ページ)/`adminapi`(管理API)/`adminui`(埋め込みSPA)の6つに分離し、Phase 2 の Wasm プラグインフック(ADR 0002)は `content.{Create,Update,Delete,List,Get,ListPublished,GetPublished}` と `theme.Render` という「関数の待ち合わせ地点」をラップするだけで追加できるようにする。**この時点でフック用インターフェースは作らない** — 実装のない抽象化は導入しない、という開発方針(guardrails、ADR 0005)に従う。

## 2. Architecture

```text
xordpress/
├── main.go                      # flag parsing → app.Run(ctx, cfg) の薄いラッパーのみ
├── internal/
│   ├── app/                     # 配線専用: storage/theme/content/auth を組み立てて mux を構成
│   │   └── app.go
│   ├── config/                  # Config struct (Addr, DataDir, BaseURL)。DataDir はデプロイルート相当で
│   │                             # data/xordpress.db と themes/ の両方がこの下に解決される(§3, §6 参照)
│   ├── storage/                 # sqlite.go, migrations/*.sql (embed), posts.go, users.go, sessions.go, settings.go
│   ├── content/                 # ドメイン層: Post, SiteSettings, Create/Update/Delete/List/Get/ListPublished/GetPublished — Phase 2 フックの接合点1
│   ├── theme/                   # Liquid レンダリング: Render(ctx, tmplName, data) — Phase 2 フックの接合点2
│   │   └── themes/default/      # 埋め込みスターターテーマ (layout/, templates/, assets/)
│   ├── auth/                    # パスワードハッシュ、セッション発行/検証、ミドルウェア
│   ├── httpapi/                 # 公開サイト: GET / (一覧), GET /{slug} (単一記事), GET /assets/
│   ├── adminapi/                # JSON API: /api/posts, /api/session, /api/setup
│   └── adminui/
│       └── dist/                # Vite の outDir をここに向ける。プレースホルダ index.html を commit
├── web/admin/                    # TanStack(React) + Vite ソース(別ビルド、`go build` の対象外、outDir=../../internal/adminui/dist)
└── themes/                      # ディスク上の上書きテーマ配置場所(存在すれば埋め込み版より優先)
```

**`go:embed` の配置に関する注意**(レビューで指摘された致命的な誤り): `go:embed` は自パッケージ配下のファイルしか埋め込めず、親ディレクトリを跨いだ相対パス(`../../admin/dist`)は指定できない。そのため SPA のビルド出力先は `internal/adminui/dist/` に固定する(Vite の `outDir` をそこに向ける)。トップレベルの `admin/` ディレクトリは作らない。

### インポート境界ルール

将来 `check:boundaries` 的な静的チェックで強制する前提の許可リスト:

| package | import 可 |
| --- | --- |
| `storage` | (内部パッケージなし。標準ライブラリ + sqlite ドライバのみ) |
| `theme` | (内部パッケージなし) |
| `auth` | `storage` |
| `content` | `storage` |
| `httpapi` | `content`, `theme` |
| `adminapi` | `content`, `auth` |
| `adminui` | (内部パッケージなし。embed のみ) |
| `app` | 上記すべて(唯一の composition root) |

**強制すべき禁止事項**: `storage` と `theme` は `httpapi`/`adminapi`/`adminui` を絶対に import しない。`httpapi` と `adminapi` も互いに import しない。これにより永続化とレンダリングは HTTP 層の存在を知らないままになり、Phase 2 のプラグインフックが `content.*` と `theme.Render` の呼び出し箇所だけをラップすればよい状態を保つ。

サイト設定(`site_name` 等)は `storage` に settings テーブルとして永続化するが、読み書きは `content` パッケージが公開する `content.GetSiteSettings()` / `content.SetSiteSettings(...)` を経由する。`httpapi`/`theme` は `storage` を直接 import しないため、レイアウトが必要とする `site.name` 等の値は `httpapi` が `content.GetSiteSettings()` 経由で取得し、`theme.Render` へのバインディングとして渡す。

**`SetSiteSettings` の呼び出し元(2ラウンド目のレビューで指摘: 前回追加したこの関数自体が呼び出し元のないまま放置されていた)**: `POST /api/setup`(§4)が `site_name` を受け取り、`content.CreateFirstAdmin` の一部として1回だけ書き込む。運用中の変更用に `PUT /api/settings`(認証必須、§4)も追加し、これが2つ目かつ唯一の追加呼び出し元になる。

### Phase 2 との接合点(今は作らない)

- 投稿のライフサイクルは必ず `content.Create` / `Update` / `Delete`(と読み取り用の `List`/`Get`/`ListPublished`/`GetPublished`)を通る。`content.CreateFirstAdmin`(初回セットアップ専用、§4)と `content.GetSiteSettings`/`SetSiteSettings` は投稿ライフサイクルの外にあるため Phase 2 のフック対象外とする。
- 公開レンダリングは必ず `theme.Render(ctx, tmplName string, data any) (string, error)` を通る。
- Phase 2 はこの関数呼び出し箇所を pre/post フックで囲むだけで Extism ディスパッチを追加できる。フックインターフェース・プラグインマニフェストテーブルは Phase 1 では一切導入しない(実装ゼロの抽象化を避ける、ADR 0005)。

## 3. Data model

```sql
-- 0001_init.sql
-- schema_migrations テーブルはマイグレーションランナー自身が `CREATE TABLE IF NOT EXISTS` で
-- 作成する(§3「マイグレーション方式」参照)。この初期マイグレーションファイルには含めない —
-- ランナーが最初に参照するテーブル自体をマイグレーションで作ろうとすると起動時に循環する。

CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,          -- bcrypt (SHA-256 プリハッシュ済み、§7参照)
  created_at    INTEGER NOT NULL
);

CREATE TABLE posts (
  id           INTEGER PRIMARY KEY,
  slug         TEXT NOT NULL,
  title        TEXT NOT NULL,
  body         TEXT NOT NULL,           -- 記載時点の生 HTML、Phase 1 では変換なし
  status       TEXT NOT NULL CHECK (status IN ('draft','published')) DEFAULT 'draft',
  version      INTEGER NOT NULL DEFAULT 1,  -- 楽観的並行性制御用(§4 PUT の If-Match 参照)
  author_id    INTEGER NOT NULL REFERENCES users(id),
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  published_at INTEGER                 -- 最初に公開されるまで NULL
);
CREATE UNIQUE INDEX idx_posts_slug ON posts(slug);
CREATE INDEX idx_posts_status_published ON posts(status, published_at DESC)
  WHERE status = 'published';           -- 公開一覧の主要アクセスパターン用の部分インデックス

CREATE TABLE sessions (
  token_hash TEXT PRIMARY KEY,          -- cookie 値の SHA-256。生トークンは保存しない
  user_id    INTEGER NOT NULL REFERENCES users(id),
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);  -- 期限切れセッションの掃除用

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);                                       -- site_name 等。新規設定追加のたびのマイグレーションを避けるため schemaless
```

**投稿ライフサイクルのルール**(`content` パッケージが担う。`content.Publish` という独立関数は導入しない — ADR 0005 の「実装のない/到達経路のない関数を作らない」方針により、`Create`/`Update` の内部に一本化する):

- `published_at` は、投稿の `status` が `draft` → `published` に遷移する瞬間(`Create` 時点で最初から `status: published` を指定した場合を含む)にのみ、その時点で `published_at` が NULL であれば現在時刻をセットする。判定とセットは `content.Create`/`content.Update` の内部、DB 保存直前の一箇所だけで行う共通ヘルパーに集約する。ハンドラ層(`adminapi`)はタイムスタンプを一切セットしない。
- 既に `published_at` が非 NULL の投稿が再度 `published` になっても値は変更しない(初回公開日時を保持する)。
- `draft` に戻した場合(`status: draft` への `Update`)、`published_at` は変更しない(過去に公開された事実の記録として残す)。

**読み取りの公開/管理分離**(レビューで指摘された「draft が公開ページから見える」問題への対応): `content` パッケージは読み取り用に2系統の関数を公開する。

- `content.List(filter)` / `content.Get(id)` — **管理API専用**。`status` を問わず全件を返す(admin が下書きを見られる必要があるため)。`adminapi` からのみ呼ばれる。
- `content.ListPublished()` / `content.GetPublished(slug)` — **公開サイト専用**。実装は必ず `WHERE status = 'published'` を伴う。`httpapi` はこの2関数以外の読み取り経路を持たない(`content.List`/`Get` を `httpapi` から呼ぶことは禁止 — レビューで指摘されたコードレビュー項目)。

決定事項:

- タイムスタンプは全て `INTEGER`(unix seconds)。ドライバ非依存でスキャンが単純になる。
- `sessions` は生トークンでなくハッシュを保存する — DB ダンプ/バックアップの漏洩がセッション乗っ取りに直結しないようにする。
- `settings` は k/v テーブル。型安全性は失うが、Phase 1 の設定項目(`site_name` 程度)ではマイグレーション不要の方が実利が大きい。
- `posts.version` は PUT の楽観的並行性制御にのみ使う整数カウンタ。更新のたびに `+1` する。

### マイグレーション方式

**手書きマイグレーション**(goose/golang-migrate は不採用)。マイグレーションランナーが起動時に以下の順で実行する:

1. `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)` をランナー自身のコードで実行する(埋め込み `.sql` ファイルには含めない — 「ランナーが最初に読むはずのテーブルをマイグレーションで作る」循環を避けるため)。
2. `//go:embed migrations/*.sql` で埋め込まれた `0001_init.sql` 等をファイル名の番号順にソートする。
3. 各ファイルにつき、`schema_migrations` に未適用であれば、そのファイル1本を1トランザクションで実行し(`BEGIN` → SQL 全体を実行 → `INSERT INTO schema_migrations` → `COMMIT`)、適用済みならスキップする。ファイル間で共有するトランザクションはない — 1ファイル=1トランザクション。

総量約60行。

却下理由:
- `pressly/goose`: 良質なライブラリだが、追加の依存 + CLI バイナリを、Phase 1 では一度きり(forward-only、ロールバック不要)の用途のために導入するのは過剰。
- `golang-migrate`: 一部の sqlite ドライバラッパーは CGO ベースの `mattn/go-sqlite3` 前提であり、無自覚に採用すると ADR 0001 が排除したはずの CGO 依存を静かに再導入するリスクがある。

将来ダウンマイグレーションや分岐スキーマ変更が必要になった時点で goose 採用を再検討する(エスケープハッチとして明記)。

### DB接続設定(検証済み)

`modernc.org/sqlite` の登録ドライバ名は `"sqlite"`(`sqlite3` ではない)。DSN は以下の形式を使う(公式ドキュメントで動作確認済み):

```go
dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate",
    filepath.Join(cfg.DataDir, "data", "xordpress.db"))
db, err := sql.Open("sqlite", dsn)
```

DB ファイルのパスは `config.DataDir` を基準に解決する(CWD 相対にしない — systemd 等で `WorkingDirectory` が異なる場合に静かに別ファイルを参照する事故を防ぐ)。起動時に `os.MkdirAll(filepath.Join(cfg.DataDir, "data"), 0o755)` でディレクトリを用意する。

`db.SetMaxOpenConns(1)` を設定し、SQLite の単一ライターの制約を Go 側のコネクションプールで素直に直列化する(`SQLITE_BUSY` の再試行ロジックを書かない)。読み取り専用の別プールは Phase 1 では導入しない — 単一管理者のトラフィック量では不要、将来の非決定事項として明記するに留める。

**プールサイズ1に関する実装上の注意**(レビューで指摘されたデッドロックの罠): プールが1コネクションしかないため、`*sql.Tx` を開始した後に同じゴルーチン内で `db.Exec`/`db.QueryRow`(トランザクション外)を呼ぶと、そのトランザクションが唯一のコネクションを保持したまま解放されず、`db` 経由の呼び出しは永久にブロックする。**ルール**: 一度 `*sql.Tx` を開始したら、そのトランザクションのスコープ内では `tx.Exec`/`tx.QueryRow` のみを使い、`db.*` を混在させない。チェック→挿入のようなアトミック性が必要な操作(`/api/setup` の初回作成、slug 衝突チェックなど)は必ず単一の `*sql.Tx` 内で完結させる。

## 4. API contracts

**認証**: 単一管理者、セッションベース(自己ホスト・シングルテナントのため OAuth 等は過剰)。セッション cookie の仕様(前回レビューで「`Secure` が接続の TLS 状態に依存しており、推奨デプロイ構成(リバースプロキシで TLS 終端)では常に外れる」と指摘された箇所への対応):

- 名前: `xordpress_session`、`Path=/`、`HttpOnly`、`SameSite=Lax`。
- `Secure` 属性は **リクエストの TLS 状態ではなく `config.BaseURL` のスキームで決める**(`https://` なら常に `Secure` を付与、リバースプロキシ配下で XordPress プロセス自体は平文 HTTP しか見えない場合でも正しく付く)。
- 有効期限: `sessions.expires_at` は発行から 7 日固定(スライディング延長なし)。`auth` パッケージがセッション検証のたびに期限切れなら 401 を返す(§7 の lazy expiry)。

**CSRF 対策**: 状態変更を伴う管理APIリクエスト(`POST`/`PUT`/`DELETE` の全て)では `Origin` ヘッダーを検証する。検証先は `config.BaseURL`(CLI 起動時に `--base-url`、既定値 `http://localhost:8080`)— サーバー自身の bind アドレスではなく、リバースプロキシ配下でも正しく機能するよう明示的に設定させる。ポリシー: **`Origin` ヘッダーが存在し、かつ `BaseURL` と一致しない場合のみ 403 で拒否する。`Origin` ヘッダー自体が存在しない場合は許可する**(同一オリジンの一部のリクエストや非ブラウザクライアントで欠落しうるため、欠落を拒否条件にはしない)。

**Content-Type 要件**(前回レビューで「ボディを持たない `DELETE` にまで `Content-Type: application/json` を必須にすると自己矛盾する」と指摘された箇所への対応): `Content-Type: application/json` の必須チェックは **リクエストボディを伴うメソッド(`POST`, `PUT`)にのみ適用する**。`DELETE`(ボディなし)には適用しない。

**ミドルウェアの適用順序**(§5 のルーティングと合わせて明示): 各 `/api/*` リクエストは (1) Origin/Content-Type チェック → (2) セッション認証 → (3) `adminapi` 内部ルーティング、の順で処理する。Origin 不一致は認証の成否によらず 403 を返す(未認証状態の応答からリクエストの真偽を推測されないようにするため、認証チェックより先に評価する)。

**初回セットアップの保護**(レビューで指摘された「誰でも最初に /api/setup を叩けば管理者を乗っ取れる」問題への対応): `app.Run` はバイナリ起動時に、DB に `users` が1件も存在しなければランダムな setup トークン(`crypto/rand`、32バイト以上、Base64 エンコードして出力)を生成して標準出力に一度だけ出力する(`setup token: <token>`)。このトークンはプロセスのメモリ上に保持し、`adminapi.Setup(content, auth, setupToken)` に明示的な引数として渡す(`app` が構築時に注入する — §2 の composition root の責務)。

`POST /api/setup` の処理順序:
1. `Authorization: Bearer <token>` ヘッダーを、起動時に生成したトークンと `crypto/subtle.ConstantTimeCompare` で比較する。不一致(または起動時にトークンが生成されなかった=既に管理者が存在した)なら 401。DB へは触れない。
2. トークンが一致したら `content.CreateFirstAdmin(ctx, username, passwordHash, siteName) (created bool, err error)` を呼ぶ。これが唯一のアトミック操作の担当者(前回レビューで「アトミック性を要求しているが担当パッケージが指定されていない」と指摘された箇所への対応)。実装は単一の `*sql.Tx`(`db.BeginTx` 〜 `tx.Commit`)内で `SELECT count(*) FROM users` → 0 件なら `INSERT INTO users ...` + `INSERT INTO settings(key='site_name', ...)` → commit、1件以上なら rollback して `created=false` を返す。`adminapi` は `created=false` を 409 にマッピングする。
3. §3 のプールサイズ1ルール(`SetMaxOpenConns(1)`)により、この `*sql.Tx` がライブ中は唯一のコネクションを保持し続けるため、チェックと挿入の間に**同一プロセス内の**他のリクエストが割り込むことはない。

DSN の `_txlock=immediate`(§3)は、この Tx の開始文を `BEGIN IMMEDIATE` にする(`db.BeginTx` がこれを発行する仕組みは `modernc.org/sqlite` の DSN オプションとして提供されており確認済み)。ただし効果を正確に述べると: `_pragma=busy_timeout(5000)` と併用した場合、これは「`SQLITE_BUSY` を早期検出する」ものでは**ない**(`busy_timeout` がある限り最大5秒リトライする)。実際の効果は、SQLite の既定(deferred)トランザクションが読み取りロックで開始し書き込み時にロックの昇格に失敗する(`SQLITE_BUSY_SNAPSHOT`)パターンを避け、書き込みロックを最初から取得することで、同一 DB ファイルを開く**別プロセス**(バックアップツールや `sqlite3` CLI 等)との競合を `busy_timeout` の再試行に一本化する点にある。

**セットアップ完了後の挙動**: `users` が1件以上存在する状態で `POST /api/setup` を叩くと、上記の手順1でトークンの生成自体が行われていない(起動時に既にユーザーがいた)ため常に 401 になる。トークン再利用や比較対象が空文字列になる曖昧さは生じない。

```
POST   /api/setup                 { username, password, site_name } → 201 {id}       (要 Authorization: Bearer <setup token>。
                                                                                        トークン不一致/未発行(=起動時に既に user が存在した)なら 401。
                                                                                        トークンは一致したが content.CreateFirstAdmin が
                                                                                        created=false を返した場合(競合する2つ目のリクエスト)は 409。
                                                                                        1回きり)
POST   /api/session               { username, password }             → 204 + Set-Cookie (401: 認証失敗)
DELETE /api/session                                                  → 204             (ログアウト、cookie + セッション行を削除。要認証)

GET    /api/posts        ?status=&limit=&offset=                 → 200 {"items":[{id,slug,title,status,version,updated_at}, ...],"total":n}
                                                                     (limit 既定20・上限100、offset 既定0。不正な値は 400。
                                                                      ORDER BY published_at DESC NULLS LAST, id DESC で安定ソート。
                                                                      total は ?status= が指定されていればそのフィルタ後の件数、
                                                                      未指定なら全件数)
POST   /api/posts                 { title, slug, body, status }  → 201 {id,slug,version:1,...}
                                                                     (400: バリデーション不備、409: slug 衝突、413: ボディ超過)
GET    /api/posts/{id}                                           → 200 {...full...}   (404, {id} が数値としてパース不能なら 400)
PUT    /api/posts/{id}             body: {title,slug,body,status}
                                   header: If-Match: "<version>" → 200 {...}
                                                                     (400: If-Match欠落・不正、404、409: slug 衝突、412: version 不一致)
DELETE /api/posts/{id}             header: If-Match: "<version>" → 204               (400: If-Match欠落・不正、404、412: version 不一致)

PUT    /api/settings              { site_name }                  → 200 {site_name}    (400: バリデーション不備。要認証)
```

`/api/setup` と `POST /api/session` 以外の `/api/*` は全てセッション認証ミドルウェアを通す(具体的なルーティング上の実現方法は §5 参照 — Go 1.22 `ServeMux` のパターン優先順位により、より具体的な登録が `authMiddleware` でラップされた汎用ハンドラより優先されるため、この2つだけを認証なしで個別登録する)。`adminapi.Handler` は内部に自身の `*http.ServeMux` を持ち、完全パス+メソッドのパターン(`"GET /api/posts"`, `"POST /api/posts"`, `"GET /api/posts/{id}"`, `"PUT /api/posts/{id}"`, `"DELETE /api/posts/{id}"`, `"DELETE /api/session"`, `"PUT /api/settings"`)で各エンドポイントを登録する。

**バリデーションルール**(レビューで「未規定」と指摘された項目への対応):

- `title`: 必須、1〜200文字。
- `body`: 必須、最大1MB(リクエストボディ全体には `http.MaxBytesReader` で 2MB の上限を設け、超過は 413)。
- `slug`: 必須、正規表現 `^[a-z0-9]+(-[a-z0-9]+)*$` にマッチすること(小文字ケバブケースのみ)。空文字列・大文字・スラッシュ・記号は 400。加えて予約プレフィックス `api`, `admin`, `assets` と完全一致する slug は 400 で拒否する(これらは `/api/`, `/admin/`, `/assets/` ルートと衝突し、作成できても永久に到達不能になるため)。
- `status`: `draft` または `published` のみ。それ以外は 400(DB の `CHECK` 制約に到達する前に弾く)。
- `username`(setup/login 共通): 3〜32文字、英数字とハイフン・アンダースコアのみ。
- `password`: 8文字以上(上限なし — bcrypt の72バイト制限は §7 の SHA-256 プリハッシュにより実質無関係になる)。
- `site_name`: 必須、1〜100文字。
- slug 衝突検出は `INSERT`/`UPDATE` を試みて `modernc.org/sqlite` のユニーク制約違反(拡張エラーコード 2067, `SQLITE_CONSTRAINT_UNIQUE`。[sqlite.org/rescode.html](https://sqlite.org/rescode.html) で確認済み)を `*sqlite.Error` の型アサーション + `Code()` で検出し 409 にマッピングする(SELECT してから INSERT する TOCTOU 方式は取らない)。

**並行更新ポリシー**: `PUT`/`DELETE` はリクエスト**ヘッダー** `If-Match: "<version>"` を必須とする(JSON ボディの一部ではない)。欠落・パース不能なら 400。DB 上の `version` と一致しなければ 412 を返し、一致すれば更新(`PUT` は `version` を +1、`DELETE` は行ごと削除)する。`POST /api/posts` のレスポンスは常に `version: 1` を含む。管理画面(SPA)は `GET`/`POST`/`PUT` のレスポンスに含まれる `version` を保持し、次の `PUT`/`DELETE` に `If-Match` として添付する。

エラーレスポンスは統一形式 `{"error": "message"}`。

**スラッグの改名について(既知の制限)**: `PUT` で `slug` を変更できるが、Phase 1 では旧 slug から新 slug へのリダイレクトや履歴保持は行わない。旧 URL は変更後に 404 になる。WordPress からの移行(旧URL維持)は Phase 2 の WP インポーター実装時に別途設計する既知のギャップとして許容する。

## 5. Routing

Go 1.22+ の `http.ServeMux` メソッドパターンのみを使用(サードパーティルーターは Phase 1 の規模では不要)。`ServeMux` はより具体的なパターンを優先するため、認証不要な2エンドポイントを個別登録してから汎用の `/api/` を後段に置くことで、§4 の認証除外を矛盾なく表現できる:

```go
mux.Handle("POST /api/setup", adminapi.Setup(content, auth, setupToken))  // 認証不要(setup token で保護)
mux.Handle("POST /api/session", adminapi.Login(content, auth))            // 認証不要(ログインそのもの)
mux.Handle("/api/", csrfMiddleware(baseURL,
              authMiddleware(adminapi.Handler(content, auth))))          // 上記2つ以外の全 /api/* (DELETE /api/session を含む)

mux.Handle("/admin/", adminui.Handler())                          // SPA 静的配信 + 未一致パスは index.html にフォールバック
mux.Handle("GET /assets/", httpapi.AssetsHandler(theme))          // アクティブテーマの assets/ を配信(path-traversal ガード付き)
mux.HandleFunc("/", httpapi.Handler(content, theme).ServeHTTP)    // "/" = 一覧, "/{slug}" = 単一記事(ListPublished/GetPublished のみ使用)。GET/HEAD 以外は 405
```

`/admin`(末尾スラッシュなし)は `/admin/` へリダイレクト。`httpapi.Handler` は `GET`/`HEAD` 以外のメソッドに対して明示的に 405 を返す(`ServeMux` の `"/"` パターンは全メソッドにマッチするため、ハンドラ内で弾く必要がある)。

**`GET /assets/` の path-traversal ガードに関する注意**(2ラウンド目のレビューで、生の `../` を使ったテストが `ServeMux` 自身のパスクリーニング機構によって `httpapi.AssetsHandler` に到達する前に 307 リダイレクトされ、テストが何も検証しないまま通ってしまうと指摘された): `net/http` の `ServeMux` は登録パターンにマッチさせる前にリクエストパスを正規化し、`..` を含むパスは正規化後のパスへ 307 リダイレクトする。したがって `GET /assets/../../../etc/passwd` は `httpapi.AssetsHandler`에 到達せず、`/` の `httpapi.Handler` に解決されて 404 になる(ガードをテストしたことにならない)。実際にガードへ到達しうるのは **パーセントエンコードされたトラバーサル**(例: `GET /assets/%2e%2e%2f%2e%2e%2fetc%2fpasswd`)であり、`filepath.Clean` + プレフィックスチェックのガード(§7)はこの経路を防ぐために存在する。§8 のテストシナリオはこちらの形式を使う。

## 6. Rendering (Liquid)

`osteele/liquid` のインクルード/レイアウト解決機構には依存せず、Go 側で明示的な2パスレンダリングを実装する(ADR 0003 で指摘した拡張ポイントの不確実性を丸ごと回避する設計判断。ADR 0003 にはこの設計判断による補足を追記済み):

1. `templates/index.liquid`(一覧)または `templates/post.liquid`(単一記事)を投稿データでレンダリングし、文字列を得る。
2. その文字列を `content_for_layout` 変数として `layout/theme.liquid` に束縛してレンダリングする。

テーマ検索順序: `filepath.Join(config.DataDir, "themes", "default")`(`os.DirFS`)が存在すればそちらを優先、なければ埋め込み版(`embed.FS` + `fs.Sub`)にフォールバックする(CWD 相対にはしない — §3 で DB パスに適用したのと同じ理由。前回レビューで「テーマ・DB パスがどちらも `--data-dir` に紐付いていない」と指摘された)。architecture.md §6 のディレクトリ構成(`themes/` 配下)を満たしつつ、テーマインストーラー機能なしで実現する。`GET /assets/` の配信ルートも同じ解決順序(ディスク上書き優先→埋め込みフォールバック)を使う。

**公開一覧の件数制限**: `content.ListPublished()` は明示的なページネーション UI を Phase 1 では提供しないが、`ORDER BY published_at DESC, id DESC LIMIT 100` を内部に固定で持つ(前回レビューで「上限が一切ない」と指摘された安全弁)。100件を超える投稿の一覧・アーカイブ機能は Phase 2 以降のスコープとする。

### スターターテーマの最小ファイル構成

- `layout/theme.liquid` — `<html><head><title>{{ site.name }}</title></head>...{{ content_for_layout }}...`(`site.name` は `content.GetSiteSettings()` から `httpapi` が取得しバインドする)
- `templates/index.liquid` — `posts` をループして一覧表示
- `templates/post.liquid` — 単一の `post.title` / `post.body`
- `assets/style.css` — `httpapi` の `GET /assets/` ルートから静的配信(§5)

**検証済み**: `osteele/liquid` は `Bindings`(`map[string]any`)経由で文字列変数をそのまま束縛できるため、上記の2パス方式(body をレンダリングした文字列を `content_for_layout` として layout に注入)は公式ドキュメントのサンプルどおりに動作する(`engine.ParseAndRenderString` を2回呼ぶだけ)。ライブラリは 2026年時点でも継続的にリリースされている(直近リリース `v1.8.1`、ソース確認済み)。

**エスケープに関する前提(明示)**: `osteele/liquid` は既定で自動エスケープを行わない(`{{ }}` はそのまま出力される。`SetAutoEscapeReplacer` を明示的に設定した場合のみエスケープが有効になる)。この2パス設計は `content_for_layout` に生の HTML 文字列をそのまま注入できることに依存しているため、**将来オートエスケープを有効化する変更を行う場合は `content_for_layout` を `{{ content_for_layout }}` ではなく明示的に非エスケープ扱いする必要がある**(現時点ではオートエスケープを有効化する予定はないため Phase 1 では問題にならない)。

## 7. Cross-cutting concerns

- パスワードハッシュ: bcrypt、コストは `bcrypt.DefaultCost`(10)。**bcrypt は入力を72バイトまでしか扱えず、超過分は `bcrypt.GenerateFromPassword` がエラーを返す**(サイレント切り詰めではない)ため、bcrypt に渡す前にパスワードを SHA-256 でプリハッシュする。**プリハッシュの出力は生バイト列ではなく16進数(hex)文字列にエンコードしてから bcrypt に渡す**(前回レビューで指摘: SHA-256 の生バイト列には約1/256の確率で `0x00` バイトが含まれ、C 文字列前提の bcrypt 実装(他言語・他ツールでの再検証時など)は NUL 以降を切り詰めて別のハッシュとして扱いうる。hex エンコードならこの曖昧さが原理的に発生しない)。パラメータチューニングの意思決定が不要で、単一管理者規模では十分。
- `go:embed` のパス制約(§2 参照): SPA ビルド出力は `internal/adminui/dist/` に直接配置する。プレースホルダ `index.html` を commit し、Node/Vite なしでも `go build ./...` が通るようにする(実ビルドで置き換わる)。
- ロギング: `log/slog` で標準エラー出力、フレームワークなし。
- テーマ/アセットの静的配信(`GET /assets/`)には path-traversal 対策(`filepath.Clean` + プレフィックスチェック)を最初から入れる — Phase 2 でテーマ ZIP アップロードを受け付ける際に同じガードが必要になるため。アクティブテーマ切り替え(ディスク上書き vs 埋め込み)時も同じガード関数を共有する。
- セッション: `expires_at` を過ぎた行は積極的な掃除ジョブを作らず、認証ミドルウェアでの参照時に期限切れなら 401 として扱う「lazy expiry」でよい(単一管理者規模では行数が問題にならない)。ただし `idx_sessions_expires` インデックスは将来の一括削除に備えて用意しておく。
- ログイン試行のレート制限は Phase 1 では実装しない(単一管理者・低頻度アクセス)。ただし平文 HTTP(§10確認事項1)上でパスワードを送信する構成である旨は運用上のトレードオフとして明記する — 本番運用者はリバースプロキシで TLS 終端するか、Phase 2 の ACME 実装を待つことを推奨する。

## 8. Test strategy

**結合テスト**(`main_test.go`, ブラックボックス): 実バイナリを一度ビルドし、一時ディレクトリ + 空きポートでサブプロセス起動。起動ログの「listening on ...」行と、標準出力に出力される setup token を読み取る(sleep ベースのポーリングより決定的)。カバレッジ計測には `go build -cover` + `GOCOVERDIR` を使い、サブプロセスとして起動したバイナリのカバレッジも CI ゲートに含める。

一連のシナリオをモックなしで検証(旧版から拡張):

1. `POST /api/setup`(正しい setup token、`site_name` 込み)→ 201。誤ったトークンで再試行 → 401。正しいトークンで再実行 → 401(2回目はトークンが再発行されないため未一致)。
2. `POST /api/session` → 204 + cookie。誤ったパスワードで → 401。
3. `POST /api/posts`(認証済み、`status: draft`)→ 201、レスポンスに `version: 1` を含むことを確認。
4. **`GET /{slug}`(公開・未認証・draft のまま)→ 404 であることを確認**(旧版で欠落していた最重要ケース — draft が公開されないことの直接的な証拠)。
5. `PUT /api/posts/{id}`(ヘッダー `If-Match: "1"`、`status: published`)→ 200。`version` が `2` になっていることを確認。
6. `GET /{slug}`(公開・未認証)→ 200、レンダリングされた HTML に投稿タイトル/本文と `site_name`(手順1で設定した値)を含む — storage → content → theme → HTTP の一気通貫を検証。
7. 手順3の古い `If-Match: "1"` で再度 `PUT` → 412(楽観的並行性制御の検証)。`If-Match` ヘッダーを省略した `PUT` → 400。
8. `POST /api/posts` で slug を既存投稿と重複させる → 409。予約 slug(`api` 等)で作成 → 400。
9. 不正な `Origin` ヘッダー付きで `POST /api/posts` → 403。`Origin` なしでは通ること。ボディなしの `DELETE /api/session`(`Content-Type` 未設定)が拒否されないこと。
10. `GET /api/posts` → 200、`{"items":[...],"total":n}` の envelope、各要素に `version` を含む。
11. `GET /assets/style.css` → 200(テーマのスタイルシート配信)。**パーセントエンコードされたトラバーサル**(例: `GET /assets/%2e%2e%2f%2e%2e%2fetc%2fpasswd`)→ 400/404 で拒否されること(生の `../` は `ServeMux` 自体が 307 リダイレクトしてしまいガードを経由しないため、テストとして意味を持たない — §5 参照)。
12. `DELETE /api/posts/{id}`(正しい `If-Match`)→ 204、その後 `GET /{slug}` → 404。誤った `If-Match` での `DELETE` → 412。
13. 未認証の `POST /api/posts` → 401。`POST /` (存在しないメソッド) → 405。
14. **マイグレーション冪等性**: 同じ `--data-dir` に対してバイナリを再起動し、2回目の起動でマイグレーションエラーが出ないことを確認(`schema_migrations` の bootstrap ロジックの検証)。
15. `PUT /api/settings`(認証済み、`site_name` 変更)→ 200。以降の `GET /{slug}` に新しい `site_name` が反映されること。

**ユニットテスト**: `content`(slug 衝突、ステータス遷移、`published_at` の初回セットのみルール、`ListPublished`/`GetPublished` が draft を返さないこと)と `theme`(layout + content_for_layout 合成)を、インメモリ SQLite を使い実ドライバで検証する(モック禁止方針に準拠)。

**インメモリ SQLite 使用時の注意**(レビューで指摘): `file::memory:?cache=shared` は同一プロセス内で**同じ名前を使う全ての接続がグローバルに1つのDBを共有する**。テストごとに異なるデータベース名を使わないと、並行実行されるテスト間でテーブル・データが衝突したり、2回目のテストでマイグレーションの `CREATE TABLE` が「既に存在する」エラーになる。テストごとに一意な名前を使うこと: `file:test-<一意なID>?mode=memory&cache=shared`(例: サブテスト名やカウンタから生成)。`SetMaxOpenConns(1)` は本番の書き込みプールと同じ設定にしておき、テストと本番で挙動を揃える。

## 9. Alternatives considered

- **chi/gorilla mux**: 不採用。Go 1.22 標準の `ServeMux` がメソッド+ワイルドカードパターンに対応済みで、この規模では外部ルーターは不要。
- **sqlc/GORM**: 不採用。テーブル5個・クエリ十数個程度の規模では、素の `database/sql` + 簡単なスキャンヘルパーの方がコード量が少ない。
- **フックインターフェース/イベントバスを今作る**: 不採用。実装がゼロの抽象化であり、guardrails の「投機的な一般化を避ける」方針に反する(ADR 0005)。接合点は関数境界のみで十分。
- **golang-migrate/goose**: 不採用(§3参照)。将来の明示的なエスケープハッチとして記録。
- **2つの `*sql.DB` (read/write プール分離)**: 不採用(Phase 1時点)。単一管理者規模では単一プール+`SetMaxOpenConns(1)`で十分。将来の非決定事項として記録。
- **独立した `content.Publish(id)` 関数**(初期案): 不採用。API に対応する公開エンドポイントがなく、ADR 0005 が禁じる「到達経路のない関数」に該当していたため、`Create`/`Update` 内部のロジックに統合した。
- **slug 変更時のリダイレクト/履歴テーブル**: Phase 1 では不採用(§4「既知の制限」参照)。WP 移行が現実的な関心事になる Phase 2 で再検討する。
- **PUT の並行性制御なし(silent last-write-wins)**: 不採用。1カラム(`version`)+ 1ヘッダー(`If-Match`)の追加コストに対し、無言のデータ消失を防げる効果が大きいため、Phase 1 から楽観的並行性制御を採用した。

## 10. 確認済み事項

以下はユーザー確認済みで、本設計に確定事項として反映済み:

1. **デフォルトポート**: `:8080`、`http://` のみ(TLS/ACME は Phase 2)。architecture.md §5.1 には Phase 1 での差分注記を追加済み。平文 HTTP 上でセッション/パスワードを扱う運用上のトレードオフは §7 に明記。
2. **CSRF対策**: Origin ヘッダー(`config.BaseURL` との照合、欠落時は許可)+ Content-Type チェックで Phase 1 として採用(CSRF トークンは導入しない)。

## 11. 監査で発見され本改訂で対応した欠陥の一覧

Opus 5 / Fable による独立した敵対的レビューを**2ラウンド**実施した。トレーサビリティのため両ラウンドの指摘と対応を記録する。

### 1ラウンド目(静的読解によるレビュー)

- 公開ページ(`httpapi`)が draft を返してしまう欠落 → `content.ListPublished`/`GetPublished` を新設し `httpapi` はこれ以外の読み取りを行わない設計に変更(§2, §3)。
- `/api/` 全体を認証ミドルウェアで包み、かつ `/api/setup`/`POST /api/session` を除外するという矛盾したルーティング → `ServeMux` の優先順位を利用した個別登録に変更(§5)。
- `published_at`/`content.Publish` の仕様矛盾(呼び出し経路がない関数) → `Publish` を廃止し `Create`/`Update` 内部ロジックに統合(§3, §9)。
- `schema_migrations` テーブルが「ランナーが最初に読むテーブルをマイグレーションで作る」循環参照になっていた → ランナー自身が `CREATE TABLE IF NOT EXISTS` で作成する方式に変更(§3)。
- `go:embed` が親ディレクトリ(`admin/dist`)を参照できず、ドキュメント記載のディレクトリ構成のままではコンパイルエラーになる → `internal/adminui/dist/` に配置場所を変更(§2, §7)。
- `GET /assets/` のルートが存在せず、テーマのスタイルシートが配信不能だった → ルートを追加(§5, §6)。
- `settings` テーブルに読み書きの経路が一切なく、architecture.md が約束する「サイト名」入力も未実装だった → `content.GetSiteSettings`/`SetSiteSettings` を追加(ただしこの対応自体に不備があった。2ラウンド目参照)。
- `/api/setup` が誰でも到達可能かつ check-then-insert の競合状態だった → setup token 保護 + 単一トランザクション化(§4。ただし担当関数が未指定という不備があった。2ラウンド目参照)。
- slug のバリデーション(予約プレフィックス、文字種)が未規定だった → 明文化(§4)。
- `PUT` の並行更新が無制御(silent last-write-wins)だった → `version` カラム + `If-Match` による楽観的並行性制御を追加(§3, §4)。
- `GET /api/posts` が無制限・envelope なしだった → ページネーション上限とレスポンス envelope を追加(§4)。
- bcrypt の72バイト制限が未考慮だった → SHA-256 プリハッシュを追加(§7。ただしエンコード方法が未指定という不備があった。2ラウンド目参照)。
- `SetMaxOpenConns(1)` によるデッドロックの罠(Tx とプレーン呼び出しの混在)が未文書化だった → 実装ルールとして明記(§3)。
- テスト戦略が draft 非公開・PUT・CSRF拒否・slug衝突・マイグレーション冪等性を検証していなかった → シナリオを追加(§8)。
- `file::memory:?cache=shared` のクロステスト汚染リスクが未考慮だった → テストごとに一意な名前を使う注意を追加(§8)。
- `architecture.md` に ADR 0001/0002 反映前の古い記述(Wasmtime、Turso/S3 自動同期)が残っていた → `architecture.md` 側を修正。

### 2ラウンド目(実コードを書いて実行検証した上でのレビュー。1ラウンド目の修正自体の不備を含む)

- **1ラウンド目の修正で追加した `content.SetSiteSettings` が、追加した本人によって呼び出し元のないまま放置されていた**(ADR 0005 が禁じる「実装のない/到達経路のない関数」を、直した張本人が別の場所で再発させていた)→ `POST /api/setup` に `site_name` を追加し、`content.CreateFirstAdmin` の一部として書き込む経路を作り、運用中の変更用に `PUT /api/settings` も追加(§2, §4, §8)。
- `architecture.md` §5.1 が「サイト名」入力(実装されていなかった)と "5秒セットアップ"(setup token の手動コピーが必要になり実態と乖離)の両面で古いままだった → 該当箇所を修正(別途参照)。
- `GET /assets/../../../etc/passwd` 形式のテストは `ServeMux` 自身のパス正規化・307リダイレクトにより実際にはガードを経由せず、**何も検証しないまま「合格」してしまう**テストだった → パーセントエンコード形式のテストに差し替え(§5, §8)。
- `GET /api/posts` の一覧レスポンスに `version` が含まれておらず、§4 で規定した「SPA は一覧のレスポンスから version を保持する」という前提が満たせなかった → `items` の各要素に `version` を追加(§4)。
- セッション cookie の `Secure` 属性が接続の TLS 状態に依存しており、推奨デプロイ構成(プロキシで TLS 終端、アプリ自体は平文)では常に付かなかった → `config.BaseURL` のスキームで決定する方式に変更。cookie 名・Path・有効期限(7日固定)も明文化(§4)。
- `/api/setup` のアトミック性(check-then-insert)を要求する記述はあったが、それを実装する関数がどのパッケージにも存在しなかった → `content.CreateFirstAdmin` を新設し、担当を明示(§2, §4)。
- `If-Match` を JSON ボディの一部であるかのように記載しており、HTTP ヘッダーであることが分かりにくかった → 表記を修正し、`DELETE` にも同じ楽観的並行性制御を追加(§4, §9)。
- `Content-Type: application/json` を全ての状態変更リクエストに必須としており、ボディを持たない `DELETE` が自己矛盾していた → `POST`/`PUT` のみに適用する形に修正し、ミドルウェアの適用順序(Origin → 認証 → ルーティング)も明記(§4)。
- SHA-256 プリハッシュの出力エンコード(生バイト列 vs hex vs base64)が未指定で、生バイト列だと NUL バイトを含む場合に bcrypt 実装間で挙動が割れうる曖昧さが残っていた → hex エンコードに固定(§7)。
- setup token の比較が定数時間比較かどうか、セットアップ完了後に何と比較するかが未規定だった → `crypto/subtle.ConstantTimeCompare` を明記し、完了後は常にトークン未発行=不一致になる設計であることを明記(§4)。
- `_txlock=immediate` の効用を「`SQLITE_BUSY` の早期検出」と誤って説明していた(`busy_timeout` 併用時はむしろ最大5秒リトライする) → 正しい効用(deferred→write のロック昇格失敗の回避、別プロセスとの競合時の挙動)に修正(§4)。
- テーマのディスク上書きパス・DB ファイルパスがどちらも `--data-dir` に紐付いておらず、CWD 相対だった → `config.DataDir` を基準に解決する方式に統一(§3, §6)。
- 一覧系エンドポイントに `ORDER BY` が一切なく、ページ跨ぎで行が重複/欠落しうる非決定的なページネーションだった。加えて公開一覧(`ListPublished`)には件数上限が一切なかった → 安定ソート(`ORDER BY published_at DESC, id DESC`)と、公開一覧側の内部上限(100件)を追加(§4, §6)。
- `username`/`password` のバリデーションルールが規定されておらず、1文字のパスワードも仕様上許容されていた → 明文化(§4)。
- `POST`/非 GET メソッドへの `httpapi` の応答(405 を返すべきか)が未規定だった → 明記(§5, §8)。
- `architecture.md` の技術スタック表に ADR 0003/0004 へのリンクがなく、フロントエンドの記述が「TanStack (or Svelte)」のまま ADR 0004 の決定と矛盾していた。また「軽量 WAF を内蔵」という、どの ADR・ロードマップにも対応しない製品上の主張が残っていた → `architecture.md` 側を修正(別途参照)。
