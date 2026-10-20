import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { checkAssetGraph, assetReferences } from './static-asset-graph.mjs';
import { assembleUI } from './assemble-ui.mjs';
function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'sshzy-ui-graph-'));
  const resolved = path.resolve(root);
  if (path.dirname(resolved) !== path.resolve(os.tmpdir()) || !path.basename(resolved).startsWith('sshzy-ui-graph-')) throw new Error('Unexpected fixture path');
  t.after(() => fs.rmSync(resolved, { recursive: true, force: true }));
  const dist = path.join(root, 'dist'), current = path.join(root, 'current'), shared = path.join(root, 'shared');
  fs.mkdirSync(path.join(dist, 'assets'), { recursive: true });
  fs.mkdirSync(path.join(current, 'assets'), { recursive: true }); fs.mkdirSync(shared);
  fs.writeFileSync(path.join(dist, 'index.html'), '<script type="module" src="/assets/fw-cachebust.js?v=new"></script>');
  const write = (name, body) => fs.writeFileSync(path.join(dist, 'assets', name), body);
  write('fw-cachebust.js', "await import('/assets/index-new.js');");
  write('index-new.js', 'import "./layout-new.js"; const deps=["assets/page-new.js","assets/theme-new.css"];');
  write('layout-new.js', 'export const layout=true;');write('page-new.js', 'export const page=true;');
  write('theme-new.css', 'body{background:url(./logo-new.svg)}');write('logo-new.svg', '<svg/>');
  return { dist, current, shared, write };
}
test('absolute bootstrap import, Vite lazy chunks and CSS URLs are all required', t => {
  const f = fixture(t);fs.unlinkSync(path.join(f.dist, 'assets/index-new.js'));
  assert.throws(() => assembleUI(f), /index-new\.js/);
  assert.equal(fs.existsSync(path.join(f.current, 'index.html')), false);
});
test('missing CSS dependency fails before switching the existing entry', t => {
  const f = fixture(t);fs.writeFileSync(path.join(f.current, 'index.html'), 'previous-entry');
  fs.unlinkSync(path.join(f.dist, 'assets/logo-new.svg'));
  assert.throws(() => assembleUI(f), /logo-new\.svg/);
  assert.equal(fs.readFileSync(path.join(f.current, 'index.html'), 'utf8'), 'previous-entry');
});
test('assembly copies the whole graph, retains history, and publishes the entry last', t => {
  const f = fixture(t);fs.writeFileSync(path.join(f.shared, 'historical-old.js'), 'old');
  const result = assembleUI(f);assert.equal(result.checkedAssets, 6);
  assert.equal(fs.readFileSync(path.join(f.shared, 'historical-old.js'), 'utf8'), 'old');
  const graph = checkAssetGraph({ htmlFile: path.join(f.current, 'index.html'), assetsDir: f.shared, publicDir: f.current });
  assert.equal(graph.assetCount, 6);
});
test('line ending differences retain the existing valid immutable asset', t => {
  const f = fixture(t);fs.writeFileSync(path.join(f.shared, 'layout-new.js'), 'export const layout=true;\r\n');
  f.write('layout-new.js', 'export const layout=true;\n');
  assembleUI(f);assert.equal(fs.readFileSync(path.join(f.shared, 'layout-new.js'), 'utf8'), 'export const layout=true;\r\n');
});
test('a real immutable content collision cannot switch the entry', t => {
  const f = fixture(t);fs.writeFileSync(path.join(f.shared, 'layout-new.js'), 'different-code');
  fs.writeFileSync(path.join(f.current, 'index.html'), 'previous-entry');
  assert.throws(() => assembleUI(f), /Immutable asset content differs/);
  assert.equal(fs.readFileSync(path.join(f.current, 'index.html'), 'utf8'), 'previous-entry');
});
test('SDK paths for third party hosts are not mistaken for local imports', () => {
  const refs = assetReferences('const sdk={payments:"/assets/elements.bundle.min.js"}; import "./real.js";', '/assets/vendor.js');
  assert.deepEqual(refs, ['/assets/real.js']);
});
