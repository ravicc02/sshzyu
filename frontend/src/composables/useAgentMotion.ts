import { onBeforeUnmount, onMounted } from 'vue'
import { gsap } from 'gsap'

// One preference/visibility subscription for the component family, even when
// a conversation contains hundreds of trace rows or attachment cards.
let media: gsap.MatchMedia | undefined
let users = 0
let enabled = false
const activeOwners = new Set<() => void>()
function stopActive() { for (const stop of [...activeOwners]) stop() }
function visibilityChanged() { if (document.hidden) stopActive() }
function acquire() {
  users++
  if (users !== 1 || typeof window.matchMedia !== 'function') return
  media = gsap.matchMedia()
  media.add({ normal: '(prefers-reduced-motion: no-preference)', reduced: '(prefers-reduced-motion: reduce)' }, context => {
    enabled = !!context.conditions?.normal && !context.conditions?.reduced
    if (!enabled) stopActive()
    return () => { enabled = false; stopActive() }
  })
  document.addEventListener('visibilitychange', visibilityChanged)
}
function release() {
  if (--users !== 0) return
  enabled = false; stopActive(); media?.revert(); media = undefined
  document.removeEventListener('visibilitychange', visibilityChanged)
}

/** Feedback-only motion. The DOM is authored visible and usable without JS motion. */
export function useAgentMotion() {
  let attached = false
  type Running = { tween?: gsap.core.Tween; finish: () => void }
  const running = new Map<HTMLElement, Running>()
  function stopAll() {
    for (const record of [...running.values()]) { record.tween?.kill(); record.finish() }
  }
  onMounted(() => { attached = true; acquire() })
  onBeforeUnmount(() => { stopAll(); if (attached) release() })
  function enter(element: Element, done: () => void = () => undefined) {
    const target = element as HTMLElement
    const old = running.get(target)
    if (old) { old.tween?.kill(); old.finish() }
    if (!enabled || document.hidden || target.closest('details:not([open])')) { done(); return }
    const original = { opacity: target.style.opacity, visibility: target.style.visibility, transform: target.style.transform }
    let finished = false
    const record: Running = { finish: () => {
      if (finished) return
      finished = true; running.delete(target)
      if (!running.size) activeOwners.delete(stopAll)
      gsap.set(target, { clearProps: 'opacity,visibility,transform' })
      for (const key of ['opacity', 'visibility', 'transform'] as const) if (original[key]) target.style[key] = original[key]
      done()
    } }
    running.set(target, record); activeOwners.add(stopAll)
    // Enter-only fades preserve visibility so native dialog/menu focus stays usable.
    try { record.tween = gsap.fromTo(target, { opacity: 0, y: 6 }, { opacity: 1, y: 0, duration: .18, ease: 'power2.out', overwrite: 'auto', onComplete: record.finish, onInterrupt: record.finish }) } catch { record.finish() }
  }
  function cancel(target: Element | null | undefined) {
    if (!target) return
    for (const [element, record] of [...running]) if (element === target || target.contains(element)) { record.tween?.kill(); record.finish() }
  }
  function reveal(target: Element | null | undefined) { if (target) enter(target) }
  function disclose(event: Event) {
    const details = event.target as HTMLDetailsElement
    for (const child of Array.from(details.children)) if (child.tagName !== 'SUMMARY') { if (details.open) reveal(child); else cancel(child) }
  }
  return { enter, reveal, disclose, cancel }
}
