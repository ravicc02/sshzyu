import { mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAgentMotion } from '../useAgentMotion'

type Animation = { onComplete: () => void; onInterrupt: () => void; kill: ReturnType<typeof vi.fn> }
const state = vi.hoisted(() => ({
  conditions: { normal: true, reduced: false },
  animations: [] as Animation[],
  add: vi.fn(), revert: vi.fn(), fromTo: vi.fn(), set: vi.fn(),
  cleanup: undefined as (() => void) | undefined,
  handler: undefined as ((context: { conditions: { normal: boolean; reduced: boolean } }) => (() => void)) | undefined,
}))
vi.mock('gsap', () => ({ gsap: { matchMedia: () => ({ add: state.add, revert: state.revert }), fromTo: state.fromTo, set: state.set } }))
let motion: ReturnType<typeof useAgentMotion>
function host() { return mount(defineComponent({ setup() { motion = useAgentMotion(); return () => h('div') } })) }
beforeEach(() => {
  vi.clearAllMocks(); state.animations = []; state.conditions = { normal: true, reduced: false }
  vi.stubGlobal('matchMedia', vi.fn())
  state.add.mockImplementation((_queries, handler) => { state.handler = handler; state.cleanup = handler({ conditions: state.conditions }) })
  state.revert.mockImplementation(() => state.cleanup?.())
  state.set.mockImplementation((target: HTMLElement) => { for (const name of ['opacity', 'visibility', 'transform']) target.style.removeProperty(name) })
  state.fromTo.mockImplementation((target: HTMLElement, _from, vars: Animation) => {
    target.style.opacity = '0'; target.style.transform = 'translateY(6px)'
    const animation = { onComplete: vars.onComplete, onInterrupt: vars.onInterrupt, kill: vi.fn(() => vars.onInterrupt()) }
    state.animations.push(animation); return animation
  })
})
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })
describe('Agent motion lifecycle', () => {
  it('keeps controls static and usable when reduced motion is requested', () => {
    state.conditions = { normal: false, reduced: true }
    const w = host(), el = document.createElement('button'), done = vi.fn()
    motion.enter(el, done)
    expect(state.fromTo).not.toHaveBeenCalled(); expect(done).toHaveBeenCalledOnce()
    expect(el.style.visibility).not.toBe('hidden'); w.unmount()
  })
  it('restores original inline styles and completes each transition once', () => {
    const w = host(), el = document.createElement('div'), done = vi.fn()
    el.style.opacity = '0.8'; motion.enter(el, done)
    expect(state.fromTo).toHaveBeenCalledOnce()
    expect(done).not.toHaveBeenCalled()
    expect(el.style.visibility).toBe('')
    state.animations[0]!.onComplete(); state.animations[0]!.onInterrupt()
    expect(el.style.opacity).toBe('0.8'); expect(el.style.visibility).toBe('')
    expect(el.style.transform).toBe(''); expect(done).toHaveBeenCalledOnce(); w.unmount()
  })
  it('finishes active motion immediately when the motion preference changes', () => {
    const w = host(), el = document.createElement('div'), done = vi.fn()
    motion.enter(el, done); state.cleanup?.()
    state.conditions = { normal: false, reduced: true }; state.cleanup = state.handler!({ conditions: state.conditions })
    expect(state.animations[0]!.kill).toHaveBeenCalledOnce()
    expect(el.style.visibility).toBe(''); expect(done).toHaveBeenCalledOnce()
    motion.enter(el); expect(state.fromTo).toHaveBeenCalledOnce(); w.unmount()
  })
  it('cleans up active tweens when a component is removed', () => {
    const w = host(), el = document.createElement('div'), done = vi.fn()
    motion.enter(el, done); w.unmount()
    expect(state.animations[0]!.kill).toHaveBeenCalledOnce()
    expect(state.revert).toHaveBeenCalledOnce(); expect(done).toHaveBeenCalledOnce()
    expect(el.style.visibility).toBe(''); expect(el.style.transform).toBe('')
  })
  it('prevents an interrupted reveal from clearing the next reveal', () => {
    const w = host(), el = document.createElement('div'), first = vi.fn(), second = vi.fn()
    motion.enter(el, first); motion.enter(el, second)
    expect(first).toHaveBeenCalledOnce(); expect(second).not.toHaveBeenCalled()
    state.animations[0]!.onComplete()
    expect(el.style.opacity).toBe('0')
    state.animations[1]!.onComplete()
    expect(second).toHaveBeenCalledOnce(); expect(el.style.visibility).toBe(''); w.unmount()
  })
  it('skips hidden disclosures and cancels their reveal on collapse', () => {
    const w = host(), details = document.createElement('details'), panel = document.createElement('div')
    details.append(panel); motion.enter(panel)
    expect(state.fromTo).not.toHaveBeenCalled()
    details.open = true; motion.reveal(panel); motion.cancel(panel)
    expect(state.animations[0]!.kill).toHaveBeenCalledOnce()
    expect(panel.style.visibility).toBe(''); w.unmount()
  })
  it('shares media subscriptions while keeping each component animation isolated', () => {
    const firstHost = host(), firstMotion = motion
    const secondHost = host(), secondMotion = motion
    expect(state.add).toHaveBeenCalledOnce()
    const first = document.createElement('div'), second = document.createElement('div')
    firstMotion.enter(first); secondMotion.enter(second)
    firstHost.unmount()
    expect(first.style.visibility).toBe('')
    expect(second.style.opacity).toBe('0')
    expect(state.revert).not.toHaveBeenCalled()
    secondHost.unmount()
    expect(second.style.visibility).toBe('')
    expect(state.revert).toHaveBeenCalledOnce()
  })

})
