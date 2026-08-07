# WordPress相当機能へのロードマップ(セキュアバイデザイン)

> 2026-08-06、ユーザーとの議論から作成。目標は「WordPressと同等の機能を、WordPressのような脆弱性・攻撃面を持たないシステムとして構築する」こと。
> 現状(このドキュメント作成時点): Phase 1 MVP実装済み(単一管理者・投稿CRUD・基本テーマ・管理SPA)。docs/plans/phase1-mvp/、docs/plans/admin-spa/参照。

## 進捗(セッション再開用)

- **P0(4項目)は完了・実機確認済み。** コミット: `70439b4`(autoescape)、`f686edf`(パスワード変更)、`1ca92b4`(ログインロックアウト)、`264c825`(401/403 JSON統一)。
- **P1(5〜8: マルチユーザー・権限モデル)は`/design`→`/breakdown`まで完了、実装未着手。**
  - 設計: `docs/plans/p1-multi-user-rbac/design.md`。4項目(複数管理者CRUD、capability-basedロール/権限モデル、TOTP 2FA、セッション管理)をまとめて1件として設計(ユーザー選択)。architect 3種(simplest-possible/security-strictness-first/migration-risk-first)並列生成→critic judgeでmigration-risk-first案を採用→Opus 5とFableによる独立REFUTEレビューを2ラウンド実施し、発見された欠陥(ロールバック時のroleデフォルト値バグ、`mfa_challenges`のFK未処理カスケード、`mfa_failures`ロック未チェック、CSRF記述の内部矛盾、app.goトップレベルmuxバイパスの過大な主張、記事所有権チェックに必要なRole伝播の欠落、降格処理の競合)を全て反映済み。
  - タスク分解: `docs/plans/p1-multi-user-rbac/tasks.md`(T1〜T17、依存関係順)。
  - **次のアクション: T1(roleカラム追加マイグレーション`0003_user_roles.sql`)。** 指示書を`.claude/handoffs/p1-t1-user-roles/{instructions,acceptance}.md`に作成済み(gitignore対象、リポジトリ非コミット)。実装はまだ行っていない。新しいセッションで再開する場合は、まず`docs/plans/p1-multi-user-rbac/design.md`と`tasks.md`を読み、`.claude/handoffs/p1-t1-user-roles/instructions.md`の指示書に従ってT1から着手する(T3・T9・T12は設計判断を伴うため高コストモデルでの実装を推奨、tasks.md参照)。
- P2の12(コメント)着手前に「実装するか自体」の要確認が必要、という指示は未消化のまま残っている。

## 前提: WordPressが脆弱になりがちな理由への対抗設計原則

WordPressコア自体は比較的安全だが、実世界の脆弱性の大半は以下から来る。xordpressでは全フェーズ共通の原則として対抗する。

1. **出力は用途ごとに毎回エスケープする**。テーマエンジンのautoescapeはデフォルトON、意図的に生HTMLを許可する箇所(投稿body等)は明示的にopt-inする形にし、「うっかり漏れる」ことを構造的に防ぐ。
2. **プラグインはcapability-basedサンドボックス**(Wasm/Extism、デフォルト拒否・明示的許可のみ)。WPの脆弱性の圧倒的多数はサードパーティプラグイン/テーマ由来であり、プラグイン導入がシステム全体の攻撃面にならないようホスト側で権限を仲介する。
3. **アップロードファイルは実行不可な場所に配置**、拡張子allowlist+マジックバイト検証+画像は再エンコードしてメタデータ/ペイロードを除去する。
4. **認証は最初からレート制限・2FA対応**を組み込む(WPは標準で持たない)。
5. **CI上で依存関係の脆弱性スキャン(govulncheck, npm audit)を常時実行**し、後追い対応にしない。
6. **権限昇格系のバグを構造的に防ぐ**: capability-basedのロールモデルを採用し、「投稿者ロールなのに管理者権限のAPIが呼べる」という抜け穴をルーティング層で一元的に塞ぐ(WPの古いroleモデルより厳格に)。

## 優先順位付きバックログ

### P0 — 今すぐ塞ぐべき既存の穴

実装済みコードの調査で確認済みの実際の欠陥。規模が小さく今すぐ着手可能。

1. **テーマのautoescape有効化**: `internal/theme/theme.go`が`liquid.Engine`に`SetAutoEscapeReplacer`を設定していないため、`{{ post.title }}`等が生HTMLとして出力される(実機で`<script>`注入を再現確認済み)。`title`/`site.name`等プレーンテキスト項目はエスケープし、`body`(意図的に生HTML許可、DBスキーマコメントに明記済み)のみ明示的に`{{ post.body | raw }}`的なopt-inにする。
2. **パスワード変更・リセット機能の追加**: 現状、初回セットアップ以降は管理者パスワードを変更する手段がAPIレベルで皆無。忘れたら詰む。
3. **ログイン試行回数制限・一時ロックアウト**: `internal/auth/`にブルートフォース対策が無い。
4. **401レスポンス形式の統一**: `AuthMiddleware`(プレーンテキスト)とハンドラ内部(`{"error":...}`)の不一致(design.md/admin-spa側で既知として記録済み、フロントは吸収済みだがバックエンド自体は未修正)。

### P1 — マルチユーザー・権限モデル(WP相当の土台)

5. 複数管理者ユーザーの追加/削除/一覧(API+管理画面UI)
6. ロール・権限モデル(Administrator/Editor/Author/Contributor相当、capability-based)
7. 2FA(TOTP)対応
8. セッション管理(アクティブセッション一覧・個別失効、パスワード変更時の全セッション失効)

### P2 — コンテンツ機能(CMSコア)

9. **メディアライブラリ/アップロード** — 歴史的に最大の攻撃面。設計時の必須要件: アップロードディレクトリからの実行不可(静的配信のみ)、Content-Type検証+マジックバイト検証、拡張子allowlist、画像は再エンコードしてメタデータ/悪意あるペイロードを除去、既存の`ResolveAsset`のパストラバーサル対策パターンを再利用。
10. カテゴリ・タグ(タクソノミー)
11. 固定ページ(投稿と別のツリー構造)
12. コメント機能 — **実装するか自体を要件として要確認**。実装する場合は既定モデレーションキュー・リンクのnofollow・レート制限・autoescape徹底が必須(WPのコメント欄はスパム/XSSの主要温床だった)。
13. リビジョン履歴(過去版の閲覧・復元。現行の`version`列は楽観的並行性制御用でリビジョン機能ではない)
14. 検索機能

### P3 — 表現力・UX

15. ブロックエディタ(D&D、docs/architecture.md記載のPhase 2予定)
16. テーマ切替・複数テーマ管理
17. メニュー・ウィジェット
18. SEO基盤(sitemap.xml, robots.txt, OGPタグ, meta description)
19. RSS/Atomフィード

### P4 — 運用・エコシステム(最も攻撃面が増える領域、特に慎重な設計が必要)

20. **プラグインシステム**(Wasm/Extism、ADR 0002で採用決定済み・未実装) — capability-based権限(プラグインごとに宣言的に許可するAPIを明示、デフォルト拒否)、プラグインごとに独立したサンドボックス(ファイルシステム/ネットワークアクセスをホスト側で仲介)、署名検証・公式レジストリ経由のみ、脆弱性DB連携。
21. 自動SSL(ACME、docs/architecture.md記載のPhase 2予定)
22. WPインポーター(WXR形式) — インポート処理のサニタイズ徹底(WPのXMLインポート系脆弱性の教訓)
23. バックアップ・エクスポート(Litestream等)
24. プラグイン・テーママーケットプレイス(Phase 3、審査プロセス必須)

### 横断的基盤(全フェーズと並行して積む)

25. CI/CDパイプライン構築(現状ローカルgitのみ。GitHub Actions等でgo test/go vet/govulncheck、npm audit/typecheck/testを毎PRで強制)
26. 依存関係脆弱性スキャンの定期実行(govulncheck, Dependabot等)
27. セキュリティヘッダー(CSP, X-Content-Type-Options, Referrer-Policy等)を公開サイト・管理画面双方に付与
28. 全APIエンドポイントへのレート制限
29. 監査ログ(誰が何をいつ変更したか)
30. セキュリティリグレッションテスト継続(今回のオープンリダイレクトのような回帰を継続的に検知する仕組み)

## 進め方

- P0(4項目)は規模が小さく、個別の`/design`無しで直接実装・検証する。
- P1以降は1項目(または関連する数項目のまとまり)ごとに`/design`→`/breakdown`→実装のサイクルを回す。まとめて一度に設計しない(手戻りが大きくなるため)。
- P2の12(コメント)着手前にユーザーへ「実装するか自体」を要確認。
- P4の20(プラグイン)は本ロードマップ中で最も攻撃面リスクが高い。着手時は必ず`/council`等で複数視点のセキュリティレビューを経ること。
