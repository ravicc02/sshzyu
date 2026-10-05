import { useEffect, useRef, useState, type ReactNode } from 'react'
import { ChevronDownIcon, CloseIcon } from './icons'

interface ConfigDrawerProps {
  /** 生图参数面板（尺寸 / 质量 / 格式 / 透明背景 / 数量…） */
  children: ReactNode
}

export default function ConfigDrawer({ children }: ConfigDrawerProps) {
  const [open, setOpen] = useState(false)
  const [desktop, setDesktop] = useState(() => window.innerWidth >= 768)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)
  const visible = desktop || open

  const close = () => {
    setOpen(false)
    triggerRef.current?.focus()
  }

  useEffect(() => {
    const resize = () => setDesktop(window.innerWidth >= 768)
    window.addEventListener('resize', resize)
    return () => window.removeEventListener('resize', resize)
  }, [])

  useEffect(() => setOpen(false), [desktop])

  useEffect(() => {
    if (!open || desktop) return
    closeRef.current?.focus()
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false)
        triggerRef.current?.focus()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, desktop])

  return (
    <>
      {!desktop && open && (
        <div
          data-config-backdrop
          className="fixed inset-0 z-40 bg-black/40 md:hidden"
          onClick={close}
          aria-hidden
        />
      )}

      <aside
        data-no-drag-select
        className={`fixed right-0 top-14 bottom-0 z-50 flex w-72 max-w-[88vw] flex-col border-l border-gray-200 bg-white transition-transform duration-200 ease-out motion-reduce:transition-none md:z-30 dark:border-white/[0.08] dark:bg-gray-950 ${
          visible ? 'translate-x-0' : 'translate-x-full'
        }`}
        id="generation-config-panel"
        aria-label="生图配置"
        aria-hidden={!visible}
        inert={!visible}
        role={desktop ? undefined : 'dialog'}
        aria-modal={!desktop && open ? true : undefined}
      >
        <div className="flex shrink-0 items-center justify-between border-b border-gray-200 px-4 py-3.5 dark:border-white/[0.08]">
          <div className="min-w-0">
            <h2 className="text-[13px] font-bold tracking-tight text-gray-800 dark:text-gray-100">生图配置</h2>
            <p className="mt-0.5 truncate text-[11px] text-gray-400 dark:text-gray-500">尺寸 · 质量 · 格式 · 数量</p>
          </div>
          {!desktop && (
            <button
              ref={closeRef}
              type="button"
              onClick={close}
              className="rounded-md p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-white/[0.06] dark:hover:text-gray-300"
              aria-label="关闭配置"
            >
              <CloseIcon className="h-4 w-4" />
            </button>
          )}
        </div>

        <div className="flex-1 overflow-y-auto p-3">{children}</div>
      </aside>

      {!desktop && (
        <button
          ref={triggerRef}
          type="button"
          onClick={() => setOpen(true)}
          className={`mobile-panel-trigger right-3 md:hidden${
            open ? ' hidden' : ''
          }`}
          aria-label="打开生图配置"
          aria-controls="generation-config-panel"
          aria-expanded={open}
        >
          <span className="shrink-0 text-[13px] font-semibold">配置</span>
          <span className="h-3.5 w-px shrink-0 bg-gray-200 dark:bg-white/15" aria-hidden />
          <span className="min-w-0 flex-1 truncate text-[12px] font-medium text-gray-600 dark:text-gray-300">生图参数</span>
          <ChevronDownIcon className="h-3.5 w-3.5 shrink-0 text-gray-500 dark:text-gray-400" />
        </button>
      )}
    </>
  )
}
