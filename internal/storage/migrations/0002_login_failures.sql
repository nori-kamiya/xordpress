-- 0002_login_failures.sql
-- roadmap-to-wp-parity.md P0-3: ログイン試行回数制限・一時ロックアウト用。
-- ユーザー名ごとに直近の失敗回数とロックアウト期限を1行で保持する
-- (WAL+busy_timeoutでの単一プロセス運用を前提とし、DB永続化することで
-- プロセス再起動でロックアウト状態がリセットされないようにする)。

CREATE TABLE login_failures (
  username     TEXT PRIMARY KEY,
  fail_count   INTEGER NOT NULL DEFAULT 0,
  last_failed_at INTEGER NOT NULL,
  locked_until INTEGER                  -- NULL = ロックされていない
);
