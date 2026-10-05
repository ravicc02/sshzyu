import { describe, expect, it } from 'vitest'
import type { AppSettings, FavoriteCollection } from '../types'
import { DEFAULT_PARAMS } from '../types'
import { DEFAULT_SETTINGS } from './apiProfiles'
import { createPersistedState, migratePersistedState, normalizePersistedState } from './persistedState'

const imageA = { id: 'image-a', dataUrl: 'data:image/png;base64,image-a' }
const collectionA: FavoriteCollection = { id: 'collection-a', name: '收藏夹 A', createdAt: 1, updatedAt: 1 }

function source(settings: AppSettings = DEFAULT_SETTINGS) {
  return {
    settings,
    params: { ...DEFAULT_PARAMS },
    prompt: '画廊输入',
    inputImages: [imageA],
    maskDraft: null,
    maskEditorImageId: null,
    dismissedCodexCliPrompts: [],
    galleryInputDraft: null,
    favoriteCollections: [collectionA],
    defaultFavoriteCollectionId: collectionA.id,
    supportPromptDismissed: false,
    supportPromptOpen: false,
    supportPromptSkippedForImportedData: false,
  }
}

function fallback() {
  return {
    settings: DEFAULT_SETTINGS,
    params: { ...DEFAULT_PARAMS },
    dismissedCodexCliPrompts: ['current'],
    favoriteCollections: [collectionA],
    defaultFavoriteCollectionId: collectionA.id,
  }
}

describe('persisted state codec', () => {
  it('rejects non-record unknown data and falls back field-by-field for an invalid record', () => {
    class ExternalState {}

    expect(normalizePersistedState(null, fallback(), 100)).toBeNull()
    expect(normalizePersistedState([], fallback(), 100)).toBeNull()
    expect(normalizePersistedState(new Date(), fallback(), 100)).toBeNull()
    expect(normalizePersistedState(new Map(), fallback(), 100)).toBeNull()
    expect(normalizePersistedState(new ExternalState(), fallback(), 100)).toBeNull()

    const result = normalizePersistedState({
      params: { quality: 'invalid', n: Number.NaN },
      dismissedCodexCliPrompts: 'invalid',
      favoriteCollections: 'invalid',
      appMode: 'invalid',
      setPrompt: 'external action must not escape the codec',
    }, fallback(), 100)!

    expect(result.state.params).toEqual(DEFAULT_PARAMS)
    expect(result.state.dismissedCodexCliPrompts).toEqual(['current'])
    expect(result.state.favoriteCollections).toEqual([collectionA])
    expect(result.state).not.toHaveProperty('setPrompt')
    expect(result.state).not.toHaveProperty('appMode')
  })

  it('persists gallery drafts only when input persistence is enabled', () => {
    const previousPresetConfig = {
      customProviders: [],
      profiles: [DEFAULT_SETTINGS.profiles[0]],
    }
    const galleryDraft = {
      prompt: '画廊输入',
      inputImages: [imageA],
      maskDraft: null,
      maskEditorImageId: null,
      updatedAt: 1,
    }
    const enabled = createPersistedState({
      ...source(),
      previousPresetConfig,
      galleryInputDraft: galleryDraft,
    })
    const disabled = createPersistedState({
      ...source({ ...DEFAULT_SETTINGS, persistInputOnRestart: false }),
      prompt: '不应持久化的可见输入',
      galleryInputDraft: galleryDraft,
    })

    expect(enabled.prompt).toBe('画廊输入')
    expect(enabled.inputImages).toEqual([{ id: imageA.id, dataUrl: '' }])
    expect(enabled.galleryInputDraft?.inputImages).toEqual([{ id: imageA.id, dataUrl: '' }])
    expect(enabled.previousPresetConfig).toEqual(previousPresetConfig)
    expect(disabled).not.toHaveProperty('prompt')
    expect(disabled).not.toHaveProperty('inputImages')
    expect(disabled.galleryInputDraft).toBeNull()
  })

  it('restores the gallery draft and visible input from persisted data', () => {
    const result = normalizePersistedState({
      galleryInputDraft: {
        prompt: '已保存草稿',
        inputImages: [imageA],
        maskDraft: null,
        maskEditorImageId: null,
        updatedAt: 1,
      },
    }, fallback(), 100)!

    expect(result.state.prompt).toBe('已保存草稿')
    expect(result.state.inputImages).toEqual([imageA])
    expect(result.state.galleryInputDraft).toMatchObject({ prompt: '已保存草稿' })
  })

  it('does not restore drafts when input persistence is disabled', () => {
    const result = normalizePersistedState({
      settings: { ...DEFAULT_SETTINGS, persistInputOnRestart: false },
      prompt: '旧版可见输入',
      inputImages: [imageA],
    }, fallback(), 100)!

    expect(result.state.prompt).toBe('')
    expect(result.state.inputImages).toEqual([])
    expect(result.state.galleryInputDraft).toBeNull()
  })

  it('strips removed Agent mode fields when migrating old persisted data', () => {
    const migrated = migratePersistedState({
      appMode: 'agent',
      agentConversations: [{ id: 'conversation-a' }],
      activeAgentConversationId: 'conversation-a',
      agentInputDrafts: { 'conversation-a': { prompt: '旧草稿' } },
      agentSidebarCollapsed: true,
      agentAssetTab: 'outputs',
      agentAssetPanelCollapsed: false,
      prompt: '保留字段',
    }, 1) as Record<string, unknown>

    expect(migrated.prompt).toBe('保留字段')
    expect(migrated).not.toHaveProperty('appMode')
    expect(migrated).not.toHaveProperty('agentConversations')
    expect(migrated).not.toHaveProperty('activeAgentConversationId')
    expect(migrated).not.toHaveProperty('agentInputDrafts')
    expect(migrated).not.toHaveProperty('agentSidebarCollapsed')
    expect(migrated).not.toHaveProperty('agentAssetTab')
    expect(migrated).not.toHaveProperty('agentAssetPanelCollapsed')
    expect(migratePersistedState('invalid', 1)).toBe('invalid')
  })

  it('preserves an empty deployed profile snapshot when restoring persisted state', () => {
    const result = normalizePersistedState({
      previousPresetConfig: {
        customProviders: [{ id: 'provider-a', name: 'Provider A', submit: { path: 'generate' } }],
        profiles: [],
      },
    }, fallback())!

    expect(result.state.previousPresetConfig?.profiles).toEqual([])
    expect(result.state.previousPresetConfig?.customProviders).toEqual([
      expect.objectContaining({ id: 'provider-a' }),
    ])
  })
})
