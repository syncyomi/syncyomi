import { createRouter, createWebHistory } from "vue-router";
import { baseUrl } from "@/utils";
import { useAuthStore } from "@/store/auth/authStore";
import { APIClient } from "@/api/APIClient";

const routes = [
  {
    path: "/",
    name: "Dashboard",
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

  {
    path: "/:catchAll(.*)",
    redirect: "/",
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
    // If the route requires authentication and the user is not logged in, check if the user can onboard and redirect to the onboard page if they can.
    APIClient.auth.canOnboard().then((canOnboard) => {
      if (canOnboard) {
        next({ name: "Onboard" });
      }
    });

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
