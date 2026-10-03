import { getCurrentScope, onScopeDispose } from 'vue'

export function createRequestGuard() {
  let generation = 0
  const invalidate = () => { generation++ }
  if (getCurrentScope()) onScopeDispose(invalidate)
  return {
    start() {
      const current = ++generation
      return () => current === generation
    },
    invalidate
  }
}
