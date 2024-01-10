/**
 * main.ts
 *
 * Bootstraps Vuetify and other plugins then mounts the App`
 */

// Components
import App from './App.vue'

// Composables
import { createApp } from 'vue'

// Main styles
import './index.css'

// Plugins
import { registerPlugins } from '@/plugins'

declare global {
  interface Window { APP: APP; }
}

window.APP = window.APP || {};

const app = createApp(App)

registerPlugins(app)

app.mount('#app')
