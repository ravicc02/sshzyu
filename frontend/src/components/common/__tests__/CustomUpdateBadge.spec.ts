import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CustomUpdateBadge from '../CustomUpdateBadge.vue'
import type { CustomRelease, UpdateOperation } from '@/api/admin/customUpdate'

const mocks = vi.hoisted(() => ({
  prepare: vi.fn(), status: vi.fn(), activate: vi.fn(), cancel: vi.fn(), list: vi.fn(), fetchVersion: vi.fn()
}))

vi.mock('@/stores', () => ({ useAppStore: () => ({ fetchVersion: mocks.fetchVersion }) }))
vi.mock('vue-i18n', async (loadActual) => ({
  ...await loadActual<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ visible: { value: false }, run: (callback: () => Promise<unknown>) => callback() }),
  isStepUpCancelled: () => false
}))
vi.mock('@/api/admin/customUpdate', async (loadActual) => ({
  ...await loadActual<typeof import('@/api/admin/customUpdate')>(),
  prepareCustomUpdate: mocks.prepare, getCustomOperation: mocks.status, activateCustomUpdate: mocks.activate,
  cancelCustomUpdate: mocks.cancel, listCustomReleases: mocks.list
}))

const target: CustomRelease = {
  release_id: 1,
  manifest_hash: 'b'.repeat(64),
  manifest: { version: '0.2.13-r3', repository: 'ravicc02/sshzyu', source_sha: 'c'.repeat(40), upstream: { tag: 'v0.2.13', commit: 'd'.repeat(40) }, migrations: [] }
}
const ready: UpdateOperation = {
  id: 'a'.repeat(32), kind: 'update', stage: 'ready', manifest_hash: target.manifest_hash, target: target.manifest,
  pending_migrations: [{ filename: '246_new.sql', checksum: 'e'.repeat(64), description: 'New optional field', risk: 'backward-compatible', non_transactional: false }]
}
const info = { current_version: '0.2.13-r2', latest_version: '0.2.13-r3', has_update: true, build_type: 'release', cached: false, check_status: 'verified', can_update: true, custom_release: target }

function render(versionInfo = info) {
  return mount(CustomUpdateBadge, {
    props: { version: versionInfo.current_version, info: versionInfo },
    global: { stubs: {
      BaseDialog: { props: ['show'], template: '<section v-if="show"><slot /></section>' },
      TotpStepUpDialog: true,
      Teleport: true
    } }
  })
}

describe('custom update user flow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mocks.fetchVersion.mockResolvedValue(info)
    mocks.list.mockResolvedValue([target])
    mocks.prepare.mockResolvedValue({ ...ready, stage: 'queued' })
    mocks.status.mockResolvedValue(ready)
    mocks.activate.mockResolvedValue({ ...ready, stage: 'completed' })
  })

  it('separates preparation from explicit migration and downtime confirmation', async () => {
    const wrapper = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    const prepare = wrapper.findAll('button').find(button => button.text() === 'customUpdate.prepare')!
    await prepare.trigger('click')
    await flushPromises()
    expect(mocks.prepare).toHaveBeenCalledOnce()
    expect(mocks.activate).not.toHaveBeenCalled()
    const activate = wrapper.findAll('button').find(button => button.text() === 'customUpdate.activate')!
    expect(activate.attributes('disabled')).toBeDefined()
    const consent = wrapper.findAll('input[type="checkbox"]')
    await consent[0].setValue(true)
    expect(activate.attributes('disabled')).toBeDefined()
    await consent[1].setValue(true)
    expect(activate.attributes('disabled')).toBeUndefined()
    mocks.status.mockResolvedValue({ ...ready, stage: 'completed' })
    await activate.trigger('click')
    await flushPromises()
    expect(mocks.activate).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('customUpdate.stages.completed')
    expect(wrapper.text()).not.toContain('weishaw/sub2api')
    wrapper.unmount()
  })

  it('does not present an unavailable source as already up to date', async () => {
    const unavailable = { ...info, check_status: 'unknown', can_update: false, has_update: false }
    mocks.fetchVersion.mockResolvedValue(unavailable)
    const wrapper = render(unavailable)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('customUpdate.notConfigured')
    expect(mocks.list).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('version.upToDate')
    wrapper.unmount()
  })
})
