<!-- ManifestDialog.vue — Manifest 查看 / 单行策略 / JSON 后备 -->
<template>
  <v-dialog v-model="model" max-width="960">
    <v-card :title="`${t('artifacts.manifest')} · ${os}/${arch}`">
      <v-card-text>
        <ErrorAlert :error="error" />

        <div v-if="loading" class="d-flex justify-center py-8">
          <v-progress-circular color="primary" indeterminate />
        </div>

        <template v-else-if="manifest">
          <div class="d-flex flex-wrap ga-4 mb-3">
            <div class="flex-grow-1" style="min-width: 280px">
              <div class="text-caption text-medium-emphasis">{{ t('artifacts.manifestRootHash') }}</div>
              <CopyField :value="manifest.root_hash || '—'" />
            </div>

            <div>
              <div class="text-caption text-medium-emphasis">{{ t('artifacts.manifestCount') }}</div>
              <div class="text-h6">{{ manifest.count }}</div>
            </div>
          </div>

          <template v-if="!jsonMode">
            <EmptyState v-if="editRows.length === 0" icon="mdi-file-table-outline" :text="t('artifacts.manifestEmpty')" />

            <v-table v-else class="rounded-lg" density="compact">
              <thead>
                <tr>
                  <th>{{ t('installPolicy.path') }}</th>
                  <th>{{ t('artifacts.size') }}</th>
                  <th>{{ t('artifacts.sha256') }}</th>
                  <th>{{ t('installPolicy.md5') }}</th>
                  <th>{{ t('artifacts.installPolicy') }}</th>
                </tr>
              </thead>

              <tbody>
                <tr v-for="entry in editRows" :key="entry.path">
                  <td>
                    <code>{{ entry.path }}</code>

                    <v-chip v-if="entry.install_policy === 'KEEP_IF_EXISTS' && !editing" class="ml-2" color="accent" size="x-small">
                      {{ t('artifacts.keepIfExists') }}
                    </v-chip>
                  </td>

                  <td>{{ entry.size }}</td>
                  <td><code class="text-caption">{{ shortHash(entry.sha256) }}</code></td>
                  <td><code class="text-caption">{{ shortHash(entry.md5) }}</code></td>

                  <td style="min-width: 180px">
                    <v-select
                      v-if="editing"
                      v-model="entry.install_policy"
                      density="compact"
                      hide-details
                      :items="policyItems"
                    />

                    <span v-else>{{ entry.install_policy === 'KEEP_IF_EXISTS' ? t('artifacts.keepIfExists') : t('artifacts.overwrite') }}</span>
                  </td>
                </tr>
              </tbody>
            </v-table>
          </template>

          <template v-else>
            <v-textarea
              v-model="jsonText"
              auto-grow
              class="changelog-area"
              :error-messages="jsonError ? [String(jsonError)] : []"
              :rows="16"
            />

            <div class="text-caption text-medium-emphasis">
              [{ "path": "bin/app", "size": 123, "sha256": "…", "md5": "…", "install_policy": "OVERWRITE" | "KEEP_IF_EXISTS" }]
            </div>
          </template>
        </template>
      </v-card-text>

      <v-card-actions>
        <v-spacer />
        <v-btn v-if="editing || jsonMode" variant="text" @click="cancelEdit">{{ t('common.cancel') }}</v-btn>

        <v-btn
          v-if="!editing && !jsonMode"
          :disabled="!manifest || editRows.length === 0"
          prepend-icon="mdi-pencil-outline"
          variant="tonal"
          @click="startEdit"
        >
          {{ t('artifacts.manifestEdit') }}
        </v-btn>

        <v-btn
          v-if="editing && !jsonMode"
          prepend-icon="mdi-code-json"
          variant="text"
          @click="startJson"
        >
          JSON
        </v-btn>

        <v-btn v-if="editing || jsonMode" color="primary" :loading="saving" @click="save">{{ t('common.save') }}</v-btn>
        <v-btn v-else variant="text" @click="model = false">{{ t('common.close') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
  import type { ManifestEntry, ManifestResult } from '@/api/generated'
  import { computed, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { getManifest, putManifest } from '@/api/generated'
  import CopyField from '@/components/CopyField.vue'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import { useSnackbarStore } from '@/stores/snackbar'

  const props = defineProps<{ projectRef: string, version: string, os: string, arch: string }>()
  const model = defineModel<boolean>({ default: false })
  const emit = defineEmits<{ changed: [] }>()

  const { t } = useI18n()
  const snackbar = useSnackbarStore()

  const manifest = ref<ManifestResult | null>(null)
  const loading = ref(false)
  const saving = ref(false)
  const editing = ref(false)
  const jsonMode = ref(false)
  const jsonText = ref('')
  const jsonError = ref<unknown>(null)
  const error = ref<unknown>(null)
  const editRows = ref<ManifestEntry[]>([])

  const policyItems = computed(() => [
    { title: t('artifacts.overwrite'), value: 'OVERWRITE' },
    { title: t('artifacts.keepIfExists'), value: 'KEEP_IF_EXISTS' },
  ])

  watch(model, open => {
    if (open) void load()
    else {
      editing.value = false
      jsonMode.value = false
    }
  }, { immediate: true })

  async function load (): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const { data } = await getManifest({
        path: {
          project_ref: props.projectRef,
          version: props.version,
          os: props.os,
          arch: props.arch,
        },
      })
      manifest.value = data ?? null
      editRows.value = cloneEntries(data?.entries ?? [])
    } catch (error_) {
      error.value = error_
    } finally {
      loading.value = false
    }
  }

  function cloneEntries (entries: ManifestEntry[]): ManifestEntry[] {
    return entries.map(entry => ({ ...entry }))
  }

  function startEdit (): void {
    editRows.value = cloneEntries(manifest.value?.entries ?? [])
    jsonError.value = null
    jsonMode.value = false
    editing.value = true
  }

  function startJson (): void {
    jsonText.value = JSON.stringify(editRows.value, null, 2)
    jsonError.value = null
    jsonMode.value = true
  }

  function cancelEdit (): void {
    editing.value = false
    jsonMode.value = false
    jsonError.value = null
    editRows.value = cloneEntries(manifest.value?.entries ?? [])
  }

  async function save (): Promise<void> {
    let entries: ManifestEntry[]
    if (jsonMode.value) {
      try {
        entries = JSON.parse(jsonText.value) as ManifestEntry[]
        if (!Array.isArray(entries)) throw new Error('expected array')
      } catch (error_) {
        jsonError.value = error_
        return
      }
    } else {
      entries = editRows.value
    }
    saving.value = true
    jsonError.value = null
    try {
      const { data } = await putManifest({
        path: {
          project_ref: props.projectRef,
          version: props.version,
          os: props.os,
          arch: props.arch,
        },
        body: { entries },
      })
      manifest.value = data ?? null
      editRows.value = cloneEntries(data?.entries ?? [])
      editing.value = false
      jsonMode.value = false
      snackbar.show(t('common.save') + ' ✓')
      emit('changed')
    } catch (error_) {
      jsonError.value = error_
      error.value = error_
    } finally {
      saving.value = false
    }
  }

  function shortHash (value: string | undefined): string {
    if (!value) return '—'
    return value.length > 16 ? `${value.slice(0, 16)}…` : value
  }
</script>
