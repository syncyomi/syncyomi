<template>
  <v-app-bar app fixed>
    <v-toolbar-title class="text-capitalize" @click="$router.push('/')">
      TachiyomiSync
    </v-toolbar-title>

    <v-toolbar-items>
      <v-col>
        <v-btn
          height="100%"
          rounded
          size="large"
          to="/"
          variant="flat"
          width="100%"
        >
          Dashboard
        </v-btn>
      </v-col>

      <v-col>
        <v-btn
          height="100%"
          rounded
          size="large"
          to="logs"
          variant="flat"
          width="100%"
        >
          Logs
        </v-btn>
      </v-col>

      <v-col>
        <v-btn
          height="100%"
          rounded
          size="large"
          to="settings"
          variant="flat"
          width="100%"
        >
          Settings
        </v-btn>
      </v-col>
    </v-toolbar-items>
    <v-spacer></v-spacer>
  </v-app-bar>
</template>

<script lang="ts" setup>
//
</script>

<style scoped></style>
