export const agentFileAccept = '.png,.jpg,.jpeg,.webp,.pdf,.txt,.md,.csv,.tsv,.json,.docx,.xlsx,.pptx'
export type AgentAttachmentView = { status?: string; content_available?: boolean; expires_at?: string; id: string; url?: string; original_url?: string; thumbnail?: boolean; preview_bytes?: number; original_failed?: boolean; filename?: string; mime_type?: string; byte_size?: number; text?: string; failed?: boolean }
export function attachmentType(file?: Pick<AgentAttachmentView, 'filename' | 'mime_type'>) {
  const extension = file?.filename?.split('.').pop()?.toUpperCase()
  if (extension && extension !== file?.filename?.toUpperCase()) return extension === 'JPEG' ? 'JPG 图片' : extension
  return ({ 'image/png': 'PNG 图片', 'image/jpeg': 'JPG 图片', 'image/webp': 'WebP 图片', 'application/pdf': 'PDF', 'text/plain': 'TXT' } as Record<string, string>)[file?.mime_type || ''] || '文件'
}
export function attachmentSize(size?: number) {
  if (size === undefined) return ''
  return size < 1024 ? `${size} B` : size < 1024 * 1024 ? `${(size / 1024).toFixed(1)} KB` : `${(size / (1024 * 1024)).toFixed(1)} MB`
}
export function attachmentIsImage(file?: Pick<AgentAttachmentView, 'mime_type'>) { return !!file?.mime_type?.startsWith('image/') }
export function supportedAgentFile(file: File) { return agentFileAccept.split(',').some(ext => file.name.toLowerCase().endsWith(ext)) }

export async function readAttachmentPreview(blob: Blob): Promise<string> {
  const part = blob.slice(0, 64000)
  const text = typeof part.text === 'function' ? await part.text() : await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '')
    reader.onerror = () => reject(new Error('文字预览暂时不可用'))
    reader.readAsText(part, 'UTF-8')
  })
  return text + (blob.size > 64000 ? '\n…预览已截断' : '')
}

export interface AgentReferenceUploadResult { assets: AgentAttachmentView[]; errors: string[] }
export type AgentReferenceUploader = (files: File[]) => Promise<AgentReferenceUploadResult>
export const agentReferenceAccept = '.png,.jpg,.jpeg,.webp'

// Clipboard items and files are two views of the same data: use one source only.
export function agentFilesFromTransfer(transfer: DataTransfer | null, pasted = false): File[] {
  if (!transfer) return []
  const direct = Array.from(transfer.files || [])
  const files = direct.length ? direct : Array.from(transfer.items || []).filter(item => item.kind === 'file').map(item => item.getAsFile()).filter((file): file is File => !!file)
  if (!pasted) return files
  const extensions: Record<string, string> = { 'image/png': 'png', 'image/jpeg': 'jpg', 'image/webp': 'webp' }
  const stamp = Date.now()
  return files.map((file, index) => {
    const ext = extensions[file.type]
    return ext && (!file.name || !file.name.includes('.')) ? new File([file], `粘贴图片-${stamp}-${index + 1}.${ext}`, { type: file.type, lastModified: file.lastModified }) : file
  })
}
export function agentTransferHasFiles(transfer: DataTransfer | null): boolean {
  return !!transfer && (Array.from(transfer.types || []).includes('Files') || !!transfer.files?.length || Array.from(transfer.items || []).some(item => item.kind === 'file'))
}
export function agentTransferHasDirectory(transfer: DataTransfer | null): boolean {
  return !!transfer && Array.from(transfer.items || []).some(item => item.kind === 'file' && !!item.webkitGetAsEntry?.()?.isDirectory)
}
