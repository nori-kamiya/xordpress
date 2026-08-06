# 管理画面フロントエンド(admin SPA)設計

> 前提: [ADR 0004](../../adr/0004-tanstack-react-for-admin-frontend.md)(TanStack React + Vite + go:embed)は決定済みとして扱う。
> スコープ: [docs/architecture.md](../../architecture.md) Phase 1「基本管理画面(記事作成・一覧)」。D&D ビジュアルブロックエディタ・自動SSL・プラグインはPhase 2/3スコープで対象外。
> バックエンドAPIは [docs/plans/phase1-mvp/design.md](../phase1-mvp/design.md) §4 で確定済み。本書はその上に乗るフロントエンドのみを設計する。ユーザー確認済みの決定は「決定」節に明記。

## 決定(ユーザー確認済み)

1. **`GET /api/settings` を新規追加する。** 設定画面が現在の `site_name` を表示するために必要(既存 `content.GetSiteSettings` をそのまま再利用でき、モデル変更なし)。これは本設計における唯一のバックエンドAPI変更。
2. **初回起動時のログイン/セットアップ判定はフロントエンドのみで解決する。** 新規の `GET /api/setup/status` 的なエンドポイントは追加しない。ルートは常に `/admin/login` にリダイレクトし、ログイン画面に「初回セットアップはこちら」リンクを置く。セットアップフォームが `409`(既にセットアップ済み)を受け取ったら「セットアップ済みです。ログインしてください」とインラインで案内する。

## 1. Overview

Vite + React 18 + TypeScript(strict)で構築するSPAを `web/` に新規作成し、ビルド出力を `internal/adminui/dist/` に直接吐き出す(既存の `go:embed dist` 受け皿・SPAフォールバック・`/admin`→`/admin/`リダイレクトはT21で実装済み、変更不要)。ルーティングは TanStack Router(コード定義、ファイルベースではなく明示的なルートツリー)。データ取得・キャッシュは TanStack Query。認証はCookieベース(既存の `xordpress_session` セッションクッキー)で、SPA側は特別なトークン管理を持たない。画面は 設定(初回セットアップ)・ログイン・投稿一覧・投稿作成・投稿編集・サイト設定 の6画面。

## 2. Architecture

```
web/                          (新規, リポジトリルート直下 — Goモジュール外)
├── src/
│   ├── main.tsx              エントリポイント、QueryClientProvider + RouterProvider
│   ├── router.tsx            createRouter({basepath:'/admin', ...}) — ルートツリーの組み立て、authGuard の登録
│   ├── routes/
│   │   ├── root.tsx          ルートレイアウト(ヘッダー、ログアウトボタン)
│   │   ├── setup.tsx         /admin/setup
│   │   ├── login.tsx         /admin/login
│   │   ├── posts.index.tsx   /admin/posts (一覧)
│   │   ├── posts.new.tsx     /admin/posts/new
│   │   ├── posts.edit.tsx    /admin/posts/$postId/edit
│   │   └── settings.tsx      /admin/settings
│   ├── api/
│   │   ├── client.ts         apiFetch() 共通ラッパー、ApiError型、401検知→リダイレクト
│   │   ├── posts.ts          投稿系クエリ/ミューテーション定義
│   │   ├── session.ts        login/logout
│   │   ├── setup.ts          setup
│   │   └── settings.ts       GET/PUT /api/settings
│   └── types.ts              Post, PostSummary, PostListResponse 等(バックエンドJSON形状の型)
├── index.html
├── vite.config.ts            build.outDir=../internal/adminui/dist, dev proxy
├── tsconfig.json             strict, noUncheckedIndexedAccess
└── package.json

internal/adminui/
├── embed.go                  変更なし(//go:embed dist)
├── handler.go                変更なし(SPAフォールバック・リダイレクト実装済み)
└── dist/                     Viteビルド出力(コミット対象、既存プレースホルダを上書き)

internal/adminapi/
└── settings.go                変更: GetSettings ハンドラを追加
└── handler.go                 変更: "GET /api/settings" を1行追加登録
```

データフロー: ブラウザ → (同一オリジン) `/admin/*` は `adminui.Handler()` が静的配信 → SPA内のfetchは `/api/*` へ同一オリジンで直接リクエスト → 既存の `CSRFMiddleware`→`AuthMiddleware`→`adminapi.Handler` を通過。開発時のみ Vite dev server (`:5173`) から Go バックエンド(`:8099` 等)へのプロキシを挟む(§9)。

新規/変更ファイルのみ: `web/` 一式(新規)、`internal/adminui/dist/*`(新規ビルド出力に置換)、`internal/adminapi/settings.go`(`GetSettings` 追加)、`internal/adminapi/handler.go`(1行追加)、`internal/adminapi/settings_test.go`(追加テスト)。既存の `handler.go`(adminui)・`embed.go`・他のadminapiファイルは無変更。

## 3. データモデル

新規モデルなし。既存 `content.GetSiteSettings(db) (SiteSettings, error)`(`internal/content/settings.go`)をそのまま再利用する。マイグレーション不要。

## 4. API契約

### 4.1 新規: `GET /api/settings`

`internal/adminapi/settings.go` に追加する `GetSettings` ハンドラ。`internal/adminapi/handler.go` の `Handler()` に `mux.Handle("GET /api/settings", GetSettings(contentDB))` を1行追加(既存の `PUT /api/settings` 登録の隣)。認証は既存同様、外側の `AuthMiddleware` に委譲(ハンドラ自身は再チェックしない — `UpdateSettings` と同じ方針)。

```go
func GetSettings(contentDB *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settings, err := content.GetSiteSettings(contentDB)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, settingsResponse{SiteName: settings.Name})
	})
}
```

- レスポンス: `200 {"site_name": "..."}`(`content.GetSiteSettings` はセットアップ前は空文字を返す仕様 — 既存実装済み。管理画面は基本セットアップ後にのみアクセスされるため空文字ケースは事実上発生しないが、フロントエンドは空文字を「未設定」として素直に空欄表示すればよく、特別分岐は不要)。
- エラー: `500 {"error":"internal error"}`。
- テスト: `internal/adminapi/settings_test.go` に `TestGetSettings`(値が設定済みなら反映、未設定なら空文字)を追加。

### 4.2 既存API(変更なし、フロントエンドが呼ぶ契約として明記)

以下はすべて `internal/adminapi/*.go` に実装済み。フロントエンドはこの契約に**厳密に**従う(推測・拡張しない)。

| メソッド+パス | 認証 | リクエスト | 成功 | 主な失敗 |
|---|---|---|---|---|
| `POST /api/setup` | `Authorization: Bearer <token>` | `{username,password,site_name}` | `201` 空body | `401` token不一致 / `400` 検証エラー / `409` 既にセットアップ済み |
| `POST /api/session` | 不要 | `{username,password}` | `204` + `Set-Cookie` | `401` 認証失敗 / `400` |
| `DELETE /api/session` | Cookie | 空 | `204` + クッキー失効 | `401` |
| `GET /api/posts` | Cookie | `?status=&limit=&offset=` | `200 {"items":[...],"total":n}` (`items`は`postSummary`: id,slug,title,status,version,updated_at) | `400` limit/offset不正 |
| `POST /api/posts` | Cookie | `{title,slug,body,status}` | `201` フル`postResponse` | `400` 検証 / `409` slug衝突 / `413` body過大 |
| `GET /api/posts/{id}` | Cookie | — | `200` フル`postResponse` | `404` |
| `PUT /api/posts/{id}` | Cookie + `If-Match: "<version>"` | `{title,slug,body,status}` | `200` フル`postResponse` | `400` If-Match不正 / `404` / `409` slug衝突 / `412` version不一致 |
| `DELETE /api/posts/{id}` | Cookie + `If-Match: "<version>"` | — | `204` | `400` If-Match不正 / `404` / `412` version不一致 |
| `PUT /api/settings` | Cookie | `{site_name}` | `200 {"site_name":...}` | `400` 検証 |

`postResponse` フル形状: `{id,slug,title,body,status,version,author_id,created_at,updated_at,published_at}`(`published_at`はnull許容)。

### 4.3 エラーレスポンスの不整合(フロントエンドが吸収する)

調査で確認済みの実際の不整合 — **バックエンドは変更せず、フロントエンドの `apiFetch` が両方を正規化する**:

- ハンドラ内部からの4xx/5xx(`writeError`経由): `Content-Type: application/json`, body `{"error":"message"}`。
- `AuthMiddleware`(401)・`CSRFMiddleware`(403/400)からの拒否: `http.Error` 経由の**プレーンテキスト**(`"unauthorized\n"` 等)、`Content-Type: text/plain`。

`apiFetch`(`web/src/api/client.ts`)は `Content-Type` を見て分岐し、いずれの場合も統一した `ApiError{status, message}` に正規化する。バックエンド側のこの不整合自体は既知の技術的負債として本書では修正しない(修正するなら別タスク・別レビューが必要な `internal/auth/middleware.go` の変更になるため、フロントエンド専任の本タスクのスコープ外)。

## 5. 主要フロー

### 5.1 初回セットアップ → ログイン(happy path)

1. 運用者がバイナリを起動し、標準出力の `setup token: ...` をコピー。
2. ブラウザで `/admin/` → `/admin/login` にリダイレクト。「初回セットアップはこちら」リンクをクリック → `/admin/setup`。
3. トークン・username・password・site_name を入力して送信 → `POST /api/setup`(Authorizationヘッダーにトークン)。
4. `201` → `/admin/login?setup=done` にナビゲート、成功バナー表示。
5. ログイン → `POST /api/session` → `204` → TanStack Query の投稿一覧クエリを `invalidateQueries` してから `/admin/posts` にナビゲート。

### 5.2 認証切れ(セッションCookie期限切れ)でAPI呼び出しが401

1. `/admin/posts/$postId/edit` で編集中に7日超過(または別端末でログアウト)によりセッション失効。
2. `PUT /api/posts/{id}` が401(プレーンテキスト)を返す。
3. `apiFetch` が401を検知 → `queryClient.clear()` → `router.navigate({to:'/admin/login', search:{redirect: 現在のパス}})`。
4. 編集中の未保存内容は失われる(MVPでは自動保存/ドラフト復元は実装しない — 6章参照)。ログイン画面に「セッションが切れました。再度ログインしてください」バナー。
5. 再ログイン成功後、`redirect` searchパラメータがあれば元のパスへ戻す(単純なリダイレクトのみ。編集内容の復元はしない)。

### 5.3 楽観的並行性制御の衝突(If-Match 412)

1. 投稿編集画面ロード時に `GET /api/posts/{id}` で取得した `version` をフォーム状態に保持(表示はしない、hidden state)。
2. 別タブ/別ユーザーが同じ投稿を先に更新し `version` が進む。
3. 本タブで保存 → `PUT` に古い `version` を `If-Match` で送信 → `412`。
4. `apiFetch` は412を汎用エラーとして投げ、フォームのミューテーション `onError` がこれを捕捉し、フォーム下部に固定の衝突バナーを表示:「この投稿は他の場所で更新されています。最新の内容を読み込みますか?」+「最新を読み込む」ボタン。
5. ボタン押下 → `GET /api/posts/{id}` を再実行しキャッシュ更新 → フォームの値・保持versionを最新値でリセット(未保存の編集内容は破棄。破棄前の `window.confirm` は不要 — バナーのボタン押下自体が明示的な意思表示のため)。
6. 一覧画面での削除時も同様に412を検知したら「この投稿は変更されています」トースト表示 + 一覧を再フェッチ(`invalidateQueries(['posts'])`)して終了(自動リトライはしない)。

## 6. 代替案として検討し却下したもの

- **ファイルベースルーティング(`@tanstack/router-plugin` のコード生成)**: 画面数が6つのみで手動ルートツリーの方が依存を減らせる。今後画面数が増えたら乗り換え候補(可逆な決定)。
- **楽観的更新(mutation の `onMutate` でキャッシュを即時書き換え)**: MVPは画面数・同時編集頻度が小さく、`invalidateQueries` による単純な再フェッチで十分。複雑なロールバックロジックのバグリスクを避ける。
- **編集画面の自動保存/ローカルドラフト永続化(localStorage等)**: Phase 1 スコープ外の追加要件。412/401での編集内容破棄は許容する(WordPressの自動保存相当機能はPhase 2以降で要件化されれば別途設計)。
- **`GET /api/setup/status` エンドポイント追加によるルート自動判定**: ユーザー確認の結果、フロントエンドのみで解決する方針を採用(決定1参照)。
- **Svelte(ADR 0004で既に却下済み)**: 本書では再検討しない。

## 7. Cross-cutting

- **入力検証**: バックエンドの検証ルール(username `^[A-Za-z0-9_-]{3,32}$`、password 8文字以上、site_name 1-100文字、slug `^[a-z0-9]+(-[a-z0-9]+)*$`かつ予約語`api`/`admin`/`assets`禁止、title 1-200文字、body 1文字〜1MB)をフロントエンドの型/フォームバリデーションにもミラーする(`web/src/api/validation.ts` に定数として集約し、両フォームから参照)。ミラーする理由: サーバーへの往復なしに即時フィードバックを出すため。サーバー側のエラー文字列(`"invalid slug"`等)は固定の対応表(`web/src/api/errorMessages.ts`)で日本語UIメッセージにマッピングし、対応表にないエラー文字列はそのまま汎用バナーに表示する(未知のエラーでも握り潰さない)。
- **認可**: 全画面(setup/loginを除く)はルートの `beforeLoad` で「最初のAPI呼び出しが401なら `/admin/login` にリダイレクト」を共通化(個別画面に認可チェックを書かない)。
- **秘密情報**: セットアップトークンはURLやlocalStorageに保存しない(フォーム入力欄の値のみ、送信後は破棄)。セッションはHTTPOnlyクッキーのみで、SPAのJSからは読み書きしない。
- **パフォーマンス**: 一覧は既存の `limit`(既定20/上限100)によるページネーションのみ。無限スクロールや先読みはMVPスコープ外。
- **可観測性**: フロントエンドにログ送信基盤はなし(Phase 1)。`apiFetch` はエラーを `console.error` するのみ。

## 8. テスト戦略概要

- **ユニット**: `apiFetch` のエラー正規化(JSON/プレーンテキスト両方)、slug自動生成ロジック、フォームバリデーション関数。Vitest。
- **コンポーネント**: 各フォーム(setup/login/post編集/settings)の検証エラー表示、412/409バナー表示を React Testing Library でモックfetch越しに検証。
- **バックエンド(Go側の追加分のみ)**: `TestGetSettings`(`internal/adminapi/settings_test.go`)。
- **E2E**: 既存の `main_test.go` 統合テストは変更しない(バックエンドAPI経路のみ検証、フロントエンドのビルド物は対象外)。SPAのE2E(Playwright等)はPhase 1では**意図的に導入しない**。理由: (1) このリポジトリのツールチェーンにブラウザテストランナーが無く新規導入コストが発生する、(2) 管理画面は単一運用者が使う内部ツールで同時実行・多様な環境の組合せテストの価値が薄い、(3) SPAが呼ぶ全APIパスは既に `main_test.go` の15シナリオでHTTP経由で検証済みで、SPA側の追加リスクは「表示・フォーム結線」のみであり手動確認で十分カバーできる。Phase 2でD&Dブロックエディタ等の複雑なクライアント状態が増えたら再検討する。
- **フロントエンドのカバレッジゲート**: Go側(`go build -cover`+`GOCOVERDIR`)のような強制ゲートはPhase 1では**設けない**。理由: 新規導入するフロントエンドツールチェーンにCI連携がまだ無く、この1タスクの中でCIパイプライン自体を新設するのはスコープ外(既存リポジトリに `.git`/CI設定が無い時点の状態からの差分が大きくなりすぎる)。Vitestは導入するが閾値なしで「テストがある」ことのみを要求する。CI導入時に閾値を設定するのは別タスク。

## 9. ロールアウト

- feature flag不要(既存の空プレースホルダを実装で置き換えるだけで、バックエンドAPIの互換性は保たれる)。
- 段階: (1) `GET /api/settings` 追加+テスト → (2) `web/` プロジェクト雛形(Vite+React+TS+TanStack Router/Query) → (3) APIクライアント層 → (4) setup/login画面 → (5) 投稿一覧・作成・編集画面 → (6) 設定画面 → (7) `npm run build` を実行し `internal/adminui/dist/` を実物に置換 → **`internal/adminui/handler_test.go`(T21)の2アサーションを実データ非依存に書き換える(下記参照)** → `go build ./...`・`go test ./...` 確認 → (8) 実バイナリ起動してブラウザで全フロー手動確認、特にdevプロキシ経由の `GET /api/settings` がOriginチェックで403にならないことを最初に確認する。
- **`internal/adminui/handler_test.go` の既存テスト修正(breakdownの独立タスクとする)**: `TestHandler_ServesIndexAtAdminRoot` と `TestHandler_FallsBackToIndexForUnknownSubpath` は現在プレースホルダの `<h1>Admin UI</h1>` 文字列を直接アサートしている(T21実装時のフィクスチャ)。Vite実ビルド後は内容が変わり失敗する。**フィクスチャの正当な変更であり、テストを弱めるわけではない** — 実Vite出力に依存しない安定した条件(`200`・`Content-Type: text/html`・非空body、および `/admin/` と未知サブパスで同一内容が返ること)に書き換える。`fstest.MapFS` に差し替えて埋め込み自体をテスト対象から外す案は却下(実際の `embed.FS` を通す経路の検証価値を落とすため)。
- リバート: `internal/adminui/dist/` を現在のプレースホルダにgit revertすれば即座に旧状態(プレースホルダ表示)に戻る。バックエンド変更(`GET /api/settings`)は追加のみで既存エンドポイントを変更しないため、後方互換を壊さずに残しても問題ない。

## 開発時プロキシ(重要な実装詳細)

本番はSPAとAPIが同一オリジン(`/admin/*` と `/api/*` を同じGoバイナリが配信)だが、開発時はVite dev serverと Goバックエンドが別オリジンになる。これは `CSRFMiddleware`(`internal/auth/middleware.go`)が `Origin` ヘッダーを `cfg.BaseURL` と**文字列完全一致**で比較するため、素朴なプロキシだけでは403になる。

`web/vite.config.ts`:

**重要**: `base: '/admin/'` を明示すること。指定を忘れると、Viteが吐くハッシュ付きアセットURL(`/assets/index-abc123.js`)が `GET /assets/` — 公開テーマの静的配信(`httpapi.AssetsHandler`, `internal/httpapi/assets.go`)と**衝突**し、テーマのアセット配信に飲まれて404になる(SPAの `/admin/assets/...` とは別物)。同様に `router.tsx` の `createRouter()` にも `basepath: '/admin'` を指定し、TanStack Router内部のパス解決をこのプレフィックス配下に固定する。

```ts
export default defineConfig({
  base: '/admin/', // 上記の /assets/ 衝突を避けるため必須
  plugins: [react(), tanstackRouter()],
  build: { outDir: '../internal/adminui/dist', emptyOutDir: true },
  server: {
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8099', // 開発用バックエンドの --base-url と一致させる
        changeOrigin: true,
        configure: (proxy) => {
          proxy.on('proxyReq', (proxyReq) => {
            // ブラウザが送るOriginをそのまま転送するとCSRFMiddlewareの
            // 文字列完全一致チェックに落ちるため、バックエンドの
            // base-urlに書き換える。
            proxyReq.setHeader('Origin', 'http://127.0.0.1:8099')
          })
        },
      },
    },
  },
})
```

開発手順: `go run . -addr=127.0.0.1:8099 -base-url=http://127.0.0.1:8099`(別ターミナル)→ `cd web && npm run dev` → ブラウザで Vite dev server(`http://localhost:5173/`)を開く。Cookieは `credentials: 'include'` を全fetchで指定(`apiFetch` に固定で付与)し、プロキシ経由でブラウザから見て同一オリジンのため `SameSite=Lax` でも送受信される。
