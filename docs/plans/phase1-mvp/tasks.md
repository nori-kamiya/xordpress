# XordPress Phase 1 (MVP) — 実装タスク一覧

> 設計書: [docs/plans/phase1-mvp/design.md](design.md)(2ラウンドの敵対的レビュー済み、確定として扱う)。
> このタスク一覧は設計の決定を再検討しない。実装可能・独立検証可能な単位に分解するのみ。

## 設計書に残っていた未指定の1点(分解時に補完)

設計書は `adminapi` が `content` と `auth` を import してよいとし、ログイン(`POST /api/session`)の実装を要求しているが、「ユーザー名からユーザーを引き当ててパスワードを検証する」処理をどちらのパッケージが持つかは明記していなかった。`content` パッケージの公開面は投稿・サイト設定・`CreateFirstAdmin` のみで、認証情報の検証はドメインの関心事というより認証の関心事に近いため、**`auth.Authenticate(ctx, username, password) (userID int64, ok bool, err error)` を `auth` パッケージに追加する**(`auth` は境界表で `storage` の import が許可されているため、直接 `storage.GetUserByUsername` を呼べる)。これが唯一、設計書に対して行った補完であり、他は全て設計書の記述通り。

## 依存関係と並列実行可能なグループ

```
T1 (scaffolding)
 ├─ T2 (config)
 │   └─ T3 (storage: migration runner)
 │       ├─ T4 (storage: posts repo) ─────────┐
 │       ├─ T5 (storage: users/sessions/settings repo) ─┤
 │       │   └─ T6 (storage: CreateFirstAdmin atomic tx) ┤
 │       │                                                ├─ T9 (content: post lifecycle)
 │       │                                                │   └─ T10 (content: ListPublished/GetPublished)
 │       │                                                └─ T11 (content: settings + CreateFirstAdmin wrapper)
 │       │
 │       ├─ T12 (auth: password hashing) ── 独立(storage 不要)
 │       ├─ T15 (auth: setup token) ────── 独立(storage 不要)
 │       ├─ T13 (auth: session issue/validate) [depends: T5, T12]
 │       │   └─ T13b (auth: Authenticate) [depends: T5, T12]
 │       │       └─ T14 (auth: middleware) [depends: T13]
 │       │
 │       └─ T7 (theme: 2パスレンダラ+スターターテーマ) ── 独立(storage 不要)
 │           └─ T8 (theme: assets path-traversal ガード)
 │
 ├─ T21 (adminui: embed + SPA ハンドラ) ── 独立
 │
 └─ (T9,T10,T11,T12,T13b,T15 が揃った後)
     T16 (adminapi: setup) [T11,T12,T15]
     T17 (adminapi: session) [T13,T13b,T14]
     T18 (adminapi: posts CRUD) [T9,T14]
         └─ T19 (adminapi: settings + 内部ルーティング) [T11,T16,T17,T18]

     T20 (httpapi: 公開サイト + assets) [T10,T7,T8]

     T22 (app: 配線) [T3,T16,T17,T18,T19,T20,T21,T14,T15,T2]
         └─ T23 (main.go)
             └─ T24 (結合テスト — §8 全シナリオ)
```

**並列化の目安**: T4/T5 は T3 完了後に並行可。T7(theme)・T12/T15(auth の一部)は T3 系列と完全に独立なので最初から並行可。T21(adminui)も独立。T16〜T20 は各々の依存が揃い次第並行可。T22 以降は直列(composition root → entrypoint → E2Eテスト)。

**意図的にタスク化していないもの(Phase 2 以降のスコープ、design.md 参照)**: Wasm プラグイン実行、D&D ブロックエディタ、WP インポーター、ACME/自動SSL、slug 変更時のリダイレクト/履歴、read専用DBプール分離、goose/golang-migrate 導入。

---

### T1: リポジトリ scaffolding
- depends on: なし
- files: `go.mod`(create), `main.go`(create, スタブ), `internal/adminui/dist/index.html`(create, プレースホルダ), `internal/adminui/embed.go`(create)
- what: `go.mod` を `module github.com/nori-kamiya/xordpress`، `go 1.22` で作成。`main.go` は `func main() {}` のスタブ。`internal/adminui/dist/index.html` に最小のプレースホルダ HTML を置き、`//go:embed dist` が空ディレクトリでコンパイルエラーにならないようにする(design.md §2, §7)。
- verify: `go build ./...` → エラーなく成功
- executor: haiku

### T2: config パッケージ
- depends on: T1
- files: `internal/config/config.go`(create)
- what: `Config{Addr, DataDir, BaseURL string}` 構造体と、既定値(`Addr=":8080"`, `DataDir="."`, `BaseURL="http://localhost:8080"`)を返すコンストラクタ。flag 解析自体は T23(main.go)側で行う(config パッケージは構造体定義のみ)。
- verify: `go test ./internal/config/...` → 既定値のテストが green
- executor: sonnet

### T3: storage — DB接続 + マイグレーションランナー
- depends on: T2
- files: `internal/storage/sqlite.go`(create), `internal/storage/migrate.go`(create), `internal/storage/migrations/embed.go`(create), `internal/storage/migrations/0001_init.sql`(create)
- what: `Open(dataDir string) (*sql.DB, error)` — design.md §3 で検証済みの DSN(`_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate`)を `filepath.Join(dataDir,"data","xordpress.db")` に対して使用し、`os.MkdirAll` でディレクトリを用意し、`SetMaxOpenConns(1)` を設定する。マイグレーションランナーは起動時に `CREATE TABLE IF NOT EXISTS schema_migrations(...)` を自前で実行してから、埋め込み `.sql` を番号順に1ファイル=1トランザクションで適用する(§3「マイグレーション方式」)。`0001_init.sql` は `users`/`posts`/`sessions`/`settings` テーブルのみを含み、`schema_migrations` 自体は含めない(循環回避)。
- verify: `go test ./internal/storage/...` → (a) `t.TempDir()` に対して `Open`+`Migrate` で全テーブルが作成されることを確認するテスト、(b) 同じ DataDir に対して `Migrate` を2回実行してもエラーにならない(再起動の冪等性、§8 シナリオ14の前提)テストが green
- executor: sonnet

### T4: storage — posts リポジトリ
- depends on: T3
- files: `internal/storage/posts.go`(create)
- what: `InsertPost`, `GetPostByID`, `GetPostBySlug`, `ListPosts(filter)`(status問わず全件、`ORDER BY published_at DESC, id DESC`)、`UpdatePost(id, expectedVersion, fields)`(`WHERE id=? AND version=?` で更新し、影響行数0なら `ErrVersionMismatch`)、`DeletePost(id, expectedVersion)`(同様)、`ListPublished(limit)`(`WHERE status='published' ORDER BY published_at DESC, id DESC LIMIT ?`)、`GetPublishedBySlug(slug)`。slug 衝突は `INSERT`/`UPDATE` のエラーを `*sqlite.Error` に型アサーションし `Code()==2067` を `ErrSlugConflict` にマッピングするヘルパーを含む。
- verify: `go test ./internal/storage/...` → CRUD の往復、slug 重複時に `ErrSlugConflict`、version 不一致時に `ErrVersionMismatch` を返すことを確認するテストが green
- executor: sonnet

### T5: storage — users / sessions / settings リポジトリ
- depends on: T3
- files: `internal/storage/users.go`(create), `internal/storage/sessions.go`(create), `internal/storage/settings.go`(create)
- what: `CountUsers`, `GetUserByUsername`(auth.Authenticate から使用); `InsertSession(tokenHash, userID, expiresAt)`, `GetSessionByTokenHash`, `DeleteSession(tokenHash)`; `GetSetting(key)`, `SetSetting(key, value)`(UPSERT)。
- verify: `go test ./internal/storage/...` → 各リポジトリの CRUD が green
- executor: sonnet

### T6: storage — CreateFirstAdmin アトミックトランザクション
- depends on: T4, T5
- files: `internal/storage/setup.go`(create)
- what: `CreateFirstAdmin(ctx, username, passwordHash, siteName) (created bool, err error)` — 単一の `*sql.Tx`(`db.BeginTx`〜`tx.Commit`)内で `SELECT count(*) FROM users` → 0件なら `users` へ INSERT + `settings(key='site_name')` へ INSERT → commit・`created=true`。1件以上なら rollback・`created=false`。design.md §4 の「アトミック性の担当者不在」指摘への対応。
- verify: `go test ./internal/storage/...` → (a) 通常系で `created=true` とその後の `CountUsers()==1` を確認、(b) 2つのゴルーチンから同時に呼び、`SetMaxOpenConns(1)` の直列化により正確に1つだけが `created=true` を返すことを確認する並行性テストが green
- executor: sonnet

### T7: theme — Liquid 2パスレンダラ + スターターテーマ
- depends on: T1
- files: `internal/theme/theme.go`(create), `internal/theme/themes/default/layout/theme.liquid`(create), `internal/theme/themes/default/templates/index.liquid`(create), `internal/theme/themes/default/templates/post.liquid`(create), `internal/theme/themes/default/assets/style.css`(create)
- what: `Render(ctx, tmplName string, data any) (string, error)` — design.md §6 の2パス方式(body テンプレートを `engine.ParseAndRenderString` でレンダリング→文字列を `content_for_layout` として layout に束縛)。テーマ検索順序は `filepath.Join(dataDir,"themes","default")`(存在すれば優先)→埋め込み `embed.FS`(`fs.Sub`)フォールバック。`storage` は import しない。
- verify: `go test ./internal/theme/...` → index/post テンプレートが layout に正しく合成されること、`site.name` バインディングが `<title>` に反映されることを確認するテストが green
- executor: sonnet

### T8: theme — assets 配信 + path-traversal ガード
- depends on: T7
- files: `internal/theme/assets.go`(create)
- what: アクティブテーマルート配下のファイルパスを `filepath.Clean` + プレフィックスチェックで検証し、範囲外なら拒否する `ResolveAsset(name string) (fs.File, error)`。ディスク上書き/埋め込みの両方に同じガードを適用。
- verify: `go test ./internal/theme/...` → 正規のアセット(`style.css`)は取得できる、パーセントエンコードされたトラバーサル文字列(`%2e%2e%2f...`)を含む名前は拒否されることを確認するテストが green
- executor: sonnet

### T9: content — 投稿ライフサイクル(Create/Update/Delete/List/Get)
- depends on: T4
- files: `internal/content/post.go`(create)
- what: ドメイン `Post` 構造体と `Create/Update/Delete/List/Get` — `storage.posts` をラップする。`published_at` は「`draft`→`published` に遷移し、かつ現在 NULL の場合にのみ現在時刻をセット」する共通ヘルパーに一本化(design.md §3)。フォーマット系バリデーション(文字数・正規表現など)は `adminapi` 側(T18)の責務とし、`content` は `ErrSlugConflict`/`ErrNotFound`/`ErrVersionMismatch` 等の型付きエラーを `storage` から透過的に伝播させる。
- verify: `go test ./internal/content/...` → slug 衝突時に型付きエラーが返る、`published_at` が初回のみセットされる、`draft` に戻しても `published_at` が消えないことを確認するテストが green
- executor: sonnet

### T10: content — ListPublished / GetPublished(公開読み取り専用)
- depends on: T9
- files: `internal/content/public.go`(create)
- what: `ListPublished() ([]Post, error)`(内部で `LIMIT 100` を固定)/ `GetPublished(slug string) (Post, error)` — `storage.ListPublished`/`GetPublishedBySlug` をラップし、`status='published'` 以外を一切返さない。
- verify: `go test ./internal/content/...` → draft 1件・published 1件を作成し、`ListPublished`/`GetPublished` が published のみを返す(draft を返さない)ことを直接アサートするテストが green — これが「公開ページが draft を漏らす」バグの回帰テスト
- executor: sonnet

### T11: content — サイト設定 + CreateFirstAdmin ラッパー
- depends on: T6, T9
- files: `internal/content/settings.go`(create), `internal/content/setup.go`(create)
- what: `GetSiteSettings() (SiteSettings, error)` / `SetSiteSettings(name string) error` — `storage.settings` をラップ。`CreateFirstAdmin(ctx, username, passwordHash, siteName) (bool, error)` — `storage.CreateFirstAdmin` への薄いパススルー。
- verify: `go test ./internal/content/...` → 設定の読み書き往復、`CreateFirstAdmin` の成功/重複ケースが green
- executor: sonnet

### T12: auth — パスワードハッシュ
- depends on: T1
- files: `internal/auth/password.go`(create)
- what: `HashPassword(plain string) (string, error)` — SHA-256 でプリハッシュした結果を **16進数文字列にエンコードしてから** `bcrypt.GenerateFromPassword`(コスト `bcrypt.DefaultCost`)にかける。`VerifyPassword(hash, plain string) bool`。
- verify: `go test ./internal/auth/...` → hash/verify の往復が成功、72バイトを超える平文パスワードでもエラーにならない(hex エンコード後は常に64文字であるため)ことを確認するテストが green
- executor: sonnet

### T13: auth — セッション発行/検証
- depends on: T5, T12
- files: `internal/auth/session.go`(create)
- what: `IssueSession(ctx, baseURL, userID) (*http.Cookie, error)` — `crypto/rand` でトークン生成、SHA-256 ハッシュを `storage.sessions` に保存、cookie を design.md §4 の仕様通りに構築(`xordpress_session`, `Path=/`, `HttpOnly`, `SameSite=Lax`, `Secure` は `baseURL` のスキームで決定, 有効期限7日)。`ValidateSession(ctx, token string) (userID int64, ok bool)` — ハッシュ照合 + `expires_at` の lazy expiry チェック。
- verify: `go test ./internal/auth/...` → 発行→検証の往復が成功、期限切れセッション(手動で過去の `expires_at` を挿入)は失敗、誤ったトークンは失敗することを確認するテストが green
- executor: sonnet

### T13b: auth — Authenticate(ユーザー名+パスワード照合)
- depends on: T5, T12
- files: `internal/auth/authenticate.go`(create)
- what: 設計書に明記されていなかったログインの認証ロジックの担当箇所(本タスク一覧の冒頭「未指定の1点」参照)。`Authenticate(ctx, username, password string) (userID int64, ok bool, err error)` — `storage.GetUserByUsername` でユーザーを引き、`VerifyPassword` で照合する。
- verify: `go test ./internal/auth/...` → 正しい資格情報で成功、存在しないユーザー名・誤ったパスワードで `ok=false` を返すテストが green
- executor: sonnet

### T14: auth — HTTP ミドルウェア(認証 + CSRF/Origin + Content-Type)
- depends on: T13
- files: `internal/auth/middleware.go`(create)
- what: `AuthMiddleware(next http.Handler) http.Handler` — cookie 抽出 → `ValidateSession` → 失敗時 401。`CSRFMiddleware(baseURL string, next http.Handler) http.Handler` — `Origin` ヘッダーが存在し `baseURL` と不一致なら 403(認証の成否によらず先に評価)、`POST`/`PUT` に限り `Content-Type: application/json` を要求(`DELETE` は対象外、design.md §4)。
- verify: `go test ./internal/auth/...` → `httptest` で正常系、期限切れ/不正 cookie で401、`Origin` 不一致で403(未認証でも403が優先されること)、`DELETE`(Content-Type未設定)が通ることを確認するテストが green
- executor: sonnet

### T15: auth — setup token 生成 + 定数時間比較
- depends on: T1
- files: `internal/auth/setuptoken.go`(create)
- what: `GenerateSetupToken() (string, error)` — `crypto/rand` で32バイト以上、Base64 エンコード。`CompareSetupToken(expected, provided string) bool` — `crypto/subtle.ConstantTimeCompare` を使用(`expected` が空文字列なら常に `false` を返す — セットアップ完了後の安全側動作)。
- verify: `go test ./internal/auth/...` → 生成したトークンの往復一致、不一致時に `false`、`expected=""` の場合に常に `false` を返すことを確認するテストが green
- executor: sonnet

### T16: adminapi — setup ハンドラ
- depends on: T11, T12, T15
- files: `internal/adminapi/setup.go`(create)
- what: `Setup(content, auth, setupToken string) http.Handler` — `POST /api/setup`。`Authorization: Bearer` を `CompareSetupToken` で検証(不一致→401)。ボディの `username`(3〜32文字・英数字/ハイフン/アンダースコア)、`password`(8文字以上)、`site_name`(1〜100文字)をバリデーション(不備→400)。`auth.HashPassword` でハッシュ化後 `content.CreateFirstAdmin` を呼び、`created=false` なら 409、成功なら 201。
- verify: `go test ./internal/adminapi/...` → `httptest` で誤トークン401・バリデーション不備400・正常系201・二重セットアップ409(トークン再利用のケースも含む)が green
- executor: sonnet

### T17: adminapi — session(ログイン/ログアウト)ハンドラ
- depends on: T13, T13b, T14
- files: `internal/adminapi/session.go`(create)
- what: `Login(content, auth) http.Handler` — `POST /api/session`。`auth.Authenticate` で照合、成功なら `auth.IssueSession` で cookie を発行し204、失敗なら401。`Logout(auth) http.Handler` — `DELETE /api/session`(要認証)。cookie とセッション行を削除し204。
- verify: `go test ./internal/adminapi/...` → 正しい資格情報で204+Set-Cookie、誤ったパスワードで401、ログアウトで cookie が失効することを確認するテストが green
- executor: sonnet

### T18: adminapi — posts CRUD ハンドラ
- depends on: T9, T14
- files: `internal/adminapi/posts.go`(create)
- what: `GET/POST /api/posts`, `GET/PUT/DELETE /api/posts/{id}` を design.md §4 の契約通りに実装。フォーマットバリデーション(title/body/slug正規表現・予約語・status enum)、`http.MaxBytesReader`(2MB, 超過413)、`If-Match` ヘッダーの必須チェック(欠落/パース不能→400)と `content` の `ErrVersionMismatch`→412・`ErrSlugConflict`→409・`ErrNotFound`→404 へのマッピング、一覧のページネーション(`limit`既定20上限100・`offset`既定0、不正値400)と envelope(`{"items":[...],"total":n}`、各要素に`version`含む)。
- verify: `go test ./internal/adminapi/...` → create→list(version含む envelope)→get→update(正しい/誤った If-Match)→delete の一気通貫、予約slug 400、slug衝突409、非数値id 400 が green
- executor: sonnet

### T19: adminapi — settings ハンドラ + 内部ルーティング組み立て
- depends on: T11, T16, T17, T18
- files: `internal/adminapi/settings.go`(create), `internal/adminapi/handler.go`(create)
- what: `PUT /api/settings`(要認証、`site_name` 更新、200/400)。`Handler(content, auth) *http.ServeMux` — T16〜T18・本タスクの各ハンドラを完全パス+メソッドのパターン(`"GET /api/posts"` 等、design.md §4)で内部 `ServeMux` に登録する(この関数自体は `/api/setup`・`POST /api/session` を登録しない — それらは §5 でトップレベル mux に個別登録される)。
- verify: `go test ./internal/adminapi/...` → 各パターンへのディスパッチ、未知パスへの404が green
- executor: sonnet

### T20: httpapi — 公開サイトハンドラ + assets ルート
- depends on: T10, T7, T8
- files: `internal/httpapi/handler.go`(create), `internal/httpapi/assets.go`(create)
- what: `Handler(content, theme) http.Handler` — `GET /` は `content.ListPublished()`→`theme.Render("index",...)`、`GET /{slug}` は `content.GetPublished(slug)`→ 見つからなければ404、見つかれば `theme.Render("post",...)`。`GET`/`HEAD` 以外は405。`AssetsHandler(theme) http.Handler` — `GET /assets/` を `theme.ResolveAsset` 経由で配信。
- verify: `go test ./internal/httpapi/...` → **draft は `GET /{slug}` で404になる(回帰テスト)**、published は200でタイトル/本文/site_name を含む、存在しないslugは404、`POST /`は405、パーセントエンコードされたトラバーサルは拒否されることを確認するテストが green
- executor: sonnet

### T21: adminui — embed + SPA 静的配信ハンドラ
- depends on: T1
- files: `internal/adminui/handler.go`(create)
- what: `Handler() http.Handler` — `dist/` を静的配信し、ファイルが見つからないサブパスは `index.html` にフォールバック(SPA クライアントルーティング対応)。`/admin`(末尾スラッシュなし)は `/admin/` へリダイレクト。
- verify: `go test ./internal/adminui/...` → `GET /admin/` でプレースホルダ index.html が返る、存在しないサブパスでも200(フォールバック)、`GET /admin` がリダイレクトされることを確認するテストが green
- executor: sonnet

### T22: app — composition root(配線)
- depends on: T2, T3, T14, T15, T16, T17, T18, T19, T20, T21
- files: `internal/app/app.go`(create)
- what: `Run(ctx context.Context, cfg config.Config) error` — `storage.Open`+マイグレーション実行 → `content`/`auth`/`theme` を構築 → `users` が0件なら `auth.GenerateSetupToken` を生成し標準出力に `setup token: <token>` を一度だけ出力 → design.md §5 の mux をそっくりそのまま構成(`POST /api/setup`・`POST /api/session` を個別登録 → `csrfMiddleware`+`authMiddleware`でラップした `/api/` → `/admin/` → `GET /assets/` → `/`)→ `http.Server` を起動し `listening on <addr>` を標準出力に出力 → `ctx.Done()` で `Shutdown`。
- verify: `go build ./...` が成功。`internal/app` のテストで `Run` を goroutine 内で起動(`t.TempDir()` 使用)→ `GET /` が200を返すことを確認 → `ctx` をキャンセルしてシャットダウンが完了することを確認するテストが green
- executor: sonnet

### T23: main.go — エントリポイント
- depends on: T22
- files: `main.go`(modify — T1のスタブを実装に差し替え)
- what: `--addr`/`--data-dir`/`--base-url` フラグを解析して `config.Config` を構築し、`signal.NotifyContext`(`SIGINT`/`SIGTERM`)で `ctx` を作り `app.Run(ctx, cfg)` を呼ぶ。
- verify: `go build ./...` → バイナリが生成される。`go run . --addr :0 --data-dir <tmp>` を起動し、`Ctrl-C` 相当のシグナルで正常終了することを手動確認(自動テストは T24 の結合テストでカバー)
- executor: haiku

### T24: 結合テスト(§8 全シナリオ、ブラックボックス)
- depends on: T22, T23
- files: `main_test.go`(create、または `internal/integration/integration_test.go`)
- what: 実バイナリを一度ビルドし、`t.TempDir()` + 空きポートでサブプロセス起動。標準出力の `listening on ...` 行と `setup token: ...` 行を読み取る。design.md §8 の15シナリオ全て(setup+site_name、ログイン、draft非公開、CRUD+楽観的並行性制御(If-Match の200/412/400)、slug衝突・予約語400、CSRF Origin 403、Content-Type/DELETE整合性、一覧envelope+version、assets配信+パーセントエンコードトラバーサル拒否、401/405、マイグレーション冪等性(バイナリ再起動)、`PUT /api/settings`)を実装する。カバレッジは `go build -cover` + `GOCOVERDIR` で計測する。
- verify: `go test ./... -run TestIntegration -v` → 全シナリオ green。`go test -cover ./...` がカバレッジレポートを出力する
- executor: main

---

## 最初に着手するタスク

**T1**(scaffolding)から始め、T1 完了後は T2→T3 の直列進行と並行して T7(theme)・T12/T15(auth の一部)・T21(adminui)を並行着手できる。最もリスクが高い(=最初に潰すべき)のは **T3**(マイグレーションランナー、循環参照バグの再発防止)と **T6**(CreateFirstAdmin のアトミック性、並行性テスト)——設計レビューで2度指摘された箇所であり、ここが壊れると後続の全タスクの土台が揺らぐ。

T1 の `/handoff` を作成する場合はお申し付けください。
