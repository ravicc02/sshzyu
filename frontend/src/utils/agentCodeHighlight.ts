import hljs from 'highlight.js/lib/core'
import javascript from 'highlight.js/lib/languages/javascript'
import typescript from 'highlight.js/lib/languages/typescript'
import json from 'highlight.js/lib/languages/json'
import bash from 'highlight.js/lib/languages/bash'
import python from 'highlight.js/lib/languages/python'
import xml from 'highlight.js/lib/languages/xml'
import css from 'highlight.js/lib/languages/css'
import sql from 'highlight.js/lib/languages/sql'
import go from 'highlight.js/lib/languages/go'
import yaml from 'highlight.js/lib/languages/yaml'
import markdown from 'highlight.js/lib/languages/markdown'
import java from 'highlight.js/lib/languages/java'
import cpp from 'highlight.js/lib/languages/cpp'
import csharp from 'highlight.js/lib/languages/csharp'
import rust from 'highlight.js/lib/languages/rust'
import diff from 'highlight.js/lib/languages/diff'
import dockerfile from 'highlight.js/lib/languages/dockerfile'
import powershell from 'highlight.js/lib/languages/powershell'
import DOMPurify from 'dompurify'

for (const [name, grammar] of Object.entries({ javascript, typescript, json, bash, python, xml, css, sql, go, yaml, markdown, java, cpp, csharp, rust, diff, dockerfile, powershell })) hljs.registerLanguage(name, grammar)
export function highlightAgentCode(source: string, language: string): string {
  if (!hljs.getLanguage(language) || source.length > 50000) return ''
  return DOMPurify.sanitize(hljs.highlight(source, { language, ignoreIllegals: true }).value, { ALLOWED_TAGS: ['span'], ALLOWED_ATTR: ['class'], ALLOW_DATA_ATTR: false })
}
