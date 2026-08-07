-- 0006_sessions_meta.sql
-- roadmap-to-wp-parity.md P1-8: セッション一覧UI用のメタデータ。
-- last_seen_at は追加しない — SetMaxOpenConns(1) 下で認証済みリクエスト
-- 毎に書き込みが発生するコストに対し、UI上の価値が低いため。
--
-- design.md §3では0004として設計されたが、Migrateはファイル名の辞書順で
-- versionを連番付与するため(内容からversionを解析しない)、T10のmfaテーブル
-- 追加(0005_mfa.sql)がT7より先に実施されすでに出荷済みの版数を占有した後で
-- 0004を追記すると、既存DB上でschema_migrationsのversion番号がずれて
-- (旧0005がversion=4として記録済みのため、新0004がversion=4衝突で
-- 適用スキップされる)このマイグレーションが永久に走らなくなる。
-- ADR-0006の追記のみ規約を守るため0006として追記する。
ALTER TABLE sessions ADD COLUMN user_agent TEXT;
ALTER TABLE sessions ADD COLUMN created_ip TEXT;
CREATE INDEX idx_sessions_user ON sessions(user_id);
