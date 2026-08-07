-- 0003_user_roles.sql
-- roadmap-to-wp-parity.md P1-6: 複数管理者・ロールベース権限モデルの土台。
-- role列を追加し、既存の唯一の管理者行を administrator に補正する。
--
-- DEFAULT は 'administrator' とする(最小権限の 'contributor' ではない)。
-- 理由: この列を追加した段階のバイナリをロールバックした場合に実行され
-- 得る唯一の INSERT は storage.CreateFirstAdmin (setup.go、role列を持たない
-- 旧SQL文)であり、そのINSERTが依拠するのがこのDEFAULTである。
-- 'contributor' にしていると、デプロイ→ロールバック→初回セットアップ、
-- という順序で管理者が一人も存在しないインストールが生成されてしまう。
-- 将来のユーザーCRUD(P1-T9で追加予定)はroleを常に明示指定するため、
-- このDEFAULTが「不慮のINSERT」に対するフェイルセーフとして機能する
-- 場面は実質ない。
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'administrator'
  CHECK (role IN ('administrator','editor','author','contributor'));

UPDATE users SET role = 'administrator';
