import { describe, expect, it } from 'vitest'
import { effectScope } from 'vue'
import { createRequestGuard } from '../requestGuard'

describe('request guard', () => {
  it('invalidates an older request when a newer one starts', () => {
    const guard = createRequestGuard()
    const previous = guard.start()
    const current = guard.start()
    expect(previous()).toBe(false)
    expect(current()).toBe(true)
  })

  it('invalidates pending work explicitly and accepts the next request', () => {
    const guard = createRequestGuard()
    const current = guard.start()
    guard.invalidate()
    expect(current()).toBe(false)
    expect(guard.start()()).toBe(true)
  })

  it('invalidates work when its owning effect scope is disposed', () => {
    const scope = effectScope()
    const guard = scope.run(createRequestGuard)!
    const current = guard.start()
    scope.stop()
    expect(current()).toBe(false)
  })

  it('keeps independent owners isolated', () => {
    const first = createRequestGuard()
    const second = createRequestGuard()
    const current = second.start()
    first.start()
    first.invalidate()
    expect(current()).toBe(true)
  })
})
