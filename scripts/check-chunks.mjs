// 防白屏检查：入口与所有 chunk 的动态引用是否都能解析到实体文件。
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const CUR = path.resolve(fileURLToPath(new URL('../ui/current/assets', import.meta.url)));
const SITE = path.resolve(fileURLToPath(new URL('../ui/shared/assets', import.meta.url)));

const curFiles = new Set(fs.readdirSync(CUR));
const siteFiles = fs.readdirSync(SITE).filter((f) => f.endsWith('.js'));

// 固定文件名的 fw-cachebust.js 由 Nginx 从 shared/assets 服务。
// 它必须与 current 构建产物完全一致，否则 index.html 会加载错误的入口 chunk。
const currentCacheBust = path.join(CUR, 'fw-cachebust.js');
const sharedCacheBust = path.join(SITE, 'fw-cachebust.js');
if (!fs.existsSync(currentCacheBust) || !fs.existsSync(sharedCacheBust)) {
  throw new Error('fw-cachebust.js 缺失于 current/assets 或 shared/assets');
}
if (fs.readFileSync(currentCacheBust, 'utf8') !== fs.readFileSync(sharedCacheBust, 'utf8')) {
  throw new Error('fw-cachebust.js 与 current 构建产物不一致');
}
console.log('fw-cachebust.js: current/shared 一致');

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
  // 剥离 ?v= query（fw-cachebust.js?v=<hash> 这类入口引用），否则会把带 query 的路径当文件名检查
  const file = p.split('?')[0];
  const ok = fs.existsSync(path.join(SITE, file));
  if (!ok) { missPre++; console.log('   MISSING preload:', p); }
}
console.log('index.html asset refs:', preloads.length, ' missing:', missPre);