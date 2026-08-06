// バックエンドの検証ルールをフロントエンドにミラーする定数群。
// 参照: docs/plans/admin-spa/design.md §7
//
// サーバー往復なしに即時フィードバックを出すためのミラーであり、
// 最終的な正当性判定はサーバー側が行う(ここでの合格がAPI成功を保証しない)。

export const USERNAME_PATTERN = /^[A-Za-z0-9_-]{3,32}$/

export const PASSWORD_MIN_LENGTH = 8

export const SITE_NAME_MIN_LENGTH = 1
export const SITE_NAME_MAX_LENGTH = 100

export const SLUG_PATTERN = /^[a-z0-9]+(-[a-z0-9]+)*$/
export const RESERVED_SLUGS = ['api', 'admin', 'assets'] as const

export const TITLE_MIN_LENGTH = 1
export const TITLE_MAX_LENGTH = 200

export const BODY_MIN_LENGTH = 1
export const BODY_MAX_BYTES = 1024 * 1024 // 1MB

export function isValidUsername(value: string): boolean {
  return USERNAME_PATTERN.test(value)
}

export function isValidPassword(value: string): boolean {
  return value.length >= PASSWORD_MIN_LENGTH
}

export function isValidSiteName(value: string): boolean {
  return value.length >= SITE_NAME_MIN_LENGTH && value.length <= SITE_NAME_MAX_LENGTH
}

export function isValidSlug(value: string): boolean {
  if (!SLUG_PATTERN.test(value)) {
    return false
  }
  return !RESERVED_SLUGS.includes(value as (typeof RESERVED_SLUGS)[number])
}

export function isValidTitle(value: string): boolean {
  return value.length >= TITLE_MIN_LENGTH && value.length <= TITLE_MAX_LENGTH
}

export function isValidBody(value: string): boolean {
  const byteLength = new TextEncoder().encode(value).length
  return byteLength >= BODY_MIN_LENGTH && byteLength <= BODY_MAX_BYTES
}
