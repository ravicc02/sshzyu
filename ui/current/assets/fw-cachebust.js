// SSHZYU UI cache-bust bootstrap  (2026-09-12)
// 目的：浏览器对 /assets/ 下的 chunk 是 immutable 一年缓存。当某个 chunk 被
// 热补丁过（文件名未变）时，已缓存用户会一直拿到旧副本。入口 index.html 是
// no-cache，每次都会回源，所以在这里先强制回源校验被补丁过的 chunk，
// 再加载真正的 Vue 入口。任何一步失败都不阻塞页面。
const BUST = [
  '/assets/AppLayout.vue_vue_type_script_setup_true_lang-2ODOra5a.js',
];
await Promise.all(
  BUST.map((u) => fetch(u, { cache: 'no-cache' }).catch(() => {}))
);
await import('/assets/index-wat8wBTj.js');
