import { buildGatewayUrl } from './client'

export type BatchImageStatus =
  | 'queued'
  | 'running'
  | 'indexing'
  | 'processing_results'
  | 'settling'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'output_deleted'
  | string

export interface BatchImageSubmitItem {
  custom_id: string
  prompt: string
  output_count?: number
  reference_images?: BatchImageReferenceImage[]
}

export interface BatchImageReferenceImage {
  id?: string
  type?: string
  mime_type: string
  data?: string
  file_uri?: string
}

export interface BatchImageSubmitRequest {
  model: string
  task_name?: string
  collection_id?: string
  parent_batch_id?: string
  provider?: '' | 'gemini_api' | 'vertex' | string
  image_size?: '1K' | '2K' | '4K' | string
  response_mime_type?: string
  aspect_ratio?: string
  items: BatchImageSubmitItem[]
  metadata?: Record<string, string>
}

export interface BatchImageJob {
  id: string
  object: string
  task_name: string
  collection_id?: string
  parent_batch_id?: string | null
  status: BatchImageStatus
  model: string
  provider: string
  image_size?: string
  aspect_ratio?: string
  response_mime_type?: string
  item_count: number
  success_count: number
  fail_count: number
  estimated_cost: number
  hold_amount: number
  actual_cost: number | null
  created_at: number
  submitted_at: number | null
  settled_at: number | null
  downloaded_at?: number | null
  output_deleted_at?: number | null
}

export interface BatchImageItem {
  batch_id?: string
  source_task_name?: string
  custom_id: string
  status: string
  prompt_preview?: string | null
  mime_type: string | null
  file_extension: string | null
  image_count: number
  error?: {
    code: string
    message: string
    source?: 'provider' | 'system' | string
  } | null
}

export interface BatchImageItemsResponse {
  object: string
  data: BatchImageItem[]
  has_more: boolean
}

export interface BatchImageJobsResponse {
  object: string
  data: BatchImageJob[]
  has_more: boolean
}

export interface BatchImageModel {
  id: string
  object: string
  provider: string
  supported_image_sizes?: string[]
  supported_mime_types?: string[]
  supports_custom_dimensions?: boolean
}

export interface BatchImageModelsResponse {
  object: string
  data: BatchImageModel[]
}

export interface BatchImageJobsListOptions {
  limit?: number
  cursor?: string
  status?: string
  taskName?: string
  downloaded?: '' | 'true' | 'false' | string
  from?: string
  to?: string
}

async function parseBatchImageError(response: Response): Promise<Error> {
  try {
    const body = await response.json()
    const message = body?.error?.message || body?.message || response.statusText
    const error = new Error(message)
    ;(error as any).code = body?.error?.code || response.status
    ;(error as any).status = response.status
    ;(error as any).requestId = response.headers.get('X-Request-Id') || ''
    return error
  } catch {
    const error = new Error(response.statusText || `HTTP ${response.status}`)
    ;(error as any).code = response.status
    ;(error as any).status = response.status
    ;(error as any).requestId = response.headers.get('X-Request-Id') || ''
    return error
  }
}

function authHeaders(apiKey: string, extra?: HeadersInit): HeadersInit {
  return {
    Authorization: `Bearer ${apiKey}`,
    ...extra,
  }
}

export async function submitBatchImageJob(
  apiKey: string,
  payload: BatchImageSubmitRequest,
  idempotencyKey: string,
): Promise<BatchImageJob> {
  const response = await fetch(buildGatewayUrl('/v1/images/batches'), {
    method: 'POST',
    headers: authHeaders(apiKey, {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    }),
    body: JSON.stringify(payload),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
}

export async function getBatchImageRetryInput(apiKey: string, batchId: string): Promise<BatchImageSubmitRequest> {
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}/items?retry_input=true`), {
    headers: authHeaders(apiKey), cache: 'no-store',
  })
  if (!response.ok) throw await parseBatchImageError(response)
  const body = await response.json()
  // Older backends ignore unknown query parameters. Never fall back to previews.
  if (!body.retry_request || !Array.isArray(body.retry_request.items)) {
    const error = new Error('Original retry input is unavailable')
    ;(error as any).code = 'BATCH_IMAGE_RETRY_INPUT_UNAVAILABLE'
    throw error
  }
  return body.retry_request
}

export async function getBatchImageJob(apiKey: string, batchId: string): Promise<BatchImageJob> {
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
}

export async function listBatchImageJobs(apiKey: string, options: number | BatchImageJobsListOptions = 20): Promise<BatchImageJobsResponse> {
  const params = new URLSearchParams()
  if (typeof options === 'number') {
    params.set('limit', String(options))
  } else {
    params.set('limit', String(options.limit || 20))
    if (options.cursor) params.set('cursor', options.cursor)
    if (options.status) params.set('status', options.status)
    if (options.taskName) params.set('task_name', options.taskName)
    if (options.downloaded) params.set('downloaded', options.downloaded)
    if (options.from) params.set('from', options.from)
    if (options.to) params.set('to', options.to)
  }
  const response = await fetch(buildGatewayUrl(`/v1/images/batches?${params.toString()}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
}

export async function listBatchImageModels(apiKey: string): Promise<BatchImageModelsResponse> {
  const response = await fetch(buildGatewayUrl('/v1/images/batches/models'), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
}

export async function listBatchImageItems(
  apiKey: string,
  batchId: string,
  status = '',
): Promise<BatchImageItemsResponse> {
  const query = status ? `?status=${encodeURIComponent(status)}` : ''
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}/items${query}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
}

export async function cancelBatchImageJob(apiKey: string, batchId: string): Promise<BatchImageJob> {
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}/cancel`), {
    method: 'POST',
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.json()
}

export async function downloadBatchImageZip(apiKey: string, batchId: string, maxBytes?: number): Promise<Blob> {
  if (maxBytes !== undefined && (!Number.isSafeInteger(maxBytes) || maxBytes <= 0)) throw new Error('ZIP 下载大小限制无效。')
  const controller = new AbortController()
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}/download`), {
    headers: authHeaders(apiKey), signal: controller.signal,
  })
  if (!response.ok) throw await parseBatchImageError(response)
  if (maxBytes === undefined) return response.blob()
  const tooLarge = () => {
    controller.abort()
    return new Error('ZIP 超过合并下载大小限制，请分开下载或减少选择的任务。')
  }
  const length = Number(response.headers.get('Content-Length'))
  if (Number.isFinite(length) && length > maxBytes) throw tooLarge()
  if (!response.body) throw new Error('浏览器不支持受限流式下载，请使用最新版本浏览器。')
  const reader = response.body.getReader()
  const chunks: Uint8Array[] = []
  let size = 0
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      size += value.byteLength
      if (size > maxBytes) throw tooLarge()
      chunks.push(value)
    }
    return new Blob(chunks as BlobPart[], { type: 'application/zip' })
  } catch (error) {
    controller.abort()
    await reader.cancel().catch(() => {})
    throw error
  } finally {
    reader.releaseLock()
  }
}

export async function getBatchImageItemContent(apiKey: string, batchId: string, customId: string, imageIndex = 0): Promise<Blob> {
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}/items/${encodeURIComponent(customId)}/content?image_index=${encodeURIComponent(String(imageIndex))}`), {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
  return response.blob()
}

export async function deleteBatchImageJobRecord(apiKey: string, batchId: string): Promise<void> {
  const response = await fetch(buildGatewayUrl(`/v1/images/batches/${encodeURIComponent(batchId)}`), {
    method: 'DELETE',
    headers: authHeaders(apiKey),
  })
  if (!response.ok) throw await parseBatchImageError(response)
}

export function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  window.setTimeout(() => URL.revokeObjectURL(url), 40_000)
}
