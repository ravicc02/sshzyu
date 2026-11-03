import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import AccountManagementTabs from '../AccountManagementTabs.vue'

describe('account inventory navigation', () => {
  it('navigates between platform and hosted accounts and exposes the current page', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/admin/accounts', component: { template: '<div />' } },
        { path: '/admin/accounts/hosted', component: { template: '<div />' } }
      ]
    })
    await router.push('/admin/accounts')
    await router.isReady()
    const i18n = createI18n({ legacy: false, locale: 'zh', messages: { zh: { nav: { accounts: '账号管理', platformAccounts: '平台账号', hostedAccounts: '托管账号' } } } })
    const wrapper = mount(AccountManagementTabs, { props: { hosted: false }, global: { plugins: [router, i18n] } })
    expect(wrapper.get('a[href="/admin/accounts"]').attributes('aria-current')).toBe('page')
    await wrapper.get('a[href="/admin/accounts/hosted"]').trigger('click')
    await router.isReady()
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/admin/accounts/hosted')
    await wrapper.setProps({ hosted: true })
    expect(wrapper.get('a[href="/admin/accounts/hosted"]').attributes('aria-current')).toBe('page')
    expect(wrapper.get('a[href="/admin/accounts"]').attributes('aria-current')).toBeUndefined()
    wrapper.unmount()
  })
})
