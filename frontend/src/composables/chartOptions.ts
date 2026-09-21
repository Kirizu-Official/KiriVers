import type { NameCount } from '@/api/generated'
import type { ChartTheme } from '@/composables/useChartTheme'
/**
 * composables/chartOptions.ts — reusable ECharts option fragments for NameCount buckets.
 */
import type { EChartsCoreOption } from 'echarts/core'

function bucketData (buckets: NameCount[] | undefined): Array<{ name: string, value: number }> {
  return (buckets ?? []).map(row => ({
    name: row.name && row.name !== '' ? row.name : '—',
    value: row.count ?? 0,
  }))
}

export function pieOption (theme: ChartTheme, buckets: NameCount[] | undefined): EChartsCoreOption {
  return {
    color: theme.palette,
    tooltip: { trigger: 'item' },
    legend: { type: 'scroll', bottom: 0 },
    series: [{
      type: 'pie',
      radius: ['42%', '68%'],
      avoidLabelOverlap: true,
      itemStyle: { borderRadius: 6 },
      data: bucketData(buckets),
    }],
  }
}

export function barOption (theme: ChartTheme, buckets: NameCount[] | undefined): EChartsCoreOption {
  const data = bucketData(buckets)
  return {
    color: [theme.primary],
    tooltip: { trigger: 'axis' },
    legend: { show: false },
    grid: { left: 48, right: 16, top: 24, bottom: 48 },
    dataZoom: data.length > 8 ? [{ type: 'inside' }, { type: 'slider', height: 16 }] : [{ type: 'inside' }],
    xAxis: {
      type: 'category',
      data: data.map(row => row.name),
      axisLabel: { color: theme.muted, hideOverlap: true },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: theme.muted },
      splitLine: { lineStyle: { color: theme.border } },
    },
    series: [{ type: 'bar', data: data.map(row => row.value), barMaxWidth: 36 }],
  }
}

export function lineSeriesOption (
  theme: ChartTheme,
  categories: string[],
  series: Array<{ name: string, data: number[], color?: string }>,
): EChartsCoreOption {
  return {
    color: series.map(row => row.color).filter(Boolean) as string[],
    tooltip: { trigger: 'axis' },
    legend: { type: 'scroll', bottom: 0 },
    grid: { left: 48, right: 16, top: 24, bottom: 56 },
    dataZoom: [{ type: 'inside' }, { type: 'slider', height: 16 }],
    xAxis: {
      type: 'category',
      data: categories,
      axisLabel: { color: theme.muted, hideOverlap: true },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: theme.muted },
      splitLine: { lineStyle: { color: theme.border } },
    },
    series: series.map(row => ({
      name: row.name,
      type: 'line',
      smooth: true,
      showSymbol: categories.length <= 24,
      data: row.data,
    })),
  }
}

export function dualBarOption (
  theme: ChartTheme,
  categories: string[],
  left: { name: string, data: number[], color?: string },
  right: { name: string, data: number[], color?: string },
): EChartsCoreOption {
  return {
    color: [left.color ?? theme.success, right.color ?? theme.error],
    tooltip: { trigger: 'axis' },
    legend: { bottom: 0 },
    grid: { left: 48, right: 16, top: 24, bottom: 56 },
    dataZoom: [{ type: 'inside' }, { type: 'slider', height: 16 }],
    xAxis: {
      type: 'category',
      data: categories,
      axisLabel: { color: theme.muted, hideOverlap: true },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: theme.muted },
      splitLine: { lineStyle: { color: theme.border } },
    },
    series: [
      { name: left.name, type: 'bar', data: left.data, barMaxWidth: 28 },
      { name: right.name, type: 'bar', data: right.data, barMaxWidth: 28 },
    ],
  }
}
