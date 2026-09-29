// SSHZYU UI 构建后处理（统一仓库适配版）
// 1) 补丁已源码化，跳过产物文本替换
// 2) 生成 /assets/fw-cachebust.js 引导脚本
// 3) index.html 入口指向 fw-cachebust.js?v=<entry-hash>
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const DIST = path.resolve(fileURLToPath(new URL('../backend/internal/web/dist', import.meta.url)));

const assetsDir = path.join(DIST, 'assets');
const fail = (m) => { console.error('FAIL:', m); process.exit(1); };

console.log('DIST:', DIST);

// --- 1) AppLayout patch 确认 ---
const appLayouts = fs.readdirSync(assetsDir).filter((f) => /^AppLayout.*\.js$/.test(f));
if (appLayouts.length !== 1) fail(`AppLayout chunk 数量异常: ${appLayouts.length}`);
const apName = appLayouts[0];
console.log('AppLayout: OK-SOURCE-PATCHED (补丁已源码化)', apName);

// --- 2) fw-cachebust.js ---
const idxHtml = fs.readFileSync(path.join(DIST, 'index.html'), 'utf8');
const entryMatch = idxHtml.match(/src="\/assets\/(index-[A-Za-z0-9_-]+\.js)"/);
let entryName = entryMatch ? entryMatch[1] : null;
if (!entryName) {
  const bustPath = path.join(assetsDir, 'fw-cachebust.js');
  if (!fs.existsSync(bustPath)) fail('index.html 无原始入口且 fw-cachebust.js 不存在');
  entryName = (fs.readFileSync(bustPath, 'utf8').match(/import\('\/assets\/(index-[A-Za-z0-9_-]+\.js)'\)/) || [])[1];
  if (!entryName) fail('无法从 fw-cachebust.js 解析出入口 chunk');
}
console.log('entry chunk:', entryName);

const bustTemplate = [
  '// SSHZYU UI cache-bust bootstrap  (2026-09-12)',
  '// 目的：浏览器对 /assets/ 下的 chunk 是 immutable 一年缓存。当某个 chunk 被',
  '// 热补丁过（文件名未变）时，已缓存用户会一直拿到旧副本。入口 index.html 是',
  '// no-cache，每次都会回源，所以在这里先强制回源校验被补丁过的 chunk，',
  '// 再加载真正的 Vue 入口。任何一步失败都不阻塞页面。',
  'const BUST = [',
  `  '/assets/${apName}',`,
  '];',
  'await Promise.all(',
  "  BUST.map((u) => fetch(u, { cache: 'no-cache' }).catch(() => {}))",
  ');',
  `await import('/assets/${entryName}');`,
  '',
].join('\n');

fs.writeFileSync(path.join(assetsDir, 'fw-cachebust.js'), bustTemplate, 'utf8');
console.log('fw-cachebust.js generated:', (bustTemplate.length + 'B').padStart(6));

// --- 3) index.html entry ---
let out = idxHtml;
const bustRef = `src="/assets/fw-cachebust.js?v=${entryName.slice(6, -3)}"`;
if (!out.includes('/assets/fw-cachebust.js')) {
  out = out.replace(`src="/assets/${entryName}"`, bustRef);
  fs.writeFileSync(path.join(DIST, 'index.html'), out, 'utf8');
  console.log('index.html: entry -> fw-cachebust.js?v=<entry-hash>');
} else if (!out.includes('fw-cachebust.js?v=')) {
  out = out.replace('src="/assets/fw-cachebust.js"', bustRef);
  fs.writeFileSync(path.join(DIST, 'index.html'), out, 'utf8');
  console.log('index.html: fw-cachebust.js -> +?v=<entry-hash>');
} else {
  console.log('index.html: OK-ALREADY (含版本参数)');
}

console.log('DONE');