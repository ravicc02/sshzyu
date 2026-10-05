// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ConfigDrawer from './ConfigDrawer'

let container: HTMLDivElement
let root: Root

async function resize(width: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width })
  await act(async () => { window.dispatchEvent(new Event('resize')) })
}

async function renderPanel(width: number) {
  await resize(width)
  await act(async () => {
    root.render(<ConfigDrawer><button type="button">选择质量</button></ConfigDrawer>)
  })
}

beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})

afterEach(async () => {
  await act(async () => root.unmount())
  container.remove()
  vi.unstubAllGlobals()
})

describe('responsive generation configuration', () => {
  it('shows configuration permanently on desktop without a floating opener', async () => {
    await renderPanel(1200)
    expect(container.querySelector('aside')?.getAttribute('aria-hidden')).not.toBe('true')
    expect(container.querySelector('[aria-label="打开生图配置"]')).toBeNull()
    expect(container.querySelector('[aria-label="关闭配置"]')).toBeNull()
  })

  it('opens a mobile drawer and closes it with Escape', async () => {
    await renderPanel(390)
    expect(container.querySelector('aside')?.getAttribute('aria-hidden')).toBe('true')
    await act(async () => {
      container.querySelector<HTMLButtonElement>('[aria-label="打开生图配置"]')!.click()
    })
    expect(container.querySelector('aside')?.getAttribute('aria-hidden')).toBe('false')
    await act(async () => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    })
    expect(container.querySelector('aside')?.getAttribute('aria-hidden')).toBe('true')
  })

  it('closes the mobile drawer with its close button and backdrop', async () => {
    await renderPanel(390)
    const open = async () => act(async () => {
      container.querySelector<HTMLButtonElement>('[aria-label="打开生图配置"]')!.click()
    })
    await open()
    await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="关闭配置"]')!.click())
    expect(container.querySelector('aside')?.getAttribute('aria-hidden')).toBe('true')
    await open()
    await act(async () => container.querySelector<HTMLElement>('[data-config-backdrop]')!.click())
    expect(container.querySelector('aside')?.getAttribute('aria-hidden')).toBe('true')
  })

  it('switches to a visible desktop panel and resets the mobile drawer on resize', async () => {
    await renderPanel(390)
    await act(async () => container.querySelector<HTMLButtonElement>('[aria-label="打开生图配置"]')!.click())
    await resize(1024)
    expect(container.querySelector('[data-config-backdrop]')).toBeNull()
    expect(container.querySelector('aside')?.getAttribute('aria-hidden')).not.toBe('true')
    await resize(390)
    expect(container.querySelector('aside')?.getAttribute('aria-hidden')).toBe('true')
  })
})
