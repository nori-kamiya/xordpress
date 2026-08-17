# P2-9 メディアライブラリ/アップロード 設計

対象: `docs/plans/roadmap-to-wp-parity.md` P2-9。管理者が画像等のファイルをアップロードし、投稿本文から参照できるようにする。roadmapが明示する必須要件(非実行ディレクトリからの静的配信のみ、Content-Type検証+マジックバイト検証、拡張子allowlist、画像の再エンコード、`ResolveAsset`のパストラバーサル対策パターン再利用)を満たす。

## 1. Overview

新規`media`テーブルにアップロードのメタデータ(ファイル名・MIME・サイズ・アップロード者・作成日時)を保存し、実ファイルはリポジトリ管理下データディレクトリ配下の`media/`ディレクトリにUUIDファイル名で書き込む。アップロード時はContent-Type宣言とマジックバイトの両方を許可リスト(jpeg/png/gif画像のみ)と照合し、画像は`image`標準パッケージでデコード→再エンコードして埋め込みメタデータや細工されたペイロードを除去する。配信は`internal/theme/assets.go`の`ResolveAsset`と同じ「Clean+prefix一致」方式で新設する`ResolveMedia`が担い、既存の`AssetsHandler`と同型の`MediaHandler`が`GET /media/{path...}`で配信する。管理APIは既存のcapabilityベース認可・page/limitページネーション・`{items,total}`レスポンス規約に完全準拠する`internal/adminapi/media.go`を追加し、`CapMediaUpload`/`CapMediaManageOthers`という新capabilityで権限分離する。

## 2. Architecture

```
[Admin SPA]
   │ POST /api/media (multipart/form-data)
   │ GET  /api/media?limit=&offset=
   │ DELETE /api/media/{id}
   ▼
[adminapi.MediaHandler群]  (internal/adminapi/media.go)  ── 新規
   │ requireCap(CapMediaUpload / CapMediaManageOthers)
   │ MaxBytesReader でリクエストサイズ上限
   ▼
[internal/media パッケージ]  ── 新規
   ├─ Validate(declaredMIME, header []byte) (Kind, error)  … allowlist + マジックバイト照合
   ├─ Reencode(kind, io.Reader) ([]byte, width, height, error) … image.Decode → image.Encode
   └─ ResolveMedia(dataDir, storedName) (path, error) … ResolveAsset と同じ Clean+prefix 方式
   ▼
[internal/storage/media.go]  ── 新規、posts.go と同型
   └─ media テーブル (SQLite)
   ▼
[ファイルシステム] <dataDir>/media/<uuid>.<ext>   非実行・静的配信専用ディレクトリ

[公開/管理共通]
GET /media/{path...} → httpapi.MediaHandler → media.ResolveMedia → io.Copy
```

新規: `internal/media/`(検証・再エンコード・パス解決)、`internal/storage/media.go`、`internal/adminapi/media.go`、`internal/httpapi/media.go`(配信ハンドラ)、`web/src/api/media.ts`、`web/src/routes/media.index.tsx`。
変更: `internal/auth/capability.go`(capability追加)、`internal/adminapi/routes.go`(ルート登録)、`internal/storage/migrations/0007_media.sql`(新規マイグレーション)、投稿エディタへのメディア挿入導線(既存投稿編集画面への軽微な追記)。

## 3. Data model

```sql
-- 0007_media.sql
CREATE TABLE media (
  id            INTEGER PRIMARY KEY,
  stored_name   TEXT NOT NULL UNIQUE,   -- <uuid>.<ext>、ファイルシステム上の実名
  original_name TEXT NOT NULL,          -- アップロード時のファイル名(表示用、パス要素として不使用)
  mime_type     TEXT NOT NULL,          -- 再エンコード後に確定した値(image/jpeg 等)
  size_bytes    INTEGER NOT NULL,       -- 再エンコード後のサイズ
  width         INTEGER,                -- 画像のみ。非画像対応時はNULL許容のまま拡張可
  height        INTEGER,
  uploader_id   INTEGER NOT NULL REFERENCES users(id),
  created_at    INTEGER NOT NULL
);
CREATE INDEX idx_media_created ON media(created_at DESC);   -- 一覧の既定ソート
CREATE INDEX idx_media_uploader ON media(uploader_id);      -- 自分のアップロードのみ表示するフィルタ用
```

- `width`/`height`は画像のみ設定(全対応形式が画像のためNULLは実質発生しないが、将来の非画像対応拡張性のためNULL許容のままとする)。
- `stored_name`はサーバ側で`uuid.New()`(既存依存の`google/uuid`を利用、新規依存不要)+検証済み拡張子から生成し、クライアント入力を一切含めない。これによりパストラバーサル・上書き・拡張子偽装の余地をデータ生成時点で断つ。
- `version`列(楽観的並行性制御)は不要と判断: メディアは`update`を持たず、`delete`のみの単純なライフサイクルのため(§6で後述)。
- forward/backward互換性: 新規テーブルのみで既存テーブル変更なし。ロールバック時はマイグレーションランナーがdown migrationを持たない現行方針(既存の0001〜0006も同様)に合わせ、テーブル追加のみで対応。

## 4. API contracts

すべて`internal/adminapi`配下、既存の`{"error": "..."}`エラー規約・`writeJSON`/`writeError`を再利用。

### `POST /api/media` — アップロード
- 認可: `CapMediaUpload`(全ロールがcontributor以上で保持、詳細は§7)
- リクエスト: `multipart/form-data`、フィールド名`file`。`http.MaxBytesReader`で10MB上限(投稿本文2MBより緩いが、画像用途として妥当な値。設定可能にはせず固定値でMVP)。
- 処理: ①`r.FormFile("file")`で取得 → ②宣言されたContent-Typeとファイル先頭512バイトを`internal/media.Validate`に渡しallowlist(image/jpeg, image/png, image/gif)照合(webpは非対応、後述) → ③`internal/media.Reencode`でデコード・再エンコード → ④UUIDファイル名で`<dataDir>/media/`に書き込み → ⑤`storage.InsertMedia`でメタデータ保存。
- レスポンス: `201 Created`, `{id, storedName, url, mimeType, sizeBytes, width, height, uploaderId, createdAt}`
- エラー: `400`(allowlist外/マジックバイト不一致/デコード失敗/フォーム不正)、`413`(サイズ超過)、`401`/`403`(認可)、`500`(ファイルI/O失敗、DB挿入失敗時は書き込み済みファイルを削除してロールバック)

### `GET /api/media` — 一覧
- 認可: `CapMediaUpload`(自分のアップロード一覧は投稿と同様、投稿権限モデルに合わせ全アップロード可能ロールに一覧閲覧も許可)
- クエリ: `limit`(既定20、上限100)、`offset`(既定0)。投稿の`ListPosts`と同一パターン。
- レスポンス: `{items: [...], total: N}`

### `DELETE /api/media/{id}` — 削除
- 認可: 所有者本人は`CapMediaUpload`で削除可、他人のものは`CapMediaManageOthers`(editor/administrator)が必要 — `posts.go`の`ownership check`パターンをそのまま踏襲。
- 処理: DB行削除 → 成功時のみファイル削除(DB削除が先: ファイル削除が先だとDB削除失敗時に孤立ゴースト参照が残り404を返せなくなるため)。ファイル削除失敗はログのみでレスポンスは`204`のまま(孤立ファイルは許容、次点で定期クリーンアップの余地を残すが本設計のスコープ外)。
- エラー: `404`(存在しない)、`403`(所有者以外かつ`CapMediaManageOthers`なし)

## 5. Key flows

**Happy path**: multipart受信 → allowlist+マジックバイト検証 → デコード → 再エンコード(EXIF等のメタデータはデコード/再エンコード往復で自然に失われる) → UUID名で書き込み → DB行挿入 → 201。

**検証失敗(悪意あるペイロード)**: 例えばPHPスクリプトを`.jpg`にリネームしてアップロード。Content-Typeを`image/jpeg`と偽装していても、`image.Decode`がマジックバイト不一致でエラーを返すため再エンコード段階で確実に弾かれる。`Validate`でのマジックバイトチェックは早期リジェクトの最適化であり、最終防衛線はデコード成功可否そのもの(デコードできないものは画像として配信されようがない)。

**書き込み後DB挿入失敗**: ファイルはすでにディスクに書かれているが、`InsertMedia`が制約違反等で失敗した場合、`os.Remove`で書き込み済みファイルを削除してから500を返す(孤立ファイル防止)。書き込み前にDB行を仮作成する方式は採らない — `stored_name`はファイル書き込み前に確定できる(UUID生成にDBの採番は不要)ため、順序をファイル書き込み→DB挿入に固定でき、逆順(DB先)より一貫性維持が単純。

**同時アップロード**: SQLiteは`SetMaxOpenConns(1)`で書き込みをGo側シリアライズ済み(既存パターン踏襲)。ファイルシステム書き込みはUUID名のため衝突しない。

## 6. Alternatives considered

- **画像をDBにBLOB格納**: SQLiteの単一ファイルが肥大化し、既存の`ResolveAsset`型の静的配信パターン(ファイルシステム+HTTPハンドラ)と整合しない。ファイルシステム保存を採用。
- **webp対応**: Go標準ライブラリは`image/webp`のデコード・エンコードいずれも提供しない(`golang.org/x/image/webp`はデコードのみ提供、エンコードは実質存在せず、いずれにせよ新規依存)。新規依存を避けるためwebpはMVPでは非対応とし、アップロード時にallowlist外として`400`で拒否する。将来的に需要が生じた場合、`golang.org/x/image/webp`追加(デコードのみ→PNG化)を再検討する。
- **アップロードディレクトリ名をタイトルベースのスラッグにする**: WordPress同様の`original_name`ベース命名は可読性が高いが、パストラバーサル・上書き・重複衝突の対策が複雑化する。UUID命名で単純化し、`original_name`は表示用メタデータとしてのみDB保持。
- **画像以外(PDF等)のアップロード許可**: マジックバイト検証・再エンコードによる無害化の手法が画像以外に一般化しにくく(PDF再エンコードで悪性JSペイロード除去を保証するのは非現実的)、P2-9のMVPスコープでは画像allowlistのみに限定。将来ファイル種別を追加する場合は種別ごとに個別の無害化戦略が必要になる旨をroadmapに残す。

## 7. Cross-cutting

- **Capability追加**(`internal/auth/capability.go`): `CapMediaUpload`(全ロールがcontributor以上で保持。投稿の`CapPostsEditOwn`と同じ配布)、`CapMediaManageOthers`(editor/administratorのみ、`CapPostsEditOthers`と同じ配布)。`roleCaps`・`capabilityNames`両方に追加、`docs/plans/p1-multi-user-rbac/design.md`のcapability一覧にも追記が必要(このdesign.mdからの参照として明記)。
- **セキュリティ**:
  - 拡張子allowlist: `.jpg`/`.jpeg`/`.png`/`.gif`のみ(webpは非対応、上記Alternatives参照)。
  - マジックバイト検証: `http.DetectContentType`または画像フォーマット別シグネチャ照合を`internal/media.Validate`に実装。
  - 再エンコードによる無害化: `image.Decode`→`jpeg.Encode`/`png.Encode`/`gif.Encode`。EXIF等のメタデータ、ポリグロットファイルへの埋め込みペイロードはデコード時に破棄される。
  - 配信ディレクトリは非実行: `<dataDir>/media/`はアプリケーションが直接readするのみで、Webサーバーの静的ファイル委譲(nginx等)を使わない限りスクリプト実行経路が存在しない。将来nginx等の前段プロキシを置く場合は、このディレクトリに対する`.php`等の実行設定を明示的に無効化する運用ドキュメントを別途残す(本設計はアプリケーション層の対策のみをカバー)。
  - パストラバーサル: `ResolveMedia`が`ResolveAsset`と同じClean+prefix方式。加えて配信対象は常にDBの`stored_name`(UUID生成、外部入力非依存)のみなので、配信ハンドラに渡る時点で実質的にトラバーサル不可能な値のみが流れる(ResolveAssetの防御は多層防御としての位置づけ)。
  - リクエストサイズ上限: `http.MaxBytesReader`で10MB(投稿本文の2MBパターンを踏襲した固定値)。
- **パフォーマンス**: 再エンコードはCPU律速だが管理者操作(低頻度)であり、SQLiteの単一ライター制約と合わせても問題にならない規模。一覧はcreated_at降順の部分インデックスなしの通常インデックスで十分(投稿の公開一覧のような高頻度アクセスパターンではない)。
- **可観測性**: アップロード成功/失敗、削除、ファイルI/Oエラー(孤立ファイル発生時)は既存のログ方式(標準の`log`パッケージ、他ハンドラと同一)でログ出力。

## 8. Test strategy outline

- `internal/media`: 単体テスト。allowlist外MIME拒否、マジックバイト不一致拒否(拡張子偽装ケース含む)、正常画像(jpeg/png/gif)の再エンコード成功、webpアップロードの400拒否、`ResolveMedia`のトラバーサル拒否(`ResolveAsset`のテストパターンを踏襲)。
- `internal/storage/media_test.go`: `InsertMedia`/`GetMediaByID`/`ListMedia`/`DeleteMedia`、in-memory SQLite。
- `internal/adminapi/media_test.go`: `httptest`によるHTTPレベルテーブルテスト。認可(401/403)、allowlist外アップロードの400、サイズ超過413、所有者以外削除の403、`routes_test.go`の「全ルートが実capabilityを要求する」アサーションに新ルートも含まれることを確認。
- `internal/httpapi/media_test.go`: 配信ハンドラのトラバーサル攻撃ケース(`AssetsHandler`のテストと同型)。
- フロントエンド: `web/src/routes/media.index.test.tsx`、アップロード・一覧・削除のコンポーネントテスト(`users.index.test.tsx`を踏襲)。

## 9. Rollout

- フィーチャーフラグ不要(新規テーブル・新規エンドポイントの追加のみで既存機能に影響しない)。
- マイグレーション順序: `0007_media.sql`を追加するのみ、既存マイグレーションへの影響なし。
- リバート: このマイグレーションのみをdownすることは現行のdown migration機構がないため、リバートは「新規追加ファイル・コードの取り消しコミット」で対応(既存の0001〜0006と同じ運用)。本番デプロイ後に問題が発覚した場合、`media`テーブルへの書き込みが発生している可能性があるため、コード側のロールバックとテーブル放置(未使用テーブルとして残す)を許容する。
