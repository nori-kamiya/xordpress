// サーバーの固定エラー文字列 → 日本語UI文言の対応表。
// 参照: docs/plans/admin-spa/design.md §7
//
// 対応表にない文字列(未知のエラー)はそのまま素通しする。
// 握り潰さず汎用バナーに表示できるようにするため。

const ERROR_MESSAGES: Record<string, string> = {
  // internal/adminapi/*.go: writeError 経由(JSON)
  'invalid request body': '入力内容を確認してください。',
  'invalid username': 'ユーザー名の形式が正しくありません(3〜32文字の英数字・アンダースコア・ハイフン)。',
  'invalid password': 'パスワードは8文字以上で入力してください。',
  'invalid site_name': 'サイト名は1〜100文字で入力してください。',
  'invalid title': 'タイトルは1〜200文字で入力してください。',
  'invalid body': '本文を入力してください(最大1MB)。',
  'invalid slug': 'スラッグの形式が正しくありません(英数字とハイフンのみ、予約語は使用できません)。',
  'invalid limit or offset': 'ページネーションの指定が正しくありません。',
  'invalid id': 'IDの形式が正しくありません。',
  'missing or invalid If-Match header': '編集内容が古い可能性があります。ページを再読み込みしてください。',
  'setup already completed': 'セットアップ済みです。ログインしてください。',
  'slug already exists': 'このスラッグは既に使用されています。',
  'post not found': '記事が見つかりません。',
  'version mismatch': '他の変更と競合しました。最新の内容を読み込み直してください。',
  'request body too large': '本文が大きすぎます(最大1MB)。',
  'internal error': 'サーバーエラーが発生しました。しばらくしてから再度お試しください。',
  'invalid new_password': '新しいパスワードは8文字以上で入力してください。',
  'current password is incorrect': '現在のパスワードが正しくありません。',
  'too many failed login attempts, try again later': 'ログイン試行回数が多すぎます。しばらくしてから再度お試しください。',

  // internal/adminapi/users.go
  'invalid role': 'ロールの指定が正しくありません。',
  'username already taken': 'このユーザー名は既に使用されています。',
  'cannot change your own role': '自分自身のロールは変更できません。',
  'not found': '対象が見つかりません。',
  'cannot remove the last administrator': '最後の管理者は削除・降格できません。',
  'reassign_to is required': '記事の引き継ぎ先ユーザーを指定してください。',
  'cannot delete your own account': '自分自身のアカウントは削除できません。',
  'reassign_to cannot equal the deleted user': '引き継ぎ先には削除対象以外のユーザーを指定してください。',
  'reassign_to user not found': '引き継ぎ先のユーザーが見つかりません。',

  // internal/adminapi/totp.go, session_mfa.go
  'incorrect password': 'パスワードが正しくありません。',
  '2FA is already enabled; disable it before re-enrolling': '2段階認証は既に有効です。再登録する前に無効化してください。',
  'invalid code': 'コードが正しくありません。',
  'no pending enrollment': '登録中の2段階認証がありません。',
  'too many failed 2FA attempts, try again later': '2段階認証の試行回数が多すぎます。しばらくしてから再度お試しください。',

  // internal/auth/middleware.go: http.Error 経由(プレーンテキスト、apiFetchが末尾の改行をtrim済み)
  unauthorized: '認証が必要です。再度ログインしてください。',
  forbidden: 'この操作は許可されていません。',
  'unsupported content type': 'リクエストの形式が正しくありません。',
}

/**
 * サーバーの固定エラー文字列を日本語UI文言に変換する。
 * 対応表にない文字列はそのまま返す(未知のエラーでも握り潰さない)。
 */
export function toUserMessage(serverMessage: string): string {
  return ERROR_MESSAGES[serverMessage] ?? serverMessage
}
