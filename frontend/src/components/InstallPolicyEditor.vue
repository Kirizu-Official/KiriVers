<!-- InstallPolicyEditor.vue — project/channel path KEEP/OVERWRITE templates -->
<template>
  <div>
    <ErrorAlert :error="error" />

    <p class="text-body-2 text-medium-emphasis mb-4">{{ t('installPolicy.description') }}</p>

    <div v-if="loading && pairs.length === 0" class="d-flex justify-center py-8">
      <v-progress-circular color="primary" indeterminate />
    </div>

    <EmptyState
      v-else-if="!loading && pairs.length === 0"
      icon="mdi-vector-polyline"
      :text="t('installPolicy.noMatrix')"
    />

    <template v-else>
      <v-row dense>
        <v-col cols="12" md="4">
          <v-select
            v-model="pairKey"
            :items="pairItems"
            :label="t('installPolicy.platform')"
            :loading="loading"
          />
        </v-col>

        <v-col v-if="scope === 'project'" cols="12" md="4">
          <v-select
            v-model="referenceChannel"
            :items="channelItems"
            :label="t('installPolicy.referenceChannel')"
          />
        </v-col>

        <v-col class="d-flex align-center" cols="12" md="4">
          <div class="text-caption text-medium-emphasis">
            {{ t('installPolicy.referenceVersion') }}:
            <strong>{{ referenceVersionLabel }}</strong>
          </div>
        </v-col>
      </v-row>

      <v-table v-if="rows.length > 0" class="rounded-lg mb-4" density="compact">
        <thead>
          <tr>
            <th>{{ t('installPolicy.path') }}</th>
            <th>{{ t('artifacts.size') }}</th>
            <th>{{ t('artifacts.sha256') }}</th>
            <th>{{ t('installPolicy.md5') }}</th>
            <th>{{ t('artifacts.installPolicy') }}</th>
            <th />
          </tr>
        </thead>

        <tbody>
          <tr v-for="row in rows" :key="row.path">
            <td>
              <code>{{ row.path }}</code>

              <v-chip v-if="row.inherited" class="ml-2" size="x-small" variant="tonal">
                {{ t('installPolicy.inherited') }}
              </v-chip>
            </td>

            <td>{{ row.size ?? '—' }}</td>
            <td><code class="text-caption">{{ shortHash(row.sha256) }}</code></td>
            <td><code class="text-caption">{{ shortHash(row.md5) }}</code></td>

            <td style="min-width: 180px">
              <v-select
                v-model="row.install_policy"
                density="compact"
                hide-details
                :items="policyItems"
                @update:model-value="markOwned(row)"
              />
            </td>

            <td class="text-end">
              <v-btn
                v-if="!row.inherited"
                icon="mdi-delete-outline"
                size="x-small"
                variant="text"
                @click="removeRow(row.path)"
              />
            </td>
          </tr>
        </tbody>
      </v-table>

      <EmptyState
        v-else-if="!loading && uncheckedReference.length === 0"
        icon="mdi-file-document-outline"
        :text="t('installPolicy.referenceEmpty')"
      />

      <div v-if="uncheckedReference.length > 0" class="mb-4">
        <div class="text-subtitle-2 mb-2">{{ t('installPolicy.selectReference') }}</div>

        <v-table class="rounded-lg" density="compact">
          <thead>
            <tr>
              <th style="width: 48px" />
              <th>{{ t('installPolicy.path') }}</th>
              <th>{{ t('artifacts.sha256') }}</th>
              <th>{{ t('artifacts.installPolicy') }}</th>
            </tr>
          </thead>

          <tbody>
            <tr v-for="entry in uncheckedReference" :key="entry.path">
              <td>
                <v-btn icon="mdi-plus" size="x-small" variant="text" @click="addFromReference(entry)" />
              </td>

              <td><code>{{ entry.path }}</code></td>
              <td><code class="text-caption">{{ shortHash(entry.sha256) }}</code></td>
              <td>{{ policyLabel(entry.install_policy) }}</td>
            </tr>
          </tbody>
        </v-table>
      </div>

      <v-row class="mb-2" dense>
        <v-col cols="12" md="6">
          <v-text-field
            v-model="manualPath"
            hide-details
            :label="t('installPolicy.manualPath')"
          />
        </v-col>

        <v-col cols="12" md="3">
          <v-select
            v-model="manualPolicy"
            hide-details
            :items="policyItems"
          />
        </v-col>

        <v-col class="d-flex align-center" cols="12" md="3">
          <v-btn prepend-icon="mdi-plus" variant="tonal" @click="addManual">{{ t('installPolicy.addPath') }}</v-btn>
        </v-col>
      </v-row>

      <v-btn color="primary" :loading="saving" @click="save">{{ t('common.save') }}</v-btn>
    </template>
  </div>
</template>

<script lang="ts" setup>
  import type { Channel, InstallPolicyEntry, ManifestEntry, PlatformMatrix } from '@/api/generated'
  import { computed, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import {
    getChannelInstallPolicyRules,
    getInstallPolicyRulesReference,
    getProjectInstallPolicyRules,
    listChannels,
    listMatrix,
    putChannelInstallPolicyRules,
    putProjectInstallPolicyRules,
  } from '@/api/generated'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import { useSnackbarStore } from '@/stores/snackbar'

  type PolicyRow = {
    path: string
    install_policy: 'OVERWRITE' | 'KEEP_IF_EXISTS'
    inherited: boolean
    size?: number
    sha256?: string
    md5?: string
  }

  const props = defineProps<{
    projectRef: string
    scope: 'project' | 'channel'
    channelSlug?: string
  }>()

  const { t } = useI18n()
  const snackbar = useSnackbarStore()

  const loading = ref(false)
  const saving = ref(false)
  const error = ref<unknown>(null)
  const pairs = ref<PlatformMatrix[]>([])
  const pairKey = ref('')
  const channels = ref<Channel[]>([])
  const referenceChannel = ref('stable')
  const rows = ref<PolicyRow[]>([])
  const referenceEntries = ref<ManifestEntry[]>([])
  const referenceVersion = ref<string | null>(null)
  const manualPath = ref('')
  const manualPolicy = ref<'OVERWRITE' | 'KEEP_IF_EXISTS'>('KEEP_IF_EXISTS')

  const policyItems = computed(() => [
    { title: t('artifacts.overwrite'), value: 'OVERWRITE' as const },
    { title: t('artifacts.keepIfExists'), value: 'KEEP_IF_EXISTS' as const },
  ])

  const pairItems = computed(() =>
    pairs.value
      .filter(row => row.os && row.arch)
      .map(row => ({ title: `${row.os}/${row.arch}`, value: `${row.os}/${row.arch}` })),
  )

  const channelItems = computed(() =>
    channels.value.map(ch => ch.slug).filter(Boolean),
  )

  const osArch = computed(() => {
    const [os, arch] = pairKey.value.split('/')
    return { os: os ?? '', arch: arch ?? '' }
  })

  const referenceVersionLabel = computed(() => referenceVersion.value || t('common.none'))

  const uncheckedReference = computed(() =>
    (referenceEntries.value ?? []).filter(entry => entry.path && !rows.value.some(row => row.path === entry.path)),
  )

  const ready = ref(false)

  watch(
    () => [props.projectRef, props.scope, props.channelSlug] as const,
    () => {
      void bootstrap()
    },
    { immediate: true },
  )

  watch([pairKey, referenceChannel], () => {
    if (ready.value && pairKey.value) void loadRules()
  })

  async function bootstrap (): Promise<void> {
    ready.value = false
    loading.value = true
    error.value = null
    try {
      const [matrixRes, channelRes] = await Promise.all([
        listMatrix({ path: { project_ref: props.projectRef } }),
        listChannels({ path: { project_ref: props.projectRef } }),
      ])
      pairs.value = matrixRes.data?.matrix ?? []
      channels.value = channelRes.data?.channels ?? []
      const keys = pairs.value
        .filter(row => row.os && row.arch)
        .map(row => `${row.os}/${row.arch}`)
      if (!pairKey.value || !keys.includes(pairKey.value)) {
        pairKey.value = keys[0] ?? ''
      }
      const slugs = channels.value.flatMap(ch => ch.slug ? [ch.slug] : [])
      if (props.scope === 'channel' && props.channelSlug) {
        referenceChannel.value = props.channelSlug
      } else if (!slugs.includes(referenceChannel.value)) {
        referenceChannel.value = slugs.includes('stable') ? 'stable' : (slugs[0] ?? 'stable')
      }
      if (pairKey.value) await loadRules()
      ready.value = true
    } catch (error_) {
      error.value = error_
    } finally {
      loading.value = false
    }
  }

  async function loadRules (): Promise<void> {
    const { os, arch } = osArch.value
    if (!os || !arch) return
    loading.value = true
    error.value = null
    try {
      const channel = props.scope === 'channel' ? (props.channelSlug ?? '') : referenceChannel.value
      const refRes = await getInstallPolicyRulesReference({
        path: { project_ref: props.projectRef },
        query: { os, arch, channel },
      })
      referenceEntries.value = refRes.data?.entries ?? []
      referenceVersion.value = refRes.data?.version ?? null

      const refByPath = new Map((referenceEntries.value ?? []).map(entry => [entry.path ?? '', entry]))

      if (props.scope === 'channel') {
        const { data } = await getChannelInstallPolicyRules({
          path: { project_ref: props.projectRef, slug: props.channelSlug ?? '' },
          query: { os, arch },
        })
        const owned = new Map((data?.entries ?? []).map(entry => [entry.path ?? '', entry]))
        const next: PolicyRow[] = []
        for (const entry of data?.entries ?? []) {
          if (!entry.path) continue
          const ref = refByPath.get(entry.path)
          next.push(toRow(entry, false, ref))
        }
        for (const entry of data?.effective ?? []) {
          if (!entry.path || owned.has(entry.path)) continue
          const ref = refByPath.get(entry.path)
          next.push(toRow(entry, true, ref))
        }
        rows.value = next
      } else {
        const { data } = await getProjectInstallPolicyRules({
          path: { project_ref: props.projectRef },
          query: { os, arch },
        })
        rows.value = (data?.entries ?? []).filter(entry => entry.path).map(entry => toRow(entry, false, refByPath.get(entry.path ?? '')))
      }
    } catch (error_) {
      error.value = error_
    } finally {
      loading.value = false
    }
  }

  function toRow (entry: InstallPolicyEntry, inherited: boolean, ref?: ManifestEntry): PolicyRow {
    const policy = entry.install_policy === 'KEEP_IF_EXISTS' ? 'KEEP_IF_EXISTS' : 'OVERWRITE'
    return {
      path: entry.path ?? '',
      install_policy: policy,
      inherited,
      size: ref?.size,
      sha256: ref?.sha256,
      md5: ref?.md5,
    }
  }

  function markOwned (row: PolicyRow): void {
    row.inherited = false
  }

  function removeRow (path: string): void {
    rows.value = rows.value.filter(row => row.path !== path)
  }

  function addFromReference (entry: ManifestEntry): void {
    if (!entry.path || rows.value.some(row => row.path === entry.path)) return
    rows.value = [
      ...rows.value,
      {
        path: entry.path,
        install_policy: entry.install_policy === 'KEEP_IF_EXISTS' ? 'KEEP_IF_EXISTS' : 'OVERWRITE',
        inherited: false,
        size: entry.size,
        sha256: entry.sha256,
        md5: entry.md5,
      },
    ]
  }

  function addManual (): void {
    const path = manualPath.value.trim().replaceAll('\\', '/')
    if (!path || rows.value.some(row => row.path === path)) return
    rows.value = [
      ...rows.value,
      { path, install_policy: manualPolicy.value, inherited: false },
    ]
    manualPath.value = ''
  }

  async function save (): Promise<void> {
    const { os, arch } = osArch.value
    if (!os || !arch) return
    saving.value = true
    error.value = null
    const entries = rows.value
      .filter(row => !row.inherited)
      .map(row => ({ path: row.path, install_policy: row.install_policy }))
    try {
      await (props.scope === 'channel'
        ? putChannelInstallPolicyRules({
          path: { project_ref: props.projectRef, slug: props.channelSlug ?? '' },
          query: { os, arch },
          body: { entries },
        })
        : putProjectInstallPolicyRules({
          path: { project_ref: props.projectRef },
          query: { os, arch },
          body: { entries },
        }))
      snackbar.show(t('common.save') + ' ✓')
      await loadRules()
    } catch (error_) {
      error.value = error_
    } finally {
      saving.value = false
    }
  }

  function shortHash (value: string | undefined): string {
    if (!value) return '—'
    return value.length > 16 ? `${value.slice(0, 16)}…` : value
  }

  function policyLabel (policy: string | undefined): string {
    return policy === 'KEEP_IF_EXISTS' ? t('artifacts.keepIfExists') : t('artifacts.overwrite')
  }
</script>
