-- 0005_mfa.sql
-- roadmap-to-wp-parity.md P1-7: TOTP登録情報・リカバリコード・
-- ログイン2段目チャレンジ。mfa_challenges を sessions と分離するのは、
-- 半認証トークンが ValidateSession の参照先テーブルに一切現れない
-- ようにするため(構造的な安全性 — auth.ValidateSession のバグが
-- あっても本セッションは発行され得ない)。
CREATE TABLE user_totp (
  user_id         INTEGER PRIMARY KEY REFERENCES users(id),
  secret          TEXT NOT NULL,
  confirmed_at    INTEGER,          -- NULL = 登録開始したが未確認(未有効)
  last_used_step  INTEGER NOT NULL DEFAULT 0  -- リプレイ防止
);

CREATE TABLE mfa_recovery_codes (
  user_id   INTEGER NOT NULL REFERENCES users(id),
  code_hash TEXT NOT NULL,          -- bcrypt
  used_at   INTEGER,
  PRIMARY KEY (user_id, code_hash)
);

CREATE TABLE mfa_challenges (
  token_hash TEXT PRIMARY KEY,      -- cookie値のSHA-256、sessionsと同じ方式
  user_id    INTEGER NOT NULL REFERENCES users(id),
  expires_at INTEGER NOT NULL       -- 5分固定TTL
);

CREATE TABLE mfa_failures (
  user_id        INTEGER PRIMARY KEY,
  fail_count     INTEGER NOT NULL DEFAULT 0,
  last_failed_at INTEGER NOT NULL,
  locked_until   INTEGER
);
