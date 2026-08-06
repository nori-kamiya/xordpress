-- 0001_init.sql
-- schema_migrations テーブルはマイグレーションランナー自身が `CREATE TABLE IF NOT EXISTS` で
-- 作成する(migrate.go 参照)。この初期マイグレーションファイルには含めない —
-- ランナーが最初に参照するテーブル自体をマイグレーションで作ろうとすると起動時に循環する。

CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,          -- bcrypt (SHA-256 プリハッシュ済み)
  created_at    INTEGER NOT NULL
);

CREATE TABLE posts (
  id           INTEGER PRIMARY KEY,
  slug         TEXT NOT NULL,
  title        TEXT NOT NULL,
  body         TEXT NOT NULL,           -- 記載時点の生 HTML、Phase 1 では変換なし
  status       TEXT NOT NULL CHECK (status IN ('draft','published')) DEFAULT 'draft',
  version      INTEGER NOT NULL DEFAULT 1,  -- 楽観的並行性制御用
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
