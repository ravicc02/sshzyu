import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import AgentAttachmentCard from '../AgentAttachmentCard.vue'
import { supportedAgentFile } from '@/utils/agentAttachments'
describe('Agent attachment cards', () => {
  it('shows image thumbnails before sending and exposes preview/removal', async () => {
    const w = mount(AgentAttachmentCard, { props: { compact: true, removable: true, asset: { id: 'a', filename: '参考.png', mime_type: 'image/png', byte_size: 2048, url: 'blob:image' } } })
    expect(w.get('img').attributes('src')).toBe('blob:image'); expect(w.text()).toBe(''); expect(w.find('figcaption').exists()).toBe(false); expect(w.find('.image-overlay').exists()).toBe(true)
    await w.get('[aria-label=预览图片]').trigger('click'); expect(w.emitted('preview')).toHaveLength(1)
    await w.get('[aria-label="移除附件 参考.png"]').trigger('click'); expect(w.emitted('remove')).toHaveLength(1); w.unmount()
  })
  it('keeps sent images thumbnail-only with preview and no removal or text actions', async () => {
    const w = mount(AgentAttachmentCard, { props: { canReference: true, asset: { id: 'a', filename: '参考.png', mime_type: 'image/png', byte_size: 2048, url: 'blob:image' } } })
    expect(w.text()).toBe('')
    expect(w.find('figcaption').exists()).toBe(false)
    expect(w.find('.image-remove').exists()).toBe(false)
    expect(w.find('a').exists()).toBe(false)
    await w.get('[aria-label=预览图片]').trigger('click')
    expect(w.emitted('preview')).toHaveLength(1)
    w.unmount()
  })
  it('shows document name, format and size without attempting image decoding', async () => {
    const w = mount(AgentAttachmentCard, { props: { asset: { id: 'a', filename: '收入.xlsx', mime_type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', byte_size: 1536, url: 'blob:sheet' } } })
    expect(w.find('img').exists()).toBe(false); expect(w.text()).toContain('收入.xlsx'); expect(w.text()).toContain('XLSX'); expect(w.text()).toContain('1.5 KB')
    await w.get('[aria-label=预览文件]').trigger('click'); expect(w.emitted('preview')).toHaveLength(1); expect(w.get('a').attributes('download')).toBe('收入.xlsx'); w.unmount()
  })
  it('supports common document formats but excludes executable or HTML uploads', () => {
    expect(supportedAgentFile(new File(['data'], '报告.CSV'))).toBe(true)
    expect(supportedAgentFile(new File(['data'], '年度报表.xlsx'))).toBe(true)
    expect(supportedAgentFile(new File(['data'], 'page.html'))).toBe(false)
    expect(supportedAgentFile(new File(['data'], 'setup.exe'))).toBe(false)
  })
  it('keeps the expired filename and requests reupload instead of offering stale downloads', () => {
    const w = mount(AgentAttachmentCard, { props: { asset: { id: 'expired', filename: '参考模板.png', mime_type: 'image/png', status: 'expired', content_available: false } } })
    expect(w.text()).toContain('参考模板.png'); expect(w.text()).toContain('重新上传')
    expect(w.find('img').exists()).toBe(false); expect(w.find('a').exists()).toBe(false)
    expect(w.findAll('button')).toHaveLength(0); w.unmount()
  })

})
