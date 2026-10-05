<template>
  <AppLayout>
    <div class="image-playground-layout">
      <div class="image-playground-toolbar">
        <button
          type="button"
          class="image-playground-open-window"
          :disabled="!authStore.token"
          :title="t('common.openInNewWindow')"
          @click="openInNewWindow"
        >
          <svg
            class="image-playground-open-window-icon"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
            <polyline points="15 3 21 3 21 9" />
            <line x1="10" y1="14" x2="21" y2="3" />
          </svg>
          <span>{{ t('common.openInNewWindow') }}</span>
        </button>
      </div>
      <div class="card flex-1 min-h-0 overflow-hidden">
        <!-- Embedded Image Playground (same-origin /image/, fills the content area) -->
        <div class="image-playground-shell">
          <iframe
            :src="embeddedUrl"
            class="image-playground-frame"
            allowfullscreen
          ></iframe>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAuthStore } from '@/stores/auth'
import { buildEmbeddedUrl, detectTheme } from '@/utils/embedded-url'

const { t, locale } = useI18n()
const authStore = useAuthStore()

const pageTheme = ref<'light' | 'dark'>('light')
let themeObserver: MutationObserver | null = null

// 同源相对路径即可满足 new URL()；token/user_id 供生图应用同步用户 key 列表
const embeddedUrl = computed(() =>
  buildEmbeddedUrl(
    `${window.location.origin}/image/`,
    authStore.user?.id,
    authStore.token,
    pageTheme.value,
    locale.value,
  ),
)

/**
 * 在新窗口打开：用「宿主当前最新」token 重新构造 URL。
 *
 * 不能复制 iframe 内的 URL —— 那只是 iframe 加载那一刻注入的 token 快照；
 * 宿主 token 会轮换/过期，新窗口冷启动直接请求 `/api/v1/keys` 就会 401，
 * 表现为「key 获取失败」（iframe 内因已有旧列表兜底而不报错）。
 */
function openInNewWindow() {
  const token = authStore.token
  if (!token) return
  const url = buildEmbeddedUrl(
    `${window.location.origin}/image/`,
    authStore.user?.id,
    token,
    pageTheme.value,
    locale.value,
  )
  window.open(url, '_blank', 'noopener,noreferrer')
}

onMounted(() => {
  pageTheme.value = detectTheme()
  if (typeof document !== 'undefined') {
    themeObserver = new MutationObserver(() => {
      pageTheme.value = detectTheme()
    })
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })
  }
})

onUnmounted(() => {
  themeObserver?.disconnect()
  themeObserver = null
})
</script>

<style scoped>
.image-playground-layout {
  @apply flex flex-col;
  height: calc(100vh - 64px - 4rem);
}

.image-playground-shell {
  @apply relative;
  @apply h-full w-full overflow-hidden rounded-2xl;
  @apply bg-gradient-to-b from-gray-50 to-white dark:from-dark-900 dark:to-dark-950;
}

.image-playground-toolbar {
  @apply mb-2 flex items-center justify-end;
}

.image-playground-open-window {
  @apply inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors;
  @apply border-gray-200 bg-white text-gray-700 hover:bg-gray-50;
  @apply dark:border-white/10 dark:bg-gray-900 dark:text-gray-200 dark:hover:bg-gray-800;
}

.image-playground-open-window:disabled {
  @apply cursor-not-allowed opacity-50;
}

.image-playground-open-window-icon {
  @apply h-4 w-4;
}

.image-playground-frame {
  display: block;
  margin: 0;
  width: 100%;
  height: 100%;
  border: 0;
  border-radius: 0;
  box-shadow: none;
  background: transparent;
}
</style>
