<template>
  <v-dialog v-model="modalVisible" max-width="440" @after-leave="restoreFocus">
    <v-card>
      <v-card-title>{{ props.title }}</v-card-title>
      <v-card-text>{{ props.message }}</v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn @click="cancel">Cancel</v-btn>
        <v-btn color="error" variant="flat" @click="confirm">{{
          props.confirmLabel
        }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
import { ref, watch } from "vue";

const props = defineProps({
  title: { type: String, required: true },
  message: { type: String, required: true },
  confirmLabel: { type: String, default: "Confirm" },
});

const emit = defineEmits(["confirmed", "canceled"]);

const modalVisible = ref(false);
let confirmed = false;
let opener: HTMLElement | null = null;
let fallback: HTMLElement | null = null;

watch(modalVisible, (open) => {
  if (open) {
    confirmed = false;
    return;
  }
  if (!confirmed) emit("canceled");
});

const confirm = () => {
  confirmed = true;
  modalVisible.value = false;
  emit("confirmed");
};

const cancel = () => {
  modalVisible.value = false;
};

const showModal = () => {
  opener =
    document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null;
  fallback = opener?.closest<HTMLElement>(".v-card") ?? null;
  modalVisible.value = true;
};

const restoreFocus = () => {
  const target = opener?.isConnected ? opener : fallback;
  if (target?.isConnected) {
    if (!target.hasAttribute("tabindex")) target.tabIndex = -1;
    target.focus();
  }
  opener = null;
  fallback = null;
};

defineExpose({ showModal });
</script>

<style scoped></style>
