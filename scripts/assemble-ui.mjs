import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { checkAssetGraph } from './static-asset-graph.mjs';
import { randomUUID } from 'node:crypto';
function publishFile(source, target) {
  const temporary = `${target}.stage-${randomUUID()}`;
  try { fs.copyFileSync(source, temporary); fs.renameSync(temporary, target); }
  finally { if (fs.existsSync(temporary)) fs.unlinkSync(temporary); }
}

export function assembleUI({ dist, current, shared }) {
  const source = path.join(dist, 'assets');
  checkAssetGraph({ htmlFile: path.join(dist, 'index.html'), assetsDir: source, publicDir: dist });
  fs.mkdirSync(path.join(current, 'assets'), { recursive: true });
  fs.mkdirSync(shared, { recursive: true });
  const bootstrap = path.join(source, 'fw-cachebust.js');
  if (!fs.existsSync(bootstrap)) throw new Error('Build bootstrap missing');
  let added = 0;
  // Validate collisions before changing any published bootstrap or HTML. CRLF
  // differences from an older checkout are harmless for these text resources.
  const entries = fs.readdirSync(source, { withFileTypes: true });
  for (const entry of entries) {
    if (!entry.isFile()) throw new Error(`Unsupported nested asset directory: ${entry.name}`);
    if (entry.name === 'fw-cachebust.js') continue;
    const a = path.join(source, entry.name), b = path.join(shared, entry.name);
    if (!fs.existsSync(b)) continue;
    const bytesA = fs.readFileSync(a), bytesB = fs.readFileSync(b);
    const equal = bytesA.equals(bytesB) || (/\.(?:m?js|css|svg|json|map|txt)$/i.test(entry.name) && bytesA.toString('utf8').replace(/\r\n/g, '\n') === bytesB.toString('utf8').replace(/\r\n/g, '\n'));
    if (!equal) throw new Error(`Immutable asset content differs: ${entry.name}`);
  }
  for (const entry of entries) {
    if (entry.name === 'fw-cachebust.js') continue;
    const a = path.join(source, entry.name), b = path.join(shared, entry.name);
    if (!fs.existsSync(b)) { fs.copyFileSync(a, b); added++; }
    fs.copyFileSync(a, path.join(current, 'assets', entry.name));
  }
  for (const entry of fs.readdirSync(dist, { withFileTypes: true })) {
    if (entry.isFile() && entry.name !== 'index.html') fs.copyFileSync(path.join(dist, entry.name), path.join(current, entry.name));
  }
  const checked = checkAssetGraph({ htmlFile: path.join(dist, 'index.html'), assetsDir: shared, publicDir: current, bootstrapOverride: bootstrap });
  // Assets first; fixed bootstrap and entry last. Preserve all historical files.
  publishFile(bootstrap, path.join(shared, 'fw-cachebust.js'));
  publishFile(bootstrap, path.join(current, 'assets', 'fw-cachebust.js'));
  publishFile(path.join(dist, 'index.html'), path.join(current, 'index.html'));
  checkAssetGraph({ htmlFile: path.join(current, 'index.html'), assetsDir: shared, publicDir: current });
  return { added, checkedAssets: checked.assetCount };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = fileURLToPath(new URL('../', import.meta.url));
  const result = assembleUI({ dist: path.join(root, 'backend/internal/web/dist'), current: path.join(root, 'ui/current'), shared: path.join(root, 'ui/shared/assets') });
  console.log(`UI assembled: ${result.added} assets added; ${result.checkedAssets} reachable assets verified`);
}
