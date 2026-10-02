import { ReadableStream } from 'node:stream/web'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { downloadBatchImageZip, saveBlob } from '../batchImage'

// 只替换测试网络边界；生产函数使用真实 fetch、AbortController 和流读取。
afterEach(() => vi.restoreAllMocks())

describe('批量 ZIP 受限下载', () => {
  it('keeps the download URL alive while the browser starts reading the ZIP', () => {
    vi.useFakeTimers()
    const revoke = vi.fn()
    const originalCreate = URL.createObjectURL
    const originalRevoke = URL.revokeObjectURL
    URL.createObjectURL = vi.fn(() => 'blob:test-zip')
    URL.revokeObjectURL = revoke
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      expect(this.href).toBe('blob:test-zip')
      expect(this.download).toBe('result.zip')
      expect(this.isConnected).toBe(true)
      expect(revoke).not.toHaveBeenCalled()
    })
    try {
      saveBlob(new Blob(['zip']), 'result.zip')
      expect(click).toHaveBeenCalledOnce()
      expect(revoke).not.toHaveBeenCalled()
      vi.advanceTimersByTime(40_000)
      expect(revoke).toHaveBeenCalledWith('blob:test-zip')
    } finally {
      URL.createObjectURL = originalCreate
      URL.revokeObjectURL = originalRevoke
      vi.useRealTimers()
    }
  })

  it('在响应长度已超限时立即中止，不读取内容', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true, headers: new Headers({ 'Content-Length': '100' }),
    } as Response)
    await expect(downloadBatchImageZip('test-token', 'task/id', 10)).rejects.toThrow('大小限制')
    expect(fetchMock.mock.calls[0][1]?.signal?.aborted).toBe(true)
    expect(fetchMock.mock.calls[0][0]).toContain('task%2Fid/download')
  })

  it('没有 Content-Length 时仍检查累计流大小，并取消超限流', async () => {
    const cancel = vi.fn()
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new Uint8Array(6))
        controller.enqueue(new Uint8Array(6))
      }, cancel,
    })
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true, headers: new Headers(), body } as unknown as Response)
    await expect(downloadBatchImageZip('test-token', 'task', 10)).rejects.toThrow('大小限制')
    expect(cancel).toHaveBeenCalledOnce()
    expect(fetchMock.mock.calls[0][1]?.signal?.aborted).toBe(true)
    expect(body.locked).toBe(false)
  })

  it('只在完整流大小不超过预算后返回 ZIP', async () => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new Uint8Array([1, 2, 3]))
        controller.enqueue(new Uint8Array([4, 5]))
        controller.close()
      },
    })
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({ ok: true, headers: new Headers(), body } as unknown as Response)
    const blob = await downloadBatchImageZip('test-token', 'task', 5)
    expect(blob.size).toBe(5)
    expect(blob.type).toBe('application/zip')
    expect(body.locked).toBe(false)
  })
})
