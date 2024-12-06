<template>
  <v-dialog v-model="modalVisible" :fullscreen="false" :scrim="false">
    <v-card>
      <v-card-title>{{ props.title }}</v-card-title>
      <v-card-text>{{ props.message }}</v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="primary" @click="confirm">Confirm</v-btn>
        <v-btn @click="cancel">Cancel</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
import { ref } from "vue";

const props = defineProps({
  title: String,
  message: String,
});

const emit = defineEmits(["confirmed", "canceled"]);

const modalVisible = ref(false);
const confirm = () => {
  modalVisible.value = false;
  emit("confirmed");
};

const cancel = () => {
  modalVisible.value = false;
  emit("canceled");
};

const showModal = () => {
  modalVisible.value = true;
};

defineExpose({ showModal });
</script>

<style scoped></style>
