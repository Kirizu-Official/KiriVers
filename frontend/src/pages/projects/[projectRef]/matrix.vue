<!-- pages/projects/[projectRef]/matrix.vue — 平台矩阵管理 -->
<template>
  <div>
    <PageHeader :description="t('matrix.description')" :title="t('matrix.title')">
      <template #actions>
        <v-btn color="primary" prepend-icon="mdi-plus" @click="openCreate">{{ t('matrix.create') }}</v-btn>
      </template>
    </PageHeader>

    <ErrorAlert :error="resource.error.value" />
    <EmptyState v-if="!resource.loading.value && resource.items.value.length === 0" icon="mdi-server-network" :text="t('common.empty')" />

    <v-card v-else>
      <v-data-table
        :headers="headers"
        hide-default-footer
        :items="resource.items.value"
        items-key="id"
        :loading="resource.loading.value"
      >
        <template #item.os="{ item }">
          <code>{{ item.os }}</code> <span class="text-medium-emphasis">/</span> <code>{{ item.arch }}</code>
        </template>

        <template #item.package_type="{ item }">
          <v-chip :color="item.package_type === 'multi_file' ? 'accent' : 'secondary'" size="small" variant="tonal">
            {{ item.package_type === 'multi_file' ? t('matrix.multiFile') : t('matrix.singleFile') }}
          </v-chip>
        </template>

        <template #item.delta_algo="{ item }">
          <template v-if="item.package_type === 'multi_file'">
            <span class="text-medium-emphasis">×{{ item.delta_source_count ?? 3 }}</span>
          </template>

          <template v-else>
            {{ item.delta_algo ?? t('common.none') }} <span class="text-medium-emphasis">×{{ item.delta_source_count ?? 3 }}</span>
          </template>
        </template>

        <template #item.hw_variant_policy="{ item }">
          {{ item.hw_variant_policy === 'higher_compatible_with_lower' ? t('matrix.higherCompatible') : t('matrix.independent') }}
        </template>

        <template #item.actions="{ item }">
          <v-btn prepend-icon="mdi-pencil-outline" size="small" variant="text" @click="openEdit(item)">
            {{ t('common.edit') }}
          </v-btn>
        </template>
      </v-data-table>
    </v-card>

    <v-dialog v-model="dialog" max-width="640">
      <v-card :title="editing ? t('common.edit') : t('matrix.create')">
        <v-card-text>
          <ErrorAlert :error="formError" />

          <v-row dense>
            <v-col cols="12" md="6">
              <v-combobox
                v-model="form.os"
                :disabled="Boolean(editing)"
                :hint="t('matrix.osHint')"
                :items="osItems"
                :label="t('matrix.os')"
                persistent-hint
                :rules="editing ? [] : [required, osArchSlugRule]"
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-combobox
                v-model="form.arch"
                :disabled="Boolean(editing)"
                :hint="t('matrix.archHint')"
                :items="archItems"
                :label="t('matrix.arch')"
                persistent-hint
                :rules="editing ? [] : [required, osArchSlugRule]"
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-select
                v-model="form.package_type"
                :disabled="Boolean(editing) && packageLocked"
                :hint="editing && packageLocked ? t('matrix.packageTypeLocked') : t('matrix.packageTypeHint')"
                :items="[
                  { title: t('matrix.singleFile'), value: 'single_file' },
                  { title: t('matrix.multiFile'), value: 'multi_file' },
                ]"
                :label="t('matrix.packageType')"
                persistent-hint
              />
            </v-col>

            <v-col v-if="form.package_type !== 'multi_file'" cols="12" md="6">
              <v-select
                v-model="form.delta_algo"
                :hint="t('matrix.deltaAlgoHint')"
                :items="['hdiffpatch', 'bsdiff', 'xdelta3']"
                :label="t('matrix.deltaAlgo')"
                persistent-hint
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-text-field
                v-model.number="form.delta_source_count"
                :hint="t('matrix.deltaSourceCountHint')"
                :label="t('matrix.deltaSourceCount')"
                persistent-hint
                type="number"
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-select
                v-model="form.hw_variant_policy"
                :hint="t('matrix.hwVariantPolicyHint')"
                :items="[
                  { title: t('matrix.independent'), value: 'independent' },
                  { title: t('matrix.higherCompatible'), value: 'higher_compatible_with_lower' },
                ]"
                :label="t('matrix.hwVariantPolicy')"
                persistent-hint
              />
            </v-col>

            <v-col cols="12" md="6">
              <v-combobox
                v-model="form.fallback_arch"
                :hint="t('matrix.fallbackArchHint')"
                :items="archItems"
                :label="t('matrix.fallbackArch')"
                persistent-hint
                :rules="[optionalOsArchSlugRule]"
              />
            </v-col>
          </v-row>
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dialog = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" :loading="saving" @click="save">{{ t('common.save') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script lang="ts" setup>
  import type { PlatformCatalogOutput, PlatformMatrix } from '@/api/generated'
  import { computed, onMounted, reactive, ref } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import { createMatrix, getPlatformCatalog, listMatrix, updateMatrix } from '@/api/generated'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import { archComboboxItems, osComboboxItems } from '@/components/platformCatalog'
  import { useApiResource } from '@/composables/useApiResource'
  import { isOsArchSlug } from '@/constants/platforms'
  import { useSnackbarStore } from '@/stores/snackbar'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/matrix')
  const snackbar = useSnackbarStore()

  const projectRef = computed(() => route.params.projectRef)
  const resource = useApiResource<PlatformMatrix>(async () => {
    const { data } = await listMatrix({ path: { project_ref: projectRef.value } })
    return data?.matrix ?? []
  })

  const catalog = ref<PlatformCatalogOutput | null>(null)
  const osItems = computed(() => osComboboxItems(catalog.value?.os))
  const archItems = computed(() => archComboboxItems(catalog.value?.arch))

  const headers = computed(() => [
    { title: 'OS / ARCH', key: 'os' },
    { title: t('matrix.packageType'), key: 'package_type' },
    { title: t('matrix.deltaAlgo'), key: 'delta_algo' },
    { title: t('matrix.hwVariantPolicy'), key: 'hw_variant_policy' },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const dialog = ref(false)
  const editing = ref<PlatformMatrix | null>(null)
  const packageLocked = ref(false)
  const saving = ref(false)
  const formError = ref<unknown>(null)
  const form = reactive({
    os: '',
    arch: '',
    package_type: 'single_file' as 'single_file' | 'multi_file',
    delta_algo: 'hdiffpatch' as 'hdiffpatch' | 'bsdiff' | 'xdelta3',
    delta_source_count: 3,
    hw_variant_policy: 'independent' as 'independent' | 'higher_compatible_with_lower',
    fallback_arch: '',
  })

  function required (value: string): boolean | string {
    return Boolean(value) || t('common.required')
  }

  function osArchSlugRule (value: unknown): boolean | string {
    const slug = String(value ?? '').trim().toLowerCase()
    return isOsArchSlug(slug) || t('matrix.slugRule')
  }

  function optionalOsArchSlugRule (value: unknown): boolean | string {
    const slug = String(value ?? '').trim().toLowerCase()
    return !slug || isOsArchSlug(slug) || t('matrix.slugRule')
  }

  function openCreate (): void {
    editing.value = null
    packageLocked.value = false
    Object.assign(form, { os: '', arch: '', package_type: 'single_file', delta_algo: 'hdiffpatch', delta_source_count: 3, hw_variant_policy: 'independent', fallback_arch: '' })
    formError.value = null
    void loadCatalog()
    dialog.value = true
  }

  function openEdit (item: PlatformMatrix): void {
    editing.value = item
    packageLocked.value = resource.items.value.some(v => v.os === item.os && v.arch === item.arch && v.package_type === item.package_type)
    Object.assign(form, {
      os: item.os ?? '',
      arch: item.arch ?? '',
      package_type: item.package_type === 'multi_file' ? 'multi_file' : 'single_file',
      delta_algo: item.delta_algo === 'bsdiff' || item.delta_algo === 'xdelta3' ? item.delta_algo : 'hdiffpatch',
      delta_source_count: item.delta_source_count ?? 3,
      hw_variant_policy: item.hw_variant_policy === 'higher_compatible_with_lower' ? 'higher_compatible_with_lower' : 'independent',
      fallback_arch: item.fallback_arch ?? '',
    })
    formError.value = null
    void loadCatalog()
    dialog.value = true
  }

  async function loadCatalog (): Promise<void> {
    try {
      const { data } = await getPlatformCatalog({ path: { project_ref: projectRef.value } })
      catalog.value = data ?? null
    } catch {
      catalog.value = null
    }
  }

  onMounted(() => {
    void loadCatalog()
  })

  async function save (): Promise<void> {
    saving.value = true
    formError.value = null
    try {
      const payload = {
        package_type: form.package_type,
        delta_algo: form.package_type === 'multi_file' ? undefined : form.delta_algo,
        delta_source_count: form.delta_source_count,
        hw_variant_policy: form.hw_variant_policy,
        fallback_arch: form.fallback_arch ? String(form.fallback_arch).trim().toLowerCase() : undefined,
      }
      await (editing.value
        ? updateMatrix({
          path: { project_ref: projectRef.value, os: editing.value.os ?? '', arch: editing.value.arch ?? '' },
          body: payload,
        })
        : createMatrix({
          path: { project_ref: projectRef.value },
          body: {
            ...payload,
            os: String(form.os ?? '').trim().toLowerCase(),
            arch: String(form.arch ?? '').trim().toLowerCase(),
          },
        }))
      snackbar.show(t('common.save') + ' ✓')
      dialog.value = false
      await resource.refresh()
    } catch (error) {
      formError.value = error
    } finally {
      saving.value = false
    }
  }
</script>
