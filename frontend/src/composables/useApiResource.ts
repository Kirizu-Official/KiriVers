/**
 * composables/useApiResource.ts
 *
 * 列表资源的加载/刷新/错误状态包装：
 * - fetcher 返回数组；
 * - loading / error 直接绑定页面；
 * - 错误经拦截器归一化为 ApiError，交给 ErrorAlert 呈现。
 */
import { onMounted, ref, shallowRef, type ShallowRef } from 'vue'

export interface ApiResource<T> {
  items: ShallowRef<T[]>
  loading: ReturnType<typeof ref<boolean>>
  error: ReturnType<typeof shallowRef<unknown>>
  refresh: () => Promise<void>
}

export function useApiResource<T> (fetcher: () => Promise<T[]>): ApiResource<T> {
  const items = shallowRef<T[]>([]) as ShallowRef<T[]>
  const loading = ref(false)
  const error = shallowRef<unknown>(null)

  async function refresh (): Promise<void> {
    loading.value = true
    error.value = null
    try {
      items.value = await fetcher()
    } catch (error_) {
      error.value = error_
    } finally {
      loading.value = false
    }
  }

  onMounted(() => {
    void refresh()
  })

  return { items, loading, error, refresh }
}
