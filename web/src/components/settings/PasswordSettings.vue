<template>
  <v-card variant="flat">
    <v-card-title>Security</v-card-title>
    <v-card-subtitle class="mb-3 text-wrap">
      Change the web UI password. Every other signed in device is signed out.
    </v-card-subtitle>
    <v-divider></v-divider>

    <v-card-item>
      <v-form
        ref="form"
        :readonly="isPending"
        novalidate
        @submit.prevent="submit"
      >
        <v-row class="mt-2">
          <v-col v-if="username" cols="12" lg="6" md="8">
            <v-text-field
              :model-value="username"
              autocomplete="username"
              label="Username"
              prepend-inner-icon="mdi-account-outline"
              readonly
              tabindex="-1"
              variant="outlined"
            ></v-text-field>
          </v-col>

          <v-col
            v-for="field in fields"
            :key="field.key"
            cols="12"
            lg="6"
            md="8"
          >
            <v-text-field
              :ref="(el) => setFieldRef(field.key, el)"
              v-model="values[field.key]"
              :autocomplete="field.autocomplete"
              :hint="field.key === 'next' ? lengthHint : undefined"
              :label="field.label"
              :persistent-hint="field.key === 'next'"
              :rules="rulesFor[field.key]"
              :type="visible[field.key] ? 'text' : 'password'"
              aria-required="true"
              prepend-inner-icon="mdi-lock-outline"
              variant="outlined"
            >
              <template #append-inner>
                <v-btn
                  :aria-label="field.toggleLabel"
                  :aria-pressed="visible[field.key]"
                  :icon="
                    visible[field.key]
                      ? 'mdi-eye-off-outline'
                      : 'mdi-eye-outline'
                  "
                  density="comfortable"
                  type="button"
                  variant="text"
                  @click="visible[field.key] = !visible[field.key]"
                ></v-btn>
              </template>
            </v-text-field>

            <v-progress-linear
              v-if="field.key === 'next' && values.next"
              :color="STRENGTH_COLORS[strength.score]"
              :model-value="(strength.score / MAX_STRENGTH_SCORE) * 100"
              :aria-label="`Password strength: ${strength.label}`"
              class="mt-1"
              height="6"
              rounded
            ></v-progress-linear>
          </v-col>

          <v-col v-if="status" cols="12" lg="6" md="8">
            <v-alert
              :text="status.message"
              :type="status.kind"
              closable
              variant="tonal"
              @click:close="status = null"
            ></v-alert>
          </v-col>

          <v-col cols="12">
            <v-btn
              :loading="isPending"
              color="primary"
              type="submit"
              variant="flat"
            >
              Change password
            </v-btn>
          </v-col>
        </v-row>
      </v-form>
    </v-card-item>
  </v-card>
</template>

<script lang="ts" setup>
import { computed, nextTick, reactive, ref } from "vue";
import { useMutation } from "@tanstack/vue-query";
import { APIClient } from "@/api/APIClient";
import { useAuthStore } from "@/store/auth/authStore";
import {
  MAX_PASSWORD_BYTES,
  MIN_PASSWORD_LENGTH,
  passwordBytes,
  passwordLength,
} from "@/domain/passwordPolicy";
import { MAX_STRENGTH_SCORE, passwordStrength } from "@/utils/passwordStrength";
import type { VForm, VTextField } from "vuetify/components";
import type { ComponentPublicInstance } from "vue";

type FieldKey = "current" | "next" | "confirm";
type Rule = (value: string) => true | string;

interface Status {
  kind: "success" | "error";
  message: string;
}

const TEXT = {
  required: "Required.",
  tooShort: `Use at least ${MIN_PASSWORD_LENGTH} characters.`,
  tooLong: `Use at most ${MAX_PASSWORD_BYTES} bytes.`,
  sameAsCurrent: "Choose a password different from the current password.",
  mismatch: "Passwords must match.",
  success:
    "Password changed. Every other signed in device has been signed out.",
  network: "Could not reach the server. Check your connection and try again.",
  failed: "Could not change the password. Try again.",
};

const STRENGTH_COLORS = ["error", "error", "warning", "info", "success"];

const fields: {
  key: FieldKey;
  label: string;
  toggleLabel: string;
  autocomplete: string;
}[] = [
  {
    key: "current",
    label: "Current password",
    toggleLabel: "Show current password",
    autocomplete: "current-password",
  },
  {
    key: "next",
    label: "New password",
    toggleLabel: "Show new password",
    autocomplete: "new-password",
  },
  {
    key: "confirm",
    label: "Confirm new password",
    toggleLabel: "Show password confirmation",
    autocomplete: "new-password",
  },
];

const authStore = useAuthStore();
const username = computed(() => authStore.loggedInUser);

const form = ref<InstanceType<typeof VForm>>();
type TextFieldInstance = InstanceType<typeof VTextField>;
const fieldRefs: Partial<Record<FieldKey, TextFieldInstance>> = {};
const values = reactive<Record<FieldKey, string>>({
  current: "",
  next: "",
  confirm: "",
});
const visible = reactive<Record<FieldKey, boolean>>({
  current: false,
  next: false,
  confirm: false,
});
const status = ref<Status | null>(null);

const strength = computed(() => passwordStrength(values.next));
const lengthHint = computed(() => (values.next ? undefined : TEXT.tooShort));

const required: Rule = (value) => !!value || TEXT.required;

const rulesFor: Record<FieldKey, Rule[]> = {
  current: [required],
  next: [
    required,
    (value) => passwordLength(value) >= MIN_PASSWORD_LENGTH || TEXT.tooShort,
    (value) => passwordBytes(value) <= MAX_PASSWORD_BYTES || TEXT.tooLong,
    (value) => value !== values.current || TEXT.sameAsCurrent,
  ],
  confirm: [required, (value) => value === values.next || TEXT.mismatch],
};

const isTextField = (
  element: Element | ComponentPublicInstance | null,
): element is TextFieldInstance =>
  !!element && "focus" in element && "resetValidation" in element;

const setFieldRef = (
  key: FieldKey,
  element: Element | ComponentPublicInstance | null,
) => {
  fieldRefs[key] = isTextField(element) ? element : undefined;
};

const firstInvalidField = () =>
  fields.find(({ key }) =>
    rulesFor[key].some((rule) => rule(values[key]) !== true),
  );

const errorMessage = (error: unknown) => {
  if (error instanceof TypeError) return TEXT.network;
  return (error instanceof Error && error.message.trim()) || TEXT.failed;
};

const clearPasswords = async () => {
  for (const { key } of fields) {
    values[key] = "";
    visible[key] = false;
  }
  await nextTick();
  form.value?.resetValidation();
};

const mutation = useMutation({
  mutationFn: (passwords: { current: string; next: string }) =>
    APIClient.auth.changePassword(passwords.current, passwords.next),
  onSuccess: async () => {
    await clearPasswords();
    status.value = { kind: "success", message: TEXT.success };
  },
  onError: (error) => {
    status.value = { kind: "error", message: errorMessage(error) };
  },
});

const isPending = computed(() => mutation.isPending.value);

const submit = async () => {
  if (mutation.isPending.value) return;

  status.value = null;
  const { valid } = (await form.value?.validate()) ?? { valid: false };
  if (!valid) {
    const invalid = firstInvalidField();
    if (invalid) fieldRefs[invalid.key]?.focus();
    return;
  }

  mutation.mutate({ current: values.current, next: values.next });
};
</script>

<style scoped></style>
