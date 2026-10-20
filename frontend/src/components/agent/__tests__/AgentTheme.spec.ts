import { describe, expect, it } from 'vitest'
import postcss from 'postcss'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
const theme = readFileSync(resolve('src/styles/workspace-shell.css'), 'utf8')
import workspace from '@/views/user/AgentView.vue?raw'
import question from '../AgentQuestion.vue?raw'

const components = import.meta.glob<string>('../*.vue', { query: '?raw', import: 'default', eager: true })
function css(source: string) { return source.match(/<style[^>]*>([\s\S]*?)<\/style>/)?.[1] || source }
function declarations(source: string, selector: string) {
  const result: Record<string, string> = {}
  postcss.parse(css(source)).walkRules(rule => {
    if (rule.selector === selector) rule.walkDecls(decl => { result[decl.prop] = decl.value })
  })
  return result
}
function definitions(source: string) { return [...source.matchAll(/["']?(--[\w-]+)["']?\s*:/g)].map(match => match[1]!) }
function color(value: string, tokens: Record<string, string>) {
  const token = value.match(/^var\((--[\w-]+)\)$/)?.[1]
  const hex = token ? tokens[token] : value
  if (!hex || !/^#[\da-f]{6}$/i.test(hex)) throw new Error(`Unresolved color: ${value}`)
  return [1, 3, 5].map(index => parseInt(hex.slice(index, index + 2), 16) / 255)
}
function luminance(rgb: number[]) { return rgb.map(value => value <= .04045 ? value / 12.92 : ((value + .055) / 1.055) ** 2.4).reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index]!, 0) }
function contrast(a: number[], b: number[]) { const x = luminance(a), y = luminance(b); return (Math.max(x, y) + .05) / (Math.min(x, y) + .05) }

describe('Agent theme color contracts', () => {
  it('resolves every component color token or an explicit fallback', () => {
    const inherited = [...definitions(theme), ...definitions(css(workspace))]
    for (const [name, source] of Object.entries(components)) {
      const available = new Set([...inherited, ...definitions(source)])
      const missing = [...css(source).matchAll(/var\(\s*(--[\w-]+)\s*([,)])/g)].filter(match => match[2] !== ',' && !available.has(match[1]!)).map(match => match[1])
      expect(missing, `Undefined CSS variables in ${name}`).toEqual([])
    }
  })
  it('gates pointer hover feedback so it does not stick on touch screens', () => {
    for (const [name, source] of Object.entries(components)) {
      postcss.parse(css(source)).walkRules(rule => {
        if (!rule.selector.includes(':hover')) return
        let owner = rule.parent
        let gated = false
        while (owner) { if (owner.type === 'atrule' && 'params' in owner && String(owner.params).includes('hover')) gated = true; owner = owner.parent }
        expect(gated, `Ungated hover feedback in ${name}: ${rule.selector}`).toBe(true)
      })
    }
  })
  it.each(['light', 'dark'])('keeps enabled and disabled confirmation text readable in %s mode', mode => {
    const tokens = { ...declarations(theme, '.workspace-shell'), ...(mode === 'dark' ? declarations(theme, '.dark .workspace-shell') : {}) }
    const normal = declarations(question, '.question-submit')
    const disabled = { ...normal, ...declarations(question, '.question-submit:disabled') }
    for (const state of [normal, disabled]) expect(contrast(color(state.color!, tokens), color(state.background!, tokens))).toBeGreaterThanOrEqual(4.5)
    expect(Number(disabled.opacity)).toBe(1)
  })
})
