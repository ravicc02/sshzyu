import { Marked, type Token } from 'marked'
import DOMPurify from 'dompurify'

export interface AgentMarkdownToken {
  type: string; raw: string; text?: string; lang?: string; depth?: number
  tokens?: AgentMarkdownToken[]; items?: AgentMarkdownToken[]
  ordered?: boolean; start?: number | ''; task?: boolean; checked?: boolean
  href?: string; title?: string | null
  header?: { tokens: AgentMarkdownToken[] }[]
  rows?: { tokens: AgentMarkdownToken[] }[][]
  align?: ('left' | 'center' | 'right' | null)[]
}

const parser = new Marked({ gfm: true, breaks: true })
parser.use({ extensions: [
  {
    name: 'agent_math_block', level: 'block',
    start: (source: string) => source.search(/(?:^|\n) {0,3}(?:\$\$|\\\[)/),
    tokenizer(source: string) {
      const match = /^ {0,3}\$\$([\s\S]+?)\$\$[ \t]*(?:\n|$)/.exec(source)
        || /^ {0,3}\\\[([\s\S]+?)\\\][ \t]*(?:\n|$)/.exec(source)
      if (match) return { type: 'agent_math_block', raw: match[0], text: match[1].trim() }
    },
  },
  {
    name: 'agent_math_inline', level: 'inline',
    start: (source: string) => source.search(/\$|\\\(/),
    tokenizer(source: string) {
      // Whitespace/currency boundaries keep "$20 and $30" as ordinary text.
      const match = /^\$(?![\s$])((?:\\.|[^$\\\n])+?)\$(?![0-9])/.exec(source)
        || /^\\\(([^\n]+?)\\\)/.exec(source)
      if (match) return { type: 'agent_math_inline', raw: match[0], text: match[1] }
    },
  },
] })

export function parseAgentMarkdown(source: string): AgentMarkdownToken[] {
  return parser.lexer(source) as unknown as AgentMarkdownToken[]
}

// Only renderer-owned Vue components can create controls, classes or SVG.
export function renderAgentInlineToken(token: AgentMarkdownToken): string {
  return DOMPurify.sanitize(parser.Parser.parseInline([token as unknown as Token]), {
    ALLOWED_TAGS: ['br', 'strong', 'em', 'del', 'code', 'a'],
    ALLOWED_ATTR: ['href', 'title'], ALLOW_DATA_ATTR: false,
  })
}

export function sanitizeAgentHtml(source: string): string {
  return DOMPurify.sanitize(source, {
    ALLOWED_TAGS: ['p', 'br', 'strong', 'em', 'del', 'ul', 'ol', 'li', 'blockquote', 'pre', 'code', 'h1', 'h2', 'h3', 'h4', 'hr', 'a', 'table', 'thead', 'tbody', 'tr', 'th', 'td'],
    ALLOWED_ATTR: ['href', 'title', 'start'], ALLOW_DATA_ATTR: false,
  })
}

export function agentMarkdownLink(href?: string): string | undefined {
  if (!href) return undefined
  try {
    const url = new URL(href, window.location.origin)
    return ['http:', 'https:', 'mailto:', 'tel:'].includes(url.protocol) ? href : undefined
  } catch { return undefined }
}
