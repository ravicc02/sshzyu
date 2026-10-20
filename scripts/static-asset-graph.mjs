import fs from 'node:fs';
import path from 'node:path';

const assetFile = /\.(?:m?js|css|svg|png|jpe?g|webp|gif|avif|ico|woff2?|ttf|otf|eot)(?:[?#].*)?$/i;
export function assetReferences(text, parentUrl) {
  const refs = new Set();
  const add = ref => {
    if (!assetFile.test(ref) || /^(?:https?:|data:|blob:|\/\/|#)/i.test(ref)) return;
    const resolved = new URL(ref, `http://local.invalid${parentUrl}`).pathname;
    refs.add(decodeURIComponent(resolved));
  };
  // Imports and Vite's explicitly rooted dependency map, rather than arbitrary
  // relative-looking strings belonging to external SDK public paths.
  for (const match of text.matchAll(/(?:\bfrom\s*|\bimport\s*(?:\(\s*)?)["']([^"'\s]+)["']/g)) add(match[1]);
  for (const match of text.matchAll(/["']((?:assets\/)[A-Za-z0-9_./-]+\.(?:m?js|css|svg|png|jpe?g|webp|gif|avif|ico|woff2?|ttf|otf|eot)(?:[?#][^"'\s]*)?)["']/g)) add(match[1].startsWith('assets/') ? `/${match[1]}` : match[1]);
  for (const match of text.matchAll(/\b(?:src|href)\s*=\s*["']([^"']+)["']/g)) add(match[1]);
  for (const match of text.matchAll(/new\s+URL\(\s*["']([^"']+)["']\s*,\s*import\.meta\.url\s*\)/g)) add(match[1]);
  if (/\.css$/i.test(parentUrl)) for (const match of text.matchAll(/url\(\s*["']?([^\s)'";]+)["']?\s*\)/g)) add(match[1]);
  return [...refs];
}

// Validate only the module graph used by this entry. Historical assets stay on
// disk without forcing a scan of every version or accepting an unvalidated entry.
export function checkAssetGraph({ htmlFile, assetsDir, publicDir, bootstrapOverride }) {
  const html = fs.readFileSync(htmlFile, 'utf8');
  const queue = assetReferences(html, '/');
  const seen = new Set(), missing = [];
  while (queue.length) {
    const url = queue.shift();
    if (seen.has(url)) continue;
    seen.add(url);
    const root = url.startsWith('/assets/') ? assetsDir : publicDir;
    const relative = url.startsWith('/assets/') ? url.slice('/assets/'.length) : url.slice(1);
    const file = url === '/assets/fw-cachebust.js' && bootstrapOverride ? bootstrapOverride : path.resolve(root, relative);
    if (!(url === '/assets/fw-cachebust.js' && bootstrapOverride)) {
      const rel = path.relative(path.resolve(root), file);
      if (rel.startsWith('..') || path.isAbsolute(rel)) throw new Error(`Asset path escapes its root: ${url}`);
    }
    if (!fs.existsSync(file) || !fs.statSync(file).isFile()) { missing.push(url); continue; }
    if (/\.(?:m?js|css)$/i.test(file)) queue.push(...assetReferences(fs.readFileSync(file, 'utf8'), url));
  }
  if (missing.length) throw new Error(`Missing ${missing.length} asset(s):\n${missing.join('\n')}`);
  return { urls: [...seen], assetCount: seen.size };
}
