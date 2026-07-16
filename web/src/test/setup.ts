import { beforeAll } from "vitest";
import { createVuetify } from "vuetify";

// happy-dom has no ResizeObserver; several Vuetify components construct one.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

beforeAll(() => {
  if (!("ResizeObserver" in globalThis)) {
    globalThis.ResizeObserver =
      ResizeObserverStub as unknown as typeof ResizeObserver;
  }
});

// Components/directives are auto-imported per-SFC by vite-plugin-vuetify (already
// in vite.config), so a bare instance is enough — importing vuetify/components
// here would eagerly pull every component's CSS and break the run.
export const vuetify = createVuetify();
