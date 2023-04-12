import { createRouter, createWebHistory } from "vue-router";
import { baseUrl } from "@/utils";
import { useAuthStore } from "@/store/auth/authStore";

const routes = [
  {
    path: "/",
    components: {
      default: () => import("@/views/DashboardView.vue"),
      navbar: () => import("@/layouts/default/Navbar.vue"),
    },
    meta: { requiresAuth: true },
  },

  {
    path: "/login",
    name: "Login",
    component: () => import("@/views/LoginView.vue"),
  },

  {
    path: "/onboard",
    name: "Onboard",
    component: () => import("@/views/OnBoardView.vue"),
  },

  {
    path: "/logs",
    name: "Logs",
    components: {
      default: () => import("@/views/LogsView.vue"),
      navbar: () => import("@/layouts/default/Navbar.vue"),
    },
    meta: { requiresAuth: true },
  },
  {
    path: "/settings",
    name: "Settings",
    components: {
      default: () => import("@/views/SettingsView.vue"),
      navbar: () => import("@/layouts/default/Navbar.vue"),
    },
    meta: { requiresAuth: true },
  },
];

const router = createRouter({
  history: createWebHistory(baseUrl()),
  routes,
});

router.beforeEach((to, from, next) => {
  const authStore = useAuthStore();
  const requiresAuth = to.matched.some((record) => record.meta.requiresAuth);

  if (requiresAuth && !authStore.isAuthenticated) {
    // If the route requires authentication and the user is not logged in, redirect to the login page
    next({ name: "Login" });
  } else if (to.name === "Login" && authStore.isAuthenticated) {
    // If the user is already logged in and tries to access the login page, redirect to the dashboard
    next({ path: "/" });
  } else {
    // If none of the above conditions apply, proceed to the requested route
    next();
  }
});

export default router;
