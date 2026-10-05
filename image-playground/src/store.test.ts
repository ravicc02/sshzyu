import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { strToU8, zipSync } from 'fflate'
import { DEFAULT_PARAMS } from './types'
import { createDefaultFalProfile, createDefaultOpenAIProfile, DEFAULT_IMAGES_MODEL, DEFAULT_RESPONSES_MODEL, DEFAULT_SETTINGS, normalizeSettings } from './lib/apiProfiles'
import type { ExportData, StoredImage, StoredImageThumbnail, TaskRecord } from './types'
import { getSelectedImageMentionLabel } from './lib/promptImageMentions'
import { hasActiveDataOperations } from './lib/dataOperations'
import { normalizePersistedState } from './lib/persistedState'
import { setPresetConfig } from './lib/presetConfig'
vi.mock('./lib/db', () => {
  const tasks = new Map<string, TaskRecord>()
  const images = new Map<string, StoredImage>()
  const thumbnails = new Map<string, StoredImageThumbnail>()
  let imageSeq = 0

  return {
    CURRENT_THUMBNAIL_VERSION: 2,
    getAllTasks: async () => [...tasks.values()],
    putTask: async (task: TaskRecord) => {
      tasks.set(task.id, task)
      return task.id
    },
    deleteTask: vi.fn(async (id: string) => {
      tasks.delete(id)
    }),
    commitTaskDeletion: vi.fn(async (deletedTaskIds: string[]) => {
      for (const id of deletedTaskIds) tasks.delete(id)
    }),
    clearTasks: async () => {
      tasks.clear()
    },
    getImage: async (id: string) => images.get(id),
    getStoredImageThumbnail: async (id: string) => thumbnails.get(id),
    getImageThumbnail: async (id: string) => thumbnails.get(id),
    getStoredFreshImageThumbnail: async (id: string) => thumbnails.get(id),
    getAllImageIds: async () => [...images.keys()],
    getAllImages: async () => [...images.values()],
    putImage: async (image: StoredImage) => {
      images.set(image.id, image)
      return image.id
    },
    putImageThumbnail: async (thumbnail: StoredImageThumbnail) => {
      thumbnails.set(thumbnail.id, thumbnail)
      return thumbnail.id
    },
    deleteImage: vi.fn(async (id: string) => {
      images.delete(id)
      thumbnails.delete(id)
    }),
    clearImages: async () => {
      images.clear()
      thumbnails.clear()
    },
    storeImage: async (dataUrl: string, source: StoredImage['source'] = 'upload') => {
      const id = `stored-image-${++imageSeq}`
      images.set(id, { id, dataUrl, source, createdAt: Date.now() })
      return id
    },
    storeImageWithSize: async (dataUrl: string, source: StoredImage['source'] = 'upload') => {
      const id = `stored-image-${++imageSeq}`
      const size = dataUrl.match(/(\d+)x(\d+)/)
      const width = size ? Number(size[1]) : undefined
      const height = size ? Number(size[2]) : undefined
      images.set(id, { id, dataUrl, source, createdAt: Date.now(), width, height })
      return { id, width, height }
    },
  }
})
vi.mock('./lib/api', () => ({
  callImageApi: vi.fn(async () => ({
    images: [],
    actualParams: {},
    actualParamsList: [],
    revisedPrompts: [],
  })),
}))
vi.mock('./lib/falAiImageApi', () => ({
  getFalErrorMessage: vi.fn((err: unknown) => err instanceof Error ? err.message : String(err)),
  getFalQueuedImageResult: vi.fn(async () => ({
    images: [],
    actualParams: {},
    actualParamsList: [],
    revisedPrompts: [],
  })),
}))
vi.mock('./lib/transparentImage', () => ({
  GREEN_KEY_COLOR: '#00FF00',
  MAGENTA_KEY_COLOR: '#FF00FF',
  createTransparentOutputMeta: vi.fn((prompt: string) => ({
    transparentOutput: true,
    effectivePrompt: `transparent:${prompt}`,
  })),
  getTransparentRequestParams: vi.fn((params: typeof DEFAULT_PARAMS) => ({
    ...params,
    output_format: params.output_format === 'webp' ? 'webp' : 'png',
    output_compression: params.output_format === 'webp' ? params.output_compression : null,
    transparent_output: true,
  })),
  removeKeyedBackgroundFromDataUrl: vi.fn(async (dataUrl: string) => `transparent:${dataUrl}`),
}))
import { clearImages, clearTasks, commitTaskDeletion, deleteImage as deleteDbImage, deleteTask as deleteDbTask, getAllImageIds, getAllTasks, getImage, getStoredFreshImageThumbnail, putImage, putImageThumbnail, putTask as putDbTask } from './lib/db'
import { callImageApi } from './lib/api'
import { getFalQueuedImageResult } from './lib/falAiImageApi'
import { removeKeyedBackgroundFromDataUrl } from './lib/transparentImage'
import { clearData, clearFailedTasks, deleteFavoriteCollection, editOutputs, getErrorToastMessage, getPersistedState, getTaskApiProfile, importData, initStore, removeMultipleTasks, removeTask, restoreExplicitPresetConfig, retryTask, reuseConfig, submitTask, taskMatchesFilterStatus, taskMatchesSearchQuery, useStore } from './store'

const commitTaskDeletionImplementation = vi.mocked(commitTaskDeletion).getMockImplementation()!
const deleteDbImageImplementation = vi.mocked(deleteDbImage).getMockImplementation()!
const deleteDbTaskImplementation = vi.mocked(deleteDbTask).getMockImplementation()!

const imageA = { id: 'image-a', dataUrl: 'data:image/png;base64,a' }
const imageB = { id: 'image-b', dataUrl: 'data:image/png;base64,b' }

describe('error toast messages', () => {

  it('uses a generic message for long raw errors without a title', () => {
    expect(getErrorToastMessage(`invalid request ${'x'.repeat(90)}`)).toBe('操作失败，请查看详情')
  })
})


function task(overrides: Partial<TaskRecord> = {}): TaskRecord {
  return {
    id: 'task-a',
    prompt: 'prompt',
    params: { ...DEFAULT_PARAMS },
    inputImageIds: [],
    maskTargetImageId: null,
    maskImageId: null,
    outputImages: [],
    status: 'done',
    error: null,
    createdAt: 1,
    finishedAt: 2,
    elapsed: 1,
    ...overrides,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function importFile(data: ExportData, files: Record<string, Uint8Array> = {}): File {
  const zipped = zipSync({ ...files, 'manifest.json': strToU8(JSON.stringify(data)) })
  const buffer = zipped.buffer.slice(zipped.byteOffset, zipped.byteOffset + zipped.byteLength)
  return { name: 'backup.zip', size: zipped.byteLength, arrayBuffer: async () => buffer.slice(0) } as File
}

describe('favorite collection deletion', () => {
  const collectionA = { id: 'collection-a', name: '收藏夹 A', createdAt: 1, updatedAt: 1 }
  const collectionB = { id: 'collection-b', name: '收藏夹 B', createdAt: 1, updatedAt: 1 }

  beforeEach(async () => {
    await clearTasks()
    await clearImages()
    useStore.setState({
      tasks: [],
      favoriteCollections: [collectionA, collectionB],
      defaultFavoriteCollectionId: collectionA.id,
      activeFavoriteCollectionId: collectionA.id,
      selectedFavoriteCollectionIds: [collectionA.id],
      selectedTaskIds: [],
      inputImages: [],
      galleryInputDraft: null,
      showToast: vi.fn(),
    })
  })

  it('keeps tasks that are still referenced by another collection when deleting collection tasks', async () => {
    const sharedTask = task({
      id: 'shared-task',
      isFavorite: true,
      favoriteCollectionIds: [collectionA.id, collectionB.id],
    })
    const collectionOnlyTask = task({
      id: 'collection-only-task',
      isFavorite: true,
      favoriteCollectionIds: [collectionA.id],
    })
    useStore.setState({ tasks: [sharedTask, collectionOnlyTask] })
    await putDbTask(sharedTask)
    await putDbTask(collectionOnlyTask)

    await deleteFavoriteCollection(collectionA.id, true)

    const state = useStore.getState()
    expect(state.favoriteCollections.map((collection) => collection.id)).toEqual([collectionB.id])
    expect(state.activeFavoriteCollectionId).toBeNull()
    expect(state.selectedFavoriteCollectionIds).toEqual([])
    expect(state.tasks).toHaveLength(1)
    expect(state.tasks[0]).toMatchObject({
      id: sharedTask.id,
      isFavorite: true,
      favoriteCollectionIds: [collectionB.id],
    })
    expect((await getAllTasks()).map((item) => item.id)).toEqual([sharedTask.id])
  })
})

describe('mask draft lifecycle in store actions', () => {
  beforeEach(() => {
    vi.mocked(callImageApi).mockReset().mockResolvedValue({ images: [], actualParams: {}, actualParamsList: [], revisedPrompts: [] })
    vi.mocked(removeKeyedBackgroundFromDataUrl).mockReset().mockImplementation(async (dataUrl) => `transparent:${dataUrl}`)
    useStore.setState({
      settings: {
        ...DEFAULT_SETTINGS,
        baseUrl: 'https://api.example.com/v1',
        apiKey: 'test-key',
        profiles: DEFAULT_SETTINGS.profiles.map((profile) => ({ ...profile, transparentBackgroundMethod: 'local' })),
      },
      prompt: 'prompt',
      inputImages: [],
      maskDraft: null,
      maskEditorImageId: null,
      params: { ...DEFAULT_PARAMS },
      tasks: [],
      detailTaskId: null,
      lightboxImageId: null,
      lightboxImageList: [],
      showSettings: false,
      toast: null,
      confirmDialog: null,
      showToast: vi.fn(),
      setConfirmDialog: vi.fn(),
    })
  })

  it('preserves an existing mask when quick edit-output adds outputs as references', async () => {
    const maskDraft = {
      targetImageId: imageA.id,
      maskDataUrl: 'data:image/png;base64,mask',
      updatedAt: 1,
    }
    useStore.setState({
      inputImages: [imageA],
      maskDraft,
    })

    await editOutputs(task({ outputImages: [imageA.id] }))

    expect(useStore.getState().maskDraft).toEqual(maskDraft)
  })

  it('clears an invalid mask draft when submit cannot find the mask target image', async () => {
    useStore.setState({
      inputImages: [imageA],
      maskDraft: {
        targetImageId: 'missing-image',
        maskDataUrl: 'data:image/png;base64,mask',
        updatedAt: 1,
      },
    })

    await submitTask()

    expect(useStore.getState().maskDraft).toBeNull()
  })

  it('shows a submitted toast after creating a gallery task', async () => {
    await submitTask()
    await vi.waitFor(() => expect(useStore.getState().tasks[0]?.status).toBe('done'))

    const state = useStore.getState()
    expect(state.tasks).toHaveLength(1)
    expect(state.showToast).toHaveBeenCalledWith('任务已提交', 'success')
  })

  it('does not apply the outer watchdog to concurrent Codex CLI custom requests', async () => {
    const request = deferred<Awaited<ReturnType<typeof callImageApi>>>()
    vi.mocked(callImageApi).mockImplementationOnce(() => request.promise)
    const profile = {
      ...createDefaultOpenAIProfile({ id: 'custom-sync-profile', apiKey: 'custom-key', timeout: 1, codexCli: true }),
      provider: 'custom-sync',
    }
    useStore.setState({
      settings: normalizeSettings({
        ...DEFAULT_SETTINGS,
        customProviders: [{
          id: 'custom-sync',
          name: 'Custom Sync',
          submit: { path: 'images/generations' },
        }],
        profiles: [profile],
        activeProfileId: profile.id,
      }),
      params: { ...DEFAULT_PARAMS, n: 2 },
    })
    const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout')

    await submitTask()
    await vi.waitFor(() => expect(callImageApi).toHaveBeenCalledOnce())

    expect(setTimeoutSpy.mock.calls.some(([, delay]) => delay === 1000)).toBe(false)
    request.resolve({
      images: ['data:image/png;base64,success'],
      actualParams: { n: 1 },
      actualParamsList: [{ n: 1 }],
      revisedPrompts: [],
      failedRequests: [{ requestIndex: 1, error: 'The operation was aborted' }],
    })
    await vi.waitFor(() => expect(useStore.getState().tasks[0]?.status).toBe('done'))

    expect(useStore.getState().tasks[0]).toMatchObject({
      outputErrors: [{ requestIndex: 1, error: 'The operation was aborted' }],
    })
    expect(useStore.getState().tasks[0].outputImages).toHaveLength(1)
    setTimeoutSpy.mockRestore()
  })

  it('stores decoded image size as actual size when the API omits size', async () => {
    const { callImageApi } = await import('./lib/api')
    vi.mocked(callImageApi).mockClear()
    vi.mocked(callImageApi).mockResolvedValueOnce({
      images: ['data:image/png;base64,actual-1254x1254'],
      actualParams: { output_format: 'png' },
      actualParamsList: [{ output_format: 'png' }],
      revisedPrompts: [],
    })
    useStore.setState({
      prompt: 'prompt',
      params: { ...DEFAULT_PARAMS, size: '2048x2048' },
    })

    await submitTask()
    await vi.waitFor(() => expect(useStore.getState().tasks[0]?.status).toBe('done'))

    const [task] = useStore.getState().tasks
    expect(task.actualParams).toMatchObject({ size: '1254x1254', output_format: 'png', n: 1 })
    expect(task.actualParamsByImage?.[task.outputImages[0]]).toMatchObject({ size: '1254x1254', output_format: 'png' })
    await clearTasks()
    await clearImages()
  })

  it('keeps API-returned actual size over decoded image size', async () => {
    const { callImageApi } = await import('./lib/api')
    vi.mocked(callImageApi).mockClear()
    vi.mocked(callImageApi).mockResolvedValueOnce({
      images: ['data:image/png;base64,actual-1254x1254'],
      actualParams: { output_format: 'png', size: '1024x1024' },
      actualParamsList: [{ output_format: 'png', size: '1024x1024' }],
      revisedPrompts: [],
    })
    useStore.setState({
      prompt: 'prompt',
      params: { ...DEFAULT_PARAMS, size: '2048x2048' },
    })

    await submitTask()
    await vi.waitFor(() => expect(useStore.getState().tasks[0]?.status).toBe('done'))

    const [task] = useStore.getState().tasks
    expect(task.actualParams?.size).toBe('1024x1024')
    expect(task.actualParamsByImage?.[task.outputImages[0]].size).toBe('1024x1024')
    await clearTasks()
    await clearImages()
  })

  it('stores transparent background output after local post-processing', async () => {
    const { callImageApi } = await import('./lib/api')
    vi.mocked(callImageApi).mockClear()
    vi.mocked(removeKeyedBackgroundFromDataUrl).mockClear()
    vi.mocked(callImageApi).mockResolvedValueOnce({
      images: ['data:image/png;base64,generated'],
      actualParams: { output_format: 'png' },
      actualParamsList: [{ output_format: 'png' }],
      revisedPrompts: [],
    })
    useStore.setState({
      prompt: '单主体贴纸素材',
      params: {
        ...DEFAULT_PARAMS,
        output_format: 'png',
        output_compression: null,
        transparent_output: true,
      },
    })

    await submitTask()
    await vi.waitFor(() => expect(useStore.getState().tasks[0]?.status).toBe('done'))

    expect(callImageApi).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'transparent:单主体贴纸素材',
      params: expect.objectContaining({
        output_format: 'png',
        output_compression: null,
        transparent_output: true,
      }),
    }))
    expect(removeKeyedBackgroundFromDataUrl).toHaveBeenCalledWith('data:image/png;base64,generated')
    const [task] = useStore.getState().tasks
    expect(task).toMatchObject({
      prompt: '单主体贴纸素材',
      transparentOutput: true,
      transparentPrompt: 'transparent:单主体贴纸素材',
      status: 'done',
    })
    expect(task.transparentOriginalImages).toHaveLength(1)
    const outputImage = await getImage(task.outputImages[0])
    const originalImage = await getImage(task.transparentOriginalImages![0])
    expect(outputImage?.dataUrl).toBe('transparent:data:image/png;base64,generated')
    expect(originalImage?.dataUrl).toBe('data:image/png;base64,generated')
    await clearTasks()
    await clearImages()
  })

  it('stores locally post-processed transparent output as WebP', async () => {
    const { callImageApi } = await import('./lib/api')
    vi.mocked(callImageApi).mockResolvedValueOnce({
      images: ['data:image/webp;base64,generated'],
      actualParams: { output_format: 'webp' },
      actualParamsList: [{ output_format: 'webp' }],
      revisedPrompts: [],
    })
    useStore.setState({
      prompt: '单主体贴纸素材',
      params: {
        ...DEFAULT_PARAMS,
        output_format: 'webp',
        output_compression: 25,
        transparent_output: true,
      },
    })

    await submitTask()
    await vi.waitFor(() => expect(useStore.getState().tasks[0]?.status).toBe('done'))

    expect(callImageApi).toHaveBeenCalledWith(expect.objectContaining({
      params: expect.objectContaining({
        output_format: 'webp',
        output_compression: 25,
        transparent_output: true,
      }),
    }))
    expect(removeKeyedBackgroundFromDataUrl).toHaveBeenCalledWith(
      'data:image/webp;base64,generated',
      undefined,
      'webp',
      25,
    )
    await clearTasks()
    await clearImages()
  })

  it('keeps native transparent output unchanged and requests API transparency', async () => {
    const { callImageApi } = await import('./lib/api')
    vi.mocked(callImageApi).mockClear()
    vi.mocked(removeKeyedBackgroundFromDataUrl).mockClear()
    vi.mocked(callImageApi).mockResolvedValueOnce({
      images: ['data:image/png;base64,native-transparent'],
      actualParams: { output_format: 'png' },
      actualParamsList: [{ output_format: 'png' }],
      revisedPrompts: [],
    })
    useStore.setState({
      settings: { ...DEFAULT_SETTINGS, baseUrl: 'https://api.example.com/v1', apiKey: 'test-key' },
      prompt: '透明玻璃瓶',
      params: {
        ...DEFAULT_PARAMS,
        output_format: 'png',
        transparent_output: true,
      },
    })

    await submitTask()
    await vi.waitFor(() => expect(useStore.getState().tasks[0]?.status).toBe('done'))

    expect(callImageApi).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '透明玻璃瓶',
      nativeTransparentBackground: true,
    }))
    expect(removeKeyedBackgroundFromDataUrl).not.toHaveBeenCalled()
    const [task] = useStore.getState().tasks
    expect(task.transparentOutput).toBeUndefined()
    expect(task.transparentPrompt).toBeUndefined()
    expect(task.transparentOriginalImages).toBeUndefined()
    expect((await getImage(task.outputImages[0]))?.dataUrl).toBe('data:image/png;base64,native-transparent')
    await clearTasks()
    await clearImages()
  })

  it('falls back to the original output when transparent post-processing fails', async () => {
    const { callImageApi } = await import('./lib/api')
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    vi.mocked(callImageApi).mockClear()
    vi.mocked(removeKeyedBackgroundFromDataUrl).mockClear()
    vi.mocked(removeKeyedBackgroundFromDataUrl).mockRejectedValueOnce(new Error('post-process failed'))
    vi.mocked(callImageApi).mockResolvedValueOnce({
      images: ['data:image/png;base64,generated'],
      actualParams: { output_format: 'png' },
      actualParamsList: [{ output_format: 'png' }],
      revisedPrompts: [],
    })
    useStore.setState({
      prompt: '单主体贴纸素材',
      params: {
        ...DEFAULT_PARAMS,
        output_format: 'png',
        output_compression: null,
        transparent_output: true,
      },
    })

    await submitTask()
    await vi.waitFor(() => expect(useStore.getState().tasks[0]?.status).toBe('done'))

    const [task] = useStore.getState().tasks
    expect(task).toMatchObject({
      transparentOutput: true,
      status: 'done',
    })
    expect(task.transparentOriginalImages).toEqual([''])
    const outputImage = await getImage(task.outputImages[0])
    expect(outputImage?.dataUrl).toBe('data:image/png;base64,generated')
    warnSpy.mockRestore()
    await clearTasks()
    await clearImages()
  })

  it('supports transparent background post-processing for fal gallery tasks', async () => {
    const { callImageApi } = await import('./lib/api')
    const falProfile = createDefaultFalProfile({ id: 'fal-profile', apiKey: 'fal-key' })
    vi.mocked(callImageApi).mockClear()
    vi.mocked(removeKeyedBackgroundFromDataUrl).mockClear()
    vi.mocked(callImageApi).mockResolvedValueOnce({
      images: ['data:image/png;base64,fal-generated'],
      actualParams: { output_format: 'png' },
      actualParamsList: [{ output_format: 'png' }],
      revisedPrompts: [],
    })
    useStore.setState({
      settings: normalizeSettings({
        ...DEFAULT_SETTINGS,
        profiles: [{ ...falProfile, transparentBackgroundMethod: 'local' }],
        activeProfileId: falProfile.id,
      }),
      prompt: '单主体图标素材',
      params: {
        ...DEFAULT_PARAMS,
        output_format: 'png',
        transparent_output: true,
      },
    })

    await submitTask()
    await vi.waitFor(() => expect(useStore.getState().tasks[0]?.status).toBe('done'))

    expect(callImageApi).toHaveBeenCalledWith(expect.objectContaining({
      params: expect.objectContaining({
        output_format: 'png',
        transparent_output: true,
      }),
    }))
    expect(removeKeyedBackgroundFromDataUrl).toHaveBeenCalledWith('data:image/png;base64,fal-generated')
    const [task] = useStore.getState().tasks
    expect(task.apiProvider).toBe('fal')
    expect(task.transparentOutput).toBe(true)
    expect(task.transparentOriginalImages).toHaveLength(1)
    await clearTasks()
    await clearImages()
  })

  it('preserves selected image mentions when replacing a mask target with an equivalent image id', () => {
    const replacement = { id: 'image-a-replacement', dataUrl: imageA.dataUrl }
    const prompt = `参考 ${getSelectedImageMentionLabel(0)} 生成`
    useStore.setState({
      prompt,
      inputImages: [imageA, imageB],
    })

    useStore.getState().setInputImages([replacement, imageB], {
      equivalentImageIds: { [imageA.id]: replacement.id },
    })

    const state = useStore.getState()
    expect(state.inputImages.map((img) => img.id)).toEqual([replacement.id, imageB.id])
    expect(state.prompt).toBe(prompt)
  })
})

describe('input persistence setting', () => {
  beforeEach(() => {
    useStore.setState({
      settings: { ...DEFAULT_SETTINGS },
      prompt: 'prompt',
      inputImages: [imageA],
      galleryInputDraft: null,
      dismissedCodexCliPrompts: [],
    })
  })

  it('persists input when restart input restore is enabled', () => {
    const persisted = getPersistedState(useStore.getState())

    expect(persisted.prompt).toBe('prompt')
    expect(persisted.inputImages).toEqual([{ id: imageA.id, dataUrl: '' }])
  })


  it('writes empty input when persisted input is cleared', () => {
    useStore.setState({ prompt: '', inputImages: [] })

    const persisted = getPersistedState(useStore.getState())

    expect(persisted.prompt).toBe('')
    expect(persisted.inputImages).toEqual([])
  })

  it('persists and restores normalized dismissed preset provider IDs', () => {
    useStore.setState({
      dismissedPresetProfileIds: ['profile-a'],
      dismissedPresetProviderIds: ['provider-a'],
    })

    const persisted = getPersistedState(useStore.getState())
    const restored = normalizePersistedState({
      ...persisted,
      dismissedPresetProviderIds: ['provider-a', 1, null, 'provider-b'],
    }, useStore.getState())!

    expect(persisted.dismissedPresetProviderIds).toEqual(['provider-a'])
    expect(restored.state.dismissedPresetProviderIds).toEqual(['provider-a', 'provider-b'])
  })
})

describe('preset deletion state', () => {
  afterEach(() => {
    setPresetConfig(null)
    useStore.setState({ previousPresetConfig: null })
  })

  it('removes an untouched preset after deployment removes it', async () => {
    const providers = [
      { id: 'preset-provider-a', name: 'Provider A', submit: { path: 'a' } },
      { id: 'preset-provider-b', name: 'Provider B', submit: { path: 'b' } },
    ]
    const profiles = [
      createDefaultOpenAIProfile({ id: 'preset-profile-a', provider: providers[0].id, isDefault: true }),
      createDefaultOpenAIProfile({ id: 'preset-profile-b', provider: providers[1].id }),
    ]
    const previous = { customProviders: providers, profiles }
    const next = { customProviders: [providers[0]], profiles: [profiles[0]] }
    useStore.setState({
      settings: normalizeSettings(DEFAULT_SETTINGS),
      previousPresetConfig: null,
      tasks: [],
    })
    setPresetConfig(previous)
    await useStore.getState().setPresetImportedSettings(previous)

    setPresetConfig(next)
    await useStore.getState().setPresetImportedSettings(next)

    const state = useStore.getState()
    expect(state.settings.profiles.map((profile) => profile.id)).toEqual(['preset-profile-a'])
    expect(state.settings.customProviders.map((provider) => provider.id)).toEqual(['preset-provider-a'])
  })

  it('removes untouched presets when deployment removes the entire preset config', async () => {
    const provider = { id: 'preset-provider', name: 'Preset Provider', submit: { path: 'generate' } }
    const profile = createDefaultOpenAIProfile({ id: 'preset-profile', provider: provider.id, isDefault: true })
    const preset = { customProviders: [provider], profiles: [profile] }
    useStore.setState({
      settings: normalizeSettings(DEFAULT_SETTINGS),
      previousPresetConfig: null,
      tasks: [],
    })
    setPresetConfig(preset)
    await useStore.getState().setPresetImportedSettings(preset)

    setPresetConfig(null)
    await useStore.getState().setPresetImportedSettings({ customProviders: [], profiles: [] })

    const state = useStore.getState()
    expect(state.settings.profiles.map((item) => item.id)).toEqual([DEFAULT_SETTINGS.profiles[0].id])
    expect(state.settings.customProviders).toEqual([])
    expect(state.previousPresetConfig).toBeNull()
  })

  it('clearData clears both dismissal lists and reapplies the current preset', async () => {
    const provider = { id: 'preset-provider', name: 'Preset Provider', submit: { path: 'generate' } }
    const profile = createDefaultOpenAIProfile({
      id: 'preset-profile',
      isDefault: true,
      provider: provider.id,
      model: 'preset-model',
    })
    const preset = { customProviders: [provider], profiles: [profile] }
    setPresetConfig(preset)
    useStore.setState({
      settings: normalizeSettings({
        ...DEFAULT_SETTINGS,
        customProviders: [],
        profiles: [createDefaultFalProfile({ id: 'user-profile' })],
        activeProfileId: 'user-profile',
      }),
      dismissedPresetProfileIds: [profile.id],
      dismissedPresetProviderIds: [provider.id],
      dismissedCodexCliPrompts: ['prompt-a'],
    })

    await clearData({ clearConfig: true, clearTasks: false })

    const state = useStore.getState()
    expect(state.dismissedPresetProfileIds).toEqual([])
    expect(state.dismissedPresetProviderIds).toEqual([])
    expect(state.dismissedCodexCliPrompts).toEqual([])
    expect(state.settings.customProviders).toEqual([expect.objectContaining({ id: provider.id })])
    expect(state.settings.profiles).toEqual([expect.objectContaining({ id: profile.id, provider: provider.id })])
  })

  it('restores an explicitly reimported preset provider', () => {
    const provider = { id: 'preset-provider', name: 'Preset Provider', submit: { path: 'generate' } }
    const profile = createDefaultOpenAIProfile({ id: 'preset-profile', provider: provider.id })
    setPresetConfig({ customProviders: [provider], profiles: [profile] })
    useStore.setState({
      settings: normalizeSettings({
        ...DEFAULT_SETTINGS,
        customProviders: [],
        profiles: [{ ...profile, provider: 'openai' }],
      }),
      dismissedPresetProviderIds: [provider.id],
    })

    const state = useStore.getState()
    state.restorePresetProvider(provider.id)
    state.setSettings({ customProviders: [provider], profiles: [profile] })

    expect(useStore.getState().dismissedPresetProviderIds).toEqual([])
    expect(useStore.getState().settings.customProviders).toEqual([expect.objectContaining({ id: provider.id })])
    expect(useStore.getState().settings.profiles[0].provider).toBe(provider.id)
  })

  it('restores only preset IDs explicitly selected by a URL import', async () => {
    const providers = [
      { id: 'preset-provider-a', name: 'Preset Provider A', submit: { path: 'generate-a' } },
      { id: 'preset-provider-b', name: 'Preset Provider B', submit: { path: 'generate-b' } },
    ]
    const profiles = [
      createDefaultOpenAIProfile({ id: 'preset-profile-a', provider: providers[0].id, isDefault: true }),
      createDefaultOpenAIProfile({ id: 'preset-profile-b', provider: providers[1].id }),
    ]
    setPresetConfig({ customProviders: providers, profiles })
    useStore.setState({
      settings: normalizeSettings(DEFAULT_SETTINGS),
      dismissedPresetProviderIds: providers.map((provider) => provider.id),
      dismissedPresetProfileIds: profiles.map((profile) => profile.id),
    })

    const restored = await restoreExplicitPresetConfig({
      providerIds: [providers[0].id, 'not-a-preset'],
      profileIds: [profiles[0].id, 'not-a-preset'],
    })

    const state = useStore.getState()
    expect(restored).toBe(true)
    expect(state.dismissedPresetProviderIds).toEqual([providers[1].id])
    expect(state.dismissedPresetProfileIds).toEqual([profiles[1].id])
    expect(state.settings.customProviders).toEqual([expect.objectContaining({ id: providers[0].id })])
    expect(state.settings.profiles).toEqual(expect.arrayContaining([expect.objectContaining({ id: profiles[0].id })]))
  })
})


describe('fal task recovery', () => {
  beforeEach(async () => {
    await clearTasks()
    await clearImages()
    vi.mocked(getFalQueuedImageResult).mockReset().mockResolvedValue({ images: [], actualParams: {}, actualParamsList: [], revisedPrompts: [] })
    vi.mocked(removeKeyedBackgroundFromDataUrl).mockReset().mockImplementation(async (dataUrl) => `transparent:${dataUrl}`)
    const falProfile = createDefaultFalProfile({ id: 'fal-profile', apiKey: 'fal-key' })
    useStore.setState({
      settings: normalizeSettings({
        ...DEFAULT_SETTINGS,
        profiles: [falProfile],
        activeProfileId: falProfile.id,
      }),
      tasks: [],
      inputImages: [],
      galleryInputDraft: null,
      showToast: vi.fn(),
    })
  })

  it('applies transparent post-processing when a fal task recovers', async () => {
    const falTask = task({
      id: 'fal-transparent-task',
      apiProvider: 'fal',
      apiProfileId: 'fal-profile',
      apiProfileName: 'fal',
      apiModel: 'fal-model',
      params: {
        ...DEFAULT_PARAMS,
        output_format: 'png',
        transparent_output: true,
      },
      transparentOutput: true,
      transparentPrompt: 'transparent:prompt',
      status: 'error',
      error: '连接已断开，等待自动恢复',
      falRequestId: 'fal-request-id',
      falEndpoint: 'fal-endpoint',
      falRecoverable: true,
      finishedAt: null,
      elapsed: null,
    })
    await putDbTask(falTask)
    vi.mocked(getFalQueuedImageResult).mockResolvedValueOnce({
      images: ['data:image/png;base64,fal-recovered'],
      actualParams: { output_format: 'png' },
      actualParamsList: [{ output_format: 'png' }],
      revisedPrompts: [],
    })

    await initStore()
    await vi.waitFor(() => {
      expect(useStore.getState().tasks.find((item) => item.id === falTask.id)).toMatchObject({ status: 'done', falRecoverable: false })
    })

    expect(removeKeyedBackgroundFromDataUrl).toHaveBeenCalledWith('data:image/png;base64,fal-recovered')
    const recovered = useStore.getState().tasks.find((item) => item.id === falTask.id)
    expect(recovered).toMatchObject({
      status: 'done',
      falRecoverable: false,
      transparentOutput: true,
    })
    expect(recovered?.transparentOriginalImages).toHaveLength(1)
    const outputImage = await getImage(recovered!.outputImages[0])
    const originalImage = await getImage(recovered!.transparentOriginalImages![0])
    expect(outputImage?.dataUrl).toBe('transparent:data:image/png;base64,fal-recovered')
    expect(originalImage?.dataUrl).toBe('data:image/png;base64,fal-recovered')
  })







})



describe('data import', () => {
  beforeEach(async () => {
    useStore.setState({
      tasks: [],
      showToast: vi.fn(),
    })
  })

  it('restores favorite collections and default collection when importing task data', async () => {
    await clearTasks()
    const importedCollections = [
      { id: 'imported-collection-a', name: '导入收藏夹 A', createdAt: 1, updatedAt: 1 },
      { id: 'imported-collection-b', name: '导入收藏夹 B', createdAt: 2, updatedAt: 2 },
    ]
    const importedTask = task({
      id: 'imported-favorite-task',
      isFavorite: true,
      favoriteCollectionIds: [importedCollections[1].id],
    })

    const imported = await importData(importFile({
      version: 3,
      exportedAt: new Date(0).toISOString(),
      tasks: [importedTask],
      favoriteCollections: importedCollections,
      defaultFavoriteCollectionId: importedCollections[1].id,
      imageFiles: {},
    }), { importConfig: false, importTasks: true })

    const state = useStore.getState()
    expect(imported).toBe(true)
    expect(state.favoriteCollections).toEqual(expect.arrayContaining(importedCollections))
    expect(state.defaultFavoriteCollectionId).toBe(importedCollections[1].id)
    expect(state.tasks.find((item) => item.id === importedTask.id)).toMatchObject({
      favoriteCollectionIds: [importedCollections[1].id],
      isFavorite: true,
    })
    expect((await getAllTasks()).find((item) => item.id === importedTask.id)).toMatchObject({
      favoriteCollectionIds: [importedCollections[1].id],
      isFavorite: true,
    })
  })





  it('imports multiple regular backups together', async () => {
    await clearTasks()
    await clearImages()
    const sharedCollection = { id: 'regular-collection-shared', name: '共享收藏夹', createdAt: 1, updatedAt: 1 }
    const collectionA = { id: 'regular-collection-a', name: '普通备份 A', createdAt: 1, updatedAt: 1 }
    const collectionB = { id: 'regular-collection-b', name: '普通备份 B', createdAt: 2, updatedAt: 2 }
    const sharedTask = task({ id: 'regular-task-shared', outputImages: ['regular-image-shared'] })
    const backupA = importFile({
      version: 3,
      exportedAt: new Date(0).toISOString(),
      tasks: [sharedTask, task({ id: 'regular-task-a', outputImages: ['regular-image-a'], favoriteCollectionIds: [collectionA.id], isFavorite: true })],
      favoriteCollections: [sharedCollection, collectionA],
      defaultFavoriteCollectionId: collectionA.id,
      imageFiles: {
        'regular-image-shared': { path: 'images/shared.png' },
        'regular-image-a': { path: 'images/image-a.png' },
      },
    }, {
      'images/shared.png': new Uint8Array([5, 6]),
      'images/image-a.png': new Uint8Array([1, 2]),
    })
    const backupB = importFile({
      version: 3,
      exportedAt: new Date(1).toISOString(),
      tasks: [sharedTask, task({ id: 'regular-task-b', outputImages: ['regular-image-b'], favoriteCollectionIds: [collectionB.id], isFavorite: true })],
      favoriteCollections: [sharedCollection, collectionB],
      defaultFavoriteCollectionId: collectionB.id,
      imageFiles: {
        'regular-image-shared': { path: 'images/shared.png' },
        'regular-image-b': { path: 'images/image-b.png' },
      },
    }, {
      'images/shared.png': new Uint8Array([5, 6]),
      'images/image-b.png': new Uint8Array([3, 4]),
    })

    const imported = await importData([backupA, backupB], { importConfig: false, importTasks: true })

    const state = useStore.getState()
    const taskIds = (await getAllTasks()).map((item) => item.id)
    const collectionIds = state.favoriteCollections.map((collection) => collection.id)
    expect(imported).toBe(true)
    expect(taskIds).toEqual(expect.arrayContaining(['regular-task-shared', 'regular-task-a', 'regular-task-b']))
    expect(taskIds.filter((id) => id === sharedTask.id)).toHaveLength(1)
    expect(await getImage('regular-image-shared')).toMatchObject({ dataUrl: 'data:image/png;base64,BQY=' })
    expect(await getImage('regular-image-a')).toMatchObject({ dataUrl: 'data:image/png;base64,AQI=' })
    expect(await getImage('regular-image-b')).toMatchObject({ dataUrl: 'data:image/png;base64,AwQ=' })
    expect(collectionIds).toEqual(expect.arrayContaining([sharedCollection.id, collectionA.id, collectionB.id]))
    expect(collectionIds.filter((id) => id === sharedCollection.id)).toHaveLength(1)
  })

  it('deduplicates shared config when merging multiple regular backups', async () => {
    const sharedProfile = createDefaultOpenAIProfile({ id: 'regular-profile-shared', name: '共享配置', apiKey: 'shared-key' })
    const profileA = createDefaultOpenAIProfile({ id: 'regular-profile-a', name: '普通配置 A', apiKey: 'key-a' })
    const profileB = createDefaultOpenAIProfile({ id: 'regular-profile-b', name: '普通配置 B', apiKey: 'key-b' })
    const backupA = importFile({
      version: 3,
      exportedAt: new Date(0).toISOString(),
      settings: normalizeSettings({ ...DEFAULT_SETTINGS, profiles: [sharedProfile, profileA], activeProfileId: profileA.id }),
    })
    const backupB = importFile({
      version: 3,
      exportedAt: new Date(1).toISOString(),
      settings: normalizeSettings({ ...DEFAULT_SETTINGS, profiles: [sharedProfile, profileB], activeProfileId: profileB.id }),
    })

    const imported = await importData([backupA, backupB], { importConfig: true, importTasks: false })

    const apiKeys = useStore.getState().settings.profiles.map((profile) => profile.apiKey)
    expect(imported).toBe(true)
    expect(apiKeys).toEqual(expect.arrayContaining(['shared-key', 'key-a', 'key-b']))
    expect(apiKeys.filter((apiKey) => apiKey === 'shared-key')).toHaveLength(1)
  })

  it('preserves internal IDs when restoring config', async () => {
    const provider = {
      id: 'backup-provider-id',
      name: 'Backup Provider',
      submit: { path: 'v1/generate' },
    }
    const profile = createDefaultOpenAIProfile({
      id: 'backup-profile-id',
      isDefault: true,
      provider: provider.id,
      model: 'model-v1',
    })
    useStore.setState({ settings: DEFAULT_SETTINGS })

    const imported = await importData(importFile({
      version: 3,
      exportedAt: new Date(0).toISOString(),
      settings: normalizeSettings({ ...DEFAULT_SETTINGS, customProviders: [provider], profiles: [profile], activeProfileId: profile.id }),
    }), { importConfig: true, importTasks: false })
    await useStore.getState().setPresetImportedSettings({
      customProviders: [{ id: provider.id, name: 'Backup Provider', submit: { path: 'v2/generate' } }],
      profiles: [{ ...profile, provider: provider.id, model: 'model-v2' }],
    })

    const settings = useStore.getState().settings
    expect(imported).toBe(true)
    expect(settings.customProviders[0]).toMatchObject({ id: provider.id })
    expect(settings.profiles[0]).toMatchObject({ id: profile.id })
  })

  it('restores dismissed preset provider and profile IDs explicitly included in a backup', async () => {
    const provider = { id: 'preset-provider', name: 'Preset Provider', submit: { path: 'generate' } }
    const profile = createDefaultOpenAIProfile({ id: 'preset-profile', provider: provider.id })
    const otherProfile = createDefaultOpenAIProfile({ id: 'other-preset-profile' })
    setPresetConfig({ customProviders: [provider], profiles: [profile, otherProfile] })
    useStore.setState({
      settings: normalizeSettings({
        ...DEFAULT_SETTINGS,
        customProviders: [],
        profiles: [{ ...profile, provider: 'openai' }],
      }),
      dismissedPresetProviderIds: [provider.id],
      dismissedPresetProfileIds: [profile.id, 'other-preset-profile'],
    })

    try {
      const imported = await importData(importFile({
        version: 3,
        exportedAt: new Date(0).toISOString(),
        settings: normalizeSettings({
          ...DEFAULT_SETTINGS,
          customProviders: [provider],
          profiles: [profile],
          activeProfileId: profile.id,
        }),
      }), { importConfig: true, importTasks: false })

      expect(imported).toBe(true)
      expect(useStore.getState().dismissedPresetProviderIds).toEqual([])
      expect(useStore.getState().dismissedPresetProfileIds).toEqual(['other-preset-profile'])
      expect(useStore.getState().settings.customProviders).toEqual([expect.objectContaining({ id: provider.id })])
      expect(useStore.getState().settings.profiles[0].provider).toBe(provider.id)
    } finally {
      setPresetConfig(null)
    }
  })

  it('preserves imported task references when restoring config into a non-empty workspace', async () => {
    await clearTasks()
    const localProfile = createDefaultFalProfile({ id: 'local-profile', apiKey: 'local-key' })
    const provider = { id: 'backup-provider', name: 'Backup Provider', submit: { path: 'generate' } }
    const profile = createDefaultOpenAIProfile({ id: 'backup-profile', provider: provider.id, apiKey: 'backup-key' })
    const importedTask = task({ id: 'backup-task', apiProfileId: profile.id, apiProvider: provider.id })
    useStore.setState({
      settings: normalizeSettings({ ...DEFAULT_SETTINGS, profiles: [localProfile], activeProfileId: localProfile.id }),
      tasks: [],
    })

    const imported = await importData(importFile({
      version: 3,
      exportedAt: new Date(0).toISOString(),
      settings: normalizeSettings({ ...DEFAULT_SETTINGS, customProviders: [provider], profiles: [profile], activeProfileId: profile.id }),
      tasks: [importedTask],
      imageFiles: {},
    }), { importConfig: true, importTasks: true })

    const state = useStore.getState()
    expect(imported).toBe(true)
    expect(state.settings.profiles.map((item) => item.id)).toEqual(expect.arrayContaining([localProfile.id, profile.id]))
    expect(getTaskApiProfile(state.settings, state.tasks.find((item) => item.id === importedTask.id)!)).toMatchObject({ id: profile.id, provider: provider.id })
  })

  it('rejects an incomplete multipart backup before importing data', async () => {
    await clearTasks()
    const part1 = importFile({
      version: 3,
      exportedAt: new Date(0).toISOString(),
      backupPart: { id: 'backup-a', index: 1, total: 2 },
      tasks: [task({ id: 'incomplete-task' })],
      imageFiles: {},
    })

    const imported = await importData([part1], { importConfig: false, importTasks: true })

    expect(imported).toBe(false)
    expect((await getAllTasks()).some((item) => item.id === 'incomplete-task')).toBe(false)
  })

  it('validates image entries in every part before writing earlier parts', async () => {
    await clearImages()
    const part1 = importFile({
      version: 3,
      exportedAt: new Date(0).toISOString(),
      backupPart: { id: 'backup-a', index: 1, total: 2 },
      tasks: [],
      imageFiles: { 'preflight-image-a': { path: 'images/image-a.png' } },
    }, { 'images/image-a.png': new Uint8Array([1, 2]) })
    const part2 = importFile({
      version: 3,
      exportedAt: new Date(0).toISOString(),
      backupPart: { id: 'backup-a', index: 2, total: 2 },
      imageFiles: { 'preflight-image-b': { path: 'images/missing.png' } },
    })

    const imported = await importData([part1, part2], { importConfig: false, importTasks: true })

    expect(imported).toBe(false)
    expect(await getImage('preflight-image-a')).toBeUndefined()
  })

  it('imports config with running tasks without requiring image parts', async () => {
    useStore.setState({ tasks: [task({ status: 'running' })] })
    const part1 = importFile({
      version: 3,
      exportedAt: new Date(0).toISOString(),
      backupPart: { id: 'config-backup', index: 1, total: 3 },
      settings: DEFAULT_SETTINGS,
      tasks: [],
      imageFiles: { 'unused-image': { path: 'images/missing.png' } },
    })

    const imported = await importData([part1], { importConfig: true, importTasks: false })

    expect(imported).toBe(true)
  })

})



describe('task deletion', () => {
  beforeEach(async () => {
    await clearTasks()
    await clearImages()
    vi.mocked(callImageApi).mockReset().mockResolvedValue({ images: [], actualParams: {}, actualParamsList: [], revisedPrompts: [] })
    vi.mocked(commitTaskDeletion).mockReset().mockImplementation(commitTaskDeletionImplementation)
    vi.mocked(deleteDbImage).mockReset().mockImplementation(deleteDbImageImplementation)
    vi.mocked(getFalQueuedImageResult).mockReset().mockResolvedValue({ images: [], actualParams: {}, actualParamsList: [], revisedPrompts: [] })
    vi.mocked(removeKeyedBackgroundFromDataUrl).mockReset().mockImplementation(async (dataUrl) => `transparent:${dataUrl}`)
    useStore.setState({
      tasks: [],
      selectedTaskIds: [],
      inputImages: [],
      maskDraft: null,
      maskEditorImageId: null,
      galleryInputDraft: null,
      streamPreviews: {},
      streamPreviewSlots: {},
      detailTaskId: null,
      reusedTaskApiProfileId: null,
      reusedTaskApiProfileName: null,
      reusedTaskApiProfileMissing: false,
      showToast: vi.fn(),
      setConfirmDialog: vi.fn(),
    })
  })

  it('removes a deleted task from the current selection', async () => {
    const deleted = task({ id: 'task-deleted' })
    const remaining = task({ id: 'task-remaining' })
    await putDbTask(deleted)
    await putDbTask(remaining)
    useStore.setState({ tasks: [deleted, remaining], selectedTaskIds: [deleted.id, remaining.id] })

    await removeTask(deleted)

    const state = useStore.getState()
    expect(state.tasks.map((item) => item.id)).toEqual([remaining.id])
    expect(state.selectedTaskIds).toEqual([remaining.id])
    expect((await getAllTasks()).map((item) => item.id)).toEqual([remaining.id])
    expect(state.showToast).toHaveBeenCalledWith('任务已删除', 'success')
  })





  it('restores an orphan image and thumbnail referenced during the check-delete window', async () => {
    const deleteImage = vi.mocked(deleteDbImage).getMockImplementation()!
    vi.mocked(deleteDbImage).mockImplementationOnce(async (id) => {
      await deleteImage(id)
      useStore.setState({ tasks: [task({ id: 'new-task', inputImageIds: [id] })] })
    })
    await putImage({ id: 'referenced-late', dataUrl: 'data:image/png;base64,second', createdAt: 1 })
    await putImageThumbnail({
      id: 'referenced-late',
      thumbnailDataUrl: 'data:image/webp;base64,thumb',
      width: 10,
      height: 10,
      thumbnailVersion: 2,
    })
    const deleted = task({ id: 'task-deleted', outputImages: ['referenced-late'] })
    useStore.setState({ tasks: [deleted] })

    await removeTask(deleted)

    await expect(getImage('referenced-late')).resolves.toMatchObject({ dataUrl: 'data:image/png;base64,second' })
    await expect(getStoredFreshImageThumbnail('referenced-late')).resolves.toMatchObject({ thumbnailDataUrl: 'data:image/webp;base64,thumb' })
    expect(deleteDbImage).toHaveBeenCalledTimes(1)
  })





  it('counts duplicate and missing ids only when they match an existing task', async () => {
    const deleted = task({ id: 'task-deleted' })
    const remaining = task({ id: 'task-remaining' })
    await putDbTask(deleted)
    await putDbTask(remaining)
    useStore.setState({ tasks: [deleted, remaining], selectedTaskIds: [deleted.id, 'task-missing'] })

    await removeMultipleTasks([deleted.id, deleted.id, 'task-missing'])

    const state = useStore.getState()
    expect(state.tasks.map((item) => item.id)).toEqual([remaining.id])
    expect(state.selectedTaskIds).toEqual([])
    expect((await getAllTasks()).map((item) => item.id)).toEqual([remaining.id])
    expect(state.showToast).toHaveBeenCalledWith('已删除 1 个任务', 'success')
  })

  it('does not show a success toast when no task id exists', async () => {
    const showToast = vi.fn()
    useStore.setState({ selectedTaskIds: ['task-missing'], showToast })

    await removeMultipleTasks(['task-missing', 'task-missing'])
    await removeTask(task({ id: 'another-missing-task' }))

    expect(useStore.getState().selectedTaskIds).toEqual([])
    expect(showToast).not.toHaveBeenCalled()
  })




})




describe('reused task API profile', () => {
  const openaiProfile = createDefaultOpenAIProfile({ id: 'openai-profile', apiKey: 'openai-key' })
  const falProfile = createDefaultFalProfile({ id: 'fal-profile', name: 'fal 配置', apiKey: 'fal-key' })

  beforeEach(async () => {
    await clearTasks()
    useStore.setState({
      settings: normalizeSettings({
        ...DEFAULT_SETTINGS,
        profiles: [openaiProfile, falProfile],
        activeProfileId: openaiProfile.id,
        reuseTaskApiProfileTemporarily: true,
      }),
      prompt: '',
      inputImages: [],
      maskDraft: null,
      params: { ...DEFAULT_PARAMS },
      tasks: [],
      showSettings: false,
      toast: null,
      reusedTaskApiProfileId: null,
      reusedTaskApiProfileName: null,
      reusedTaskApiProfileMissing: false,
      showToast: vi.fn(),
      setConfirmDialog: vi.fn(),
    })
  })

  it('resolves a task API profile by stored profile id', () => {
    const resolved = getTaskApiProfile(useStore.getState().settings, task({ apiProvider: 'fal', apiProfileId: falProfile.id }))

    expect(resolved?.id).toBe(falProfile.id)
  })

  it('does not resolve a task API profile by stored name or model', () => {
    const resolved = getTaskApiProfile(useStore.getState().settings, task({
      apiProvider: 'fal',
      apiProfileName: falProfile.name,
      apiModel: falProfile.model,
    }))

    expect(resolved).toBeNull()
  })

  it('keeps unlocked preset settings and task profile references on refresh', async () => {
    const provider = { id: 'provider-internal', name: 'Custom Provider', submit: { path: 'v1/generate' } }
    const profile = createDefaultOpenAIProfile({ id: 'profile-internal', isDefault: true, provider: provider.id, model: 'model-v1' })
    const sourceTask = task({ apiProvider: provider.id, apiProfileId: profile.id })
    await putDbTask(sourceTask)
    useStore.setState({
      settings: normalizeSettings({
        ...useStore.getState().settings,
        profiles: [profile, openaiProfile],
        customProviders: [provider],
        activeProfileId: openaiProfile.id,
      }),
      tasks: [sourceTask],
      reusedTaskApiProfileId: profile.id,
    })

    await useStore.getState().setPresetImportedSettings({
      customProviders: [{ id: provider.id, name: provider.name, submit: { path: 'v2/generate' } }],
      profiles: [{ ...profile, provider: provider.id, model: 'model-v2' }],
    })

    const state = useStore.getState()
    expect(state.tasks[0]).toMatchObject({
      apiProfileId: profile.id,
      apiProvider: provider.id,
    })
    expect(state.settings.profiles[0]).toMatchObject({ id: profile.id, model: 'model-v1' })
    expect(state.settings.customProviders[0]).toMatchObject({ id: provider.id, submit: { path: 'generate' } })
    expect(state.reusedTaskApiProfileId).toBe(profile.id)
    expect((await getAllTasks())[0]).toMatchObject({
      apiProfileId: profile.id,
      apiProvider: provider.id,
    })
  })

  it('does not change an unlocked preset provider on refresh', async () => {
    const oldProvider = { id: 'provider-old', name: 'Old Provider', submit: { path: 'old' } }
    const profile = createDefaultOpenAIProfile({ id: 'stable-profile', isDefault: true, provider: oldProvider.id })
    const sourceTask = task({ apiProvider: oldProvider.id, apiProfileId: profile.id })
    useStore.setState({
      settings: normalizeSettings({ ...DEFAULT_SETTINGS, customProviders: [oldProvider], profiles: [profile] }),
      tasks: [sourceTask],
    })

    await useStore.getState().setPresetImportedSettings({
      customProviders: [{ id: 'provider-new', name: 'New Provider', submit: { path: 'new' } }],
      profiles: [{ ...profile, provider: 'provider-new', model: 'model-new' }],
    })

    expect(getTaskApiProfile(useStore.getState().settings, sourceTask)).toMatchObject({
      id: profile.id,
      provider: 'provider-old',
      model: DEFAULT_IMAGES_MODEL,
    })
  })

  it('reuses the task API profile temporarily without switching the active profile', async () => {
    await reuseConfig(task({
      apiProvider: 'fal',
      apiProfileId: falProfile.id,
      params: { ...DEFAULT_PARAMS, n: 8, size: 'auto', quality: 'auto' },
    }))

    const state = useStore.getState()
    expect(state.settings.activeProfileId).toBe(openaiProfile.id)
    expect(state.reusedTaskApiProfileId).toBe(falProfile.id)
    expect(state.params).toMatchObject({ n: 4, size: '1360x1024', quality: 'high' })
    expect(state.showToast).toHaveBeenCalledWith('已临时复用该任务的 API 配置「fal 配置」', 'success')
  })

  it('keeps selected image mentions when reusing a task with different current input images', async () => {
    await clearImages()
    await putImage(imageA)
    await putImage(imageB)
    const taskPrompt = `参考 ${getSelectedImageMentionLabel(1)} 生成`

    useStore.setState({
      prompt: `当前 ${getSelectedImageMentionLabel(1)}`,
      inputImages: [
        { id: 'current-x', dataUrl: 'data:image/png;base64,x' },
        { id: 'current-y', dataUrl: 'data:image/png;base64,y' },
      ],
    })

    await reuseConfig(task({
      apiProvider: 'openai',
      apiProfileId: openaiProfile.id,
      prompt: taskPrompt,
      inputImageIds: [imageA.id, imageB.id],
    }))

    const state = useStore.getState()
    expect(state.inputImages.map((img) => img.id)).toEqual([imageA.id, imageB.id])
    expect(state.prompt).toBe(taskPrompt)
  })

  it('clears temporary reuse when switching current settings to the reused API profile', async () => {
    await reuseConfig(task({ apiProvider: 'fal', apiProfileId: falProfile.id }))

    useStore.getState().setSettings({ activeProfileId: falProfile.id })

    const state = useStore.getState()
    expect(state.settings.activeProfileId).toBe(falProfile.id)
    expect(state.reusedTaskApiProfileId).toBeNull()
    expect(state.reusedTaskApiProfileMissing).toBe(false)
  })

  it('normalizes reused params to the current API profile when temporary reuse is disabled', async () => {
    useStore.setState({
      settings: normalizeSettings({
        ...useStore.getState().settings,
        reuseTaskApiProfileTemporarily: false,
      }),
    })

    await reuseConfig(task({
      apiProvider: 'fal',
      apiProfileId: falProfile.id,
      params: { ...DEFAULT_PARAMS, n: 8, size: 'auto', quality: 'auto' },
    }))

    const state = useStore.getState()
    expect(state.settings.activeProfileId).toBe(openaiProfile.id)
    expect(state.reusedTaskApiProfileId).toBeNull()
    expect(state.params).toMatchObject({ n: 8, size: 'auto', quality: 'auto' })
  })

  it('asks whether to submit with current API profile when the reused API profile is missing', async () => {
    await reuseConfig(task({ apiProvider: 'fal', apiProfileId: 'missing-profile' }))

    const state = useStore.getState()
    expect(state.tasks).toEqual([])
    expect(state.setConfirmDialog).toHaveBeenCalledWith(expect.objectContaining({
      title: '找不到 API 配置',
      message: '找不到复用任务所使用的 API 配置「未知配置」，要使用当前的 API 配置「默认」提交任务吗？',
      confirmText: '使用当前配置提交',
      cancelText: '放弃提交',
    }))
    expect(state.showSettings).toBe(false)
  })
})

describe('input clearing and in-place retry', () => {
  beforeEach(() => {
    vi.mocked(callImageApi).mockReset().mockResolvedValue({ images: [], actualParams: {}, actualParamsList: [], revisedPrompts: [] })
    vi.mocked(removeKeyedBackgroundFromDataUrl).mockReset().mockImplementation(async (dataUrl) => `transparent:${dataUrl}`)
    useStore.setState({
      settings: {
        ...DEFAULT_SETTINGS,
        baseUrl: 'https://api.example.com/v1',
        apiKey: 'test-key',
      },
      prompt: 'prompt',
      inputImages: [],
      maskDraft: null,
      maskEditorImageId: null,
      params: { ...DEFAULT_PARAMS },
      tasks: [],
      detailTaskId: null,
      lightboxImageId: null,
      lightboxImageList: [],
      showSettings: false,
      toast: null,
      confirmDialog: null,
      showToast: vi.fn(),
      setConfirmDialog: vi.fn(),
    })
  })

  it('clears prompt and input images after a gallery submit', async () => {
    useStore.setState({ prompt: '一只猫', inputImages: [imageA] })
    await submitTask()
    await vi.waitFor(() => expect(useStore.getState().tasks[0]?.status).toBe('done'))
    expect(useStore.getState().prompt).toBe('')
    expect(useStore.getState().inputImages).toHaveLength(0)
  })


  it('retries a failed task in place without adding a new card', async () => {
    const failed = task({ id: 'task-failed', status: 'error', error: 'boom' })
    useStore.setState({ tasks: [failed] })
    await retryTask(failed)
    const tasks = useStore.getState().tasks
    expect(tasks).toHaveLength(1)
    expect(tasks[0].id).toBe('task-failed')
    expect(tasks[0].status).toBe('running')
    expect(tasks[0].error).toBeNull()
    await vi.waitFor(() => expect(useStore.getState().tasks[0]?.status).toBe('done'))
  })

  it('still creates a new card when retrying a task that already has outputs', async () => {
    const finished = task({ id: 'task-with-outputs', status: 'error', outputImages: [imageA.id] })
    useStore.setState({ tasks: [finished] })
    await retryTask(finished)
    const tasks = useStore.getState().tasks
    expect(tasks).toHaveLength(2)
    expect(tasks.map(item => item.id)).toContain('task-with-outputs')
    expect(tasks.every(item => item.status !== 'running' || item.id !== 'task-with-outputs')).toBe(true)
  })
})
