import { Blob as NodeBlob } from 'node:buffer'
import { describe, expect, it, vi } from 'vitest'
import { strFromU8, strToU8, unzipSync, zipSync } from 'fflate'
import {
  batchImageZipMergeLimits,
  buildBatchImageConfigByCustomId,
  mergeBatchImageZips,
  type BatchImageZipInput,
} from '../mergeBatchImageZips'

function sourceZip(
  batchId: string,
  files: Array<{ custom_id: string; image_index: number; filename: string; data: string; mime_type?: string }> = [],
  errors: Array<{ custom_id: string; code: string; message: string }> | null = [],
  extra: Record<string, Uint8Array> = {},
  overrides: Record<string, unknown> = {},
): Blob {
  const entries: Record<string, Uint8Array> = {}
  for (const file of files) entries[file.filename] = strToU8(file.data)
  const successes = new Set(files.map(file => file.custom_id)).size
  const failures = errors?.length ?? 0
  entries['manifest.json'] = strToU8(JSON.stringify({ batch_id: batchId, model: 'gpt-image-1', item_count: successes + failures,
    success_count: successes, fail_count: failures,
    files: files.map(({ data: _data, ...file }) => ({ ...file, mime_type: file.mime_type ?? 'image/png' })),
    ...overrides,
  }))
  entries['errors.json'] = strToU8(JSON.stringify(errors))
  Object.assign(entries, extra)
  return new NodeBlob([zipSync(entries, { level: 6 })], { type: 'application/zip' }) as Blob
}

const image = (custom_id: string, filename: string, image_index = 0, data = 'real image bytes') =>
  ({ custom_id, filename, image_index, data })

async function unzipBlob(blob: Blob): Promise<Record<string, Uint8Array>> {
  const data = typeof blob.arrayBuffer === 'function' ? await blob.arrayBuffer() : await new Promise<ArrayBuffer>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(reader.result as ArrayBuffer)
    reader.onerror = () => reject(reader.error)
    reader.readAsArrayBuffer(blob)
  })
  return unzipSync(new Uint8Array(data))
}

function input(batchId: string, zip: Blob): BatchImageZipInput {
  return { batchId, zip }
}

// Mutates a real Deflate archive's central directory to emulate false size declarations.
async function corruptCentralUncompressed(blob: Blob, declared: number): Promise<Blob> {
  const data = new Uint8Array(await blob.arrayBuffer())
  const view = new DataView(data.buffer)
  const central = data.findIndex((_, i) => view.getUint32(i, true) === 0x02014b50)
  view.setUint32(central + 24, declared, true)
  return new NodeBlob([data]) as Blob
}

describe('mergeBatchImageZips', () => {
  it('records unavailable batches and their item mappings without inventing image files', async () => {
    const files = await unzipBlob(await mergeBatchImageZips([
      input('a', sourceZip('a', [image('one', 'images/one.png')])),
      { batchId: 'b', unavailable: { code: 'OUTPUT_DELETED', message: 'Expired output', model: 'gpt-image-2', itemCount: 2 },
        configByCustomId: buildBatchImageConfigByCustomId({ items: [{ custom_id: 'lost', prompt: 'two', output_count: 2 }] }) },
    ]))
    expect(Object.keys(files).filter(name => name.startsWith('images/'))).toHaveLength(1)
    const errors = JSON.parse(strFromU8(files['errors.json']))
    expect(errors.map((error: { custom_id: string }) => error.custom_id)).toEqual(['lost_01', 'lost_02'])
    expect(errors[0]).toMatchObject({ source_batch_id: 'b', code: 'OUTPUT_DELETED', config: { prompt: 'two', output_count: 2 } })
    expect(JSON.parse(strFromU8(files['manifest.json']))).toMatchObject({ success_count: 1, fail_count: 2 })
  })

  it('expands exact output IDs and preserves configuration for both images and errors', async () => {
    const mapping = buildBatchImageConfigByCustomId({ items: [{ custom_id: ' img_001 ', prompt: ' draw ', output_count: 2 }] })
    expect(Object.keys(mapping)).toEqual(['img_001_01', 'img_001_02'])
    const files = await unzipBlob(await mergeBatchImageZips([
      { batchId: 'a', zip: sourceZip('a', [image('img_001_01', 'images/a.png')], [{ custom_id: 'img_001_02', code: 'FAILED', message: 'failed' }]), configByCustomId: mapping },
      input('b', sourceZip('b')),
    ]))
    expect(JSON.parse(strFromU8(files['manifest.json'])).files[0].config).toEqual({ prompt: 'draw', output_count: 2 })
    expect(JSON.parse(strFromU8(files['errors.json']))[0].config).toEqual({ prompt: 'draw', output_count: 2 })
    expect(buildBatchImageConfigByCustomId({ items: [{ custom_id: 'natural_01', prompt: 'one' }, { custom_id: '', prompt: 'two', output_count: 0 }] })).toEqual({
      natural_01: { prompt: 'one', output_count: 1 }, item_000002: { prompt: 'two', output_count: 1 },
    })
    expect(() => buildBatchImageConfigByCustomId({ items: [{ custom_id: 'a', prompt: 'one', output_count: 2 }, { custom_id: 'a_01', prompt: 'two' }] })).toThrow('duplicate custom_id')
    expect(Object.hasOwn(buildBatchImageConfigByCustomId({ items: [{ custom_id: '__proto__', prompt: 'safe' }] }), '__proto__')).toBe(true)
  })

  it('preserves missing-result errors from legacy zero-image exports without accepting unlisted files', async () => {
    const errors = [{ custom_id: 'missing', code: 'RESULT_MISSING', message: 'missing output' }]
    const empty = sourceZip('a', [], errors, {}, { files: null, success_count: 1, fail_count: 0, item_count: 1 })
    const files = await unzipBlob(await mergeBatchImageZips([input('a', empty), input('b', sourceZip('b', [], null, {}, { files: null }))]))
    expect(JSON.parse(strFromU8(files['manifest.json'])).success_count).toBe(0)
    expect(JSON.parse(strFromU8(files['errors.json']))).toEqual([{ source_batch_id: 'a', ...errors[0] }])
    await expect(mergeBatchImageZips([input('a', sourceZip('a', [], [], { 'images/orphan.png': strToU8('orphan') }, { files: null })), input('b', sourceZip('b'))])).rejects.toThrow('unlisted')
    await expect(mergeBatchImageZips([input('a', sourceZip('a', [], [], {}, { fail_count: 1 })), input('b', sourceZip('b'))])).rejects.toThrow('failure records are incomplete')
  })
  it('merges real compressed ZIPs with source mapping, failure records and unique images', async () => {
    const first = sourceZip('batch-a', [image('same', 'images/same.png'), { ...image('multi', 'images/multi_2.jpg', 1, 'jpeg'), mime_type: 'image/jpeg' }],
      [{ custom_id: 'bad', code: 'SAFETY_BLOCKED', message: 'blocked' }])
    const second = sourceZip('batch-b', [image('same', 'images/same.png', 0, 'other image')],
      [{ custom_id: 'bad', code: 'PROVIDER_FAILED', message: 'failed' }])
    const a = { batchId: 'batch-a', zip: first, config: { image_size: '1K', aspect_ratio: '1:1' },
      configByCustomId: { same: { prompt: 'draw same' }, bad: { prompt: 'blocked input' } } }
    const b = { batchId: 'batch-b', zip: second, config: { image_size: '2K' } }
    const out = await mergeBatchImageZips([a, b, a])
    const files = await unzipBlob(out)
    expect(Object.keys(files).sort()).toEqual(['errors.json', 'images/000001.png', 'images/000002.jpg', 'images/000003.png', 'manifest.json'])
    expect(strFromU8(files['images/000001.png'])).toBe('real image bytes')
    expect(strFromU8(files['images/000002.jpg'])).toBe('jpeg')
    expect(strFromU8(files['images/000003.png'])).toBe('other image')
    const manifest = JSON.parse(strFromU8(files['manifest.json']))
    expect(manifest.format).toBe('batch-image-merged-v1')
    expect(manifest.success_count).toBe(3)
    expect(manifest.fail_count).toBe(2)
    expect(manifest.sources).toHaveLength(2)
    expect(manifest.sources[0]).toMatchObject({ batch_id: 'batch-a', config: { image_size: '1K', aspect_ratio: '1:1' } })
    expect(manifest.files).toEqual([
      { source_batch_id: 'batch-a', custom_id: 'same', image_index: 0, filename: 'images/000001.png', mime_type: 'image/png', config: { prompt: 'draw same' } },
      { source_batch_id: 'batch-a', custom_id: 'multi', image_index: 1, filename: 'images/000002.jpg', mime_type: 'image/jpeg' },
      { source_batch_id: 'batch-b', custom_id: 'same', image_index: 0, filename: 'images/000003.png', mime_type: 'image/png' },
    ])
    expect(JSON.parse(strFromU8(files['errors.json']))).toEqual([
      { source_batch_id: 'batch-a', custom_id: 'bad', code: 'SAFETY_BLOCKED', message: 'blocked', config: { prompt: 'blocked input' } },
      { source_batch_id: 'batch-b', custom_id: 'bad', code: 'PROVIDER_FAILED', message: 'failed' },
    ])
  })

  it('accepts null errors.json produced by successful Go ZIP exports', async () => {
    const first = sourceZip('success-a', [image('a', 'images/a.png')], [], { 'errors.json': strToU8('null') })
    const second = sourceZip('success-b', [image('b', 'images/b.png')], [], { 'errors.json': strToU8('null') })
    const files = await unzipBlob(await mergeBatchImageZips([input('success-a', first), input('success-b', second)]))
    const manifest = JSON.parse(strFromU8(files['manifest.json']))
    expect(manifest.success_count).toBe(2)
    expect(manifest.fail_count).toBe(0)
    expect(JSON.parse(strFromU8(files['errors.json']))).toEqual([])
  })

  it('requires multiple inputs; never fetches any upstream endpoint', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    try {
      await expect(mergeBatchImageZips([input('a', sourceZip('a'))])).rejects.toThrow('at least two')
      await mergeBatchImageZips([input('a', sourceZip('a')), input('b', sourceZip('b'))])
      expect(fetchSpy).not.toHaveBeenCalled()
    } finally {
      fetchSpy.mockRestore()
    }
  })

  it('rejects same image identity with different content, and extra or missing images', async () => {
    const initial = sourceZip('a', [image('id', 'images/image.png')])
    const changed = sourceZip('a', [image('id', 'images/image.png', 0, 'different')])
    await expect(mergeBatchImageZips([input('a', initial), input('a', changed)])).rejects.toThrow('conflicting duplicate')
    await expect(mergeBatchImageZips([input('a', initial), input('b', sourceZip('b', [], [], { 'images/unlisted.png': strToU8('orphan') }))])).rejects.toThrow('unlisted')
    const missing = sourceZip('b', [image('missing', 'images/nope.png')])
    const entries = unzipSync(new Uint8Array(await missing.arrayBuffer()))
    delete entries['images/nope.png']
    const broken = new NodeBlob([zipSync(entries)]) as Blob
    await expect(mergeBatchImageZips([input('a', initial), input('b', broken)])).rejects.toThrow('missing manifest file')
  })

  it('rejects malicious path entries, central-header size forgeries, and corrupted deflate payload', async () => {
    const good = input('a', sourceZip('a'))
    const traversal = sourceZip('b', [], [], { 'images/../secret.png': strToU8('hidden') })
    await expect(mergeBatchImageZips([good, input('b', traversal)])).rejects.toThrow('unexpected ZIP path')
    const forged = await corruptCentralUncompressed(sourceZip('b', [image('id', 'images/id.png')]), 1)
    await expect(mergeBatchImageZips([good, input('b', forged)])).rejects.toThrow()
    const source = new Uint8Array(await sourceZip('b', [image('id', 'images/id.png')]).arrayBuffer())
    const central = source.findIndex((_, index) => index + 4 < source.length &&
      source[index] === 0x50 && source[index + 1] === 0x4b && source[index + 2] === 0x01 && source[index + 3] === 0x02)
    const firstFileData = 30 + new DataView(source.buffer).getUint16(26, true) + new DataView(source.buffer).getUint16(28, true)
    expect(firstFileData).toBeLessThan(central)
    source[firstFileData] ^= 0xff
    await expect(mergeBatchImageZips([good, input('b', new NodeBlob([source]) as Blob)])).rejects.toThrow()
  })

  it('enforces compressed, decompressed, entry, count, ratio, and export size limits', async () => {
    const a = input('a', sourceZip('a'))
    const imageZip = input('b', sourceZip('b', [image('id', 'images/id.png', 0, 'a'.repeat(1200))]))
    await expect(mergeBatchImageZips([a, imageZip], { limits: { maxInputZipBytes: 100 } })).rejects.toThrow('compressed size')
    await expect(mergeBatchImageZips([a, imageZip], { limits: { maxTotalInputBytes: a.zip.size + imageZip.zip.size - 1 } })).rejects.toThrow('total compressed size')
    await expect(mergeBatchImageZips([a, imageZip], { limits: { maxFiles: 4 } })).rejects.toThrow('too many ZIP files')
    await expect(mergeBatchImageZips([a, imageZip], { limits: { maxEntryBytes: 1000 } })).rejects.toThrow('decompression limit')
    await expect(mergeBatchImageZips([a, imageZip], { limits: { maxUncompressedBytes: 1000 } })).rejects.toThrow('decompression limit')
    await expect(mergeBatchImageZips([a, imageZip], { limits: { maxCompressionRatio: 2 } })).rejects.toThrow('decompression limit')
    await expect(mergeBatchImageZips([a, imageZip], { limits: { maxMetadataBytes: 120 } })).rejects.toThrow('metadata exceeds size limit')
    await expect(mergeBatchImageZips([a, imageZip], { limits: { maxExportBytes: 100 } })).rejects.toThrow('export size limit')
    expect(batchImageZipMergeLimits.maxExportBytes).toBeGreaterThan(0)
  })
})
