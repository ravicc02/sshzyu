import { beforeEach, describe, expect, it, vi, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CustomUpdateBadge from '../CustomUpdateBadge.vue'
import { operationStorageKey, type CustomRelease, type CustomVersionInfo, type UpdateOperation } from '@/api/admin/customUpdate'
import { enableAutoUnmount } from '@vue/test-utils'

enableAutoUnmount(afterEach)

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

function render(versionInfo: CustomVersionInfo = info) {
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
    await wrapper.get('[data-test="check-custom-update"]').trigger('click')
    await flushPromises()
    const prepare = wrapper.get('[data-test="update-latest"]')
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

  it('refreshes installed metadata and shows only the latest version after checking', async () => {
    const newest = { ...target, manifest: { ...target.manifest, version: '0.2.13-r10' } }
    mocks.fetchVersion.mockResolvedValue({ ...info, current_version: '0.2.13-r4', commit: 'f'.repeat(40), latest_version: newest.manifest.version, custom_release: newest })
    mocks.list.mockResolvedValue([target, newest])
    const wrapper = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('0.2.13-r4')
    expect(wrapper.find('[data-test="latest-custom-version"]').exists()).toBe(false)
    await wrapper.get('[data-test="check-custom-update"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="latest-custom-version"]').text()).toContain('0.2.13-r10')
    expect(wrapper.find('select').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('0.2.13-r3')
  })

  it('does not restore an old terminal ALREADY_INSTALLED failure on entry', async () => {
    localStorage.setItem(operationStorageKey, ready.id)
    mocks.status.mockResolvedValue({ ...ready, stage: 'failed', error: 'ALREADY_INSTALLED' })
    const wrapper = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain('ALREADY_INSTALLED')
    expect(wrapper.find('[data-test="custom-operation"]').exists()).toBe(false)
    expect(localStorage.getItem(operationStorageKey)).toBeNull()
  })

  it('does not offer an update to the currently installed release', async () => {
    mocks.fetchVersion.mockResolvedValue({ ...info, current_version: target.manifest.version, has_update: false })
    const wrapper = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="check-custom-update"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('customUpdate.upToDate')
    expect(wrapper.find('[data-test="update-latest"]').exists()).toBe(false)
    expect(mocks.prepare).not.toHaveBeenCalled()
  })

  it('shows the deployed rollback version beside check updates, not unpublished history guesses', async () => {
    const rollback = { ...target, rollback_available: true, installed_at: '2026-10-03T00:00:00Z' }
    mocks.fetchVersion.mockResolvedValue({ ...info, current_version: '0.2.13-r4' })
    mocks.list.mockResolvedValue([{ ...target, manifest: { ...target.manifest, version: '0.2.13-r5' } }, rollback])
    const wrapper = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="check-custom-update"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="rollback-version"]').text()).toContain('0.2.13-r3')
    await wrapper.get('[data-test="rollback-custom-update"]').trigger('click')
    await flushPromises()
    expect(mocks.prepare).toHaveBeenCalledWith(rollback, 'rollback')
    expect(mocks.activate).not.toHaveBeenCalled()
  })

  it('continues to display manual intervention instead of discarding a safety-critical operation', async () => {
    localStorage.setItem(operationStorageKey, ready.id)
    mocks.status.mockResolvedValue({ ...ready, stage: 'manual_intervention', error: 'ACTIVATION_FAILED' })
    const wrapper = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('customUpdate.stages.manual_intervention')
    expect(localStorage.getItem(operationStorageKey)).toBe(ready.id)
  })

  it('handles a newly detected already-installed target without showing a raw error', async () => {
    mocks.fetchVersion.mockResolvedValueOnce(info).mockResolvedValueOnce(info)
      .mockResolvedValue({ ...info, current_version: target.manifest.version, has_update: false })
    mocks.status.mockResolvedValue({ ...ready, stage: 'failed', error: 'ALREADY_INSTALLED' })
    const wrapper = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="check-custom-update"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="update-latest"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('customUpdate.alreadyInstalled')
    expect(wrapper.text()).not.toContain('ALREADY_INSTALLED')
    expect(wrapper.find('[data-test="custom-operation"]').exists()).toBe(false)
    expect(wrapper.findAll('dd')[0].text()).toBe(target.manifest.version)
  })

  it('does not resume polling after unmounting while preparation is pending', async () => {
    let resolve!: (value: UpdateOperation) => void
    mocks.prepare.mockReturnValueOnce(new Promise<UpdateOperation>(result => { resolve = result }))
    const wrapper = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="check-custom-update"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="update-latest"]').trigger('click')
    wrapper.unmount()
    resolve(ready)
    await flushPromises()
    expect(mocks.status).not.toHaveBeenCalled()
  })

  it('does not resurrect a cancelled preparation when an earlier status response arrives', async () => {
    let resolve!: (value: UpdateOperation) => void
    mocks.prepare.mockResolvedValue(ready)
    mocks.status.mockReturnValueOnce(new Promise<UpdateOperation>(result => { resolve = result }))
    mocks.cancel.mockResolvedValue({ ...ready, stage: 'cancelled' })
    const wrapper = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="check-custom-update"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="update-latest"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'customUpdate.cancel')!.trigger('click')
    await flushPromises()
    resolve(ready)
    await flushPromises()
    expect(wrapper.text()).toContain('customUpdate.stages.cancelled')
    expect(wrapper.text()).not.toContain('customUpdate.stages.ready')
  })

  it('resumes the host active operation instead of a stale browser operation', async () => {
    localStorage.setItem(operationStorageKey, 'f'.repeat(32))
    mocks.fetchVersion.mockResolvedValue({ ...info, active_operation: ready })
    const wrapper = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(mocks.status).toHaveBeenCalledWith(ready.id)
    expect(mocks.status).not.toHaveBeenCalledWith('f'.repeat(32))
    expect(wrapper.text()).toContain('customUpdate.stages.ready')
    expect(mocks.prepare).not.toHaveBeenCalled()
    expect(mocks.activate).not.toHaveBeenCalled()
  })

  it('resumes a preparation created in another window during an update request race', async () => {
    mocks.fetchVersion.mockResolvedValueOnce(info).mockResolvedValueOnce(info)
      .mockResolvedValue({ ...info, active_operation: ready })
    mocks.prepare.mockRejectedValue({ error: 'UPDATE_IN_PROGRESS', message: 'The custom update operation could not proceed' })
    const wrapper = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="check-custom-update"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="update-latest"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('customUpdate.stages.ready')
    expect(wrapper.text()).not.toContain('The custom update operation could not proceed')
    expect(mocks.activate).not.toHaveBeenCalled()
  })
})
