<template>
  <v-container>
    <v-row justify="end">
      <v-dialog
        v-model="dialog"
        :fullscreen="!isDesktop"
        :scrim="false"
        transition="dialog-bottom-transition"
      >
        <template v-slot:activator="{ props }">
          <v-btn color="primary" dark v-bind="props"> Add Notification</v-btn>
        </template>
        <v-card>
          <v-form v-model="valid" ref="form" @submit.prevent="submit()">
            <v-toolbar dark color="primary">
              <v-btn icon dark @click="dialog = false">
                <v-icon>mdi-close</v-icon>
              </v-btn>
              <v-toolbar-title>New Notification</v-toolbar-title>
              <v-spacer></v-spacer>
              <v-toolbar-items>
                <v-btn variant="text" type="submit">Save</v-btn>
              </v-toolbar-items>
            </v-toolbar>
            <v-list subheader>
              <v-list-subheader>Notification Setting</v-list-subheader>
              <v-list-item>
                <v-text-field
                  v-model="initialValuesRef.name"
                  :rules="[rules.required]"
                  aria-required="true"
                  dense
                  label="Name"
                  prepend-inner-icon="mdi-text"
                  variant="filled"
                ></v-text-field>
              </v-list-item>
              <v-list-item>
                <v-select
                  v-model="initialValuesRef.type"
                  :items="NotificationTypeOptions"
                  label="Type"
                  prepend-inner-icon="mdi-mailbox"
                  variant="filled"
                  clearable
                ></v-select>
              </v-list-item>
            </v-list>
            <v-divider></v-divider>

            <v-list subheader>
              <v-list-subheader>Events</v-list-subheader>
              <v-list-item v-for="event in EventOptions" :key="event.title">
                <v-list-item-title>
                  {{ event.title }}
                </v-list-item-title>

                <v-list-item-subtitle>
                  {{ event.subtitle }}
                </v-list-item-subtitle>

                <template v-slot:prepend>
                  <v-checkbox v-model="eventStates[event.value]"></v-checkbox>
                </template>
              </v-list-item>
            </v-list>

            <div v-if="initialValuesRef.type === 'DISCORD'">
              <v-divider></v-divider>
              <v-list subheader>
                <v-list-subheader>
                  Discord
                  <v-list-item-subtitle>
                    Create a
                    <a
                      class="text-decoration-none"
                      href="https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks"
                      target="_blank"
                    >
                      webhook
                    </a>
                    in your Discord server and paste the URL
                  </v-list-item-subtitle>
                </v-list-subheader>
                <v-list-item>
                  <v-text-field
                    v-model="initialValuesRef.webhook"
                    :rules="[rules.required]"
                    aria-required="true"
                    dense
                    label="Webhook URL"
                    variant="filled"
                    :type="showPassword ? 'text' : 'password'"
                    :append-inner-icon="
                      showPassword ? 'mdi-eye' : 'mdi-eye-off'
                    "
                    @click:append-inner="showPassword = !showPassword"
                  ></v-text-field>
                </v-list-item>
              </v-list>
            </div>

            <div v-if="initialValuesRef.type === 'NOTIFIARR'">
              <v-divider></v-divider>
              <v-list subheader>
                <v-list-subheader>
                  Notifiarr
                  <v-list-item-subtitle>
                    Enable the syncyomi integration and optionally create a new
                    API Key.
                  </v-list-item-subtitle>
                </v-list-subheader>
                <v-list-item>
                  <v-text-field
                    v-model="initialValuesRef.api_key"
                    :rules="[rules.required]"
                    aria-required="true"
                    dense
                    label="API Key"
                    prepend-inner-icon="mdi-bell"
                    variant="filled"
                    :type="showPassword ? 'text' : 'password'"
                    :append-inner-icon="
                      showPassword ? 'mdi-eye' : 'mdi-eye-off'
                    "
                    @click:append-inner="showPassword = !showPassword"
                  ></v-text-field>
                </v-list-item>
              </v-list>
            </div>

            <div v-if="initialValuesRef.type === 'TELEGRAM'">
              <v-divider></v-divider>
              <v-list subheader>
                <v-list-subheader>
                  Telegram
                  <v-list-item-subtitle>
                    Read how to
                    <a
                      class="text-decoration-none"
                      href="https://core.telegram.org/bots#3-how-do-i-create-a-bot"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      create a bot
                    </a>
                  </v-list-item-subtitle>
                </v-list-subheader>
                <v-list-item>
                  <v-text-field
                    v-model="initialValuesRef.token"
                    dense
                    label="Bot Token"
                    variant="filled"
                    placeholder="1234567890:ABCDEDefacer1234567890abcdef1234567890"
                    :type="showPassword ? 'text' : 'password'"
                    :append-inner-icon="
                      showPassword ? 'mdi-eye' : 'mdi-eye-off'
                    "
                    @click:append-inner="showPassword = !showPassword"
                  ></v-text-field>

                  <v-text-field
                    v-model="initialValuesRef.channel"
                    :rules="[rules.required]"
                    aria-required="true"
                    dense
                    label="Channel ID"
                    variant="filled"
                    placeholder="1234567890"
                    :type="showPassword ? 'text' : 'password'"
                    :append-inner-icon="
                      showPassword ? 'mdi-eye' : 'mdi-eye-off'
                    "
                    @click:append-inner="showPassword = !showPassword"
                  ></v-text-field>
                </v-list-item>
              </v-list>
            </div>
          </v-form>
          <v-card-actions>
            <v-spacer></v-spacer>
            <v-btn
              :disabled="isTestButtonDisabled"
              variant="tonal"
              color="primary"
              @click="testNotification"
              >Test
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>
    </v-row>
  </v-container>
</template>

<script lang="ts" setup>
import { computed, ref, Ref } from "vue";
import {
  EventOptions as RawEventOptions,
  NotificationTypeOptions,
} from "@/domain/constants";
import {
  Notification,
  NotificationEvent,
  NotificationType,
} from "@/types/Notification";
import { useMutation, useQueryClient } from "@tanstack/vue-query";
import { APIClient } from "@/api/APIClient";
import { useDisplay } from "vuetify";

interface InitialValues {
  id: number;
  enabled: boolean;
  type: NotificationType;
  name: string;
  webhook?: string;
  token?: string;
  api_key?: string;
  channel?: string;
  events: NotificationEvent[];
  eventStates: Record<string, boolean>;
}

const dialog: Ref<boolean> = ref(false);
const valid = ref<boolean>(false);
const showPassword: Ref<boolean> = ref(false);
const form = ref();
const { width } = useDisplay();
const queryClient = useQueryClient();
const EventOptions = RawEventOptions.map((event) => ({
  ...event,
  enabled: false,
}));

const eventStates: Ref<Record<string, boolean>> = ref(
  Object.fromEntries(
    EventOptions.map((event) => [event.value, false])
  ) as Record<string, boolean>
);

const isDesktop = computed(() => {
  return width.value > 700;
});

const initialValuesRef: Ref<InitialValues> = ref({
  id: 0,
  enabled: false,
  type: "" as NotificationType,
  name: "",
  webhook: "",
  token: "",
  api_key: "",
  channel: "",
  events: [],
  eventStates: {},
});

const rules = {
  required: (value: string) => !!value || "Required.",
};

const resetSelectedNotifications = () => {
  initialValuesRef.value.type = "" as NotificationType;
  Object.keys(eventStates.value).forEach((key) => {
    eventStates.value[key] = false;
  });
};

// create new notification
const createNotificationMutation = useMutation({
  mutationFn: async (values: Notification) => {
    await APIClient.notifications.create(values);
  },
  onSuccess: () => {
    dialog.value = false;
    form.value.reset();
    resetSelectedNotifications();
    queryClient.invalidateQueries(["notifications"]);
  },
  onError: (error) => {
    console.log("createMutation error", error);
  },
});

// test notification
const testNotificationMutation = useMutation({
  mutationFn: async (values: Notification) => {
    await APIClient.notifications.test(values);
  },
  onSuccess: () => {
    dialog.value = false;
    form.value.reset();
    resetSelectedNotifications();
    queryClient.invalidateQueries(["notifications"]);
  },
  onError: (error) => {
    console.log("createMutation error", error);
  },
});

// Disable test button if required fields are not filled
const isTestButtonDisabled = computed(() => {
  if (initialValuesRef.value.type === ("" as NotificationType)) {
    return true;
  }
  if (
    initialValuesRef.value.type === "DISCORD" &&
    (!initialValuesRef.value.webhook || initialValuesRef.value.webhook === "")
  ) {
    return true;
  }
  if (
    initialValuesRef.value.type === "NOTIFIARR" &&
    (!initialValuesRef.value.api_key || initialValuesRef.value.api_key === "")
  ) {
    return true;
  }
  return (
    initialValuesRef.value.type === "TELEGRAM" &&
    (!initialValuesRef.value.token ||
      initialValuesRef.value.token === "" ||
      !initialValuesRef.value.channel ||
      initialValuesRef.value.channel === "")
  );
});

const testNotification = () => {
  if (form.value.validate()) {
    // Create an array of enabled events
    const enabledEvents = EventOptions.filter(
      (event) => eventStates.value[event.value]
    );

    const data: Notification = {
      id: initialValuesRef.value.id,
      enabled: true,
      type: initialValuesRef.value.type,
      name: initialValuesRef.value.name,
      webhook: initialValuesRef.value.webhook,
      token: initialValuesRef.value.token,
      api_key: initialValuesRef.value.api_key,
      channel: initialValuesRef.value.channel,
      events: enabledEvents.map((event) => event.value as NotificationEvent),
    };

    if (data.name === "") {
      return;
    }

    testNotificationMutation.mutateAsync(data);
  }
};

const submit = () => {
  if (form.value.validate()) {
    // Create an array of enabled events
    const enabledEvents = EventOptions.filter(
      (event) => eventStates.value[event.value]
    );

    const data: Notification = {
      id: initialValuesRef.value.id,
      enabled: true,
      type: initialValuesRef.value.type,
      name: initialValuesRef.value.name,
      webhook: initialValuesRef.value.webhook,
      token: initialValuesRef.value.token,
      api_key: initialValuesRef.value.api_key,
      channel: initialValuesRef.value.channel,
      events: enabledEvents.map((event) => event.value as NotificationEvent),
    };

    if (data.name === "") {
      return;
    }

    createNotificationMutation.mutateAsync(data);
  }
};
</script>

<style scoped></style>
