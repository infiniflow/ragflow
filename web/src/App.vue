<script setup lang="ts">
import { ConfigProvider, StyleProvider } from 'ant-design-vue';
import zhCN from 'ant-design-vue/es/locale/zh_CN';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';
import { useUserTheme } from './hooks/useTheme';

dayjs.locale('zh-cn');
useUserTheme();
</script>

<template>
  <style-provider hash-priority="high">
    <config-provider :locale="zhCN">
      <router-view />
    </config-provider>
  </style-provider>
</template>
