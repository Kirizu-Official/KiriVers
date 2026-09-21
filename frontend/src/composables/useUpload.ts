/**
 * composables/useUpload.ts
 *
 * 产物上传三通道的统一状态机：
 * - direct：可选 WebCrypto SHA-256 预计算 → 一次性 PUT（onUploadProgress）；
 * - tus：TUS 1.0 分片（默认 8MiB），支持断点续传（localStorage 记录
 *   upload_id/offset，上传前 HEAD 校验服务端 offset）与取消（Abort）；
 * - presign：请求预签名后始终走 API/TUS URL（direct_s3 恒 false，不得 PUT 未知 S3 键）。
 */
import type { Artifact } from '@/api/generated'
import { computed, reactive } from 'vue'
import { axiosInstance } from '@/api/client'
import {
  createTusUpload,
  deleteTusUpload,
  headTusUpload,
  patchTusUpload,
  presignArtifactUpload,
  putLineArtifact,
} from '@/api/generated'

export type UploadChannel = 'direct' | 'tus' | 'presign'
export type UploadPhase = 'idle' | 'hashing' | 'uploading' | 'done' | 'failed' | 'canceled'

export interface UploadOptions {
  channel: UploadChannel
  file: File
  projectRef: string
  version: string
  os: string
  arch: string
  hwRev?: string
  minHwRev?: string
  maxHwRev?: string
  compatibleHwRevs?: string[]
  precomputeSha?: boolean
  chunkMiB?: number
}

export interface UploadState {
  phase: UploadPhase
  progress: number
  message: string
  loaded: number
  total: number
  bytesPerSec: number
}

type LinePath = {
  project_ref: string
  version: string
  os: string
  arch: string
}

function sessionKey (opts: UploadOptions): string {
  return ['kirivers.tus', opts.projectRef, opts.version, `${opts.os}/${opts.arch}`, opts.file.name, opts.file.size].join('|')
}

async function sha256OfFile (file: File): Promise<string> {
  const buffer = await file.arrayBuffer()
  const digest = await crypto.subtle.digest('SHA-256', buffer)
  return Array.from(new Uint8Array(digest)).map(b => b.toString(16).padStart(2, '0')).join('')
}

function encodeTusMetadata (meta: Record<string, string | undefined>): string {
  return Object.entries(meta)
    .filter(([, value]) => value !== undefined && value !== '')
    .map(([key, value]) => `${key} ${btoa(String(value))}`)
    .join(',')
}

function artifactQuery (opts: UploadOptions): {
  filename?: string
  hw_rev?: string
  min_hw_rev?: string
  max_hw_rev?: string
  compatible_hw_revs?: string
} {
  return {
    filename: opts.file.name,
    hw_rev: opts.hwRev || undefined,
    min_hw_rev: opts.minHwRev || undefined,
    max_hw_rev: opts.maxHwRev || undefined,
    compatible_hw_revs: opts.compatibleHwRevs?.filter(Boolean).join(',') || undefined,
  }
}

type ProgressFn = (event: { loaded?: number, total?: number }) => void

export function createByteProgressTracker () {
  let lastAt = 0
  let lastLoaded = 0
  let ema = 0

  function resetSpeed (): void {
    lastAt = 0
    lastLoaded = 0
    ema = 0
  }

  function sample (loaded: number): number {
    const now = Date.now()
    if (lastAt > 0 && now > lastAt) {
      const inst = (loaded - lastLoaded) / ((now - lastAt) / 1000)
      ema = ema > 0 ? ema * 0.7 + inst * 0.3 : inst
    }
    lastAt = now
    lastLoaded = loaded
    return ema
  }

  return { resetSpeed, sample }
}

async function uploadDirect (
  opts: UploadOptions,
  path: LinePath,
  sha: string | undefined,
  signal: AbortSignal,
  track: ProgressFn,
): Promise<Artifact | null> {
  const { data } = await putLineArtifact({
    path,
    query: artifactQuery(opts),
    body: opts.file,
    headers: sha ? { 'X-Content-SHA256': sha } : undefined,
    signal,
    onUploadProgress: event => track(event),
  })
  return data ?? null
}

async function uploadPresign (
  opts: UploadOptions,
  path: LinePath,
  sha: string | undefined,
  signal: AbortSignal,
  track: ProgressFn,
): Promise<Artifact | null> {
  const { data: presigned } = await presignArtifactUpload({
    path,
    body: { filename: opts.file.name, size: opts.file.size },
  })
  const url = String(presigned?.upload_url ?? '')
  if (presigned?.direct_s3) {
    throw new Error('direct S3 artifact upload is not supported')
  }
  const artifact = await axiosInstance.put<Artifact>(url, opts.file, {
    params: artifactQuery(opts),
    headers: sha ? { 'X-Content-SHA256': sha } : undefined,
    signal,
    onUploadProgress: event => track(event),
  })
  return artifact.data
}

async function resumeTusOffset (opts: UploadOptions): Promise<{ uploadId: string, offset: number } | null> {
  const stored = localStorage.getItem(sessionKey(opts))
  if (!stored) {
    return null
  }
  try {
    const parsed = JSON.parse(stored) as { uploadId?: string }
    const head = await headTusUpload({
      path: { project_ref: opts.projectRef, upload_id: parsed.uploadId ?? '' },
      headers: { 'Tus-Resumable': '1.0.0' },
      throwOnError: true,
    })
    const resumeOffset = Number(head.headers['upload-offset'] ?? 0)
    if (resumeOffset > 0 && resumeOffset < opts.file.size) {
      return { uploadId: parsed.uploadId ?? '', offset: resumeOffset }
    }
  } catch {
    localStorage.removeItem(sessionKey(opts))
  }
  return null
}

async function createTusSession (
  opts: UploadOptions,
  path: LinePath,
  sha: string | undefined,
): Promise<string> {
  const created = await createTusUpload({
    path,
    headers: {
      'Tus-Resumable': '1.0.0',
      'Upload-Length': opts.file.size,
      'Upload-Metadata': encodeTusMetadata({
        filename: opts.file.name,
        hw_rev: opts.hwRev,
        min_hw_rev: opts.minHwRev,
        max_hw_rev: opts.maxHwRev,
        compatible_hw_revs: opts.compatibleHwRevs?.filter(Boolean).join(','),
        sha256: sha,
      }),
    },
    throwOnError: true,
  })
  const uploadId = String(created.headers.location ?? created.headers.Location ?? '').split('/').pop() ?? ''
  if (!uploadId) {
    throw new Error('TUS create: missing Location header')
  }
  return uploadId
}

async function uploadTus (
  opts: UploadOptions,
  path: LinePath,
  sha: string | undefined,
  signal: AbortSignal,
  state: UploadState,
  speed: ReturnType<typeof createByteProgressTracker>,
): Promise<null> {
  const chunkSize = (opts.chunkMiB ?? 8) * 1024 * 1024
  const resumed = await resumeTusOffset(opts)
  let offset = resumed?.offset ?? 0
  let uploadId = resumed?.uploadId ?? ''
  if (resumed) {
    state.message = `resume@${offset}`
  }
  if (!uploadId) {
    uploadId = await createTusSession(opts, path, sha)
  }
  localStorage.setItem(sessionKey(opts), JSON.stringify({ uploadId, offset }))

  while (offset < opts.file.size) {
    if (signal.aborted) {
      throw new DOMException('canceled', 'AbortError')
    }
    const end = Math.min(offset + chunkSize, opts.file.size)
    const chunk = opts.file.slice(offset, end)
    const result = await patchTusUpload({
      path: { project_ref: opts.projectRef, upload_id: uploadId },
      body: chunk,
      headers: {
        'Tus-Resumable': '1.0.0',
        'Upload-Offset': offset,
        'Content-Type': 'application/offset+octet-stream',
      },
      signal,
      onUploadProgress: event => {
        const loaded = offset + (event.loaded ?? 0)
        state.loaded = loaded
        state.total = opts.file.size
        state.bytesPerSec = speed.sample(loaded)
        state.progress = Math.round((loaded / opts.file.size) * 100)
      },
      throwOnError: true,
    })
    offset = Number(result.headers['upload-offset'] ?? offset)
    state.loaded = offset
    state.total = opts.file.size
    state.bytesPerSec = speed.sample(offset)
    state.progress = Math.round((offset / opts.file.size) * 100)
    localStorage.setItem(sessionKey(opts), JSON.stringify({ uploadId, offset }))
  }
  localStorage.removeItem(sessionKey(opts))
  return null
}

export function useUpload () {
  const state = reactive<UploadState>({
    phase: 'idle',
    progress: 0,
    message: '',
    loaded: 0,
    total: 0,
    bytesPerSec: 0,
  })
  const speed = createByteProgressTracker()
  let controller: AbortController | null = null

  const isBusy = computed(() => state.phase === 'hashing' || state.phase === 'uploading')

  function reset (): void {
    state.phase = 'idle'
    state.progress = 0
    state.message = ''
    state.loaded = 0
    state.total = 0
    state.bytesPerSec = 0
    speed.resetSpeed()
  }

  function track (event: { loaded?: number, total?: number }): void {
    const loaded = event.loaded ?? 0
    const total = event.total ?? 0
    state.loaded = loaded
    state.total = total
    state.bytesPerSec = speed.sample(loaded)
    if (total) {
      state.progress = Math.round((loaded / total) * 100)
    }
  }

  function finish (artifact: Artifact | null): Artifact | null {
    state.phase = 'done'
    state.progress = 100
    if (state.total > 0) {
      state.loaded = state.total
    }
    return artifact
  }

  async function run (opts: UploadOptions): Promise<Artifact | null> {
    reset()
    controller = new AbortController()
    const signal = controller.signal

    try {
      let sha: string | undefined
      if (opts.precomputeSha) {
        state.phase = 'hashing'
        sha = await sha256OfFile(opts.file)
        if (signal.aborted) {
          throw new DOMException('canceled', 'AbortError')
        }
      }

      state.phase = 'uploading'
      state.progress = 0
      const path: LinePath = {
        project_ref: opts.projectRef,
        version: opts.version,
        os: opts.os,
        arch: opts.arch,
      }

      if (opts.channel === 'direct') {
        state.message = opts.file.name
        return finish(await uploadDirect(opts, path, sha, signal, track))
      }
      if (opts.channel === 'presign') {
        return finish(await uploadPresign(opts, path, sha, signal, track))
      }
      return finish(await uploadTus(opts, path, sha, signal, state, speed))
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        state.phase = 'canceled'
      } else {
        state.phase = 'failed'
        state.message = error instanceof Error ? error.message : String(error)
        throw error
      }
      return null
    }
  }

  async function cancel (opts: UploadOptions, uploadId?: string): Promise<void> {
    controller?.abort()
    if (uploadId) {
      await deleteTusUpload({
        path: { project_ref: opts.projectRef, upload_id: uploadId },
        headers: { 'Tus-Resumable': '1.0.0' },
        throwOnError: true,
      }).catch(() => undefined)
    }
    if (state.phase === 'uploading') {
      state.phase = 'canceled'
    }
  }

  return { state, isBusy, run, cancel, reset }
}
