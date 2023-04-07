// Composables
import { createRouter, createWebHistory } from "vue-router";

const routes = [
  {
    path: "/",
    components: {
      default: () => import("@/views/DashboardView.vue"),
      navbar: () => import("@/layouts/default/Navbar.vue"),
    },
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
  },
  {
    path: "/settings",
    name: "Settings",
    components: {
      default: () => import("@/views/SettingsView.vue"),
      navbar: () => import("@/layouts/default/Navbar.vue"),
    },
  },
];

const router = createRouter({
  history: createWebHistory(process.env.BASE_URL),
  routes,
});

// router.beforeEach((to, from, next) => {
//   if (
//     to.name !== "Login" &&
//     to.name !== "Onboard" &&
//     !localStorage.getItem("token")
//   ) {
//     next({ name: "Login" });
//   } else {
//     next();
//   }
// });

export default router;
