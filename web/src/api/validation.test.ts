import { describe, expect, it } from 'vitest'
import {
  BODY_MAX_BYTES,
  isValidBody,
  isValidPassword,
  isValidSiteName,
  isValidSlug,
  isValidTitle,
  isValidUsername,
} from './validation'

describe('isValidUsername', () => {
  it.each(['abc', 'user_name-1', 'A'.repeat(32)])('accepts %s', (value) => {
    expect(isValidUsername(value)).toBe(true)
  })

  it.each(['ab', 'A'.repeat(33), 'has space', 'invalid!'])('rejects %s', (value) => {
    expect(isValidUsername(value)).toBe(false)
  })
})

describe('isValidPassword', () => {
  it('accepts exactly the minimum length', () => {
    expect(isValidPassword('12345678')).toBe(true)
  })

  it('rejects one below the minimum length', () => {
    expect(isValidPassword('1234567')).toBe(false)
  })
})

describe('isValidSiteName', () => {
  it('accepts 1 and 100 chars', () => {
    expect(isValidSiteName('a')).toBe(true)
    expect(isValidSiteName('a'.repeat(100))).toBe(true)
  })

  it('rejects empty and 101 chars', () => {
    expect(isValidSiteName('')).toBe(false)
    expect(isValidSiteName('a'.repeat(101))).toBe(false)
  })
})

describe('isValidSlug', () => {
  it.each(['post', 'my-post-1'])('accepts %s', (value) => {
    expect(isValidSlug(value)).toBe(true)
  })

  it.each(['api', 'admin', 'assets'])('rejects reserved word %s', (value) => {
    expect(isValidSlug(value)).toBe(false)
  })

  it.each(['Post', '-leading', 'trailing-', 'double--hyphen', 'has space'])('rejects malformed %s', (value) => {
    expect(isValidSlug(value)).toBe(false)
  })
})

describe('isValidTitle', () => {
  it('accepts 1 and 200 chars', () => {
    expect(isValidTitle('a')).toBe(true)
    expect(isValidTitle('a'.repeat(200))).toBe(true)
  })

  it('rejects empty and 201 chars', () => {
    expect(isValidTitle('')).toBe(false)
    expect(isValidTitle('a'.repeat(201))).toBe(false)
  })
})

describe('isValidBody', () => {
  it('accepts 1 byte and exactly the max', () => {
    expect(isValidBody('a')).toBe(true)
    expect(isValidBody('a'.repeat(BODY_MAX_BYTES))).toBe(true)
  })

  it('rejects empty and one byte over the max', () => {
    expect(isValidBody('')).toBe(false)
    expect(isValidBody('a'.repeat(BODY_MAX_BYTES + 1))).toBe(false)
  })
})
