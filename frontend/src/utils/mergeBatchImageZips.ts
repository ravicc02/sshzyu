import type { BatchImageSubmitRequest } from '@/api/batchImage'
import { Inflate, Zip, ZipPassThrough, strFromU8, strToU8 } from 'fflate'

/** Browser-side input/expansion/output caps. */
export const batchImageZipMergeLimits = {
  // Deliberately below the server's 512 MiB *per-request* maximum: this implementation
  // keeps the input archive, decompressed images, and output Blob in browser memory.
  maxInputs: 8,
  maxInputZipBytes: 64 * 1024 * 1024,
  maxTotalInputBytes: 128 * 1024 * 1024,
  maxFiles: 2048,
  maxEntryBytes: 64 * 1024 * 1024,
  maxUncompressedBytes: 128 * 1024 * 1024,
  maxCompressionRatio: 1000,
  maxMetadataBytes: 2 * 1024 * 1024,
  maxExportBytes: 128 * 1024 * 1024,
} as const

type Limits = typeof batchImageZipMergeLimits
type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue }
export type BatchImageMergeConfig = Record<string, JsonValue>

export interface BatchImageZipInput {
  batchId: string
  zip?: Blob
  unavailable?: { code: string; message: string; model: string; itemCount: number }
  /** Batch-wide configuration, preserved in manifest.sources. */
  config?: BatchImageMergeConfig
  /** Per-item configuration, preserved alongside the corresponding manifest file. */
  configByCustomId?: Record<string, BatchImageMergeConfig>
}

export interface BatchImageMergedManifest {
  format: 'batch-image-merged-v1'
  sources: Array<{ batch_id: string; model: string; item_count: number; success_count: number; fail_count: number; config?: BatchImageMergeConfig }>
  files: Array<{
    source_batch_id: string
    custom_id: string
    image_index: number
    filename: string
    mime_type: string
    config?: BatchImageMergeConfig
  }>
  success_count: number // Actual exported image files, not the source jobs' successful item counts.
  fail_count: number
}

export interface BatchImageMergedError {
  source_batch_id: string
  custom_id: string
  code: string
  message: string
  config?: BatchImageMergeConfig
}

type ZipEntry = { name: string; compressed: number; uncompressed: number; crc: number; method: number; start: number }
type SourceManifest = {
  batch_id: string
  model: string
  item_count: number
  success_count: number
  fail_count: number
  files: Array<{ custom_id: string; filename: string; mime_type: string; image_index: number }>
}
type SourceError = { custom_id: string; code: string; message: string }

function invalid(message: string): never {
  throw new Error(`Cannot merge batch image ZIPs: ${message}`)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function uint16(data: Uint8Array, position: number): number {
  return data[position]! | data[position + 1]! << 8
}

function uint32(data: Uint8Array, position: number): number {
  return (data[position]! | data[position + 1]! << 8 | data[position + 2]! << 16 | data[position + 3]! << 24) >>> 0
}

function checkedRange(data: Uint8Array, start: number, length: number, end = data.length): void {
  if (!Number.isSafeInteger(start) || !Number.isSafeInteger(length) || start < 0 || length < 0 || start + length > end) {
    invalid('invalid or truncated ZIP structure')
  }
}

function readName(bytes: Uint8Array, flags: number): string {
  if (!(flags & 0x800) && bytes.some((byte) => byte > 127)) invalid('non-UTF-8 ZIP filename')
  try {
    return new TextDecoder('utf-8', { fatal: true }).decode(bytes)
  } catch {
    return invalid('invalid UTF-8 ZIP filename')
  }
}

function parseEntries(data: Uint8Array, limits: Limits): Map<string, ZipEntry> {
  if (data.length < 22) invalid('invalid ZIP footer')
  let end = -1
  // ZIP comments may contain a fake footer; require its length to reach exactly EOF.
  for (let i = data.length - 22; i >= Math.max(0, data.length - 22 - 65535); i--) {
    if (uint32(data, i) === 0x06054b50 && i + 22 + uint16(data, i + 20) === data.length) {
      end = i
      break
    }
  }
  if (end < 0 || uint16(data, end + 4) || uint16(data, end + 6) ||
      uint16(data, end + 8) !== uint16(data, end + 10)) invalid('unsupported multi-disk or invalid ZIP')
  const count = uint16(data, end + 10)
  const centralSize = uint32(data, end + 12)
  const centralStart = uint32(data, end + 16)
  if (count === 0xffff || centralSize === 0xffffffff || centralStart === 0xffffffff) invalid('ZIP64 is not supported')
  if (count > limits.maxFiles) invalid('too many ZIP files')
  checkedRange(data, centralStart, centralSize, end)
  if (centralStart + centralSize !== end) invalid('invalid ZIP central directory')

  const entries = new Map<string, ZipEntry>()
  const ranges: Array<[number, number]> = []
  let offset = centralStart
  let declaredTotal = 0
  for (let i = 0; i < count; i++) {
    checkedRange(data, offset, 46, end)
    if (uint32(data, offset) !== 0x02014b50) invalid('invalid ZIP central directory')
    const flags = uint16(data, offset + 8)
    const method = uint16(data, offset + 10)
    const compressed = uint32(data, offset + 20)
    const uncompressed = uint32(data, offset + 24)
    const nameSize = uint16(data, offset + 28)
    const extraSize = uint16(data, offset + 30)
    const commentSize = uint16(data, offset + 32)
    const localOffset = uint32(data, offset + 42)
    if (flags & ~0x808 || (method !== 0 && method !== 8) || uint16(data, offset + 34) ||
        compressed === 0xffffffff || uncompressed === 0xffffffff || localOffset === 0xffffffff) {
      invalid('unsupported or encrypted ZIP entry')
    }
    if (method === 0 && compressed !== uncompressed) invalid('invalid stored ZIP entry size')
    checkedRange(data, offset, 46 + nameSize + extraSize + commentSize, end)
    const nameBytes = data.subarray(offset + 46, offset + 46 + nameSize)
    const name = readName(nameBytes, flags)
    if (!name || entries.has(name) || !['manifest.json', 'errors.json'].includes(name) &&
        (!name.startsWith('images/') || name.length <= 7 || name.slice(7).includes('/') || name.includes('\\') || name.includes('..'))) {
      invalid('duplicate or unexpected ZIP path')
    }
    if (uncompressed > limits.maxEntryBytes || uncompressed > Math.max(compressed, 1) * limits.maxCompressionRatio) {
      invalid('ZIP entry exceeds decompression limit')
    }
    declaredTotal += uncompressed
    if (declaredTotal > limits.maxUncompressedBytes) invalid('ZIP exceeds decompression limit')
    checkedRange(data, localOffset, 30, centralStart)
    if (uint32(data, localOffset) !== 0x04034b50 || uint16(data, localOffset + 6) !== flags ||
        uint16(data, localOffset + 8) !== method || uint16(data, localOffset + 26) !== nameSize) {
      invalid('ZIP local header disagrees with central directory')
    }
    const localExtra = uint16(data, localOffset + 28)
    const start = localOffset + 30 + nameSize + localExtra
    checkedRange(data, start, compressed, centralStart)
    if (!nameBytes.every((byte, index) => byte === data[localOffset + 30 + index])) {
      invalid('ZIP filename disagrees with local header')
    }
    if (!(flags & 8) && (uint32(data, localOffset + 14) !== uint32(data, offset + 16) ||
        uint32(data, localOffset + 18) !== compressed || uint32(data, localOffset + 22) !== uncompressed)) {
      invalid('ZIP sizes or checksum disagree with local header')
    }
    ranges.push([localOffset, start + compressed])
    entries.set(name, { name, compressed, uncompressed, crc: uint32(data, offset + 16), method, start })
    offset += 46 + nameSize + extraSize + commentSize
  }
  if (offset !== end) invalid('ZIP file count disagrees with central directory')
  ranges.sort((a, b) => a[0] - b[0])
  if (ranges.some((range, index) => index > 0 && range[0] < ranges[index - 1]![1])) invalid('overlapping ZIP entries')
  return entries
}

// fflate's streaming Inflate does not authenticate ZIP CRCs; verify them explicitly.
function crc32(crc: number, bytes: Uint8Array): number {
  for (const byte of bytes) {
    crc ^= byte
    for (let i = 0; i < 8; i++) crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0)
  }
  return crc
}

function extractEntry(data: Uint8Array, entry: ZipEntry, limits: Limits, total: { bytes: number }): Uint8Array {
  const chunks: Uint8Array[] = []
  let size = 0
  let crc = -1
  const emit = (chunk: Uint8Array) => {
    size += chunk.length
    total.bytes += chunk.length
    if (size > entry.uncompressed || size > limits.maxEntryBytes || total.bytes > limits.maxUncompressedBytes ||
        size > Math.max(entry.compressed, 1) * limits.maxCompressionRatio) invalid('ZIP decompression limit exceeded')
    crc = crc32(crc, chunk)
    chunks.push(chunk)
  }
  const compressed = data.subarray(entry.start, entry.start + entry.compressed)
  if (entry.method === 0) {
    emit(compressed)
  } else {
    const inflate = new Inflate((chunk: Uint8Array) => emit(chunk))
    // fflate allocates its output *before* calling ondata. Small input chunks bound
    // that transient allocation even if a forged ZIP advertises a tiny output.
    for (let offset = 0; offset < compressed.length; offset += 4 * 1024) {
      const next = Math.min(offset + 4 * 1024, compressed.length)
      inflate.push(compressed.subarray(offset, next), next === compressed.length)
    }
    if (!compressed.length) inflate.push(new Uint8Array(), true)
  }
  if (size !== entry.uncompressed || (crc ^ -1) >>> 0 !== entry.crc) invalid(`corrupt ZIP entry: ${entry.name}`)
  const result = new Uint8Array(size)
  let offset = 0
  for (const chunk of chunks) {
    result.set(chunk, offset)
    offset += chunk.length
  }
  return result
}

function parseJson(bytes: Uint8Array, label: string): unknown {
  try {
    return JSON.parse(strFromU8(bytes))
  } catch {
    return invalid(`invalid ${label}`)
  }
}

function nonnegativeCount(value: unknown): value is number {
  return Number.isSafeInteger(value) && (value as number) >= 0
}

function readManifest(raw: unknown, batchId: string): SourceManifest {
  // 兼容旧版 Go 空切片；后续条目数量校验仍拒绝未列出的图片。
  if (isRecord(raw) && raw.files === null) raw = { ...raw, files: [] }
  if (!isRecord(raw) || raw.batch_id !== batchId || typeof raw.model !== 'string' ||
      !nonnegativeCount(raw.item_count) || !nonnegativeCount(raw.success_count) ||
      !nonnegativeCount(raw.fail_count) || !Array.isArray(raw.files)) invalid('invalid or mismatched source manifest')
  for (const file of raw.files) {
    if (!isRecord(file) || typeof file.custom_id !== 'string' || !file.custom_id ||
        typeof file.filename !== 'string' || typeof file.mime_type !== 'string' ||
        !/^image\/[a-z0-9.+-]+$/i.test(file.mime_type) || !nonnegativeCount(file.image_index)) {
      invalid('invalid source manifest file')
    }
  }
  return raw as unknown as SourceManifest
}

function readErrors(raw: unknown): SourceError[] {
  // Go encodes a nil []batchImageZipError as JSON null when every item succeeds.
  if (raw === null) return []
  if (!Array.isArray(raw) || raw.some((error) => !isRecord(error) ||
      typeof error.custom_id !== 'string' || typeof error.code !== 'string' || typeof error.message !== 'string')) {
    invalid('invalid source errors')
  }
  return raw as SourceError[]
}

function boundedJson(value: unknown, maxBytes: number, label: string): string {
  let serialized: string | undefined
  try {
    serialized = JSON.stringify(value)
  } catch {
    invalid(`invalid ${label}`)
  }
  if (!serialized || strToU8(serialized).length > maxBytes) invalid(`${label} exceeds size limit`)
  return serialized
}

function copyConfig(value: BatchImageMergeConfig | undefined, maxBytes: number): BatchImageMergeConfig | undefined {
  if (value === undefined) return undefined
  if (!isRecord(value)) invalid('invalid configuration mapping')
  return JSON.parse(boundedJson(value, maxBytes, 'configuration mapping')) as BatchImageMergeConfig
}

/** 与后端 output_count 展开规则一致，不通过猜测后缀关联配置。 */
export function buildBatchImageConfigByCustomId(request: Pick<BatchImageSubmitRequest, 'items'>): Record<string, BatchImageMergeConfig> {
  const entries: Array<[string, BatchImageMergeConfig]> = []
  const seen = new Set<string>()
  for (const [index, item] of request.items.entries()) {
    const customId = item.custom_id.trim() || `item_${String(index + 1).padStart(6, '0')}`
    const count = item.output_count || 1
    if (!Number.isSafeInteger(count) || count < 1 || entries.length + count > batchImageZipMergeLimits.maxFiles) {
      invalid('invalid or excessive output_count in configuration mapping')
    }
    for (let repeat = 1; repeat <= count; repeat++) {
      const id = count > 1 ? `${customId}_${String(repeat).padStart(Math.max(2, String(count).length), '0')}` : customId
      if (seen.has(id)) invalid('duplicate custom_id in configuration mapping')
      seen.add(id)
      entries.push([id, { prompt: item.prompt.trim(), output_count: count }])
    }
  }
  return Object.fromEntries(entries)
}

function configForCustomId(configByCustomId: Record<string, BatchImageMergeConfig> | undefined, customId: string, maxBytes: number): BatchImageMergeConfig | undefined {
  return copyConfig(configByCustomId && Object.prototype.hasOwnProperty.call(configByCustomId, customId)
    ? configByCustomId[customId] : undefined, maxBytes)
}

function identity(batchId: string, customId: string, imageIndex: number): string {
  return JSON.stringify([batchId, customId, imageIndex])
}

function equalBytes(a: Uint8Array, b: Uint8Array): boolean {
  return a.length === b.length && a.every((byte, index) => byte === b[index])
}

/** Purely local operation. Rejects the entire merge (and returns no Blob) on any malformed input/limit. */
export async function mergeBatchImageZips(
  inputs: BatchImageZipInput[],
  options: { limits?: Partial<Limits> } = {},
): Promise<Blob> {
  const limits: Limits = { ...batchImageZipMergeLimits, ...options.limits }
  for (const key of Object.keys(batchImageZipMergeLimits) as Array<keyof Limits>) {
    if (!Number.isSafeInteger(limits[key]) || limits[key] <= 0 || limits[key] > batchImageZipMergeLimits[key]) {
      invalid(`invalid ${key} limit`)
    }
  }
  if (inputs.length < 2 || inputs.length > limits.maxInputs) invalid('select at least two and no more than the input limit')

  const manifest: BatchImageMergedManifest = {
    format: 'batch-image-merged-v1', sources: [], files: [], success_count: 0, fail_count: 0,
  }
  const errors: BatchImageMergedError[] = []
  const images: Uint8Array[] = []
  const byIdentity = new Map<string, number>()
  const sourceConfig = new Map<string, string>()
  const seenErrors = new Set<string>()
  const total = { bytes: 0 }
  let totalInputBytes = 0
  let totalFiles = 0
  let jsonBytes = 0
  for (const input of inputs) {
    if (input.unavailable) {
      if (!input.batchId?.trim() || input.zip || sourceConfig.has(input.batchId)) invalid('invalid unavailable source')
      const missing = input.unavailable
      if (!missing.code || !missing.message || !Number.isSafeInteger(missing.itemCount) || missing.itemCount < 0) invalid('invalid unavailable source metadata')
      const batchConfig = copyConfig(input.config, limits.maxMetadataBytes)
      const configByCustomId = input.configByCustomId
      if (configByCustomId !== undefined && !isRecord(configByCustomId)) invalid('invalid per-item configuration mapping')
      const customIds = Object.keys(configByCustomId || {})
      if (!customIds.length) customIds.push('*')
      if (customIds.length + errors.length > limits.maxFiles) invalid('too many unavailable items')
      for (const customId of customIds) {
        const itemConfig = configForCustomId(configByCustomId, customId, limits.maxMetadataBytes)
        errors.push({ source_batch_id: input.batchId, custom_id: customId, code: missing.code, message: missing.message,
          ...(itemConfig === undefined ? {} : { config: itemConfig }) })
      }
      manifest.sources.push({ batch_id: input.batchId, model: missing.model, item_count: missing.itemCount,
        success_count: 0, fail_count: missing.itemCount, ...(batchConfig === undefined ? {} : { config: batchConfig }) })
      sourceConfig.set(input.batchId, 'unavailable')
      continue
    }
    if (typeof input?.batchId !== 'string' || !input.batchId.trim() ||
        !input.zip || !Number.isSafeInteger(input.zip.size) || typeof input.zip.arrayBuffer !== 'function') invalid('invalid input')
    if (input.zip.size > limits.maxInputZipBytes) invalid('input ZIP exceeds compressed size limit')
    totalInputBytes += input.zip.size
    if (totalInputBytes > limits.maxTotalInputBytes) invalid('input ZIPs exceed total compressed size limit')
    const batchConfig = copyConfig(input.config, limits.maxMetadataBytes)
    const configByCustomId = input.configByCustomId
    if (configByCustomId !== undefined && !isRecord(configByCustomId)) invalid('invalid per-item configuration mapping')
    const configKey = boundedJson([batchConfig, configByCustomId], limits.maxMetadataBytes, 'configuration mapping')
    if (sourceConfig.has(input.batchId) && sourceConfig.get(input.batchId) !== configKey) invalid('conflicting source configuration')
    sourceConfig.set(input.batchId, configKey)

    const data = new Uint8Array(await input.zip.arrayBuffer())
    if (data.length !== input.zip.size || data.length > limits.maxInputZipBytes) invalid('input ZIP exceeds compressed size limit')
    const entries = parseEntries(data, limits)
    totalFiles += entries.size
    if (totalFiles > limits.maxFiles) invalid('too many ZIP files in total')
    const sourceManifestEntry = entries.get('manifest.json')
    const sourceErrorsEntry = entries.get('errors.json')
    if (!sourceManifestEntry || !sourceErrorsEntry) invalid('missing source manifest or errors')
    if (sourceManifestEntry.uncompressed > limits.maxMetadataBytes || sourceErrorsEntry.uncompressed > limits.maxMetadataBytes ||
        jsonBytes + sourceManifestEntry.uncompressed + sourceErrorsEntry.uncompressed > limits.maxMetadataBytes) {
      invalid('source metadata exceeds size limit')
    }
    const source = readManifest(parseJson(extractEntry(data, sourceManifestEntry, limits, total), 'manifest'), input.batchId)
    const sourceErrors = readErrors(parseJson(extractEntry(data, sourceErrorsEntry, limits, total), 'errors'))
    if (sourceErrors.length < source.fail_count) invalid('source failure records are incomplete')
    jsonBytes += sourceManifestEntry.uncompressed + sourceErrorsEntry.uncompressed
    if (source.files.length !== entries.size - 2) invalid('unlisted source image or missing manifest file')
    if (!manifest.sources.some((item) => item.batch_id === input.batchId)) {
      manifest.sources.push({
        batch_id: input.batchId, model: source.model, item_count: source.item_count,
        success_count: source.success_count, fail_count: source.fail_count, ...(batchConfig === undefined ? {} : { config: batchConfig }),
      })
    } else {
      const previous = manifest.sources.find((item) => item.batch_id === input.batchId)!
      if (previous.model !== source.model || previous.item_count !== source.item_count ||
          previous.success_count !== source.success_count || previous.fail_count !== source.fail_count) {
        invalid('conflicting source manifest')
      }
    }
    const listed = new Set<string>()
    const sourceIdentities = new Set<string>()
    for (const file of source.files) {
      const entry = entries.get(file.filename)
      const key = identity(input.batchId, file.custom_id, file.image_index)
      if (!entry || !entry.name.startsWith('images/') || listed.has(file.filename) || sourceIdentities.has(key)) {
        invalid('missing or duplicate manifest image')
      }
      listed.add(file.filename)
      sourceIdentities.add(key)
      const bytes = extractEntry(data, entry, limits, total)
      const itemConfig = configForCustomId(configByCustomId, file.custom_id, limits.maxMetadataBytes)
      const existingIndex = byIdentity.get(key)
      if (existingIndex !== undefined) {
        if (!equalBytes(images[existingIndex]!, bytes) || manifest.files[existingIndex]!.mime_type !== file.mime_type ||
            JSON.stringify(manifest.files[existingIndex]!.config) !== JSON.stringify(itemConfig)) {
          invalid('conflicting duplicate image')
        }
        continue
      }
      const extension = file.filename.match(/\.(png|jpe?g|webp|gif|avif|bmp|tiff?)$/i)?.[1]?.toLowerCase() ?? 'bin'
      const filename = `images/${String(images.length + 1).padStart(6, '0')}.${extension}`
      manifest.files.push({ source_batch_id: input.batchId, custom_id: file.custom_id, image_index: file.image_index,
        filename, mime_type: file.mime_type, ...(itemConfig === undefined ? {} : { config: itemConfig }) })
      byIdentity.set(key, images.length)
      images.push(bytes)
    }
    if (Array.from(entries.keys()).some((name) => name.startsWith('images/') && !listed.has(name))) {
      invalid('unlisted source image')
    }
    for (const error of sourceErrors) {
      const key = JSON.stringify([input.batchId, error.custom_id, error.code, error.message])
      if (seenErrors.has(key)) continue
      seenErrors.add(key)
      const itemConfig = configForCustomId(configByCustomId, error.custom_id, limits.maxMetadataBytes)
      errors.push({ source_batch_id: input.batchId, custom_id: error.custom_id,
        code: error.code, message: error.message, ...(itemConfig === undefined ? {} : { config: itemConfig }) })
    }
  }
  manifest.success_count = images.length
  manifest.fail_count = errors.length
  const manifestBytes = strToU8(boundedJson(manifest, limits.maxMetadataBytes, 'merged manifest'))
  const errorsBytes = strToU8(boundedJson(errors, limits.maxMetadataBytes, 'merged errors'))
  if (manifestBytes.length + errorsBytes.length > limits.maxMetadataBytes) invalid('merged metadata exceeds size limit')

  // ZIP pass-through avoids recompressing already-compressed images. No Blob is exposed until
  // every source has been verified, the ZIP has ended, and the final output-size check passes.
  const chunks: Uint8Array[] = []
  let exportBytes = 0
  let outputError: Error | undefined
  const zip = new Zip((error: Error | null, chunk: Uint8Array) => {
    if (error) {
      outputError = error
      return
    }
    exportBytes += chunk.length
    if (exportBytes > limits.maxExportBytes) {
      outputError = new Error('Cannot merge batch image ZIPs: merged ZIP exceeds export size limit')
      return
    }
    chunks.push(chunk)
  })
  try {
    const add = (name: string, bytes: Uint8Array) => {
      if (outputError) throw outputError
      const file = new ZipPassThrough(name)
      zip.add(file)
      file.push(bytes, true)
      if (outputError) throw outputError
    }
    images.forEach((bytes, index) => add(manifest.files[index]!.filename, bytes))
    add('manifest.json', manifestBytes)
    add('errors.json', errorsBytes)
    zip.end()
    if (outputError) throw outputError
    return new Blob(chunks as BlobPart[], { type: 'application/zip' })
  } catch (error) {
    zip.terminate()
    throw error
  }
}
