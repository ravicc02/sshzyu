import DOMPurify from 'dompurify'
let nextDiagram = 0
let diagramQueue: Promise<unknown> = Promise.resolve()
export function isSafeAgentDiagram(source: string): boolean {
  return source.length <= 10000
    && !/%%\s*\{|^\s*---|(?:^|[;\n])\s*click\b|@import|@font-face|url\s*\(|(?:https?|javascript|data|vbscript):|\b(?:img|image|icon)\s*:/im.test(source)
}
export function renderAgentDiagram(source: string, dark: boolean): Promise<string> {
  const job = diagramQueue.then(async () => {
    if (!isSafeAgentDiagram(source)) throw new Error('Unsupported diagram content')
    const { default: mermaid } = await import('mermaid')
    mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', theme: dark ? 'dark' : 'neutral', maxTextSize: 10000, maxEdges: 200, htmlLabels: false, suppressErrorRendering: true })
    const id = `agent-diagram-${++nextDiagram}`
    const host = document.createElement('div')
    host.style.cssText = 'position:absolute;visibility:hidden;pointer-events:none;left:-10000px;width:1000px'
    document.body.appendChild(host)
    try {
      const { svg } = await mermaid.render(id, source, host)
      const clean = DOMPurify.sanitize(svg, { USE_PROFILES: { svg: true, svgFilters: true }, FORBID_TAGS: ['a', 'image', 'foreignObject', 'script', 'iframe', 'use'], ALLOW_DATA_ATTR: false })
      // SVG/CSS may only refer to local definitions, never external resources.
      const externalUrl = [...clean.matchAll(/url\(([^)]+)\)/gi)].some(match => !match[1]!.trim().replace(/^["']|["']$/g, '').startsWith('#'))
      if (/@import|@font-face/i.test(clean) || externalUrl) throw new Error('External diagram resource')
      const tree = new DOMParser().parseFromString(clean, 'image/svg+xml')
      if (tree.documentElement.localName !== 'svg') throw new Error('Invalid diagram SVG')
      const bounds = tree.documentElement.getAttribute('viewBox')?.trim().split(/[\s,]+/).map(Number)
      if (bounds?.length === 4 && bounds.every(Number.isFinite) && bounds[2]! > 0 && bounds[3]! > 0) {
        if (bounds[2]! > 8000 || bounds[3]! > 16000 || bounds[2]! * bounds[3]! > 8000000) throw new Error('Diagram layout too large')
        tree.documentElement.setAttribute('width', String(Math.ceil(bounds[2]!)))
        tree.documentElement.setAttribute('height', String(Math.ceil(bounds[3]!)))
        // Preserve readable labels for ordinary diagrams and bound the minimum
        // canvas size. Wide diagrams scroll inside their own region.
        tree.documentElement.style.minWidth = `${Math.min(2400, Math.ceil(bounds[2]! * .9))}px`
      }

      for (const element of tree.querySelectorAll('*')) {
        for (const attribute of [...element.attributes]) {
          if (['href', 'xlink:href'].includes(attribute.name) && !attribute.value.startsWith('#')) element.removeAttribute(attribute.name)
        }
      }
      return tree.documentElement.outerHTML
    } finally { host.remove() }
  })
  diagramQueue = job.catch(() => undefined)
  return job
}
