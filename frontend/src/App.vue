<!-- App.vue — 按路由 meta.layout 分发外壳：默认控制台壳 / 登录空白壳 -->
<template>
  <v-app id="inspire">
    <DefaultShell v-if="!isBlank">
      <RouterView />
    </DefaultShell>

    <v-main v-else>
      <RouterView />
    </v-main>

    <GlobalSnackbar />
  </v-app>
</template>

<script lang="ts" setup>
  import { computed, onMounted } from 'vue'
  import { useRoute } from 'vue-router'
  import GlobalSnackbar from '@/components/GlobalSnackbar.vue'
  import DefaultShell from '@/layouts/DefaultShell.vue'
  import { useJobsStore } from '@/stores/jobs'
  import { useUiStore } from '@/stores/ui'

  const route = useRoute()
  const ui = useUiStore()
  const jobs = useJobsStore()

  const isBlank = computed(() => route.meta.layout === 'blank')

  onMounted(() => {
    ui.bind()
    jobs.schedule()
  })
</script>
