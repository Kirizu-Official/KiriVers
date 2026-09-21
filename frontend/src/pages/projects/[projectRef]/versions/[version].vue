<!-- pages/projects/[projectRef]/versions/[version].vue — version layout: lifecycle; 产物/灰度从列表进入 -->
<template>
  <div>
    <div v-if="loading" class="d-flex justify-center py-12">
      <v-progress-circular color="primary" indeterminate />
    </div>

    <template v-else-if="version">
      <div class="d-flex flex-wrap align-center ga-3 mb-4">
        <v-btn icon="mdi-arrow-left" size="small" variant="text" @click="router.push(`/projects/${projectRef}/versions`)" />
        <div class="text-h5 font-weight-bold">{{ displayVersion(version) }}</div>
        <StatusChip kind="version" :status="version.status ?? ''" />
        <v-chip color="primary" size="small" variant="tonal">{{ version.channel }}</v-chip>
        <v-chip v-if="version.is_lts" size="small" variant="tonal">LTS</v-chip>
        <v-chip v-if="version.is_critical" color="error" size="small" variant="tonal">critical</v-chip>

        <v-chip
          v-if="version.status !== 'draft' && !version.gray_completed_at"
          color="warning"
          size="small"
          variant="tonal"
        >
          {{ t('versions.grayActive') }} {{ version.actual_percent ?? 0 }}%
        </v-chip>

        <v-spacer />

        <v-btn prepend-icon="mdi-eye-outline" size="small" variant="tonal" @click="checkOpen = true">
          {{ t('versions.checkPreview') }}
        </v-btn>

        <v-btn prepend-icon="mdi-shield-check-outline" size="small" variant="tonal" @click="integrityOpen = true">
          {{ t('integrity.title') }}
        </v-btn>
      </div>

      <ErrorAlert :error="pageError" />

      <v-card class="mb-6">
        <v-card-text class="d-flex flex-wrap ga-2 align-center">
          <v-btn
            v-if="version.status === 'draft'"
            color="success"
            :loading="acting === 'publish'"
            prepend-icon="mdi-publish"
            @click="act('publish')"
          >
            {{ t('versions.publish') }}
          </v-btn>

          <v-btn
            v-if="version.status === 'published'"
            color="warning"
            :loading="acting === 'deprecate'"
            prepend-icon="mdi-archive-outline"
            variant="tonal"
            @click="act('deprecate')"
          >
            {{ t('versions.deprecate') }}
          </v-btn>

          <v-btn
            v-if="version.status === 'published' || version.status === 'deprecated'"
            color="error"
            prepend-icon="mdi-block-helper"
            variant="tonal"
            @click="revokeDialog = true"
          >
            {{ t('versions.revoke') }}
          </v-btn>

          <v-btn
            v-if="version.status === 'draft'"
            color="error"
            prepend-icon="mdi-delete-outline"
            variant="text"
            @click="deleteDialog = true"
          >
            {{ t('versions.deleteVersion') }}
          </v-btn>

          <v-divider class="mx-2" vertical />

          <v-select
            v-model="promoteTarget"
            density="compact"
            hide-details
            :items="channelOptions"
            :label="t('versions.promoteTo')"
            style="max-width: 160px"
          />

          <v-btn
            color="primary"
            :disabled="!promoteTarget || promoteTarget === version.channel"
            :loading="acting === 'promote'"
            prepend-icon="mdi-arrow-up-bold-box-outline"
            variant="tonal"
            @click="promote"
          >
            {{ t('versions.promote') }}
          </v-btn>

          <v-btn prepend-icon="mdi-pencil-outline" size="small" variant="text" @click="openMetaEdit">
            {{ t('common.edit') }}
          </v-btn>
        </v-card-text>
      </v-card>

      <RouterView />
    </template>

    <ErrorAlert v-else :error="pageError" />

    <CheckPreviewDialog
      v-model="checkOpen"
      :project-ref="projectRef"
      :suggested-arch="lineArchOptions[0] ?? 'x86_64'"
      :suggested-os="lineOsOptions[0] ?? 'windows'"
      :suggested-version="version ? displayVersion(version) : ''"
    />

    <IntegrityDialog
      v-model="integrityOpen"
      :arch-options="lineArchOptions"
      :os-options="lineOsOptions"
      :project-ref="projectRef"
      :suggested-arch="lineArchOptions[0] ?? 'x86_64'"
      :suggested-os="lineOsOptions[0] ?? 'windows'"
      :version="versionParam"
    />

    <ConfirmDialog
      v-model="revokeDialog"
      :confirm-word="version ? displayVersion(version) : ''"
      icon="mdi-block-helper"
      :text="t('versions.revokeConfirm')"
      :title="t('versions.revoke')"
      @confirm="act('revoke')"
    />

    <ConfirmDialog
      v-model="deleteDialog"
      icon="mdi-delete-alert-outline"
      :text="t('common.confirm')"
      :title="t('versions.deleteVersion')"
      @confirm="deleteDraft"
    />

    <v-dialog v-model="metaEditDialog" max-width="560">
      <v-card :title="t('versions.editVersionMeta')">
        <v-card-text>
          <ErrorAlert :error="metaError" />

          <v-row dense>
            <v-col cols="12" md="6">
              <v-text-field
                v-model="metaForm.min_source_version"
                :hint="t('versions.minSourceHint')"
                :label="t('versions.minSourceVersion')"
                persistent-hint
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-switch
                v-model="metaForm.is_lts"
                color="primary"
                density="compact"
                :hint="t('versions.isLtsHint')"
                :label="t('versions.isLts')"
                persistent-hint
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-switch
                v-model="metaForm.is_critical"
                color="primary"
                density="compact"
                :hint="t('versions.isCriticalHint')"
                :label="t('versions.isCritical')"
                persistent-hint
              />
            </v-col>
          </v-row>
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="metaEditDialog = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" :loading="metaSaving" @click="saveMetaEdit">{{ t('common.save') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script lang="ts" setup>
  import type { Version } from '@/api/generated'
  import { computed, onMounted, provide, reactive, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute, useRouter } from 'vue-router'
  import {
    deleteVersion,
    deprecateVersion,
    getVersion,
    listChannels,
    listVersionLines,
    patchVersion,
    promoteVersion,
    publishVersion,
    revokeVersion,
  } from '@/api/generated'
  import CheckPreviewDialog from '@/components/CheckPreviewDialog.vue'
  import ConfirmDialog from '@/components/ConfirmDialog.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import IntegrityDialog from '@/components/IntegrityDialog.vue'
  import StatusChip from '@/components/StatusChip.vue'
  import { useJobsStore } from '@/stores/jobs'
  import { useSnackbarStore } from '@/stores/snackbar'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/versions/[version]')
  const router = useRouter()
  const snackbar = useSnackbarStore()
  const jobs = useJobsStore()

  const projectRef = computed(() => route.params.projectRef)
  const versionParam = computed(() => route.params.version)

  const version = ref<Version | null>(null)
  const loading = ref(true)
  const pageError = ref<unknown>(null)
  const acting = ref<string | null>(null)
  const checkOpen = ref(false)
  const integrityOpen = ref(false)
  const revokeDialog = ref(false)
  const deleteDialog = ref(false)
  const promoteTarget = ref('')
  const channelOptions = ref<string[]>(['stable', 'beta', 'alpha'])
  const lineOsOptions = ref<string[]>([])
  const lineArchOptions = ref<string[]>([])

  provide('versionRecord', version)
  provide('reloadVersion', load)

  function displayVersion (item: Version): string {
    return item.version_semver ?? String(item.version_integer ?? item.id?.slice(0, 8) ?? '')
  }

  async function load (): Promise<void> {
    loading.value = true
    pageError.value = null
    try {
      const { data: detail } = await getVersion({
        path: { project_ref: projectRef.value, version: versionParam.value },
      })
      version.value = detail ?? null
      jobs.schedule()
      try {
        const { data } = await listVersionLines({
          path: { project_ref: projectRef.value, version: versionParam.value },
        })
        const lines = data?.lines ?? []
        lineOsOptions.value = Array.from(new Set(lines.flatMap(row => row.os ? [row.os] : [])))
        lineArchOptions.value = Array.from(new Set(lines.flatMap(row => row.arch ? [row.arch] : [])))
      } catch {
        lineOsOptions.value = []
        lineArchOptions.value = []
      }
    } catch (error) {
      pageError.value = error
    } finally {
      loading.value = false
    }
  }

  async function act (action: 'publish' | 'deprecate' | 'revoke'): Promise<void> {
    acting.value = action
    pageError.value = null
    try {
      if (action === 'publish') {
        await publishVersion({ path: { project_ref: projectRef.value, version: versionParam.value } })
      }
      if (action === 'deprecate') {
        await deprecateVersion({ path: { project_ref: projectRef.value, version: versionParam.value } })
      }
      if (action === 'revoke') {
        await revokeVersion({ path: { project_ref: projectRef.value, version: versionParam.value } })
      }
      snackbar.show(t(`versions.${action}`) + ' ✓')
      await load()
    } catch (error) {
      pageError.value = error
    } finally {
      acting.value = null
    }
  }

  async function promote (): Promise<void> {
    if (!promoteTarget.value) return
    acting.value = 'promote'
    pageError.value = null
    try {
      await promoteVersion({
        path: { project_ref: projectRef.value, version: versionParam.value },
        body: { target_channel: promoteTarget.value },
      })
      snackbar.show(t('versions.promote') + ' ✓')
      await load()
    } catch (error) {
      pageError.value = error
    } finally {
      acting.value = null
    }
  }

  async function deleteDraft (): Promise<void> {
    try {
      await deleteVersion({ path: { project_ref: projectRef.value, version: versionParam.value } })
      snackbar.show(t('common.delete') + ' ✓')
      await router.push(`/projects/${projectRef.value}/versions`)
    } catch (error) {
      pageError.value = error
    }
  }

  const metaEditDialog = ref(false)
  const metaSaving = ref(false)
  const metaError = ref<unknown>(null)
  const metaForm = reactive({
    min_source_version: '',
    is_lts: false,
    is_critical: false,
  })

  function openMetaEdit (): void {
    if (!version.value) return
    metaForm.min_source_version = version.value.min_source_version ?? ''
    metaForm.is_lts = Boolean(version.value.is_lts)
    metaForm.is_critical = Boolean(version.value.is_critical)
    metaError.value = null
    metaEditDialog.value = true
  }

  async function saveMetaEdit (): Promise<void> {
    metaSaving.value = true
    metaError.value = null
    try {
      const { data: updated } = await patchVersion({
        path: { project_ref: projectRef.value, version: versionParam.value },
        body: {
          min_source_version: metaForm.min_source_version.trim() || undefined,
          is_lts: metaForm.is_lts,
          is_critical: metaForm.is_critical,
        },
      })
      version.value = updated ?? version.value
      metaEditDialog.value = false
      snackbar.show(t('common.save') + ' ✓')
    } catch (error) {
      metaError.value = error
    } finally {
      metaSaving.value = false
    }
  }

  onMounted(async () => {
    await load()
    try {
      const channels = await listChannels({ path: { project_ref: projectRef.value } })
      channelOptions.value = (channels.data?.channels ?? []).flatMap(c => c.slug ? [c.slug] : [])
    } catch {
      // keep defaults
    }
  })
  watch(versionParam, () => {
    void load()
  })
</script>
