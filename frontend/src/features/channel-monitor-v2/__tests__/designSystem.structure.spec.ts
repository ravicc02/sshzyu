/**
 * Structure contracts: channel-monitor-v2 + studio shells must use project
 * design-system utility classes rather than isolated flat RGB skins.
 */
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const root = resolve(__dirname, '../../..')

function read(rel: string) {
  return readFileSync(resolve(root, rel), 'utf8')
}

describe('channel-monitor-v2 design system structure', () => {
  it('user ChannelStatus V2 stays a status-only board', () => {
    const src = read('views/user/ChannelStatusV2View.vue')
    expect(src).toContain('btn btn-secondary')
    expect(src).toContain('monitor-toolbar')
    expect(src).toContain('clearFilters')
    expect(src).toContain('GroupStatusCards')
    expect(src).toContain("'platform_group'")
    expect(src).not.toContain('sticky')
    expect(src).not.toContain('badge badge-warning')
    expect(src).not.toContain('summaryAria')
    expect(src).not.toContain('MonitorTrendChart')
    expect(src).not.toContain('RelayPulseMatrix')
    expect(src).not.toContain('MetricCell')
    expect(src).not.toContain('tab-active')
    // No page-level fixed min-width that forces viewport horizontal scroll
    expect(src).not.toMatch(/min-width:\s*980px/)
    expect(src).not.toMatch(/min-w-\[980px\]/)
  })

  it('RelayPulseMatrix uses card chrome, matrix scroll, and hover tooltips (no click modal)', () => {
    const src = read('features/channel-monitor-v2/RelayPulseMatrix.vue')
    expect(src).toContain('class="card')
    expect(src).toContain('card-header')
    expect(src).toContain('card-body')
    expect(src).toContain('matrix-scroll')
    expect(src).toMatch(/max-h-\[min\(42vh/)
    expect(src).toContain('overflow-auto')
    expect(src).toContain('pulse-tooltip')
    expect(src).toContain('rounded-3xl')
    expect(src).toContain('ring-1 ring-gray-900/5')
    expect(src).not.toContain('modal-overlay')
    expect(src).not.toContain('modal-content')
  })

  it('MetricCell uses stat-card utility', () => {
    const src = read('features/channel-monitor-v2/MetricCell.vue')
    expect(src).toContain('stat-card')
    expect(src).toContain('stat-label')
    expect(src).toContain('stat-value')
    expect(src).toContain('rounded-3xl')
  })

  it('MonitorTrendChart uses Ops chart shell tokens', () => {
    const src = read('features/channel-monitor-v2/MonitorTrendChart.vue')
    expect(src).toContain('class="card')
    expect(src).toContain('rounded-3xl')
    expect(src).toContain('ring-1 ring-gray-900/5')
    expect(src).toContain('EmptyState')
    expect(src).toContain('min-h-[360px]')
  })

  it('FilterMultiSelect uses rounded-xl input chrome and dropdown utility', () => {
    const src = read('features/channel-monitor-v2/FilterMultiSelect.vue')
    expect(src).toContain('rounded-xl')
    expect(src).toContain('dropdown')
    expect(src).toContain('dropdown-item')
  })

  it('MonitorSettingsPanel uses page-header, card, btn-primary, tabs', () => {
    const src = read('features/channel-monitor-v2/MonitorSettingsPanel.vue')
    expect(src).toContain('page-header')
    expect(src).toContain('btn btn-primary')
    expect(src).toContain('class="card')
    expect(src).toContain('tab-active')
    expect(src).toMatch(/max-h-\[min\(40vh/)
  })

  it('admin ChannelMonitorView uses the unified settings-first monitor layout', () => {
    const src = read('views/admin/ChannelMonitorView.vue')
    expect(src).toContain('MonitorSettingsPanel')
    expect(src).toContain('#probe-monitor')
    expect(src).toContain('ActiveProbeGroupList')
    expect(src).toContain('settings.probeTitle')
    expect(src).not.toContain('previewCards')
    expect(src).not.toContain('adminMonitorTab')
    expect(src).not.toContain('role="tablist"')
  })
})
