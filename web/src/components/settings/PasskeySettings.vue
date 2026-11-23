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
                <v-col cols="12" md="6">
                  <v-text-field
                    v-model="name"
                    :counter="MAX_NAME_LENGTH"
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
      :message="`${pending?.name ?? 'This passkey'} will no longer sign you in. You can add it again later.`"
      title="Remove passkey"
      @canceled="pending = null"
      @confirmed="remove"
    />
  </v-card>
</template>

<script lang="ts" setup>
import { computed, nextTick, ref } from "vue";
import { useMutation, useQuery, useQueryClient } from "@tanstack/vue-query";
import { APIClient } from "@/api/APIClient";
import ConfirmationModal from "@/components/modals/DeleteConfirmationModal.vue";
import { PasskeyPromptError } from "@/utils/passkeyErrors";
import { addPasskey, passkeysAvailable } from "@/utils/passkeys";
import type { VForm } from "vuetify/components";

interface Status {
  kind: "success" | "error";
  message: string;
}

const MAX_PASSKEYS = 10;
const MAX_NAME_LENGTH = 64;
const PASSKEYS_KEY = ["passkeys"];

const TEXT = {
  required: "Required.",
  nameTooLong: `Use at most ${MAX_NAME_LENGTH} characters.`,
  added: "Passkey added.",
  removed: "Passkey removed.",
  network: "Could not reach the server. Check your connection and try again.",
  addFailed: "Could not add the passkey. Try again.",
  removeFailed: "Could not remove the passkey. Try again.",
  loadFailed: "Could not load your passkeys. Reload the page to try again.",
};

const queryClient = useQueryClient();
const browserSupported = passkeysAvailable();

const capabilities = useQuery({
  queryKey: ["auth", "capabilities"],
  queryFn: APIClient.auth.capabilities,
  retry: false,
});
const serverOff = computed(
  () => capabilities.isSuccess.value && !capabilities.data.value?.passkeys,
);
const loadFailed = computed(() => capabilities.isError.value);

const listing = useQuery({
  queryKey: PASSKEYS_KEY,
  queryFn: APIClient.passkeys.list,
  enabled: computed(() => capabilities.data.value?.passkeys === true),
  retry: false,
});
const passkeys = computed(() => listing.data.value ?? []);
const atLimit = computed(() => passkeys.value.length >= MAX_PASSKEYS);
const canAdd = computed(
  () => browserSupported && !atLimit.value && listing.isSuccess.value,
);

const form = ref<InstanceType<typeof VForm>>();
const name = ref("");
const password = ref("");
const showPassword = ref(false);
const status = ref<Status | null>(null);

const required = (value: string) => !!value.trim() || TEXT.required;
const nameRules = [
  required,
  (value: string) => value.length <= MAX_NAME_LENGTH || TEXT.nameTooLong,
];

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: "medium" });
const usage = (passkey: Passkey) => {
  const added = `Added ${dateFormat.format(new Date(passkey.created_at))}`;
  const used = passkey.last_used_at
    ? `Last used ${dateFormat.format(new Date(passkey.last_used_at))}`
    : "Never used";
  return `${added} · ${used}`;
};

const errorMessage = (error: unknown, fallback: string) => {
  if (error instanceof TypeError) return TEXT.network;
  if (error instanceof PasskeyPromptError) return error.message;
  return (error instanceof Error && error.message.trim()) || fallback;
};

const adding = useMutation({
  mutationFn: (request: { name: string; password: string }) =>
    addPasskey(request.name, request.password),
  onSuccess: async () => {
    name.value = "";
    password.value = "";
    showPassword.value = false;
    status.value = { kind: "success", message: TEXT.added };
    await queryClient.invalidateQueries({ queryKey: PASSKEYS_KEY });
    await nextTick();
    form.value?.resetValidation();
  },
  onError: async (error) => {
    password.value = "";
    showPassword.value = false;
    status.value = {
      kind: "error",
      message: errorMessage(error, TEXT.addFailed),
    };
    await nextTick();
    form.value?.resetValidation();
  },
});

const submit = async () => {
  if (adding.isPending.value) return;

  status.value = null;
  const { valid } = (await form.value?.validate()) ?? { valid: false };
  if (!valid) return;

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
  onSuccess: async () => {
    status.value = { kind: "success", message: TEXT.removed };
    await queryClient.invalidateQueries({ queryKey: PASSKEYS_KEY });
  },
  onError: (error) => {
    status.value = {
      kind: "error",
      message: errorMessage(error, TEXT.removeFailed),
    };
  },
});

const remove = () => {
  const target = pending.value;
  pending.value = null;
  if (!target) return;

  status.value = null;
  removal.mutate(target.id);
};
</script>

<style scoped></style>
