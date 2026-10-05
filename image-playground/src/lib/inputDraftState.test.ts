import { afterEach, describe, expect, it, vi } from 'vitest'
import type { InputDraft } from '../types'
import { getSelectedImageMentionLabel } from './promptImageMentions'
import {
  normalizeInputDraft,
  restoreGalleryInputDraftState,
  saveGalleryInputDraft,
  syncActiveInputDraft,
  updateInputDraftImages,
} from './inputDraftState'

const imageA = { id: 'image-a', dataUrl: 'data:image/png;base64,a' }
const imageB = { id: 'image-b', dataUrl: 'data:image/png;base64,b' }

afterEach(() => {
  vi.restoreAllMocks()
})

describe('input draft normalization', () => {
  it('normalizes old external data and rejects invalid image and mask fields', () => {
    vi.spyOn(Date, 'now').mockReturnValue(90)

    expect(normalizeInputDraft({
      prompt: 123,
      inputImages: [
        imageA,
        { id: 'image-b', dataUrl: 123 },
        { id: 123, dataUrl: 'invalid' },
        null,
      ],
      maskDraft: { targetImageId: 'image-a', maskDataUrl: 123, updatedAt: 5 },
      maskEditorImageId: 123,
      updatedAt: Number.NaN,
    }, 50)).toEqual({
      prompt: '',
      inputImages: [imageA, { id: 'image-b', dataUrl: '' }],
      maskDraft: null,
      maskEditorImageId: null,
      updatedAt: 50,
    })

    expect(normalizeInputDraft({
      prompt: '旧草稿',
      maskDraft: { targetImageId: 'image-a', maskDataUrl: 'data:image/png;base64,mask' },
    }, 50)).toMatchObject({
      prompt: '旧草稿',
      maskDraft: {
        targetImageId: 'image-a',
        maskDataUrl: 'data:image/png;base64,mask',
        updatedAt: 90,
      },
      updatedAt: 50,
    })
  })
})

describe('input draft gallery save and restore', () => {
  it('round-trips the gallery draft and restores empty state for null drafts', () => {
    const state = {
      prompt: '提示词',
      inputImages: [imageA],
      maskDraft: { targetImageId: imageA.id, maskDataUrl: 'data:image/png;base64,mask', updatedAt: 1 },
      maskEditorImageId: imageA.id,
      galleryInputDraft: null,
    }

    const saved = saveGalleryInputDraft(state)
    expect(saved).toMatchObject({
      prompt: '提示词',
      inputImages: [imageA],
      maskDraft: { targetImageId: imageA.id },
      maskEditorImageId: imageA.id,
    })

    expect(restoreGalleryInputDraftState(saved)).toEqual({
      prompt: '提示词',
      inputImages: [imageA],
      maskDraft: { targetImageId: imageA.id, maskDataUrl: 'data:image/png;base64,mask', updatedAt: 1 },
      maskEditorImageId: imageA.id,
    })

    expect(restoreGalleryInputDraftState(null)).toEqual({
      prompt: '',
      inputImages: [],
      maskDraft: null,
      maskEditorImageId: null,
    })
  })

  it('syncs the visible input into the gallery draft', () => {
    const state = {
      prompt: '',
      inputImages: [] as typeof imageA[],
      maskDraft: null,
      maskEditorImageId: null,
      galleryInputDraft: null,
    }

    const patched = syncActiveInputDraft(state, { prompt: '新输入', inputImages: [imageA] })
    expect(patched.galleryInputDraft).toMatchObject({
      prompt: '新输入',
      inputImages: [imageA],
    })
  })
})

describe('input draft image and mention transforms', () => {
  it('renumbers retained image mentions and clears a mask whose target was removed', () => {
    const draft: InputDraft = {
      prompt: `保留 ${getSelectedImageMentionLabel(1)}，删除 ${getSelectedImageMentionLabel(0)}`,
      inputImages: [imageA, imageB],
      maskDraft: {
        targetImageId: imageA.id,
        maskDataUrl: 'data:image/png;base64,mask',
        updatedAt: 1,
      },
      maskEditorImageId: imageA.id,
      updatedAt: 2,
    }

    expect({ ...draft, ...updateInputDraftImages(draft, [imageB]) }).toEqual({
      prompt: `保留 ${getSelectedImageMentionLabel(0)}，删除 @已移除图片`,
      inputImages: [imageB],
      maskDraft: null,
      maskEditorImageId: null,
      updatedAt: 2,
    })
  })

  it('preserves mention position when an image id is replaced by an equivalent image', () => {
    const draft: InputDraft = {
      prompt: `修改 ${getSelectedImageMentionLabel(0)}`,
      inputImages: [imageA],
      maskDraft: null,
      maskEditorImageId: null,
    }

    expect(updateInputDraftImages(draft, [imageB], { equivalentImageIds: { [imageA.id]: imageB.id } })).toMatchObject({
      prompt: `修改 ${getSelectedImageMentionLabel(0)}`,
      inputImages: [imageB],
    })
  })
})
