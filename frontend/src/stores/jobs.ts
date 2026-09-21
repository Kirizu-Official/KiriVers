import type { Job } from '@/api/generated'
import { defineStore } from 'pinia'
/**
 * stores/jobs.ts
 *
 * 会话内任务登记与轮询：一键发版/差量生成等返回 job_id 后登记，
 * 每 2s 轮询 GET /admin/jobs/:id，终态后停止。顶栏徽章读取
 * runningCount。无 Job 列表端点（后端仅按 id 查询），故只跟踪
 * 本会话启动的任务。
 */
import { computed, ref } from 'vue'
import { getAdminJob } from '@/api/generated'

const POLL_MS = 2000

export interface JobReleaseLine {
  os?: string
  arch?: string
  status?: string
}

export interface TrackedJob {
  jobId: string
  label: string
  type: string
  projectId: string
  status: Job['status'] | 'unknown'
  progress: number
  errorMessage?: string
  result?: unknown
}

export function jobLines (result: unknown): JobReleaseLine[] {
  if (result && typeof result === 'object' && 'lines' in result) {
    return (result as { lines: JobReleaseLine[] }).lines ?? []
  }
  return []
}

let timer: ReturnType<typeof setTimeout> | null = null

export const useJobsStore = defineStore('jobs', () => {
  const jobs = ref<TrackedJob[]>([])
  const runningCount = computed(() => jobs.value.filter(j => j.status === 'queued' || j.status === 'running').length)

  function track (jobId: string, label: string, type: string, projectId: string): void {
    jobs.value.unshift({
      jobId,
      label,
      type,
      projectId,
      status: 'queued',
      progress: 0,
    })
    schedule()
  }

  function schedule (): void {
    if (timer) {
      return
    }
    timer = setTimeout(async () => {
      timer = null
      const active = jobs.value.filter(j => j.status === 'queued' || j.status === 'running')
      if (active.length === 0) {
        return
      }
      await Promise.all(active.map(async tracked => {
        try {
          const { data: job } = await getAdminJob({ path: { job_id: tracked.jobId } })
          tracked.status = job?.status
          tracked.progress = job?.progress ?? 0
          tracked.errorMessage = job?.error_message ?? undefined
          tracked.result = job?.result
        } catch {
          tracked.status = 'unknown'
        }
      }))
      schedule()
    }, POLL_MS)
  }

  function clearFinished (): void {
    jobs.value = jobs.value.filter(j => j.status === 'queued' || j.status === 'running')
  }

  return { jobs, runningCount, track, clearFinished, schedule }
})
