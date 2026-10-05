import type { InputDraft, InputImage, MaskDraft } from '../types'
import { remapImageMentionsForOrder } from './promptImageMentions'

type InputDraftFields = Pick<InputDraft, 'prompt' | 'inputImages' | 'maskDraft' | 'maskEditorImageId'>

type GalleryInputDraftState = InputDraftFields & {
  galleryInputDraft: InputDraft | null
}

type ActiveInputDraftState = GalleryInputDraftState

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value))
}

function normalizeInputImages(value: unknown): InputImage[] {
  if (!Array.isArray(value)) return []
  return value
    .map((img): InputImage | null => {
      if (!isRecord(img) || typeof img.id !== 'string') return null
      return { id: img.id, dataUrl: typeof img.dataUrl === 'string' ? img.dataUrl : '' }
    })
    .filter((img): img is InputImage => img != null)
}

function normalizeMaskDraft(value: unknown): MaskDraft | null {
  if (!isRecord(value)) return null
  if (typeof value.targetImageId !== 'string' || typeof value.maskDataUrl !== 'string') return null
  return {
    targetImageId: value.targetImageId,
    maskDataUrl: value.maskDataUrl,
    updatedAt: typeof value.updatedAt === 'number' ? value.updatedAt : Date.now(),
  }
}

export function normalizeInputDraft(value: unknown, fallbackUpdatedAt = Date.now()): InputDraft {
  const draft = isRecord(value) ? value : {}
  const updatedAt = typeof draft.updatedAt === 'number' && Number.isFinite(draft.updatedAt) ? draft.updatedAt : fallbackUpdatedAt
  return {
    prompt: typeof draft.prompt === 'string' ? draft.prompt : '',
    inputImages: normalizeInputImages(draft.inputImages),
    maskDraft: normalizeMaskDraft(draft.maskDraft),
    maskEditorImageId: typeof draft.maskEditorImageId === 'string' ? draft.maskEditorImageId : null,
    updatedAt,
  }
}

export function clearInputDraftState(): InputDraftFields {
  return {
    prompt: '',
    inputImages: [],
    maskDraft: null,
    maskEditorImageId: null,
  }
}

function copyInputDraft(draft: InputDraft): InputDraft {
  return {
    prompt: draft.prompt,
    inputImages: draft.inputImages.map((img) => ({ ...img })),
    maskDraft: draft.maskDraft ? { ...draft.maskDraft } : null,
    maskEditorImageId: draft.maskEditorImageId,
    updatedAt: draft.updatedAt ?? Date.now(),
  }
}

function getCurrentInputDraft(state: InputDraftFields): InputDraft {
  return {
    prompt: state.prompt,
    inputImages: state.inputImages,
    maskDraft: state.maskDraft,
    maskEditorImageId: state.maskEditorImageId,
    updatedAt: Date.now(),
  }
}

export function isEmptyInputDraft(draft: InputDraft) {
  return draft.prompt.length === 0 && draft.inputImages.length === 0 && !draft.maskDraft && !draft.maskEditorImageId
}

export function saveGalleryInputDraft(state: GalleryInputDraftState) {
  const draft = getCurrentInputDraft(state)
  return isEmptyInputDraft(draft) ? null : copyInputDraft(draft)
}

export function restoreGalleryInputDraftState(draft: InputDraft | null): InputDraftFields {
  if (!draft) return clearInputDraftState()
  return {
    prompt: draft.prompt,
    inputImages: draft.inputImages.map((img) => ({ ...img })),
    maskDraft: draft.maskDraft ? { ...draft.maskDraft } : null,
    maskEditorImageId: draft.maskEditorImageId,
  }
}

export function syncActiveInputDraft<T extends Partial<InputDraft>>(
  state: ActiveInputDraftState,
  patch: T,
): T & { galleryInputDraft?: InputDraft | null } {
  const draft: InputDraft = {
    prompt: patch.prompt ?? state.prompt,
    inputImages: patch.inputImages ?? state.inputImages,
    maskDraft: patch.maskDraft !== undefined ? patch.maskDraft : state.maskDraft,
    maskEditorImageId: patch.maskEditorImageId !== undefined ? patch.maskEditorImageId : state.maskEditorImageId,
  }
  return {
    ...patch,
    galleryInputDraft: isEmptyInputDraft(draft) ? null : copyInputDraft(draft),
  }
}

export function updateInputDraftImages(
  draft: InputDraftFields,
  inputImages: InputImage[],
  options: { equivalentImageIds?: Record<string, string>; clearMissingMask?: boolean } = {},
) {
  const shouldClearMask = options.clearMissingMask !== false && Boolean(draft.maskDraft) && !inputImages.some((img) => img.id === draft.maskDraft?.targetImageId)
  return {
    inputImages,
    prompt: remapImageMentionsForOrder(draft.prompt, draft.inputImages, inputImages, options.equivalentImageIds),
    ...(shouldClearMask ? { maskDraft: null, maskEditorImageId: null } : {}),
  }
}
