<template>
  <v-card variant="flat">
    <v-card-title>Passkeys</v-card-title>
    <v-card-subtitle class="mb-3 text-wrap">
      Sign in with your device's fingerprint, face or screen lock instead of
      typing the password. The password keeps working.
    </v-card-subtitle>
    <v-divider></v-divider>

    <v-card-item>
      <v-row class="mt-2">
        <v-col v-if="loadFailed" cols="12">
          <v-alert
            :text="TEXT.loadFailed"
            type="error"
            variant="tonal"
          ></v-alert>
        </v-col>

        <v-col v-else-if="serverOff" cols="12">
          <v-alert type="info" variant="tonal">
            Passkeys are off on this server. Set
            <code>webauthnOrigin</code> to the address you use to open SyncYomi
            in <code>config.toml</code> and restart to turn them on.
          </v-alert>
        </v-col>

        <template v-else-if="capabilities.isSuccess.value">
          <v-col v-if="!browserSupported" cols="12">
            <v-alert type="info" variant="tonal">
              Adding a passkey needs a secure address. Open SyncYomi through its
              https address (or localhost) to add one.
            </v-alert>
          </v-col>

          <v-col v-else-if="atLimit" cols="12">
            <v-alert type="info" variant="tonal">
              You have reached the limit of {{ MAX_PASSKEYS }} passkeys. Remove
              one to add another.
            </v-alert>
          </v-col>

          <v-col v-if="listing.isLoading.value" cols="12">
            <v-progress-linear
              aria-label="Loading passkeys"
              indeterminate
            ></v-progress-linear>
          </v-col>

          <v-col v-else-if="listing.isError.value" cols="12">
            <v-alert
              :text="TEXT.loadFailed"
              type="error"
              variant="tonal"
            ></v-alert>
          </v-col>

          <v-col v-else cols="12">
            <p v-if="!passkeys.length" class="text-medium-emphasis">
              No passkeys yet.
            </p>
            <v-list
              v-else
              class="border rounded"
              density="comfortable"
              lines="two"
            >
              <v-list-item
                v-for="passkey in passkeys"
                :key="passkey.id"
                :subtitle="usage(passkey)"
                :title="passkey.name"
                prepend-icon="mdi-key-outline"
              >
                <template #append>
                  <v-btn
                    :aria-label="`Remove passkey ${passkey.name}`"
                    :disabled="removal.isPending.value"
                    color="error"
                    density="comfortable"
                    icon="mdi-delete-outline"
                    type="button"
                    variant="text"
                    @click="askToRemove(passkey)"
                  ></v-btn>
                </template>
              </v-list-item>
            </v-list>

            <v-alert
              v-if="status"
              :text="status.message"
              :type="status.kind"
              class="mt-2"
              closable
              variant="tonal"
              @click:close="status = null"
            ></v-alert>
          </v-col>

          <v-col v-if="canAdd" cols="12">
            <v-form
              ref="form"
              :readonly="adding.isPending.value"
              novalidate
              @submit.prevent="submit"
            >
              <v-row>
                <input
                  v-if="username"
                  :value="username"
                  aria-hidden="true"
                  autocomplete="username"
                  class="pairing-username"
                  readonly
                  tabindex="-1"
                  type="text"
                />

                <v-col cols="12" md="6">
                  <v-text-field
                    ref="nameField"
                    v-model="name"
                    :counter="MAX_PASSKEY_NAME_LENGTH"
                    :counter-value="passkeyNameLength"
                    :rules="nameRules"
                    aria-required="true"
                    autocomplete="off"
                    label="Passkey name"
                    name="passkey-name"
                    placeholder="For example, Work laptop"
                    prepend-inner-icon="mdi-key-outline"
                    variant="outlined"
                  ></v-text-field>
                </v-col>

                <v-col cols="12" md="6">
                  <v-text-field
                    ref="passwordField"
                    v-model="password"
                    :rules="[required]"
                    :type="showPassword ? 'text' : 'password'"
                    aria-required="true"
                    autocomplete="current-password"
                    hint="Your password confirms that it is you adding the passkey."
                    label="Password to confirm"
                    persistent-hint
                    prepend-inner-icon="mdi-lock-outline"
                    variant="outlined"
                  >
                    <template #append-inner>
                      <v-btn
                        aria-label="Show password to confirm"
                        :aria-pressed="showPassword"
                        :icon="
                          showPassword
                            ? 'mdi-eye-off-outline'
                            : 'mdi-eye-outline'
                        "
                        density="comfortable"
                        type="button"
                        variant="text"
                        @click="showPassword = !showPassword"
                      ></v-btn>
                    </template>
                  </v-text-field>
                </v-col>

                <v-col cols="12">
                  <v-btn
                    :loading="adding.isPending.value"
                    color="primary"
                    prepend-icon="mdi-plus"
                    type="submit"
                    variant="flat"
                  >
                    Add passkey
                  </v-btn>
                </v-col>
              </v-row>
            </v-form>
          </v-col>
        </template>
      </v-row>
    </v-card-item>

    <ConfirmationModal
      ref="confirmation"
      confirm-label="Remove"
      :message="`${pending?.name ?? 'This passkey'} will no longer sign you in. You can add it again later.`"
      title="Remove passkey"
      @confirmed="remove"
    />
  </v-card>
</template>

<script lang="ts" setup>
import { computed, nextTick, ref } from "vue";
import { useMutation, useQuery, useQueryClient } from "@tanstack/vue-query";
import {
  PASSKEYS_KEY,
  capabilitiesQuery,
  passkeysQuery,
} from "@/api/passkeyQueries";
import { APIClient } from "@/api/APIClient";
import ConfirmationModal from "@/components/modals/DeleteConfirmationModal.vue";
import { settingsErrorMessage } from "@/components/settings/settingsError";
import {
  MAX_PASSKEYS,
  MAX_PASSKEY_NAME_LENGTH,
  passkeyNameLength,
} from "@/domain/passkeyPolicy";
import { useAuthStore } from "@/store/auth/authStore";
import { addPasskey, passkeysAvailable } from "@/utils/passkeys";
import type { VForm, VTextField } from "vuetify/components";

interface Status {
  kind: "success" | "error";
  message: string;
}

const TEXT = {
  required: "Required.",
  nameTooLong: `Use at most ${MAX_PASSKEY_NAME_LENGTH} characters.`,
  added: "Passkey added.",
  removed: "Passkey removed.",
  addFailed: "Could not add the passkey. Try again.",
  removeFailed: "Could not remove the passkey. Try again.",
  loadFailed: "Could not load your passkeys. Reload the page to try again.",
};

const queryClient = useQueryClient();
const browserSupported = passkeysAvailable();
const username = computed(() => useAuthStore().loggedInUser);

const capabilities = useQuery(capabilitiesQuery);
const serverOff = computed(
  () => capabilities.isSuccess.value && !capabilities.data.value?.passkeys,
);
const loadFailed = computed(() => capabilities.isError.value);

const listing = useQuery({
  ...passkeysQuery,
  enabled: computed(() => capabilities.data.value?.passkeys === true),
});
const passkeys = computed(() => listing.data.value ?? []);
const atLimit = computed(() => passkeys.value.length >= MAX_PASSKEYS);
const canAdd = computed(
  () => browserSupported && !atLimit.value && listing.isSuccess.value,
);

const form = ref<InstanceType<typeof VForm>>();
const nameField = ref<InstanceType<typeof VTextField>>();
const passwordField = ref<InstanceType<typeof VTextField>>();
const name = ref("");
const password = ref("");
const showPassword = ref(false);
const status = ref<Status | null>(null);

const required = (value: string) => !!value.trim() || TEXT.required;
const nameRules = [
  required,
  (value: string) =>
    passkeyNameLength(value) <= MAX_PASSKEY_NAME_LENGTH || TEXT.nameTooLong,
];

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: "medium" });
const usage = (passkey: Passkey) => {
  const added = `Added ${dateFormat.format(new Date(passkey.created_at))}`;
  const used = passkey.last_used_at
    ? `Last used ${dateFormat.format(new Date(passkey.last_used_at))}`
    : "Never used";
  return `${added} · ${used}`;
};

const refreshList = () =>
  queryClient.invalidateQueries({ queryKey: PASSKEYS_KEY });

const adding = useMutation({
  mutationFn: (request: { name: string; password: string }) =>
    addPasskey(request.name, request.password),
  gcTime: 0,
  onSuccess: async () => {
    name.value = "";
    password.value = "";
    showPassword.value = false;
    status.value = { kind: "success", message: TEXT.added };
    await nextTick();
    form.value?.resetValidation();
  },
  onError: async (error) => {
    password.value = "";
    showPassword.value = false;
    status.value = {
      kind: "error",
      message: settingsErrorMessage(error, TEXT.addFailed),
    };
    await nextTick();
    form.value?.resetValidation();
  },
  onSettled: async () => {
    await refreshList();
    adding.reset();
  },
});

const submit = async () => {
  if (adding.isPending.value) return;

  status.value = null;
  const { valid } = (await form.value?.validate()) ?? { valid: false };
  if (!valid) {
    const nameInvalid = nameRules.some((rule) => rule(name.value) !== true);
    (nameInvalid ? nameField : passwordField).value?.focus();
    return;
  }

  adding.mutate({ name: name.value.trim(), password: password.value });
};

const confirmation = ref<InstanceType<typeof ConfirmationModal>>();
const pending = ref<Passkey | null>(null);

const askToRemove = (passkey: Passkey) => {
  pending.value = passkey;
  confirmation.value?.showModal();
};

const removal = useMutation({
  mutationFn: (id: number) => APIClient.passkeys.delete(id),
  onSuccess: () => {
    status.value = { kind: "success", message: TEXT.removed };
  },
  onError: (error) => {
    status.value = {
      kind: "error",
      message: settingsErrorMessage(error, TEXT.removeFailed),
    };
  },
  onSettled: refreshList,
});

const remove = () => {
  if (!pending.value) return;

  status.value = null;
  removal.mutate(pending.value.id);
};
</script>

<style scoped>
.pairing-username {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
}
</style>
