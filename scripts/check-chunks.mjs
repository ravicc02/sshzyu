// Fail closed on a missing entry, absolute bootstrap import, dynamic chunk or CSS asset.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { checkAssetGraph } from './static-asset-graph.mjs';
const root = fileURLToPath(new URL('../', import.meta.url));
const current = path.join(root, 'ui/current');
const shared = path.join(root, 'ui/shared/assets');
const a = path.join(current, 'assets/fw-cachebust.js');
const b = path.join(shared, 'fw-cachebust.js');
if (!fs.existsSync(a) || !fs.existsSync(b) || !fs.readFileSync(a).equals(fs.readFileSync(b))) throw new Error('Current/shared bootstrap mismatch or missing bootstrap');
const result = checkAssetGraph({ htmlFile: path.join(current, 'index.html'), assetsDir: shared, publicDir: current });
console.log(process.argv.includes('--json') ? JSON.stringify(result) : `Entry module graph OK: ${result.assetCount} reachable JS/CSS/media assets`);
