import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import ts from 'typescript'

const inputSource = readFileSync(new URL('../InputBar.tsx', import.meta.url), 'utf8')
const tree = ts.createSourceFile('InputBar.tsx', inputSource, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)

function findComposer(node) {
  if (ts.isJsxElement(node) && node.openingElement.attributes.properties.some(
    (property) => ts.isJsxAttribute(property) && property.name.getText(tree) === 'data-prompt-composer'
  )) return node
  return ts.forEachChild(node, findComposer)
}

describe('prompt composer layout', () => {
  it('keeps the prompt editor and upload/send actions in the same row', () => {
    const composer = findComposer(tree)
    expect(composer).toBeDefined()
    const children = composer.children.filter(ts.isJsxElement)
    expect(children.some((child) => child.getText(tree).includes('contentEditable'))).toBe(true)
    expect(children.some((child) => child.getText(tree).includes('data-prompt-actions'))).toBe(true)
    const actions = children.find((child) => child.getText(tree).includes('data-prompt-actions'))
    expect(actions.getText(tree)).toContain('fileInputRef.current?.click()')
    expect(actions.getText(tree)).toContain('submitCurrentMode()')
  })

  it('reserves both sidebars instead of placing the composer over configuration', () => {
    const css = readFileSync(new URL('../../index.css', import.meta.url), 'utf8')
    const app = readFileSync(new URL('../../App.tsx', import.meta.url), 'utf8')
    expect(css).toContain('width: min(56rem, calc(100% - 33rem))')
    expect(app).toContain('md:mr-72')
  })

  it('uses matching horizontal mobile entries for Key and generation configuration', () => {
    const keys = readFileSync(new URL('../KeySidebar.tsx', import.meta.url), 'utf8')
    const config = readFileSync(new URL('../ConfigDrawer.tsx', import.meta.url), 'utf8')
    expect(keys).toContain('mobile-panel-trigger left-3')
    expect(config).toContain('mobile-panel-trigger right-3')
    expect(config).not.toContain('[writing-mode:vertical-rl]')
    expect(keys).not.toContain('shadow-blue-500/40')
  })

  it('gives both mobile entries the same height, edge spacing, and responsive width', () => {
    const css = readFileSync(new URL('../../index.css', import.meta.url), 'utf8')
    const sharedStyle = css.match(/\.mobile-panel-trigger\s*\{([^}]+)\}/)?.[1]
    expect(sharedStyle).toBeDefined()
    expect(sharedStyle).toContain('top-16')
    expect(sharedStyle).toContain('h-11')
    expect(sharedStyle).toContain('rounded-xl')
    expect(sharedStyle).toContain('width: calc((100% - 2.25rem) / 2)')
  })
})
