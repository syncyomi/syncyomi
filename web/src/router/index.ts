import { createRouter, createWebHistory } from "vue-router";
import { baseUrl } from "@/utils";
import { useAuthStore } from "@/store/auth/authStore";

const routes = [
  // {
  //   path: "/",
  //   name: "Dashboard",
  //   components: {
  //     default: () => import("@/views/DashboardView.vue"),
  //     navbar: () => import("@/layouts/default/Navbar.vue"),
  //   },
  //   meta: { requiresAuth: true },
  // },

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

  {
    path: "/:catchAll(.*)",
    redirect: "/settings",
  },
];

const router = createRouter({
  history: createWebHistory(baseUrl()),
  routes,
});

router.beforeEach((to, from, next) => {
  const authStore = useAuthStore();
  const requiresAuth = to.matched.some((record) => record.meta.requiresAuth);

  if (!authStore.isAuthenticated && requiresAuth) {
    // If the route requires authentication and the user is not logged in, redirect to the login page.
    next({ name: "Login" });
  } else if (
    authStore.isAuthenticated &&
    (to.name === "Login" || to.name === "Onboard")
  ) {
    // If the user is already authenticated and tries to access the login or onboard page, redirect to the dashboard.
    next({ path: "/settings" });
  } else {
    // If none of the above conditions apply, proceed to the requested route.
    next();
  }
});

export default router;
