import { onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { getPendingDrawCount } from '@/api/admin/lottery'
import { LOTTERY_DRAWS_UPDATED } from '@/utils/lotteryEvents'

/** Sidebar-only, admin-only indicator; never infer pending work from the current table page. */
export function useLotteryPendingIndicator(enabled: Ref<boolean>, routePath: Ref<string>) {
  const hasPending = ref(false)
  let mounted = false
  let timer: ReturnType<typeof setInterval> | undefined
  let request: AbortController | undefined
  let refreshQueued = false

  async function refresh() {
    if (!mounted || !enabled.value || document.visibilityState === 'hidden') return
    if (request) {
      refreshQueued = true
      return
    }
    const controller = new AbortController()
    request = controller
    try {
      const count = await getPendingDrawCount({ signal: controller.signal })
      if (!controller.signal.aborted && mounted && enabled.value) hasPending.value = count > 0
    } catch {
      // A network error is not evidence that pending work has been processed.
    } finally {
      if (request === controller) {
        request = undefined
        if (refreshQueued) {
          refreshQueued = false
          void refresh()
        }
      }
    }
  }

  function stop() {
    if (timer !== undefined) clearInterval(timer)
    timer = undefined
    request?.abort()
    request = undefined
    refreshQueued = false
    hasPending.value = false
  }

  function syncPermission() {
    stop()
    if (mounted && enabled.value) {
      void refresh()
      timer = setInterval(() => { void refresh() }, 60_000)
    }
  }
  const refreshFromEvent = () => { void refresh() }
  watch(enabled, syncPermission)
  watch(routePath, refreshFromEvent)
  onMounted(() => {
    mounted = true
    window.addEventListener('focus', refreshFromEvent)
    window.addEventListener(LOTTERY_DRAWS_UPDATED, refreshFromEvent)
    document.addEventListener('visibilitychange', refreshFromEvent)
    syncPermission()
  })
  onBeforeUnmount(() => {
    mounted = false
    stop()
    window.removeEventListener('focus', refreshFromEvent)
    window.removeEventListener(LOTTERY_DRAWS_UPDATED, refreshFromEvent)
    document.removeEventListener('visibilitychange', refreshFromEvent)
  })
  return { hasPending }
}
