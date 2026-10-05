import { useState, type ReactNode } from 'react'
import { CloseIcon, SettingsIcon } from './icons'

interface ConfigDrawerProps {
  /** 生图参数面板（尺寸 / 质量 / 格式 / 透明背景 / 数量…） */
  children: ReactNode
}

/**
 * 右侧「生图配置」抽屉。
 *
 * - 全局生效：嵌入模式（宿主 iframe 内）与独立访问 `/image/` 都会渲染，
 *   因为由 InputBar 挂载，而 InputBar 在两种模式下都存在。
 * - 采用浮层滑出（overlay）而非常驻侧栏：不挤压内容布局，
 *   也不与左侧 KeySidebar 的 `embedded-with-sidebar` 位移规则冲突。
 * - 参数从底部输入栏搬到这里，底部因此只保留输入框 + 上传/发送按钮。
 */
export default function ConfigDrawer({ children }: ConfigDrawerProps) {
  const [open, setOpen] = useState(false)

  return (
    <>
      {open && (
        <div
          className="fixed inset-0 z-40 bg-black/40"
          onClick={() => setOpen(false)}
          aria-hidden
        />
      )}

      <aside
        data-no-drag-select
        className={`fixed right-0 top-0 z-50 flex h-full w-80 max-w-[88vw] flex-col border-l border-gray-200 bg-white shadow-2xl transition-transform duration-300 ease-out dark:border-white/[0.08] dark:bg-gray-950 ${
          open ? 'translate-x-0' : 'translate-x-full'
        }`}
        aria-label="生图配置"
        aria-hidden={!open}
      >
        <div className="flex shrink-0 items-center justify-between border-b border-gray-200 px-4 py-3.5 dark:border-white/[0.08]">
          <div className="min-w-0">
            <h2 className="text-[13px] font-bold tracking-tight text-gray-800 dark:text-gray-100">生图配置</h2>
            <p className="mt-0.5 truncate text-[11px] text-gray-400 dark:text-gray-500">尺寸 · 质量 · 格式 · 数量</p>
          </div>
          <button
            type="button"
            onClick={() => setOpen(false)}
            className="rounded-md p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-white/[0.06] dark:hover:text-gray-300"
            aria-label="关闭配置"
          >
            <CloseIcon className="h-4 w-4" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-3">{children}</div>
      </aside>

      {/* 触发按钮：贴在右边缘的竖向标签，醒目且不遮挡内容 */}
      <button
        type="button"
        onClick={() => setOpen(true)}
        className={`fixed right-0 top-1/2 z-30 flex -translate-y-1/2 items-center gap-1.5 rounded-l-xl border border-r-0 border-gray-200 bg-white/90 px-2 py-3 text-[12px] font-medium text-gray-600 shadow-lg backdrop-blur transition-colors hover:bg-white hover:text-gray-800 dark:border-white/[0.08] dark:bg-gray-900/90 dark:text-gray-300 dark:hover:bg-gray-900 dark:hover:text-gray-100${
          open ? ' hidden' : ''
        }`}
        aria-label="打开生图配置"
      >
        <SettingsIcon className="h-4 w-4" />
        <span className="[writing-mode:vertical-rl]">配置</span>
      </button>
    </>
  )
}