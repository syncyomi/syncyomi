/**
 * main.ts
 *
 * Bootstraps Vuetify and other plugins then mounts the App`
 */

// Main styles
// Must stay first: establishes CSS layer order before Vuetify's component styles are emitted
import './index.css'

// Components
import App from './App.vue'

// Composables
import { createApp } from 'vue'

// Plugins
import { registerPlugins } from '@/plugins'

declare global {
  interface Window { APP: APP; }
}

window.APP = window.APP || {};

const app = createApp(App)

registerPlugins(app)

app.mount('#app')
