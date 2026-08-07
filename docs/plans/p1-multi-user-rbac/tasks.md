# P1タスク分解: 複数管理者・ロール/権限モデル・2FA・セッション管理

設計: [design.md](./design.md)。全タスクは`internal/storage/migrations/`の追記のみマイグレーション規約(ADR-0006)・`SetMaxOpenConns(1)`下の`*sql.Tx`/`*sql.DB`混在禁止・モック禁止(実SQLite統合テスト)を守る。

## 依存関係順序

```
T1(role列+CHECK)
 → T2(Capability/Role, roleCaps map)
   → T3(routeテーブル化 + requireCap + AuthMiddleware context格納)
     → T4(currentPrincipal + posts.go authDB化) ─┐
       → T4b(CreatePost/UpdatePostのCapPostsPublish強制)
     → T5(ユーザーCRUD storage層: *sql.Tx版)      ├→ T9(POST/GET/PATCH/DELETE /api/users)
       → T6(最後の管理者保護: 削除+降格の同トランザクション化) ┘
T1 → T7(sessions_meta列 0004)
   → T8(セッション一覧/失効API)
T1,T3 → T10(mfa テーブル 0005) → T11(TOTP実装 internal/auth/totp)
   → T12(2FAログイン2段階フロー + mfa_failures)
   → T13(TOTP登録/無効化API)
T3 → T14(CSRFMiddleware PATCH対応 + app.goトップレベルmux検証テスト)
T9,T13 → T15(フロントエンド: 現在ユーザー状態 + /users画面)
T8 → T16(フロントエンド: セッション管理画面)
T12,T13 → T17(フロントエンド: 2FA登録・ログイン2段階画面)
```

並列可能: T7とT2、T10とT4/T5、T14は他タスクと独立(T3完了後ならいつでも)。フロントエンド3本(T15/T16/T17)は対応するバックエンドが完了すれば並列可。T4bはT4完了後いつでも(T5/T6/T9とは独立)。

## T1: roleカラムの追加とバックフィル
- depends on: なし
- files: `internal/storage/migrations/0003_user_roles.sql`(新規)
- what: design.md §3の`0003_user_roles.sql`をそのまま作成。`ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'administrator' CHECK (...)` + `UPDATE users SET role='administrator'`。`internal/storage/migrations/embed.go`は`//go:embed`のglobなら自動認識、明示リストなら追記。
- verify: `go test ./internal/storage/... -run TestMigrate` (既存のマイグレーション適用テストに0003が追加されて通ること)。手動: 既存DBに対し`storage.Migrate`を実行後`SELECT role FROM users`が`'administrator'`を返す。
- executor: sonnet

## T2: Capability/Role型とroleCapsマップ
- depends on: T1
- files: `internal/auth/capability.go`(新規)、`internal/auth/capability_test.go`(新規)
- what: design.md §4の`Capability`(ビットフラグ)、`Role`、`roleCaps map[Role]Capability`、`Role.Has(c Capability) bool`を実装。未知のroleは`roleCaps`未登録=常に`false`。
- verify: `go test ./internal/auth/... -run TestRole`。4ロール×7 capabilityの期待値テーブルテストと、未知ロール文字列で全capability falseになることのテストが通る。
- executor: sonnet

## T3: AuthMiddlewareのPrincipal格納 + adminapiルートテーブル化
- depends on: T2
- files: `internal/auth/middleware.go`(修正)、`internal/auth/principal.go`(新規、`Principal`型・`PrincipalFrom`/`WithPrincipal`)、`internal/adminapi/handler.go`(修正)、`internal/adminapi/handler_test.go`(修正)
- what: `AuthMiddleware`がセッション検証と同一クエリで`users.role`をJOIN取得し`Principal{UserID,Role}`をcontextへ格納。`adminapi.Handler`を`routeEntry`テーブル+`route()`(capability必須、`CapNone`はpanic)+`requireCap`ラッパーに変更、戻り値を`*http.ServeMux`から`http.Handler`に変更。既存の全ルートに現状の権限相当のcapabilityを仮付与(記事系は`CapPostsRead`/`CapPostsEditOwn`等、設定は`CapSettingsManage`、account/password変更は`CapSelfManage`)。
- verify: `go test ./internal/adminapi/... ./internal/auth/...`。ロール×既存ルートの200/403マトリクステストを新規追加し通ること。既存の全テストがグリーンのまま。
- executor: main (設計判断を伴うため — 既存全ルートへのcapability割り当ての妥当性確認)

## T4: currentPrincipalへの置き換え + posts.goのauthDB化
- depends on: T3
- files: `internal/auth/session.go`(修正、`ValidateSessionWithRole`追加)、`internal/adminapi/posts.go`(修正)、`internal/adminapi/handler.go`(修正)、`internal/adminapi/posts_test.go`(修正)
- what: design.md §4の`currentPrincipal`実装(contextから読み取り、フォールバックは`ValidateSessionWithRole`)。`UpdatePost`/`DeletePost`/`GetPost`に`authDB *sql.DB`引数を追加し、ハンドラ内で`post.AuthorID != principal.UserID && !principal.Role.Has(auth.CapPostsEditOthers)`のownershipチェックを追加。`handler.go`の該当行に`authDB`を渡すよう修正。
- verify: `go test ./internal/adminapi/... -run TestPost`。他ユーザーの記事編集が403になる新規テストが通り、既存の`posts_test.go`は引数追加のみで挙動不変であることを確認。
- executor: sonnet

## T4b: CreatePost/UpdatePostのCapPostsPublish強制
- depends on: T4
- files: `internal/adminapi/posts.go`(修正)、`internal/adminapi/posts_test.go`(修正)
- what: 実装中に発見されたタスク分解の抜け(design.md §4に追記済み)。`routes.go`は`POST /api/posts`・`PUT /api/posts/{id}`を`CapPostsEditOwn`でしか保護しておらず、`CapPostsPublish`はどのハンドラでも検証されていない。`CreatePost`/`UpdatePost`のハンドラ内で、`req.Status == "published"`かつ`principal.Role.Has(auth.CapPostsPublish)`がfalseの場合に403を返すチェックを追加する(`UpdatePost`はT4で追加済みの`currentPrincipal`呼び出しの結果をそのまま使う。`CreatePost`は現状`currentUserID`のみ呼んでいるため、`currentPrincipal`呼び出しを追加する必要がある)。
- verify: `go test ./internal/adminapi/... -run TestPost`。contributor/authorが`status:"published"`でPOST/PUTすると403になり、editor/administratorは成功することをテーブルテストで確認。既存の(全てadministratorとして実行される)posts_testが引き続き成功すること。
- executor: sonnet

## T5: ユーザーCRUDのstorage層(*sql.Tx版)
- depends on: T1, T10(実装順の訂正 — 元の依存関係図はT10落ちだった。`DeleteUserCascadeTx`はdesign.md §6.2通り`user_totp`/`mfa_recovery_codes`/`mfa_failures`/`mfa_challenges`をDELETEするが、これらのテーブルはT10でしか作られない。T10を先に実装したためT5からも参照する)
- files: `internal/storage/users_admin.go`(新規)、`internal/storage/users_admin_test.go`(新規)
- what: `*sql.Tx`を第一引数に取る関数群: `CreateUserTx(tx, username, passwordHash, role) (int64, error)`、`ListUsersTx`/`ListUsers(db)`、`GetUserByIDTx`、`UpdateUserRoleTx(tx, id, role) error`、`CountAdministratorsExceptTx(tx, excludeID int64) (int, error)`(最後の管理者チェックで再利用)、`DeleteUserCascadeTx(tx, id, reassignTo int64) error`(design.md §6.2の(c)〜(f)を1関数に: posts再割当→sessions削除→login_failures削除(usernameで)→user_totp/mfa_recovery_codes/mfa_failures/mfa_challenges削除→users削除)。
- verify: `go test ./internal/storage/... -run TestUsersAdmin`。カスケード削除後に関連テーブル全てに対象user_idの行が残っていないことをアサートする統合テスト、および`reassignTo`が存在しない/自己参照の場合にエラーを返すこと。
- executor: sonnet

## T6: 最後の管理者保護(削除・降格の同一トランザクション化)
- depends on: T5
- files: `internal/storage/users_admin.go`(修正)、`internal/storage/users_admin_test.go`(修正)
- what: design.md §6.2の409ロジックを`DeleteUserCascadeTx`と`UpdateUserRoleTx`それぞれの冒頭で`CountAdministratorsExceptTx`を使い実装。降格は「対象が現在administrator かつ 変更後ロールがadministratorでない かつ 他管理者0人」で`ErrLastAdmin`を返す。自己削除/自己降格は別途`ErrSelfTarget`。
- verify: `go test ./internal/storage/... -run TestLastAdmin`。唯一の管理者の削除・降格が`ErrLastAdmin`を返すこと、2番目の管理者がいれば成功すること、自己削除が`ErrSelfTarget`を返すことをテーブルテストで検証。並行実行での競合はトランザクション分離で防止されるため、明示的な並行テストは(SQLiteの`immediate`ロックの性質上)スキップ可。
- executor: sonnet

## T7: sessionsメタデータ列の追加
- depends on: T1
- files: `internal/storage/migrations/0004_sessions_meta.sql`(新規)
- what: design.md §3の`0004_sessions_meta.sql`をそのまま作成(`user_agent`/`created_ip`列+`idx_sessions_user`インデックス)。`auth.IssueSession`が`userAgent, ip string`を受け取り保存するよう拡張。
- verify: `go test ./internal/storage/... ./internal/auth/...`。既存セッション行(列がNULL)が引き続き`ValidateSession`で有効に検証されることを確認する回帰テスト。
- executor: sonnet

## T8: セッション一覧/個別失効API
- depends on: T7
- files: `internal/adminapi/sessions.go`(新規)、`internal/adminapi/handler.go`(修正)、`internal/storage/sessions.go`(修正、`ListSessionsByUserID`・`DeleteSessionByPrefix`追加)
- what: `GET /api/account/sessions`(`CapSelfManage`)が`substr(token_hash,1,16)`をidとして返す(design.md §5)。`DELETE /api/account/sessions/{id}`が`WHERE user_id=? AND substr(token_hash,1,16)=?`の等価比較で削除、対象なしは204(idempotent)。`DELETE /api/users/{id}/sessions`(`CapUsersManage`)は既存`DeleteSessionsByUserID`を再利用。
- verify: `go test ./internal/adminapi/... -run TestSessions`。他ユーザーのセッションIDを指定した削除が404になること、`current`フラグが呼び出し元自身のセッションで`true`になることを検証。
- executor: sonnet

## T9: ユーザーCRUD API(users.go)
- depends on: T3, T5, T6
- files: `internal/adminapi/users.go`(新規)、`internal/adminapi/handler.go`(修正)
- what: design.md §5の`GET/POST /api/users`、`GET/PATCH/DELETE /api/users/{id}`、`DELETE /api/users/{id}/mfa`を実装(全て`CapUsersManage`)。POST bodyの`role`検証(未知ロールは400)。DELETE の`reassign_to`必須・自己参照400・存在しないID400(T5のエラー型を判定してマッピング)。PATCH bodyは`{role}`および任意で`{password}`(admin reset時は`DeleteSessionsByUserID`呼び出し)。
- verify: `go test ./internal/adminapi/... -run TestUsers`。最後の管理者削除/降格が409、reassign_to不備が400、CapUsersManage以外のロールで呼ぶと403になることをテーブルテストで確認。
- executor: main (認可境界に関わるため設計判断込みで実装)

## T10: MFA関連テーブルの追加
- depends on: T1, T3
- files: `internal/storage/migrations/0005_mfa.sql`(新規)
- what: design.md §3の`0005_mfa.sql`(`user_totp`/`mfa_recovery_codes`/`mfa_challenges`/`mfa_failures`)をそのまま作成。
- verify: `go test ./internal/storage/... -run TestMigrate`。マイグレーション適用後に4テーブルが存在し、`mfa_challenges.user_id`のFK制約が機能する(存在しないuser_idでのINSERTがエラーになる)ことを確認。
- executor: sonnet

## T11: TOTP実装(RFC 6238)
- depends on: T10
- files: `internal/auth/totp/totp.go`(新規)、`internal/auth/totp/totp_test.go`(新規)
- what: HOTP/TOTP(`crypto/hmac`+`crypto/sha1`+`encoding/base32`のみ、stdlib)。`GenerateSecret() (string, error)`、`Validate(secret string, code string, atStep int64) (step int64, ok bool)`(±1ステップ許容)、`OTPAuthURI(secret, accountName, issuer string) string`。
- verify: `go test ./internal/auth/totp/...`。RFC 6238の公式テストベクタ(SHA-1、8桁時刻)で検証するテストが通る。
- executor: sonnet

## T12: 2FAログイン2段階フロー
- depends on: T2, T11
- files: `internal/adminapi/session_mfa.go`(新規)、`internal/app/app.go`(修正)、`internal/storage/mfa_failures.go`(新規、`loginfailures.go`と同型のUPSERTロジックをuser_idキーで実装)
- what: design.md §6.1のステップ1〜5を実装。`POST /api/session`が`user_totp.confirmed_at IS NOT NULL`なら`mfa_failures`ロックチェック→期限切れchallenge掃除→32バイトランダム値生成→`mfa_challenges`INSERT→`{"mfa_required":true,"mfa_token":"..."}`返却。`POST /api/session/mfa`が`mfa_token`をハッシュ化して照合→ロック再チェック→TOTP/リカバリコード検証→成功時`last_used_step`更新+challenge削除+`IssueSession`、失敗時`mfa_failures`加算。`app.go`にトップレベル登録(既存2エンドポイントと同方式)。
- verify: `go test ./internal/adminapi/... -run TestSessionMFA`。10回連続失敗で15分ロック、ロック中は正しいコードでも429、リプレイ(同一step再送)が拒否されることを検証。
- executor: main (セキュリティクリティカルなロック順序・タイミングの実装判断を伴うため)

## T13: TOTP登録/無効化API
- depends on: T11, T12
- files: `internal/adminapi/totp.go`(新規)、`internal/adminapi/handler.go`(修正)
- what: `POST /api/account/mfa`(登録開始、パスワード必須、secret+otpauth URI返却)、`POST /api/account/mfa/confirm`(code検証→`confirmed_at`設定+リカバリコード10件生成・bcryptハッシュ保存・平文を一度だけ返却)、`DELETE /api/account/mfa`(パスワード必須)。全て`CapSelfManage`。
- verify: `go test ./internal/adminapi/... -run TestTOTPEnroll`。confirm前のsecretではログイン時に2FA要求されないこと(§6.1の「未確認は未登録扱い」)、confirm後は要求されることを検証。
- executor: sonnet

## T14: CSRFMiddlewareのPATCH対応 + app.goトップレベルmux検証テスト
- depends on: T3
- files: `internal/auth/middleware.go`(修正)、`internal/app/app_test.go`(新規または修正)
- what: `CSRFMiddleware`のContent-Type検査対象メソッドに`PATCH`を追加(design.md §2)。`internal/app`に、トップレベルmuxが`POST /api/setup`・`POST /api/session`・`POST /api/session/mfa`・`/api/`の4パターンのみを登録していることを検証するテストを追加(design.md §6.3の緩和策)。
- verify: `go test ./internal/auth/... ./internal/app/...`。PATCHリクエストでContent-Type不正時に400が返ることと、mux登録パターンテストが通ることを確認。
- executor: sonnet

## T15: フロントエンド — 現在ユーザー状態 + ユーザー管理画面
- depends on: T9, T13
- files: `web/src/api/account.ts`(修正、`GET /api/account`追加)、`web/src/hooks/useCurrentUser.ts`(新規)、`web/src/routes/users.index.tsx`(新規)、`web/src/routes/users.index.test.tsx`(新規)、`web/src/router.tsx`(修正、`/users`ルート追加)
- what: `GET /api/account`から`{id,username,role,capabilities}`を取得するhook。ユーザー一覧・追加・ロール変更・削除(reassign_to選択UI含む)画面。`CapUsersManage`を持たない場合はナビゲーションから隠す(cosmeticのみ、認可はサーバー側)。
- verify: `npm test -- users.index`(vitest)。一覧表示・追加・削除操作のテストが通る。`npx tsc --noEmit`。
- executor: sonnet

## T16: フロントエンド — セッション管理画面
- depends on: T8
- files: `web/src/api/sessions.ts`(新規)、`web/src/routes/account.sessions.tsx`(新規)、`web/src/routes/account.sessions.test.tsx`(新規)、`web/src/router.tsx`(修正)
- what: 自分のアクティブセッション一覧表示、個別失効ボタン、現在のセッションへの視覚的マーキング。
- verify: `npm test -- account.sessions`。一覧表示・失効操作のテストが通る。`npx tsc --noEmit`。
- executor: sonnet

## T17: フロントエンド — 2FA登録・ログイン2段階画面
- depends on: T12, T13
- files: `web/src/api/mfa.ts`(新規)、`web/src/routes/account.security.tsx`(新規)、`web/src/routes/login.tsx`(修正、`mfa_required`時の第2ステップ追加)、対応する`.test.tsx`群、`web/src/router.tsx`(修正)
- what: TOTP登録(secret表示+QRなしの手動入力+confirm)、無効化、リカバリコード表示(一度だけ)。ログイン画面が`mfa_required:true`を受け取った場合にコード入力フォームへ遷移。
- verify: `npm test -- account.security login`。登録フロー・ログイン2段階フローのテストが通る。`npx tsc --noEmit`。
- executor: sonnet

## 意図的に含めないもの(Non-goals、design.mdより)

- `POST /api/setup`・`POST /api/session`のCSRFMiddleware未適用ギャップの修正(design.md §6.3で別issue化)。
- リカバリコード以外のTOTPロックアウト救済手段(design.mdにはCLIサブコマンド等の言及はなく、`DELETE /api/users/{id}/mfa`のみ — T9に含む)。
- ユーザー名のリネーム機能(design.md §3で明示的に非対応)。
- `last_seen_at`のようなセッション詳細メタデータ(design.md §9でコスト超過と判断)。

## 推奨: 最初のタスク

**T1**から開始(依存元がなく、後続の大半がこれに依存する)。次にT2→T3の順で認可の骨格を固める。T3は設計判断を伴うため`main`(高コストモデル)を推奨。

T1に`/handoff`を実行しますか?
