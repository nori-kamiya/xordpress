# 管理画面フロントエンド(admin SPA) — 実装タスク一覧

> 設計書: [docs/plans/admin-spa/design.md](design.md)。このタスク一覧は設計の決定を再検討しない。

## 依存関係と並列実行可能なグループ

```
T1 (backend: GET /api/settings)  ─── 独立、他タスクと並列可
T2 (adminui: 既存テストの脱プレースホルダ化) ─── 独立、最優先で着手(リスクの先出し)
  └─ T3 (web: 雛形 + 初回ビルド)
       └─ T4 (web: APIクライアント層)
            └─ T5 (web: ルーター雛形 + ルートレイアウト)
                 ├─ T6  (web: setup画面)
                 ├─ T7  (web: login画面)
                 ├─ T8  (web: 投稿一覧画面)
                 ├─ T9  (web: 投稿作成画面)
                 ├─ T10 (web: 投稿編集画面)
                 └─ T11 (web: 設定画面) [depends also: T1]
                      └─ T12 (最終結合: npm run build → dist差し替え → go build/test → ブラウザ手動確認) [depends: T1,T6-T11]
```

**並列化の目安**: T1・T2は最初のバッチで同時着手。T3完了後、T4→T5の直列は避けられない(APIクライアント→ルーター雛形の順で他画面が依存するため)。T5完了後はT6〜T11の6画面を全て並列実行可能(いずれもroot.tsx/router.tsxを読むだけで、互いに異なるファイルしか書かない)。

## T1: backend — GET /api/settings 追加

- depends on: なし
- files: `internal/adminapi/settings.go`(modify), `internal/adminapi/handler.go`(modify), `internal/adminapi/settings_test.go`(modify)
- what: design.md §4.1 のとおり `GetSettings(contentDB *sql.DB) http.Handler` を追加し、`content.GetSiteSettings` を呼んで `200 {"site_name":...}` を返す(取得エラーのみ`500`)。`handler.go`の`Handler()`に`mux.Handle("GET /api/settings", GetSettings(contentDB))`を1行追加(既存`PUT /api/settings`登録の隣)。認証は既存の`UpdateSettings`と同じく外側`AuthMiddleware`に委譲し、ハンドラ自身は再チェックしない。
- verify: `go test ./internal/adminapi/... -run TestGetSettings -v` → 値設定済み/未設定(空文字)の両方でgreen。`go build ./...`・`go test ./...`(既存分含め全て)green。
- executor: sonnet

## T2: adminui — 既存テストのプレースホルダ依存を除去

- depends on: なし
- files: `internal/adminui/handler_test.go`(modify)
- what: design.md §9「rollout」の指示どおり、`TestHandler_ServesIndexAtAdminRoot`と`TestHandler_FallsBackToIndexForUnknownSubpath`が現在アサートしている`<h1>Admin UI</h1>`という具体的な文字列への依存を除去し、実Viteビルド後も成立する条件(`200`、`Content-Type: text/html`、非空body、`/admin/`と未知サブパス`/admin/some/unknown/route`で同一内容が返ること)に書き換える。`fstest.MapFS`への差し替えは行わない(実`embed.FS`経由の検証を保つ)。他のテスト(リダイレクト系)は無変更。
- verify: `go test ./internal/adminui/... -v` → 3テストとも green(現在のプレースホルダに対しても、将来の実ビルドに対しても)。
- executor: sonnet

## T3: web — プロジェクト雛形 + 初回ビルド配線

- depends on: T2(T2が先に完了していないと、この初回ビルドがT2未修正のテストを壊す)
- files: `web/package.json`(create), `web/vite.config.ts`(create), `web/tsconfig.json`(create), `web/index.html`(create), `web/src/main.tsx`(create, 最小の空ルートのみ), `.gitignore`(modify — `web/node_modules`追加)
- what: design.md §2のディレクトリ構成に従い、Vite + React18 + TypeScript(`strict`, `noUncheckedIndexedAccess`) + `@tanstack/react-router` + `@tanstack/react-query` の依存を持つ最小プロジェクトを作成。`vite.config.ts`は`base: '/admin/'`・`build.outDir: '../internal/adminui/dist'`・`build.emptyOutDir: true`・開発用`server.proxy['/api']`(design.md §9のOrigin書き換え含む)を設定。`main.tsx`は空の`<div>admin</div>`程度で構わない(後続タスクが実装を積む)。このタスクの最後に実際に`npm run build`を1回実行し、`internal/adminui/dist/`を最小の実ビルド出力に置換する。
- verify: `cd web && npm run build` → 成功、`internal/adminui/dist/index.html`が生成される。リポジトリルートで `go build ./...` と `go test ./internal/adminui/...` → green(T2の書き換え済みアサーションに対して)。
- executor: sonnet

## T4: web — APIクライアント層

- depends on: T3
- files: `web/src/api/client.ts`(create), `web/src/types.ts`(create), `web/src/api/posts.ts`(create), `web/src/api/session.ts`(create), `web/src/api/setup.ts`(create), `web/src/api/settings.ts`(create), `web/src/api/validation.ts`(create), `web/src/api/errorMessages.ts`(create)
- what: design.md §4.2/§4.3・§7を厳密に実装する。`client.ts`の`apiFetch`は`credentials:'include'`固定、`Content-Type`ヘッダーで`{"error":...}`(JSON)とプレーンテキスト(`AuthMiddleware`/`CSRFMiddleware`由来)の両方を`ApiError{status,message}`に正規化し、`status===401`ならグローバルに(モジュールスコープの`queryClient`参照を使い)`queryClient.clear()`してから`router.navigate({to:'/admin/login'})`する(`router`は後続T5で作られるシングルトンをここでは型のみ先取りし、実際の呼び出し配線はT5側で完成させてよい — このタスクでは`onUnauthorized`コールバックを受け取れる形にAPIを開けておく)。`types.ts`は`PostSummary`/`Post`/`PostListResponse`等をバックエンドJSON形状にそのまま対応させる。`validation.ts`はdesign.md §7の検証ルール定数(username正規表現、password最小長、slug正規表現+予約語、title/body長)。`errorMessages.ts`はサーバーの固定エラー文字列→日本語UI文言の対応表(未知の文字列はそのまま素通しできる関数として提供)。posts.ts はTanStack Queryの`queryOptions`/`mutationOptions`ヘルパー(list/get/create/update/delete)をまとめ、If-Matchヘッダーの`"${version}"`形式もここで組み立てる。
- verify: `cd web && npm run typecheck`(`tsc --noEmit`)→ エラー0。`cd web && npm run test`(Vitest、`client.ts`のエラー正規化とvalidation関数の単体テストを含む)→ green。
- executor: sonnet

## T5: web — ルーター雛形 + ルートレイアウト

- depends on: T4
- files: `web/src/router.tsx`(create), `web/src/routes/root.tsx`(create), `web/src/main.tsx`(modify — `QueryClientProvider`+`RouterProvider`に置き換え)
- what: `createRouter({basepath:'/admin', routeTree})`。ルートツリーは`root.tsx`(レイアウト: ヘッダー、ログアウトボタン→T4の`session.ts`の`logout`呼び出し)の下に、T6〜T11で埋める6ルートのプレースホルダ(空コンポーネントで良い)を先に登録して`tsc`とルーティングの型が通ることを保証する。T4で開けておいた`onUnauthorized`コールバックにこの`router`インスタンスの`navigate`を実際に渡して配線を完成させる(design.md §5.2)。
- verify: `cd web && npm run typecheck` → エラー0。`cd web && npm run build` → 成功。
- executor: sonnet

## T6: web — setup画面

- depends on: T5
- files: `web/src/routes/setup.tsx`(create, プレースホルダから実装に置換)
- what: design.md §5.1のフォーム(setup token・username・password・site_name)。`POST /api/setup`(Authorizationヘッダー)。`201`→`/admin/login`へ`setup=done`検索パラメータ付きで遷移。`409`→インラインで「セットアップ済みです。ログインしてください」+ログインへのリンク。`400`系はT4の`errorMessages.ts`でマッピングしフィールド単位に表示(`invalid username`→username欄、等)。
- verify: `cd web && npm run test -- setup` → フォーム検証・409/201分岐のコンポーネントテストがgreen。`npm run typecheck`green。
- executor: sonnet

## T7: web — login画面

- depends on: T5
- files: `web/src/routes/login.tsx`(create)
- what: username/passwordフォーム。`POST /api/session`。`204`→`queryClient.invalidateQueries(['posts'])`→`/admin/posts`へ遷移。`401`→インライン「ユーザー名またはパスワードが違います」。`?redirect=`検索パラメータがあれば成功時にそちらへ遷移(design.md §5.2)。`?setup=done`があれば成功バナー表示。
- verify: `cd web && npm run test -- login` → 204/401分岐・redirect遷移のテストがgreen。
- executor: sonnet

## T8: web — 投稿一覧画面

- depends on: T5
- files: `web/src/routes/posts.index.tsx`(create)
- what: `GET /api/posts?status=&limit=&offset=`一覧をテーブル表示(title/status/updated_at/操作列)。statusフィルタ・ページネーション(limit/offset)。「新規作成」ボタン→`/admin/posts/new`。各行に編集リンク(`/admin/posts/$postId/edit`)と削除ボタン(`DELETE`+`If-Match`、design.md §5.3の412時トースト+`invalidateQueries(['posts'])`)。
- verify: `cd web && npm run test -- posts.index` → 一覧表示・フィルタ・削除の200/412分岐テストがgreen。
- executor: sonnet

## T9: web — 投稿作成画面

- depends on: T5
- files: `web/src/routes/posts.new.tsx`(create)
- what: title/slug(titleから自動生成、編集可)/body/status(draft・published)フォーム。`POST /api/posts`。`201`→`/admin/posts/$id/edit`へ遷移。`409`(slug衝突)→slug欄にインラインエラー。`400`系は`errorMessages.ts`でフィールドマッピング。
- verify: `cd web && npm run test -- posts.new` → slug自動生成・201/409/400分岐テストがgreen。
- executor: sonnet

## T10: web — 投稿編集画面

- depends on: T5
- files: `web/src/routes/posts.edit.tsx`(create)
- what: ロード時`GET /api/posts/{id}`(`404`→not-found表示)。フォームにtitle/slug/body/status、`version`はhidden state保持。保存は`PUT`+`If-Match:"${version}"`。`412`→design.md §5.3の衝突バナー(「最新を読み込む」ボタンで再`GET`しフォーム値・versionをリセット)。`409`(slug衝突)→slug欄エラー。削除ボタン(`DELETE`+`If-Match`、成功後`/admin/posts`へ)。
- verify: `cd web && npm run test -- posts.edit` → 412衝突バナー・404・削除の各分岐テストがgreen。
- executor: sonnet

## T11: web — 設定画面

- depends on: T5, T1
- files: `web/src/routes/settings.tsx`(create)
- what: ロード時`GET /api/settings`で現在の`site_name`を表示。`PUT /api/settings`で更新。`400`(1-100文字外)→インラインエラー。成功時トースト表示。
- verify: `cd web && npm run test -- settings` → 表示・更新・検証エラー分岐テストがgreen。T1が未完了だと`GET /api/settings`がモック以外では通らないため、統合確認はT12で行う(このタスクの単体テストはfetchモックで完結させる)。
- executor: sonnet

## T12: 最終結合 — 実ビルド差し替え + 起動確認

- depends on: T1, T6, T7, T8, T9, T10, T11
- files: `internal/adminui/dist/*`(全面差し替え、生成物としてコミット)
- what: design.md §9の最終段。`cd web && npm run build`で全画面を含む実ビルドを生成し`internal/adminui/dist/`を置換。リポジトリルートで`go build ./...`・`go vet ./...`・`go test ./...`(T2で書き換え済みの`internal/adminui`テスト含む)を実行しgreenを確認。実バイナリを起動し、ブラウザで以下を手動確認: (1) devプロキシ経由の`GET /api/settings`がOriginチェックで403にならないこと(design.md §9の指摘どおり最初に確認)、(2) setup→login→投稿作成→一覧→編集→412衝突→削除→設定変更の一連のフロー、(3) `/admin`→`/admin/`リダイレクトと未知サブパスのSPAフォールバックが実ビルドでも機能すること。
- verify: `go build ./... && go vet ./... && go test ./...` → 全green。ブラウザでの手動確認結果を報告(スクリーンショット任意)。
- executor: main
