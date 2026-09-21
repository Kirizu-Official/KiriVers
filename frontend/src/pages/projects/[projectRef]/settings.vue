<!-- pages/projects/[projectRef]/settings.vue — 项目设置与成员 -->
<template>
  <div>
    <PageHeader :description="t('overview.settingsDescription')" :title="t('nav.settings')" />

    <v-row v-if="project">
      <v-col cols="12" lg="9">
        <v-expansion-panels v-model="openPanels" multiple>
          <!-- 基本信息 -->
          <v-expansion-panel :title="t('overview.basic')">
            <template #text>
              <v-row dense>
                <v-col cols="12" md="6"><v-text-field v-model="form.name" :label="t('projects.name')" /></v-col>

                <v-col cols="12" md="6">
                  <v-text-field
                    v-model="form.slug"
                    :hint="t('overview.slugHint')"
                    :label="t('projects.slug')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-select
                    v-model="form.compare_engine"
                    :disabled="engineLocked"
                    :hint="engineLocked ? t('projects.engineLocked') : t('overview.compareEngineHint')"
                    :items="engineItems"
                    :label="t('projects.compareEngine')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-text-field
                    v-model="form.slug_alias_retention_days"
                    :hint="t('overview.aliasRetentionHint')"
                    :label="t('overview.aliasRetention')"
                    persistent-hint
                    type="number"
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-text-field
                    v-model="form.minimum_supported_version"
                    :hint="t('overview.minSupportedVersionHint')"
                    :label="t('overview.minSupportedVersion')"
                    persistent-hint
                  />
                </v-col>
              </v-row>
            </template>
          </v-expansion-panel>

          <!-- 更新策略 -->
          <v-expansion-panel :title="t('overview.updatePolicy')">
            <template #text>
              <v-row dense>
                <v-col cols="12">
                  <v-switch
                    v-model="form.gray_weight_tenure_activity"
                    color="primary"
                    :hint="t('overview.grayWeightHint')"
                    :label="t('overview.grayWeight')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-text-field
                    v-model="form.file_list_max_files"
                    :hint="t('overview.fileListMaxFilesHint')"
                    :label="t('overview.fileListMaxFiles')"
                    :max="project.file_list_max_files_limit"
                    min="0"
                    persistent-hint
                    type="number"
                  />
                </v-col>
              </v-row>
            </template>
          </v-expansion-panel>

          <!-- 安装策略模板 -->
          <v-expansion-panel :title="t('installPolicy.title')">
            <template #text>
              <InstallPolicyEditor :project-ref="projectRef" scope="project" />
            </template>
          </v-expansion-panel>

          <!-- 安全 -->
          <v-expansion-panel :title="t('overview.security')">
            <template #text>
              <v-row dense>
                <v-col cols="12" md="6">
                  <v-switch
                    v-model="form.require_client_token"
                    color="primary"
                    :hint="t('overview.requireClientTokenHint')"
                    :label="t('overview.requireClientToken')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-switch
                    v-model="form.force_https"
                    color="primary"
                    :hint="t('overview.forceHttpsHint')"
                    :label="t('overview.forceHttps')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12">
                  <v-text-field
                    v-model="corsText"
                    :hint="t('overview.corsOriginsHint')"
                    :label="t('overview.corsOrigins')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-text-field
                    v-model="storeTokenDraft"
                    :disabled="clearStoreToken"
                    :hint="t('overview.storeTokenHint')"
                    :label="t('overview.storeToken')"
                    persistent-hint
                    :placeholder="project?.has_store_token ? t('overview.storeTokenSet') : t('overview.storeTokenEmpty')"
                  >
                    <template #append-inner>
                      <v-btn
                        :disabled="clearStoreToken"
                        size="small"
                        variant="text"
                        @click="generateStoreToken"
                      >
                        {{ t('overview.storeTokenGenerate') }}
                      </v-btn>
                    </template>
                  </v-text-field>

                  <v-checkbox
                    v-if="project?.has_store_token"
                    v-model="clearStoreToken"
                    :label="t('overview.storeTokenClear')"
                  />
                </v-col>
              </v-row>
            </template>
          </v-expansion-panel>

          <!-- 存储 / 签名 -->
          <v-expansion-panel :title="t('overview.storage')">
            <template #text>
              <v-row dense>
                <v-col cols="12" md="6">
                  <v-text-field
                    v-model="form.storage_prefix"
                    :hint="t('overview.storagePrefixHint')"
                    :label="t('overview.storagePrefix')"
                    persistent-hint
                  />
                </v-col>

                <v-col v-if="!isLocalStorage" cols="12" md="6">
                  <v-text-field
                    v-model="form.storage_bucket"
                    :hint="t('overview.storageBucketHint')"
                    :label="t('overview.storageBucket')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-select
                    v-model="form.signing_algo"
                    :hint="t('overview.signingAlgoHint')"
                    :items="['ed25519', 'rsa-sha256']"
                    :label="t('overview.signingAlgo')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-text-field
                    v-model="form.signing_public_key"
                    :hint="t('overview.signingPublicKeyHint')"
                    :label="t('overview.signingPublicKey')"
                    persistent-hint
                  />
                </v-col>
              </v-row>
            </template>
          </v-expansion-panel>

          <!-- 商店协议 listings -->
          <v-expansion-panel :title="t('overview.storeProtocols')">
            <template #text>
              <p class="text-body-2 text-medium-emphasis mb-4">{{ t('overview.listingsHint') }}</p>

              <ErrorAlert :error="listings.error.value" />

              <div class="d-flex justify-end mb-3">
                <v-btn color="primary" prepend-icon="mdi-plus" size="small" @click="openListingCreate">
                  {{ t('overview.listingCreate') }}
                </v-btn>
              </div>

              <div v-if="listings.loading.value && listings.items.value.length === 0" class="d-flex justify-center py-8">
                <v-progress-circular color="primary" indeterminate />
              </div>

              <EmptyState
                v-else-if="listings.items.value.length === 0"
                icon="mdi-store-outline"
                :text="t('overview.listingEmpty')"
              />

              <v-data-table
                v-else
                :headers="listingHeaders"
                hide-default-footer
                :items="listings.items.value"
                items-key="id"
                :loading="listings.loading.value"
              >
                <template #item.pins="{ item }">
                  <span class="text-body-2">{{ listingPins(item) }}</span>
                </template>

                <template #item.enabled="{ item }">
                  <v-switch
                    color="primary"
                    density="compact"
                    hide-details
                    :model-value="item.enabled"
                    @update:model-value="value => toggleListingEnabled(item, Boolean(value))"
                  />
                </template>

                <template #item.store_url="{ item }">
                  <CopyField :value="listingAbsoluteUrl(item.store_url)" />
                </template>

                <template #item.actions="{ item }">
                  <v-btn prepend-icon="mdi-pencil-outline" size="small" variant="text" @click="openListingEdit(item)">
                    {{ t('common.edit') }}
                  </v-btn>

                  <v-btn
                    color="error"
                    prepend-icon="mdi-delete-outline"
                    size="small"
                    variant="text"
                    @click="askDeleteListing(item)"
                  >
                    {{ t('common.delete') }}
                  </v-btn>
                </template>
              </v-data-table>
            </template>
          </v-expansion-panel>

          <!-- Webhook / changelog -->
          <v-expansion-panel :title="t('overview.webhook')">
            <template #text>
              <v-text-field
                v-model="form.webhook_url"
                :hint="t('overview.webhookUrlHint')"
                :label="t('overview.webhookUrl')"
                persistent-hint
              />

              <div class="d-flex ga-2 mt-2">
                <v-btn
                  prepend-icon="mdi-history"
                  size="small"
                  variant="tonal"
                  @click="openWebhookDeliveries"
                >
                  {{ t('overview.viewDeliveries') }}
                </v-btn>
              </div>
            </template>
          </v-expansion-panel>

          <v-expansion-panel :title="t('overview.changelogDefaults')">
            <template #text>
              <v-row dense>
                <v-col cols="12" md="4">
                  <v-select
                    v-model="form.changelog_scope"
                    :hint="t('overview.changelogScopeHint')"
                    :items="['range_all', 'range_platform', 'target_only']"
                    :label="t('overview.changelogScope')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="4">
                  <v-select
                    v-model="form.changelog_layout"
                    :hint="t('overview.changelogLayoutHint')"
                    :items="['aggregated', 'structured', 'both']"
                    :label="t('overview.changelogLayout')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="4">
                  <v-switch
                    v-model="form.changelog_client_override"
                    color="primary"
                    density="compact"
                    :hint="t('overview.changelogOverrideHint')"
                    :label="t('overview.changelogOverride')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-switch
                    v-model="form.changelog_include_revoked"
                    color="primary"
                    density="compact"
                    :hint="t('overview.includeRevokedHint')"
                    :label="t('overview.includeRevoked')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-switch
                    v-model="form.changelog_include_platform_notes"
                    color="primary"
                    density="compact"
                    :hint="t('overview.includePlatformNotesHint')"
                    :label="t('overview.includePlatformNotes')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-text-field
                    v-model="form.changelog_default_entries"
                    :hint="t('overview.changelogDefaultEntriesHint')"
                    :label="t('overview.changelogDefaultEntries')"
                    :max="project.changelog_default_entries_limit"
                    min="1"
                    persistent-hint
                    type="number"
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-text-field
                    v-model="form.changelog_max_entries"
                    :hint="t('overview.changelogMaxEntriesHint')"
                    :label="t('overview.changelogMaxEntries')"
                    :max="project.changelog_max_entries_limit"
                    min="0"
                    persistent-hint
                    type="number"
                  />
                </v-col>
              </v-row>
            </template>
          </v-expansion-panel>

          <!-- 速率限制 -->
          <v-expansion-panel :title="t('overview.rateLimit')">
            <template #text>
              <v-row dense>
                <v-col v-for="key in rateKeys" :key="key" cols="12" md="6">
                  <v-text-field
                    :hint="t(`overview.rateHints.${key}`)"
                    :label="key"
                    :model-value="rateForm[key]"
                    persistent-hint
                    type="number"
                    @update:model-value="value => setRate(key, value)"
                  />
                </v-col>
              </v-row>
            </template>
          </v-expansion-panel>

          <!-- 隐私 -->
          <v-expansion-panel :title="t('overview.privacy')">
            <template #text>
              <v-row dense>
                <v-col cols="12" md="6">
                  <v-select
                    v-model="form.device_id_policy"
                    :hint="t('overview.deviceIdPolicyHint')"
                    :items="['hashed', 'raw', 'none']"
                    :label="t('overview.deviceIdPolicy')"
                    persistent-hint
                  />
                </v-col>

                <v-col cols="12" md="6">
                  <v-text-field v-model="deleteDeviceHash" :hint="t('overview.deleteDeviceHint')" :label="t('overview.deleteDevice')" persistent-hint />
                </v-col>

                <v-col cols="12">
                  <v-btn
                    color="error"
                    :disabled="!deleteDeviceHash"
                    prepend-icon="mdi-delete-sweep-outline"
                    variant="tonal"
                    @click="deleteDeviceAction"
                  >
                    {{ t('overview.deleteDevice') }}
                  </v-btn>
                </v-col>
              </v-row>
            </template>
          </v-expansion-panel>

          <!-- 维护：产物清理 -->
          <v-expansion-panel :title="t('overview.maintenance')">
            <template #text>
              <v-row dense>
                <v-col cols="12" md="6">
                  <v-text-field
                    v-model.number="cleanupRetentionDays"
                    :hint="t('overview.cleanupHint')"
                    :label="t('overview.cleanupDays')"
                    persistent-hint
                    type="number"
                  />
                </v-col>

                <v-col cols="12">
                  <v-btn
                    color="error"
                    prepend-icon="mdi-broom"
                    variant="tonal"
                    @click="cleanupDialog = true"
                  >
                    {{ t('overview.cleanup') }}
                  </v-btn>
                </v-col>
              </v-row>
            </template>
          </v-expansion-panel>
        </v-expansion-panels>

        <div class="d-flex ga-2 mt-4">
          <v-btn color="primary" :loading="saving" @click="save">{{ t('common.save') }}</v-btn>
          <v-btn variant="text" @click="reload">{{ t('common.cancel') }}</v-btn>
        </div>

        <ErrorAlert class="mt-4" :error="saveError" />

        <v-card class="mt-6" :title="t('members.title')">
          <template #append>
            <v-btn
              v-if="canManageMembers"
              color="primary"
              prepend-icon="mdi-plus"
              size="small"
              @click="openMemberCreate"
            >
              {{ t('members.add') }}
            </v-btn>
          </template>

          <v-data-table
            :headers="memberHeaders"
            hide-default-footer
            :items="members.items.value"
            items-key="admin_id"
            :loading="members.loading.value"
          >
            <template #item.role="{ item }">
              {{ t(`members.role.${item.role ?? 'admin'}`) }}
            </template>

            <template #item.actions="{ item }">
              <v-btn
                v-if="auth.isPlatformAdmin"
                prepend-icon="mdi-swap-horizontal"
                size="small"
                variant="text"
                @click="toggleMemberRole(item)"
              >
                {{ t('members.changeRole') }}
              </v-btn>

              <v-btn
                v-if="canRemoveMember(item)"
                color="error"
                prepend-icon="mdi-delete-outline"
                size="small"
                variant="text"
                @click="askRemoveMember(item)"
              >
                {{ t('common.delete') }}
              </v-btn>
            </template>
          </v-data-table>
        </v-card>
      </v-col>

      <v-col cols="12" lg="3">
        <v-card>
          <v-card-text>
            <div class="text-caption text-medium-emphasis mb-2">{{ t('projects.uuid') }}</div>
            <CopyField :value="project.uuid" />
            <v-divider class="my-4" />
            <div class="text-caption text-medium-emphasis mb-2">{{ t('projects.createdAt') }}</div>
            <div class="text-body-2">{{ project.created_at }}</div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <!-- 产物清理确认对话框 -->
    <ConfirmDialog
      v-model="cleanupDialog"
      icon="mdi-broom"
      :text="t('overview.cleanupConfirm')"
      :title="t('overview.cleanup')"
      @confirm="cleanupAction"
    />

    <v-dialog v-model="storeTokenRevealOpen" max-width="560" persistent>
      <v-card prepend-icon="mdi-key-alert" :title="t('overview.storeTokenRevealTitle')">
        <v-card-text>
          <v-alert class="mb-4" density="compact" type="warning">
            {{ t('overview.storeTokenRevealText') }}
          </v-alert>

          <CopyField :value="storeTokenReveal" />
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn color="primary" @click="storeTokenRevealOpen = false">{{ t('common.close') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Webhook 投递记录对话框 -->
    <v-dialog v-model="deliveriesDialog" max-width="860">
      <v-card :title="t('overview.deliveriesTitle')">
        <v-card-text>
          <div v-if="deliveriesLoading" class="d-flex justify-center py-8">
            <v-progress-circular color="primary" indeterminate />
          </div>

          <EmptyState v-else-if="deliveries.length === 0" icon="mdi-webhook" :text="t('common.empty')" />

          <v-table v-else class="rounded-lg" density="compact">
            <thead>
              <tr>
                <th>{{ t('overview.deliveryEvent') }}</th>
                <th>{{ t('overview.deliveryStatus') }}</th>
                <th>{{ t('overview.deliveryStatusCode') }}</th>
                <th>{{ t('overview.deliveryTime') }}</th>
              </tr>
            </thead>

            <tbody>
              <tr v-for="d in deliveries" :key="d.id">
                <td><code>{{ d.event }}</code></td>

                <td>
                  <v-chip
                    :color="d.status === 'delivered' ? 'success' : d.status === 'failed' ? 'error' : 'warning'"
                    size="x-small"
                    variant="tonal"
                  >
                    {{ d.status }}
                  </v-chip>
                </td>

                <td>
                  <code v-if="d.last_status_code">{{ d.last_status_code }}</code>
                  <span v-else class="text-medium-emphasis">—</span>
                </td>

                <td><span class="text-caption text-medium-emphasis">{{ d.created_at }}</span></td>
              </tr>
            </tbody>
          </v-table>
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="deliveriesDialog = false">{{ t('common.close') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="listingDialog" max-width="640">
      <v-card :title="listingEditing ? t('common.edit') : t('overview.listingCreate')">
        <v-form ref="listingFormRef" @submit.prevent="saveListing">
          <v-card-text>
            <ErrorAlert :error="listingFormError" />

            <v-select
              v-model="listingForm.protocol"
              :disabled="Boolean(listingEditing)"
              :items="listingProtocolItems"
              :label="t('overview.listingProtocol')"
              :rules="[required]"
            />

            <v-text-field
              v-model="listingForm.slug"
              :disabled="Boolean(listingEditing)"
              :hint="t('overview.listingSlugHint')"
              :label="t('overview.listingSlug')"
              persistent-hint
              :rules="listingEditing ? [] : [required, listingSlugRule]"
            />

            <v-switch
              v-model="listingForm.enabled"
              color="primary"
              density="compact"
              :label="t('common.enabled')"
            />

            <v-select
              v-model="listingForm.package_source"
              :items="listingSourceItems"
              :label="t('overview.listingPackageSource')"
            />

            <v-text-field
              v-if="listingForm.package_source === 'manifest_path'"
              v-model="listingForm.manifest_path"
              :hint="t('overview.listingManifestPathHint')"
              :label="t('overview.listingManifestPath')"
              persistent-hint
              :rules="[required]"
            />

            <v-row dense>
              <v-col cols="12" md="4">
                <v-select
                  v-model="listingForm.os"
                  clearable
                  :hint="t('overview.listingPinHint')"
                  :items="listingOsItems"
                  :label="t('overview.listingOs')"
                  persistent-hint
                />
              </v-col>

              <v-col cols="12" md="4">
                <v-select
                  v-model="listingForm.arch"
                  clearable
                  :hint="t('overview.listingPinHint')"
                  :items="listingArchItems"
                  :label="t('overview.listingArch')"
                  persistent-hint
                />
              </v-col>

              <v-col cols="12" md="4">
                <v-select
                  v-model="listingForm.channel"
                  clearable
                  :hint="t('overview.listingPinHint')"
                  :items="listingChannelItems"
                  :label="t('overview.listingChannel')"
                  persistent-hint
                />
              </v-col>
            </v-row>

            <v-text-field
              v-model="listingForm.identifiersText"
              :hint="t('overview.listingIdentifiersHint')"
              :label="t('overview.listingIdentifiers')"
              persistent-hint
            />
          </v-card-text>

          <v-card-actions>
            <v-spacer />
            <v-btn variant="text" @click="listingDialog = false">{{ t('common.cancel') }}</v-btn>
            <v-btn color="primary" :loading="listingSaving" type="submit">{{ t('common.save') }}</v-btn>
          </v-card-actions>
        </v-form>
      </v-card>
    </v-dialog>

    <ConfirmDialog
      v-model="listingDeleteDialog"
      icon="mdi-delete-alert-outline"
      :text="listingDeleteTarget ? t('overview.listingDeleteText', { name: listingDeleteTarget.slug }) : ''"
      :title="t('overview.listingDeleteTitle')"
      @confirm="doDeleteListing"
    />

    <v-dialog v-model="memberDialog" max-width="480">
      <v-card :title="t('members.add')">
        <v-card-text>
          <ErrorAlert :error="memberError" />

          <v-text-field v-model="memberForm.username" :label="t('admins.username')" :rules="[required]" />

          <v-text-field
            v-model="memberForm.password"
            :hint="t('members.passwordHint')"
            :label="t('admins.password')"
            persistent-hint
            type="password"
          />

          <v-select
            v-model="memberForm.role"
            :disabled="!auth.isPlatformAdmin"
            :items="memberRoleItems"
            :label="t('members.roleLabel')"
          />
        </v-card-text>

        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="memberDialog = false">{{ t('common.cancel') }}</v-btn>
          <v-btn color="primary" :loading="memberSaving" @click="saveMember">{{ t('common.save') }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <ConfirmDialog
      v-model="memberDeleteDialog"
      icon="mdi-delete-alert-outline"
      :text="memberDeleteTarget ? t('members.deleteText', { name: memberDeleteTarget.username }) : ''"
      :title="t('members.deleteTitle')"
      @confirm="doRemoveMember"
    />
  </div>
</template>

<script lang="ts" setup>
  import type { Project, ProjectMember, StoreListing, WebhookDelivery } from '@/api/generated'
  import { computed, inject, onMounted, reactive, type Ref, ref, watch } from 'vue'
  import { useI18n } from 'vue-i18n'
  import { useRoute } from 'vue-router'
  import {
    cleanupArtifacts,
    createProjectMember,
    createStoreListing,
    deleteDeviceTelemetry,
    deleteProjectMember,
    deleteStoreListing,
    getProject,
    listChannels,
    listMatrix,
    listProjectMembers,
    listStoreListings,
    listWebhookDeliveries,
    updateProject,
    updateProjectMember,
    updateStoreListing,
  } from '@/api/generated'
  import ConfirmDialog from '@/components/ConfirmDialog.vue'
  import CopyField from '@/components/CopyField.vue'
  import EmptyState from '@/components/EmptyState.vue'
  import ErrorAlert from '@/components/ErrorAlert.vue'
  import InstallPolicyEditor from '@/components/InstallPolicyEditor.vue'
  import PageHeader from '@/components/PageHeader.vue'
  import { useApiResource } from '@/composables/useApiResource'
  import { useAuthStore } from '@/stores/auth'
  import { useSnackbarStore } from '@/stores/snackbar'

  const { t } = useI18n()
  const route = useRoute('/projects/[projectRef]/settings')
  const snackbar = useSnackbarStore()
  const auth = useAuthStore()

  const projectRef = computed(() => route.params.projectRef)

  const project = ref<Project | null>(null)
  const projectRecord = inject<Ref<Project | null>>('projectRecord', project)
  const loading = ref(true)
  const saving = ref(false)
  const saveError = ref<unknown>(null)
  const openPanels = ref<number[]>([0])

  interface OverviewForm {
    name: string
    slug: string
    compare_engine: string
    slug_alias_retention_days: number | null
    minimum_supported_version: string | null
    gray_weight_tenure_activity: boolean
    file_list_max_files: number | null
    require_client_token: boolean
    force_https: boolean
    cors_origins: string[]
    storage_visibility: string
    storage_prefix: string | null
    storage_bucket: string | null
    signing_algo: string | null
    signing_public_key: string | null
    webhook_url: string | null
    changelog_scope: string
    changelog_layout: string
    changelog_client_override: boolean
    changelog_include_revoked: boolean
    changelog_include_platform_notes: boolean
    changelog_default_entries: number | null
    changelog_max_entries: number | null
    device_id_policy: string
    rate_limit: Record<string, number> | null
  }
  const form = reactive<Partial<OverviewForm>>({})
  const corsText = ref('')
  const storeTokenDraft = ref('')
  const clearStoreToken = ref(false)
  const storeTokenReveal = ref('')
  const storeTokenRevealOpen = ref(false)
  const rateForm = reactive<Record<string, string>>({})
  const deleteDeviceHash = ref('')

  const engineLocked = ref(false)
  const isLocalStorage = computed(() => (project.value?.storage_driver ?? 'local') === 'local')

  const engineItems = computed(() => [
    { title: t('projects.compareEngineSemver'), value: 'semver' },
    { title: t('projects.compareEngineInteger'), value: 'integer' },
  ])
  const rateKeys = ['check_per_device_per_min', 'check_per_ip_per_min', 'diff_per_device_per_min', 'telemetry_per_device_per_min']

  const members = useApiResource<ProjectMember>(async () => {
    const { data } = await listProjectMembers({ path: { project_ref: projectRef.value } })
    return data?.members ?? []
  })

  const selfRole = computed(() => members.items.value.find(row => row.username === auth.username)?.role)
  const canManageMembers = computed(() => auth.isPlatformAdmin || selfRole.value === 'owner')
  const memberHeaders = computed(() => [
    { title: t('admins.username'), key: 'username' },
    { title: t('members.roleLabel'), key: 'role' },
    { title: t('projects.createdAt'), key: 'created_at' },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])
  const memberRoleItems = computed(() => {
    const admin = { title: t('members.role.admin'), value: 'admin' as const }
    if (auth.isPlatformAdmin) {
      return [{ title: t('members.role.owner'), value: 'owner' as const }, admin]
    }
    return [admin]
  })

  const memberDialog = ref(false)
  const memberSaving = ref(false)
  const memberError = ref<unknown>(null)
  const memberForm = reactive({ username: '', password: '', role: 'admin' as 'owner' | 'admin' })
  const memberDeleteDialog = ref(false)
  const memberDeleteTarget = ref<ProjectMember | null>(null)

  function canRemoveMember (item: ProjectMember): boolean {
    if (auth.isPlatformAdmin) return true
    return selfRole.value === 'owner' && item.role === 'admin'
  }

  function openMemberCreate (): void {
    memberForm.username = ''
    memberForm.password = ''
    memberForm.role = 'admin'
    memberError.value = null
    memberDialog.value = true
  }

  async function saveMember (): Promise<void> {
    memberSaving.value = true
    memberError.value = null
    try {
      const password = memberForm.password.trim()
      await createProjectMember({
        path: { project_ref: projectRef.value },
        body: {
          username: memberForm.username.trim(),
          role: auth.isPlatformAdmin ? memberForm.role : 'admin',
          ...(password ? { password } : {}),
        },
      })
      snackbar.show(t('members.add') + ' ✓')
      memberDialog.value = false
      await members.refresh()
    } catch (error) {
      memberError.value = error
    } finally {
      memberSaving.value = false
    }
  }

  async function toggleMemberRole (item: ProjectMember): Promise<void> {
    if (!item.admin_id) return
    const next = item.role === 'owner' ? 'admin' : 'owner'
    try {
      await updateProjectMember({
        path: { project_ref: projectRef.value, admin_id: item.admin_id },
        body: { role: next },
      })
      await members.refresh()
    } catch (error) {
      saveError.value = error
    }
  }

  function askRemoveMember (item: ProjectMember): void {
    memberDeleteTarget.value = item
    memberDeleteDialog.value = true
  }

  async function doRemoveMember (): Promise<void> {
    if (!memberDeleteTarget.value?.admin_id) return
    try {
      await deleteProjectMember({
        path: { project_ref: projectRef.value, admin_id: memberDeleteTarget.value.admin_id },
      })
      snackbar.show(t('common.delete') + ' ✓')
      await members.refresh()
    } catch (error) {
      saveError.value = error
    }
  }

  function required (value: string): boolean | string {
    return Boolean(value) || t('common.required')
  }

  const listingProtocolItems = ['sparkle', 'electron', 'tauri', 'squirrel', 'clickonce', 'appimage', 'winget', 'msix', 'fdroid']
  const listingSourceItems = computed(() => [
    { title: t('overview.listingSourceLineFull'), value: 'line_full' },
    { title: t('overview.listingSourceManifestPath'), value: 'manifest_path' },
  ])
  const listingHeaders = computed(() => [
    { title: t('overview.listingProtocol'), key: 'protocol' },
    { title: t('overview.listingSlug'), key: 'slug' },
    { title: t('overview.listingPins'), key: 'pins', sortable: false },
    { title: t('common.enabled'), key: 'enabled', sortable: false },
    { title: t('overview.listingStoreUrl'), key: 'store_url', sortable: false },
    { title: t('common.actions'), key: 'actions', sortable: false, align: 'end' as const },
  ])

  const listings = useApiResource<StoreListing>(async () => {
    const { data } = await listStoreListings({ path: { project_ref: projectRef.value } })
    return data?.listings ?? []
  })

  const listingDialog = ref(false)
  const listingEditing = ref<StoreListing | null>(null)
  const listingSaving = ref(false)
  const listingFormError = ref<unknown>(null)
  const listingFormRef = ref<{ validate: () => Promise<{ valid: boolean }> } | null>(null)
  const listingOsItems = ref<string[]>([])
  const listingArchItems = ref<string[]>([])
  const listingChannelItems = ref<string[]>([])
  const listingForm = reactive({
    protocol: 'sparkle',
    slug: '',
    enabled: true,
    package_source: 'line_full' as 'line_full' | 'manifest_path',
    manifest_path: '',
    os: null as string | null,
    arch: null as string | null,
    channel: null as string | null,
    identifiersText: '',
  })
  const listingDeleteDialog = ref(false)
  const listingDeleteTarget = ref<StoreListing | null>(null)

  function listingSlugRule (value: string): boolean | string {
    return /^[a-z0-9-]{3,64}$/.test((value ?? '').trim()) || t('channels.slugRule')
  }

  function listingPins (row: StoreListing): string {
    const parts = [row.os, row.arch, row.channel].filter(Boolean)
    return parts.length > 0 ? parts.join(' / ') : t('common.none')
  }

  async function loadListingPinOptions (): Promise<void> {
    try {
      const [channelsRes, matrixRes] = await Promise.all([
        listChannels({ path: { project_ref: projectRef.value } }),
        listMatrix({ path: { project_ref: projectRef.value } }),
      ])
      listingChannelItems.value = (channelsRes.data?.channels ?? []).flatMap(row => row.slug ? [row.slug] : [])
      const os = new Set<string>()
      const arch = new Set<string>()
      for (const row of matrixRes.data?.matrix ?? []) {
        if (row.os) os.add(row.os)
        if (row.arch) arch.add(row.arch)
      }
      listingOsItems.value = [...os]
      listingArchItems.value = [...arch]
    } catch {
      listingChannelItems.value = []
      listingOsItems.value = []
      listingArchItems.value = []
    }
  }

  function listingAbsoluteUrl (path: string | undefined): string {
    if (!path) return ''
    try {
      return new URL(path, window.location.origin).href
    } catch {
      return path
    }
  }

  function parseIdentifiers (text: string): Record<string, string> {
    const identifiers: Record<string, string> = {}
    for (const pair of text.split(',')) {
      const idx = pair.indexOf('=')
      if (idx <= 0) continue
      const key = pair.slice(0, idx).trim()
      const value = pair.slice(idx + 1).trim()
      if (key && value) identifiers[key] = value
    }
    return identifiers
  }

  function resetListingForm (): void {
    listingForm.protocol = 'sparkle'
    listingForm.slug = ''
    listingForm.enabled = true
    listingForm.package_source = 'line_full'
    listingForm.manifest_path = ''
    listingForm.os = null
    listingForm.arch = null
    listingForm.channel = null
    listingForm.identifiersText = ''
  }

  function openListingCreate (): void {
    listingEditing.value = null
    resetListingForm()
    listingFormError.value = null
    listingDialog.value = true
    void loadListingPinOptions()
  }

  function includeCurrentPins (): void {
    if (listingForm.os && !listingOsItems.value.includes(listingForm.os)) {
      listingOsItems.value = [listingForm.os, ...listingOsItems.value]
    }
    if (listingForm.arch && !listingArchItems.value.includes(listingForm.arch)) {
      listingArchItems.value = [listingForm.arch, ...listingArchItems.value]
    }
    if (listingForm.channel && !listingChannelItems.value.includes(listingForm.channel)) {
      listingChannelItems.value = [listingForm.channel, ...listingChannelItems.value]
    }
  }

  function openListingEdit (item: StoreListing): void {
    listingEditing.value = item
    listingForm.protocol = item.protocol
    listingForm.slug = item.slug
    listingForm.enabled = item.enabled
    listingForm.package_source = item.package_source
    listingForm.manifest_path = item.manifest_path ?? ''
    listingForm.os = item.os ?? null
    listingForm.arch = item.arch ?? null
    listingForm.channel = item.channel ?? null
    listingForm.identifiersText = Object.entries(item.identifiers ?? {}).map(([k, v]) => `${k}=${v}`).join(', ')
    listingFormError.value = null
    listingDialog.value = true
    void loadListingPinOptions().then(includeCurrentPins)
  }

  function listingBody () {
    return {
      protocol: listingForm.protocol,
      slug: listingForm.slug.trim(),
      enabled: listingForm.enabled,
      package_source: listingForm.package_source,
      manifest_path: listingForm.package_source === 'manifest_path' ? listingForm.manifest_path.trim() : '',
      os: (listingForm.os ?? '').trim(),
      arch: (listingForm.arch ?? '').trim(),
      channel: (listingForm.channel ?? '').trim(),
      identifiers: parseIdentifiers(listingForm.identifiersText),
    }
  }

  async function saveListing (): Promise<void> {
    const validated = await listingFormRef.value?.validate()
    if (validated && !validated.valid) return
    listingSaving.value = true
    listingFormError.value = null
    try {
      const body = listingBody()
      await (listingEditing.value
        ? updateStoreListing({
          path: { project_ref: projectRef.value, listing_id: listingEditing.value.id },
          body,
        })
        : createStoreListing({
          path: { project_ref: projectRef.value },
          body,
        }))
      snackbar.show(t('common.save') + ' ✓')
      listingDialog.value = false
      await listings.refresh()
    } catch (error) {
      listingFormError.value = error
    } finally {
      listingSaving.value = false
    }
  }

  async function toggleListingEnabled (item: StoreListing, value: boolean): Promise<void> {
    try {
      await updateStoreListing({
        path: { project_ref: projectRef.value, listing_id: item.id },
        body: { enabled: value },
      })
      await listings.refresh()
    } catch (error) {
      saveError.value = error
      snackbar.show(t('common.save'), 'error', 5000)
    }
  }

  function askDeleteListing (item: StoreListing): void {
    listingDeleteTarget.value = item
    listingDeleteDialog.value = true
  }

  async function doDeleteListing (): Promise<void> {
    if (!listingDeleteTarget.value) return
    try {
      await deleteStoreListing({
        path: { project_ref: projectRef.value, listing_id: listingDeleteTarget.value.id },
      })
      snackbar.show(t('common.delete') + ' ✓')
      await listings.refresh()
    } catch (error) {
      saveError.value = error
      snackbar.show(t('common.delete'), 'error', 5000)
    }
  }

  function setRate (key: string, value: string): void {
    rateForm[key] = value
  }

  function generateStoreToken (): void {
    const bytes = new Uint8Array(32)
    crypto.getRandomValues(bytes)
    storeTokenDraft.value = Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('')
    clearStoreToken.value = false
  }

  function fillForm (source: Project): void {
    for (const key of Object.keys(form)) delete (form as Record<string, unknown>)[key]
    Object.assign(form, source)
    form.storage_visibility = 'public'
    storeTokenDraft.value = ''
    clearStoreToken.value = false
    corsText.value = (source.cors_origins ?? []).join(', ')
    engineLocked.value = ((source.stats?.versions?.published ?? 0)
      + (source.stats?.versions?.deprecated ?? 0)
      + (source.stats?.versions?.revoked ?? 0)) > 0
    for (const key of Object.keys(rateForm)) delete rateForm[key]
    const rate = (source.rate_limit ?? {}) as Record<string, number>
    for (const key of rateKeys) {
      rateForm[key] = rate[key] == null ? '' : String(rate[key])
    }
  }

  async function load (): Promise<void> {
    loading.value = true
    try {
      const { data } = await getProject({ path: { project_ref: projectRef.value } })
      project.value = data ?? null
      if (data) fillForm(data)
    } finally {
      loading.value = false
    }
  }

  function reload (): void {
    if (project.value) fillForm(project.value)
  }

  async function save (): Promise<void> {
    saving.value = true
    saveError.value = null
    try {
      const rate: Record<string, number> = {}
      for (const key of rateKeys) {
        if (rateForm[key] !== '') rate[key] = Number(rateForm[key])
      }
      const payload: Record<string, unknown> = {
        ...form,
        cors_origins: corsText.value.split(',').map(item => item.trim()).filter(Boolean),
        slug_alias_retention_days: Number(form.slug_alias_retention_days) || null,
        file_list_max_files: Number(form.file_list_max_files) || 0,
        changelog_default_entries: Number(form.changelog_default_entries) || 5,
        changelog_max_entries: Number(form.changelog_max_entries) || 0,
        rate_limit: rate,
      }
      delete payload.stats
      delete payload.uuid
      delete payload.default_locale
      delete payload.store_token
      delete payload.storage_driver
      delete payload.has_store_token
      delete payload.has_signing_private_key
      delete payload.created_at
      delete payload.updated_at
      delete payload.file_list_max_files_limit
      delete payload.changelog_default_entries_limit
      delete payload.changelog_max_entries_limit
      payload.storage_visibility = 'public'
      if (isLocalStorage.value) {
        delete payload.storage_bucket
      }
      if (clearStoreToken.value) {
        payload.store_token = ''
      } else if (storeTokenDraft.value.trim()) {
        payload.store_token = storeTokenDraft.value.trim()
      }
      const { data: updated } = await updateProject({
        path: { project_ref: projectRef.value },
        body: payload as Project,
      })
      project.value = updated ?? null
      if (updated) {
        const rotated = typeof updated.store_token === 'string' ? updated.store_token.trim() : ''
        projectRecord.value = updated
        fillForm(updated)
        if (rotated) {
          storeTokenReveal.value = rotated
          storeTokenRevealOpen.value = true
        }
      }
      snackbar.show(t('common.save') + ' ✓')
    } catch (error) {
      saveError.value = error
    } finally {
      saving.value = false
    }
  }

  const deliveriesDialog = ref(false)
  const deliveriesLoading = ref(false)
  const deliveries = ref<WebhookDelivery[]>([])

  async function openWebhookDeliveries (): Promise<void> {
    deliveriesDialog.value = true
    deliveriesLoading.value = true
    try {
      const { data } = await listWebhookDeliveries({
        path: { project_ref: projectRef.value },
        query: { limit: 50 },
      })
      deliveries.value = data?.deliveries ?? []
    } catch {
      deliveries.value = []
    } finally {
      deliveriesLoading.value = false
    }
  }

  async function deleteDeviceAction (): Promise<void> {
    if (!deleteDeviceHash.value) return
    try {
      const { data: res } = await deleteDeviceTelemetry({
        path: { project_ref: projectRef.value, device_hash: deleteDeviceHash.value.trim() },
      })
      const countInfo = res ? ` (${res.telemetry_deleted ?? 0} events, ${res.allowlist_deleted ?? 0} allowlist)` : ''
      snackbar.show(t('overview.deviceDeleted') + countInfo)
      deleteDeviceHash.value = ''
    } catch (error) {
      saveError.value = error
    }
  }

  // ---- 产物清理（POST /artifacts/cleanup，§5.8） ----
  const cleanupDialog = ref(false)
  const cleanupRetentionDays = ref<number | null>(null)

  async function cleanupAction (): Promise<void> {
    try {
      // retention_days 留空 = 清理全部过期数据
      await cleanupArtifacts({
        path: { project_ref: projectRef.value },
        query: cleanupRetentionDays.value ? { retention_days: cleanupRetentionDays.value } : undefined,
      })
      snackbar.show(t('overview.cleanupDone') + ' ✓')
      cleanupRetentionDays.value = null
    } catch (error) {
      saveError.value = error
    }
  }

  onMounted(() => {
    void load()
    void loadListingPinOptions()
  })
  watch(projectRef, () => {
    void load()
    void members.refresh()
    void listings.refresh()
    void loadListingPinOptions()
  })
</script>
