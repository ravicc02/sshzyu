import type { ObjectDirective } from 'vue'
// Observe attachment containers, including placeholders; hidden history never fetches originals.
export function useAgentAssetVisibility(request: (id: string) => void): ObjectDirective<HTMLElement, string | string[] | undefined> {
  const entries = new WeakMap<HTMLElement, { observer?: IntersectionObserver; ids: string[]; visible: boolean }>()
  const ids = (value: string | string[] | undefined) => [...new Set(Array.isArray(value) ? value : value ? [value] : [])]
  return {
    mounted(el, binding) {
      const entry = { ids: ids(binding.value), visible: false } as { observer?: IntersectionObserver; ids: string[]; visible: boolean }
      entries.set(el, entry)
      if (typeof IntersectionObserver === 'undefined') { entry.visible = true; entry.ids.forEach(request); return }
      entry.observer = new IntersectionObserver(events => { entry.visible = events.some(event => event.isIntersecting); if (entry.visible) entry.ids.forEach(request) }, { rootMargin: '160px' })
      entry.observer.observe(el)
    },
    updated(el, binding) { const entry = entries.get(el); if (!entry) return; const next = ids(binding.value); if (next.join() !== entry.ids.join()) { entry.ids = next; if (entry.visible) next.forEach(request) } },
    unmounted(el) { entries.get(el)?.observer?.disconnect(); entries.delete(el) },
  }
}
