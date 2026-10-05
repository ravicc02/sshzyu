import type { AppSettings, FavoriteCollection, InputDraft, InputImage, MaskDraft, TaskParams } from '../types'
import { normalizeSettings } from './apiProfiles'
import { ensureDefaultFavoriteCollection, normalizeFavoriteCollections, resolveDefaultFavoriteCollectionId } from './favoriteState'
import { isEmptyInputDraft, normalizeInputDraft, saveGalleryInputDraft } from './inputDraftState'

export interface PersistedAppState {
  settings: AppSettings
  previousPresetConfig?: Pick<AppSettings, 'customProviders' | 'profiles'> | null
  dismissedPresetProfileIds?: string[]
  dismissedPresetProviderIds?: string[]
  params: TaskParams
  prompt?: string
  inputImages?: InputImage[]
  dismissedCodexCliPrompts: string[]
  galleryInputDraft: InputDraft | null
  favoriteCollections: FavoriteCollection[]
  defaultFavoriteCollectionId: string | null
  supportPromptDismissed: boolean
  supportPromptOpen: boolean
  supportPromptSkippedForImportedData: boolean
}

type PersistedStateSource = Omit<PersistedAppState, 'prompt' | 'inputImages'> & {
  prompt: string
  inputImages: InputImage[]
  maskDraft: MaskDraft | null
  maskEditorImageId: string | null
}

type PersistedStateFallback = Pick<
  PersistedAppState,
  'settings' | 'params' | 'dismissedPresetProfileIds' | 'dismissedPresetProviderIds' | 'dismissedCodexCliPrompts' | 'favoriteCollections' | 'defaultFavoriteCollectionId'
>

export type NormalizedPersistedAppState = PersistedAppState & {
  previousPresetConfig: Pick<AppSettings, 'customProviders' | 'profiles'> | null
  dismissedPresetProfileIds: string[]
  dismissedPresetProviderIds: string[]
  prompt: string
  inputImages: InputImage[]
  maskDraft: MaskDraft | null
  maskEditorImageId: string | null
}

export interface PersistedStateMergePlan {
  state: NormalizedPersistedAppState
}

function isRecord(value: unknown): value is Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const proto = Object.getPrototypeOf(value)
  return proto === Object.prototype || proto === null
}

function normalizeStringArray(value: unknown, fallback: string[]) {
  if (!Array.isArray(value)) return fallback
  return value.filter((item): item is string => typeof item === 'string')
}

function normalizeParams(value: unknown, fallback: TaskParams): TaskParams {
  if (!isRecord(value)) return fallback
  return {
    size: typeof value.size === 'string' ? value.size : fallback.size,
    quality: value.quality === 'auto' || value.quality === 'low' || value.quality === 'medium' || value.quality === 'high' ? value.quality : fallback.quality,
    output_format: value.output_format === 'png' || value.output_format === 'jpeg' || value.output_format === 'webp' ? value.output_format : fallback.output_format,
    output_compression: value.output_compression === null || (typeof value.output_compression === 'number' && Number.isFinite(value.output_compression))
      ? value.output_compression
      : fallback.output_compression,
    moderation: value.moderation === 'auto' || value.moderation === 'low' ? value.moderation : fallback.moderation,
    n: typeof value.n === 'number' && Number.isFinite(value.n) ? value.n : fallback.n,
    transparent_output: typeof value.transparent_output === 'boolean' ? value.transparent_output : fallback.transparent_output,
  }
}

export function createPersistedState(state: PersistedStateSource): PersistedAppState {
  const settings = normalizeSettings(state.settings)
  const galleryInputDraft = saveGalleryInputDraft(state)
  return {
    settings,
    previousPresetConfig: state.previousPresetConfig ?? null,
    dismissedPresetProfileIds: state.dismissedPresetProfileIds ?? [],
    dismissedPresetProviderIds: state.dismissedPresetProviderIds ?? [],
    params: state.params,
    ...(settings.persistInputOnRestart
      ? {
          prompt: galleryInputDraft?.prompt ?? '',
          inputImages: galleryInputDraft?.inputImages.map((img) => ({ id: img.id, dataUrl: '' })) ?? [],
        }
      : {}),
    dismissedCodexCliPrompts: state.dismissedCodexCliPrompts,
    galleryInputDraft: settings.persistInputOnRestart && galleryInputDraft
      ? { ...galleryInputDraft, inputImages: galleryInputDraft.inputImages.map((img) => ({ id: img.id, dataUrl: '' })) }
      : null,
    favoriteCollections: state.favoriteCollections,
    defaultFavoriteCollectionId: state.defaultFavoriteCollectionId,
    supportPromptDismissed: state.supportPromptDismissed,
    supportPromptOpen: state.supportPromptOpen,
    supportPromptSkippedForImportedData: state.supportPromptSkippedForImportedData,
  }
}

/** 旧版本持久化数据迁移：剥离已移除的 Agent 模式字段 */
export function migratePersistedState(persistedState: unknown, _version?: number): unknown {
  if (!isRecord(persistedState)) return persistedState
  const {
    agentConversations: _agentConversations,
    activeAgentConversationId: _activeAgentConversationId,
    agentInputDrafts: _agentInputDrafts,
    agentSidebarCollapsed: _agentSidebarCollapsed,
    agentAssetTab: _agentAssetTab,
    agentAssetPanelCollapsed: _agentAssetPanelCollapsed,
    appMode: _appMode,
    ...rest
  } = persistedState
  return rest
}

export function normalizePersistedState(
  persistedState: unknown,
  fallback: PersistedStateFallback,
  now = Date.now(),
): PersistedStateMergePlan | null {
  if (!isRecord(persistedState)) return null

  const settings = normalizeSettings(persistedState.settings ?? fallback.settings)
  const previousPresetConfig = isRecord(persistedState.previousPresetConfig) && Array.isArray(persistedState.previousPresetConfig.profiles)
    ? (() => {
        const normalized = normalizeSettings(persistedState.previousPresetConfig)
        return {
          customProviders: normalized.customProviders,
          profiles: persistedState.previousPresetConfig.profiles.length ? normalized.profiles : [],
        }
      })()
    : null
  const galleryInputDraft = settings.persistInputOnRestart
    ? normalizeInputDraft(persistedState.galleryInputDraft ?? {
        prompt: persistedState.prompt,
        inputImages: persistedState.inputImages,
        maskDraft: null,
        maskEditorImageId: null,
      }, now)
    : null
  const favoriteCollections = Array.isArray(persistedState.favoriteCollections)
    ? ensureDefaultFavoriteCollection(normalizeFavoriteCollections(persistedState.favoriteCollections, now), now)
    : fallback.favoriteCollections
  const preferredDefaultFavoriteCollectionId = persistedState.defaultFavoriteCollectionId === null || typeof persistedState.defaultFavoriteCollectionId === 'string'
    ? persistedState.defaultFavoriteCollectionId
    : fallback.defaultFavoriteCollectionId

  return {
    state: {
      settings,
      previousPresetConfig,
      dismissedPresetProfileIds: normalizeStringArray(persistedState.dismissedPresetProfileIds, fallback.dismissedPresetProfileIds ?? []),
      dismissedPresetProviderIds: normalizeStringArray(persistedState.dismissedPresetProviderIds, fallback.dismissedPresetProviderIds ?? []),
      params: normalizeParams(persistedState.params, fallback.params),
      dismissedCodexCliPrompts: normalizeStringArray(persistedState.dismissedCodexCliPrompts, fallback.dismissedCodexCliPrompts),
      galleryInputDraft: galleryInputDraft && !isEmptyInputDraft(galleryInputDraft) ? galleryInputDraft : null,
      favoriteCollections,
      defaultFavoriteCollectionId: resolveDefaultFavoriteCollectionId(favoriteCollections, preferredDefaultFavoriteCollectionId),
      supportPromptDismissed: Boolean(persistedState.supportPromptDismissed),
      supportPromptOpen: Boolean(persistedState.supportPromptOpen),
      supportPromptSkippedForImportedData: Boolean(persistedState.supportPromptSkippedForImportedData),
      prompt: galleryInputDraft?.prompt ?? '',
      inputImages: galleryInputDraft?.inputImages ?? [],
      maskDraft: galleryInputDraft?.maskDraft ?? null,
      maskEditorImageId: galleryInputDraft?.maskEditorImageId ?? null,
    },
  }
}
