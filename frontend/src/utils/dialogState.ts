let sequence = 0
const scrollLocks = new WeakMap<Document, Set<symbol>>()

export function nextDialogId(): string {
  return `modal-title-${++sequence}`
}

export function setDialogScrollLock(owner: symbol, active: boolean): void {
  let owners = scrollLocks.get(document)
  if (!owners) {
    owners = new Set()
    scrollLocks.set(document, owners)
  }
  if (active) owners.add(owner)
  else owners.delete(owner)
  document.body.classList.toggle('modal-open', owners.size > 0)
}
