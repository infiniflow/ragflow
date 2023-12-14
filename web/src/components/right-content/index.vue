<script setup lang="ts">
export interface CurrentUser {
  nickname: string
  avatar?: string
}

defineProps<{
  currentUser: CurrentUser
}>();

const { logout } = useUser();
const handleLogout = () => {
  logout();
};
</script>
<template>
  <div style="margin-right: 12px">
    <a-space :size="24">
      <a-dropdown :trigger="['click']">
        <template #overlay>
          <a-menu>
            <a-menu-item>
              <template #icon>
                <setting-outlined />
              </template>
              <span>个人设置</span>
            </a-menu-item>
            <a-menu-item @click="handleLogout">
              <template #icon>
                <logout-outlined />
              </template>
              <span>退出登录</span>
            </a-menu-item>
          </a-menu>
        </template>
        <a-avatar shape="circle" size="32">
          TEST
        </a-avatar>
      </a-dropdown>
    </a-space>
  </div>
</template>
