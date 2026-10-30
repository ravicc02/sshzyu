import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

const here = dirname(fileURLToPath(import.meta.url))
const read = (path: string) => readFileSync(resolve(here, path), 'utf8')

describe('Prompt Audit integration surface', () => {
  it('registers the unified security-audit route and redirects the legacy prompt page', () => {
    const router = read('../../../router/index.ts')
    expect(router).toContain("path: '/admin/risk-control'")
    const route = router.slice(router.indexOf("path: '/admin/risk-control'"), router.indexOf("path: '/admin/prompt-audit'"))
    expect(route).toContain('SecurityAuditView.vue')
    expect(route).toContain('requiresAuth: true')
    expect(route).toContain('requiresAdmin: true')
    expect(route).toContain('requiresRiskControl: true')
    expect(router).toContain("path: '/admin/prompt-audit'")
    expect(router).toContain("tab: 'model'")
  })

  it('keeps one security-audit entry in the sidebar', () => {
    const sidebar = read('../../../components/layout/AppSidebar.vue')
    const entry = sidebar.slice(sidebar.indexOf("path: '/admin/risk-control'"), sidebar.indexOf("path: '/admin/redeem'"))
    expect(entry).toContain("label: t('nav.securityAudit')")
    expect(entry).not.toContain("path: '/admin/prompt-audit'")
  })

  it('keeps Prompt Audit locale trees symmetric and all operational controls named', () => {
    expect(Object.keys(zh.admin.promptAudit)).toEqual(Object.keys(en.admin.promptAudit))
    expect(zh.nav.securityAudit).toBeTruthy()
    expect(en.nav.securityAudit).toBeTruthy()
    const endpoint = read('../components/EndpointPool.vue')
    const events = read('../components/EventWorkspace.vue')
    expect(endpoint).toContain('aria-label')
    expect(events).toContain('aria-label')
    expect(events).toContain('overflow-x-auto')
    expect(events).toContain('sm:grid-cols-2')
  })
})
