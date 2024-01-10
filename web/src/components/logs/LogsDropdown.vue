<script setup lang="ts">
import { ref } from 'vue';
import { useLogsStore } from "@/store/logStore";

const logsStore = useLogsStore();
const menu = ref(false); // Controls the visibility of the menu
</script>

<template>
  <v-menu
    v-model="menu"
    offset-y
    location="end"
    :close-on-content-click=false
  >
    <template  v-slot:activator="{ props }">
      <v-btn
        v-bind="props"
        class="ml-2"
      >
        <v-icon>mdi-dots-vertical</v-icon>
      </v-btn>
    </template>

    <v-card min-width="300" class="pa-1">
      <v-list>
       <v-list-item>
        <v-switch
          v-model="logsStore.scrollOnNewLog"
          color="purple"
          label="Scroll to bottom on new message"
          hide-details
        ></v-switch>
       </v-list-item>

        <v-list-item>
          <v-list-item-title>Indent log lines </v-list-item-title>
          <v-list-item-action>
            <v-switch
              v-model="logsStore.indentLogLines"
              color="purple"
              label="Indent each log line according to their respective starting position."
            ></v-switch>
          </v-list-item-action>
        </v-list-item>

        <v-list-item>
          <v-list-item-title>Hide Wrapped text</v-list-item-title>
          <v-list-item-action>
            <v-switch
              v-model="logsStore.hideWrappedText"
              color="purple"
              label="Hides text that is meant to be wrapped."
            ></v-switch>
          </v-list-item-action>
        </v-list-item>





        <v-divider></v-divider>
        <v-list-item @click="logsStore.clearLogs()">
          <v-btn prepend-icon="mdi-delete" variant="text">
            Clear Logs
          </v-btn>
        </v-list-item>

      </v-list>
    </v-card>
  </v-menu>
</template>
