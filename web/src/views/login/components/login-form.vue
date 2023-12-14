<script lang="ts" setup>
import { reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useStorage } from '@vueuse/core';
import { useUserStore } from '@/store';
import type { LoginData } from '@/api/user';

const router = useRouter();
const errorMessage = ref('');
const { loading, setLoading } = useLoading();
const userStore = useUserStore();

const loginConfig = useStorage('login-config', {
  rememberPassword: true,
  username: 'admin', // 演示默认值
  password: 'admin', // demo default value
});

const userInfo = reactive({
  username: loginConfig.value.username,
  password: loginConfig.value.password,
});

const handleSubmit = async (values: Record<string, any>) => {
  if (loading.value) {
    return;
  }
  setLoading(true);
  try {
    await userStore.login(values as LoginData);
    const { redirect, ...othersQuery } = router.currentRoute.value.query;
    router.push({
      name: (redirect as string) || 'welcome',
      query: {
        ...othersQuery,
      },
    });
    message.success('欢迎使用');
    const { rememberPassword } = loginConfig.value;
    const { username, password } = values;
    // 实际生产环境需要进行加密存储。
    // The actual production environment requires encrypted storage.
    loginConfig.value.username = rememberPassword ? username : '';
    loginConfig.value.password = rememberPassword ? password : '';
  } catch (err) {
    errorMessage.value = (err as Error).message;
  } finally {
    setLoading(false);
  }
};
</script>

<template>
  <div class="login-form-wrapper">
    <div class="login-form-title">
      登录
    </div>
    <div class="login-form-sub-title">
      登录
    </div>
    <div class="login-form-error-msg">
      {{ errorMessage }}
    </div>
    <a-form :model="userInfo" name="login" class="login-form" autocomplete="off" @finish="handleSubmit">
      <a-form-item name="username" :rules="[{ required: true, message: '用户名不能为空' }]"
        :validate-trigger="['change', 'blur']" hide-label>
        <a-input v-model:value="userInfo.username" placeholder="用户名：admin">
          <template #prefix>
            <user-outlined type="user" />
          </template>
        </a-input>
      </a-form-item>
      <a-form-item name="password" :rules="[{ required: true, message: '密码不能为空' }]" :validate-trigger="['change', 'blur']"
        hide-label>
        <a-input-password v-model:value="userInfo.password" placeholder="密码：admin" allow-clear>
          <template #prefix>
            <lock-outlined />
          </template>
        </a-input-password>
      </a-form-item>
      <a-space :size="16" direction="vertical" style="width: 100%;">
        <div class="login-form-password-actions">
          <a-checkbox v-model:checked="loginConfig.rememberPassword">
            记住密码
          </a-checkbox>
          <a-button type="link" size="small">
            忘记密码
          </a-button>
        </div>
        <a-button type="primary" block :loading="loading" html-type="submit">
          登录
        </a-button>
        <a-button type="text" block class="login-form-register-btn">
          注册
        </a-button>
      </a-space>
    </a-form>
  </div>
</template>

<style lang="less" scoped>
.login-form {
  &-wrapper {
    width: 320px;
  }

  &-title {
    color: rgb(29, 33, 41);
    font-weight: 500;
    font-size: 24px;
    line-height: 32px;
  }

  &-sub-title {
    color: rgb(134, 144, 156);
    font-size: 16px;
    line-height: 24px;
  }

  &-error-msg {
    height: 32px;
    color: rgb(var(--red-6));
    line-height: 32px;
  }

  &-password-actions {
    display: flex;
    justify-content: space-between;
  }

  &-register-btn {
    color: rgb(134, 144, 156) !important;
  }
}
</style>
