# P2-9タスク分解: メディアライブラリ/アップロード

設計: [design.md](./design.md)。全タスクは既存規約(マイグレーション命名`NNNN_description.sql`、`SetMaxOpenConns(1)`下の`*sql.Tx`/`*sql.DB`混在禁止、モック禁止=実SQLite統合テスト、adminapiの`route()`はcapability必須)を守る。

## 依存関係順序

```
T1(media マイグレーション 0007)
 → T2(internal/media: allowlist+マジックバイト検証+再エンコード) ── リスク最大、最優先で着手
   → T3(internal/storage/media.go: CRUD)
     → T4(Capability追加: CapMediaUpload/CapMediaManageOthers)
       → T5(POST /api/media アップロードAPI)
       → T6(GET /api/media 一覧API)
       → T7(DELETE /api/media/{id} 削除API)
T2 → T8(internal/httpapi: ResolveMedia + MediaHandler 配信)
T5,T6,T7 → T9(フロントエンド: web/src/api/media.ts + media.index.tsx)
T8 → T10(投稿エディタへのメディア挿入導線)
```

並列可能: T2完了後、T3系(storage)とT8(配信ハンドラ)は独立に進められる。T5/T6/T7は同一ファイル(`internal/adminapi/media.go`)を触るため直列を推奨(並列化してもマージコストが増えるだけ)。T9はT5〜T7が全て揃ってから。T10はT8のみに依存し他と並列可。

## T1: mediaテーブルのマイグレーション
- depends on: なし
- files: `internal/storage/migrations/0007_media.sql`(新規)
- what: design.md §3の`0007_media.sql`をそのまま作成。`media`テーブル(`id`,`stored_name` UNIQUE,`original_name`,`mime_type`,`size_bytes`,`width`,`height`,`uploader_id` REFERENCES users(id),`created_at`)+`idx_media_created`+`idx_media_uploader`。`internal/storage/migrations/embed.go`のglobパターンで自動認識されることを確認(明示リストなら追記)。
- verify: `go test ./internal/storage/... -run TestMigrate` が通り、`schema_migrations`に`0007_media.sql`が記録される。手動: `storage.Migrate`後`.schema media`相当のクエリでカラム構成を確認。
- executor: sonnet

## T2: internal/media パッケージ(検証・再エンコード・パス解決)
- depends on: T1(並行着手可、DBに依存しないため実質依存なし。ただし記載順としてT1後を推奨)
- files: `internal/media/media.go`(新規)、`internal/media/media_test.go`(新規)
- what: design.md §2・§6の3関数を実装。
  1. `Validate(declaredMIME string, header []byte) (Kind, error)`: allowlist(`image/jpeg`,`image/png`,`image/gif`)とマジックバイト照合。webpを含むallowlist外は`ErrUnsupportedType`。
  2. `Reencode(kind Kind, r io.Reader) (data []byte, mimeType string, width, height int, err error)`: `image.Decode`→種別ごとに`jpeg.Encode`/`png.Encode`/`gif.Encode`。デコード失敗(マジックバイト通過後の偽装ファイル含む)は`ErrDecodeFailed`。
  3. `ResolveMedia(dataDir, storedName string) (string, error)`: `internal/theme/assets.go`の`ResolveAsset`と同じClean+prefix方式(`filepath.Join`→`filepath.Clean`→prefix一致検証)。
- verify: `go test ./internal/media/...`。テストケース: 正常jpeg/png/gifの再エンコード成功、拡張子偽装(pngヘッダを`.jpg`と宣言等)の拒否、非画像バイト列(先頭にPHPコード等)の拒否、`ResolveMedia`への`../`を含む`storedName`が拒否されること(trailing不一致・親ディレクトリ脱出の両方)。
- executor: main(webp対応可否の設計判断とマジックバイト検証の正確性がセキュリティクリティカルなため)

## T3: internal/storage/media.go(CRUD)
- depends on: T1, T2(Kind/mimeType型を参照するため)
- files: `internal/storage/media.go`(新規)、`internal/storage/media_test.go`(新規)
- what: `posts.go`と同型。`InsertMedia(db, storedName, originalName, mimeType string, sizeBytes int64, width, height int, uploaderID int64) (int64, error)`、`GetMediaByID(db, id) (*Media, error)`(`ErrNotFound`)、`ListMedia(db, limit, offset int) ([]Media, int, error)`(`{items, total}`用、totalは別クエリ)、`DeleteMedia(db, id int64) error`。`stored_name`のUNIQUE制約違反は理論上UUID採番のため発生しない想定だが、`mapSlugConflict`同様にsqlite制約エラーを判定するヘルパーは用意せず、発生時は素の500として扱う(設計上想定外のため過剰実装しない)。
- verify: `go test ./internal/storage/... -run TestMedia`。in-memory SQLiteでInsert→Get→List(total件数確認)→Delete→Get(ErrNotFound)の一連を検証。
- executor: sonnet

## T4: Capability追加(CapMediaUpload/CapMediaManageOthers)
- depends on: T1(他タスクと独立、DBに依存しないため実質いつでも着手可)
- files: `internal/auth/capability.go`(修正)、`internal/auth/capability_test.go`(修正)
- what: design.md §7の通り`CapMediaUpload`(contributor以上、`CapPostsEditOwn`と同じロール配布)・`CapMediaManageOthers`(editor/administrator、`CapPostsEditOthers`と同じ配布)を`Capability`定数・`roleCaps`・`capabilityNames`(wire名`media:upload`/`media:manage_others`)に追加。`docs/plans/p1-multi-user-rbac/design.md`のcapability一覧表にも本タスクで追記する。
- verify: `go test ./internal/auth/... -run TestRole`。4ロール×新2capabilityの期待値をテーブルテストに追加(administrator/editor: 両方true、author/contributor: upload=true・manage_others=false)。
- executor: sonnet

## T5: POST /api/media アップロードAPI
- depends on: T2, T3, T4
- files: `internal/adminapi/media.go`(新規)、`internal/adminapi/media_test.go`(新規)、`internal/adminapi/routes.go`(修正、ルート追加)、`internal/adminapi/handler.go`(修正、dataDir等の依存注入が必要な場合)
- what: design.md §4の`POST /api/media`。`http.MaxBytesReader`で10MB上限→`r.FormFile("file")`→`media.Validate`→`media.Reencode`→`uuid.New()`ベースの`stored_name`生成→`<dataDir>/media/`へ書き込み→`storage.InsertMedia`。DB挿入失敗時は書き込み済みファイルを`os.Remove`でロールバック。`requireCap(auth.CapMediaUpload, ...)`でルート登録。レスポンスは`201`+`{id,storedName,url,mimeType,sizeBytes,width,height,uploaderId,createdAt}`。
- verify: `go test ./internal/adminapi/... -run TestMediaUpload`。正常アップロード201、allowlist外400、10MB超過413、未認証401、capability不足403、DB挿入失敗時にファイルが残らないこと(一時ディレクトリを使ったテスト)。
- executor: sonnet

## T6: GET /api/media 一覧API
- depends on: T4, T5(同一ファイル`media.go`への追記、ルート登録パターンをT5から踏襲)
- files: `internal/adminapi/media.go`(修正)、`internal/adminapi/media_test.go`(修正)、`internal/adminapi/routes.go`(修正)
- what: design.md §4の`GET /api/media`。`ListPosts`と同じlimit(既定20/上限100)・offsetパターンで`storage.ListMedia`を呼び`{items,total}`を返す。`requireCap(auth.CapMediaUpload, ...)`。
- verify: `go test ./internal/adminapi/... -run TestMediaList`。ページネーション境界値(limit上限超過時のクランプ、offset)、空一覧時の`{items:[],total:0}`。
- executor: sonnet

## T7: DELETE /api/media/{id} 削除API
- depends on: T4, T5
- files: `internal/adminapi/media.go`(修正)、`internal/adminapi/media_test.go`(修正)、`internal/adminapi/routes.go`(修正)
- what: design.md §4・§5の削除フロー。所有者本人(`CapMediaUpload`)または`CapMediaManageOthers`保持者のみ削除可(`posts.go`のownership checkパターン踏襲)。DB削除→成功時のみファイル削除(順序固定、design.md §5参照)。ファイル削除失敗はログのみで`204`を返す。
- verify: `go test ./internal/adminapi/... -run TestMediaDelete`。所有者による削除204、他人のものをcontributorが削除しようとして403、editorは他人のものも削除可、存在しないIDで404、DB削除後にファイルが実際に消えていることをファイルシステムで確認。
- executor: sonnet

## T8: internal/httpapi 配信ハンドラ
- depends on: T2(ResolveMediaのみに依存、T3〜T7とは独立)
- files: `internal/httpapi/media.go`(新規)、`internal/httpapi/media_test.go`(新規)、アプリ起動時のルート登録箇所(`internal/app`等、`AssetsHandler`登録箇所と同じファイル、修正)
- what: `AssetsHandler`(`internal/httpapi/assets.go:23-40`)と同型の`MediaHandler`。`GET /media/{path...}`で`r.URL.Path`から`/media/`プレフィックスを除去し`media.ResolveMedia(dataDir, name)`を呼び、エラーは理由を問わず404、成功時は`mime.TypeByExtension`でContent-Type設定し`io.Copy`でストリーム。認証不要(公開配信、投稿本文からの参照を想定)。
- verify: `go test ./internal/httpapi/... -run TestMedia`。正常ファイル配信200、`../`を含むパスでの404(トラバーサル拒否)、存在しないファイルの404。
- executor: sonnet

## T9: フロントエンド メディアライブラリ画面
- depends on: T5, T6, T7
- files: `web/src/api/media.ts`(新規)、`web/src/routes/media.index.tsx`(新規)、`web/src/routes/media.index.test.tsx`(新規)
- what: `web/src/api/users.ts`・`web/src/routes/users.index.tsx`(P1-T15)と同型。アップロード(ファイル選択→POST)・一覧(TanStack Queryの`queryOptions`)・削除(`mutationOptions`、自分のものでない場合はUIボタン非表示 or サーバー403をエラーメッセージ表示)。`web/src/api/errorMessages.ts`の既存パターンでエラーメッセージをマッピング。
- verify: `npm test -- media.index` (プロジェクトのテストコマンドに準拠)。アップロード成功時の一覧再取得、削除確認、エラー表示のコンポーネントテストが通る。
- executor: sonnet

## T10: 投稿エディタへのメディア挿入導線
- depends on: T8(配信URLが確定していないと本文への挿入が機能しないため)
- files: 既存の投稿編集画面コンポーネント(修正、`web/src/routes/`配下の該当ファイルを実装時に特定)
- what: 投稿本文エディタに「メディアを挿入」ボタンを追加し、T9のアップロードAPIを呼び出して返された`url`を本文カーソル位置に挿入する最小限の導線。メディアライブラリ全体のモーダル化等はスコープ外(MVP: アップロード即挿入のみ)。
- verify: 既存の投稿エディタのコンポーネントテストに、メディア挿入後に本文へURLが反映されることを確認するケースを追加。`npm test`で該当ファイルが通る。
- executor: sonnet

## スコープ外(design.md記載の通りMVPで対象外)
- 画像以外のファイル種別(PDF等)のアップロード対応。
- 孤立ファイルの定期クリーンアップ(削除時のファイルI/O失敗で発生しうるが、design.md §4の通り許容)。
- アップロードサイズ上限の設定画面からの変更(10MB固定)。
