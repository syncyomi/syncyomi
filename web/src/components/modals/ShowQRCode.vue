<template>
  <v-dialog v-model="modalVisible" :fullscreen="false" :scrim="false">
    <v-card>
      <qrcode
        :value="key"
        level="Q"
        renderAs="svg"
        background="#00000000"
        :foreground="foreground"
        margin=2
        class="w-auto h-auto aspect-square"
      />
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn @click="close">Close</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
import { computed, ref } from "vue";
import Qrcode from "qrcode.vue";

const key = ref<string>();
const foreground = computed(() => {
  const light = localStorage.getItem('theme') == 'light';
  if (light) return "#000000"
  else return "#ffffff"
});

const modalVisible = ref(false);

const close = () => {
  modalVisible.value = false;
};

const showModal = (apiKey: string) => {
  key.value = apiKey;
  modalVisible.value = true;
};

defineExpose({ showModal });
</script>

<style scoped></style>
