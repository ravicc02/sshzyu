// 防白屏检查：入口与所有 chunk 的动态引用是否都能解析到实体文件。
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const CUR = path.resolve(fileURLToPath(new URL('../ui/current/assets', import.meta.url)));
const SITE = path.resolve(fileURLToPath(new URL('../ui/shared/assets', import.meta.url)));

const curFiles = new Set(fs.readdirSync(CUR));
const siteFiles = fs.readdirSync(SITE).filter((f) => f.endsWith('.js'));

let refs = 0, missing = [], missingInCurrent = 0;
const re = /["']\.\/([A-Za-z0-9_.\-]+\.(?:js|css|svg|png|woff2?))["']/g;

for (const f of siteFiles) {
  const s = fs.readFileSync(path.join(SITE, f), 'utf8');
  for (const m of s.matchAll(re)) {
    refs++;
    const target = m[1];
    if (!fs.existsSync(path.join(SITE, target))) missing.push(`${f} -> ${target}`);
    if (!curFiles.has(target)) missingInCurrent++;
  }
}

console.log('js files scanned:', siteFiles.length);
console.log('relative refs found:', refs);
console.log('MISSING targets in shared/assets:', missing.length);
if (missing.length) for (const m of missing.slice(0, 15)) console.log('   ', m);
console.log('current/assets missing (any ref):', missingInCurrent);

// index.html 的 modulepreload 目标是否都在
const idx = fs.readFileSync(path.resolve(fileURLToPath(new URL('../ui/current/index.html', import.meta.url))), 'utf8');
const preloads = [...idx.matchAll(/(?:src|href)="\/assets\/([^"]+)"/g)].map((m) => m[1]);
let missPre = 0;
for (const p of preloads) {
  const ok = fs.existsSync(path.join(SITE, p));
  if (!ok) { missPre++; console.log('   MISSING preload:', p); }
}
console.log('index.html asset refs:', preloads.length, ' missing:', missPre);