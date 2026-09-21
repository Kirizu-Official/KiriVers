<!-- EChart.vue — tree-shaken Apache ECharts wrapper; theme follows Vuetify MD3. -->
<template>
  <div class="echart-host" :style="{ height: `${height}px` }">
    <VChart
      autoresize
      class="echart-canvas"
      :option="merged"
      :update-options="{ notMerge: true }"
    />

    <div v-if="isEmpty" class="echart-empty text-caption text-medium-emphasis">
      {{ t('common.empty') }}
    </div>
  </div>
</template>

<script lang="ts" setup>
  import type { EChartsCoreOption } from 'echarts/core'
  import { BarChart, LineChart, PieChart } from 'echarts/charts'
  import {
    DataZoomComponent,
    GridComponent,
    LegendComponent,
    TitleComponent,
    TooltipComponent,
  } from 'echarts/components'
  import { use } from 'echarts/core'
  import { CanvasRenderer } from 'echarts/renderers'
  import { computed } from 'vue'
  import VChart from 'vue-echarts'
  import { useI18n } from 'vue-i18n'
  import { useChartTheme } from '@/composables/useChartTheme'

  use([
    CanvasRenderer,
    PieChart,
    BarChart,
    LineChart,
    GridComponent,
    TooltipComponent,
    LegendComponent,
    DataZoomComponent,
    TitleComponent,
  ])

  const props = withDefaults(defineProps<{
    option: EChartsCoreOption
    height?: number
  }>(), { height: 280 })

  const { t } = useI18n()
  const theme = useChartTheme()

  const isEmpty = computed(() => seriesEmpty(props.option))

  const merged = computed<EChartsCoreOption>(() => {
    const colors = theme.value
    return {
      backgroundColor: 'transparent',
      textStyle: { color: colors.text },
      aria: { enabled: true },
      tooltip: {
        backgroundColor: colors.tooltipBg,
        borderColor: colors.border,
        textStyle: { color: colors.text },
      },
      legend: { textStyle: { color: colors.muted } },
      ...props.option,
    }
  })

  function seriesEmpty (option: EChartsCoreOption): boolean {
    const series = option.series
    const list = Array.isArray(series) ? series : (series ? [series] : [])
    if (list.length === 0) {
      return true
    }
    return list.every(item => {
      const data = (item as { data?: unknown[] }).data
      if (!Array.isArray(data) || data.length === 0) {
        return true
      }
      return data.every(point => {
        if (typeof point === 'number') {
          return point === 0
        }
        if (point && typeof point === 'object' && 'value' in point) {
          return Number((point as { value?: number }).value ?? 0) === 0
        }
        return false
      })
    })
  }
</script>

<style scoped>
.echart-host {
  position: relative;
  width: 100%;
}

.echart-canvas {
  width: 100%;
  height: 100%;
}

.echart-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}
</style>
