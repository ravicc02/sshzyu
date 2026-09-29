<template>
  <AppLayout>
    <div class="image-playground-layout">
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

const { locale } = useI18n()
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
