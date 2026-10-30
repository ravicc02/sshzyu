import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import SecurityAuditView from '../SecurityAuditView.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function createTestRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/admin/risk-control', component: SecurityAuditView },
      { path: '/admin/risk-control/config', component: { template: '<div />' } },
    ],
  })
}

describe('SecurityAuditView', () => {
  it('uses one workspace for content moderation and prompt audit tabs', async () => {
    const router = createTestRouter()
    await router.push('/admin/risk-control?tab=model')
    await router.isReady()
    const wrapper = mount(SecurityAuditView, {
      global: {
        plugins: [router],
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          PromptFilterRecordsView: { template: '<div data-test="filter-panel" />' },
          PromptAuditView: { template: '<div data-test="model-panel" />' },
          SecurityAuditOverview: { template: '<div data-test="audit-overview" />' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.get('[data-test="security-audit-tab-filter"]').text()).toContain('admin.securityAudit.tabs.filterRecords')
    expect(wrapper.get('[data-test="security-audit-tab-model"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-test="audit-overview"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="model-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="filter-panel"]').exists()).toBe(false)

    await wrapper.get('[data-test="security-audit-tab-filter"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.tab).toBe('filter')
    expect(wrapper.find('[data-test="filter-panel"]').exists()).toBe(true)

    await wrapper.get('[data-test="security-audit-settings"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/admin/risk-control/config')
  })
})
