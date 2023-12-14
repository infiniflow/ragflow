<script setup lang="ts">
import { RouterLink, RouterView, useRouter } from "vue-router";
import {
  type RouteContextProps,
  clearMenuItem,
  getMenuData,
} from "@ant-design-vue/pro-layout";

const router = useRouter();
const { menuData } = getMenuData(clearMenuItem(router.getRoutes()));

const state = reactive<Omit<RouteContextProps, "menuData">>({
  collapsed: false, // default collapsed
  openKeys: [], // defualt openKeys
  selectedKeys: [], // default selectedKeys
});
// 
const loading = ref<boolean>(false);

const proConfig = ref({
  layout: "side",
  navTheme: "light",
  fixedHeader: true,
  fixSiderbar: false,
  splitMenus: true,
  contentWidth: 'Fluid',
});

const currentUser = reactive({
  nickname: "Admin",
  avatar: "A",
});

const drawerVisible = ref(true);

watch(
  router.currentRoute,
  () => {
    const matched = router.currentRoute.value.matched.concat();
    state.selectedKeys = matched
      .filter((r) => r.name !== "index")
      .map((r) => r.path);
    state.openKeys = matched
      .filter((r) => r.path !== router.currentRoute.value.path)
      .map((r) => r.path);
  },
  {
    immediate: true,
  }
);
</script>

<template>
  <a-drawer v-model:open="drawerVisible"></a-drawer>

  <!-- <router-view v-slot="{ Component, route }">
    <transition name="slide-left" mode="out-in">
      <component :is="Component" :key="route.path" />
    </transition>
  </router-view> -->


  <pro-layout v-model:collapsed="state.collapsed" v-model:selectedKeys="state.selectedKeys"
    v-model:openKeys="state.openKeys" :loading="loading" header-theme="light" :menu-data="menuData" disable-content-margin
    style="min-height: 100vh" v-bind="proConfig">
    <template #menuHeaderRender>
      <router-link :to="{ path: '/' }">
        <h1 style="text-align: center;">docGPT</h1>
      </router-link>
    </template>
    <template #rightContentRender>
      <RightContent :current-user="currentUser" />
    </template>
    <router-view v-slot="{ Component, route }">
      <transition name="slide-left" mode="out-in">
        <component :is="Component" :key="route.path" />
      </transition>
    </router-view>
  </pro-layout>
</template>
