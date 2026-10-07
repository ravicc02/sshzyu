import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

// Defect 3: step-up rejections used to fall through to the generic
// "update could not continue" copy, leaving the operator with no hint that 2FA
// must be enabled first. These keys must exist in both locales.
describe('custom update step-up rejection locales', () => {
  it('explains how to unblock STEP_UP_TOTP_NOT_ENABLED in zh', () => {
    expect(zh.customUpdate.errors.STEP_UP_TOTP_NOT_ENABLED).toContain('两步验证')
    expect(zh.customUpdate.errors.STEP_UP_TOTP_NOT_ENABLED).not.toBe('STEP_UP_TOTP_NOT_ENABLED')
  })

  it('explains how to unblock STEP_UP_TOTP_NOT_ENABLED in en', () => {
    expect(en.customUpdate.errors.STEP_UP_TOTP_NOT_ENABLED).toMatch(/two-step verification/i)
    expect(en.customUpdate.errors.STEP_UP_TOTP_NOT_ENABLED).not.toBe('STEP_UP_TOTP_NOT_ENABLED')
  })

  it('covers STEP_UP_ADMIN_API_KEY_FORBIDDEN in both locales', () => {
    expect(zh.customUpdate.errors.STEP_UP_ADMIN_API_KEY_FORBIDDEN).toBeTruthy()
    expect(en.customUpdate.errors.STEP_UP_ADMIN_API_KEY_FORBIDDEN).toBeTruthy()
  })
})
