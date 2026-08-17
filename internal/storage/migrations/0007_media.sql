-- 0007_media.sql
-- roadmap-to-wp-parity.md P2-9: メディアライブラリ/アップロードのメタデータテーブル。
CREATE TABLE media (
  id            INTEGER PRIMARY KEY,
  stored_name   TEXT NOT NULL UNIQUE,
  original_name TEXT NOT NULL,
  mime_type     TEXT NOT NULL,
  size_bytes    INTEGER NOT NULL,
  width         INTEGER,
  height        INTEGER,
  uploader_id   INTEGER NOT NULL REFERENCES users(id),
  created_at    INTEGER NOT NULL
);
CREATE INDEX idx_media_created ON media(created_at DESC);
CREATE INDEX idx_media_uploader ON media(uploader_id);
