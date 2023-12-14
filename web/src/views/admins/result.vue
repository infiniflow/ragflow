<template>
  <div class="result-header">
    <a-breadcrumb>
      <a-breadcrumb-item> </a-breadcrumb-item>
      <a-breadcrumb-item><a href="">Application Center</a></a-breadcrumb-item>
      <a-breadcrumb-item><a href="">Application List</a></a-breadcrumb-item>
      <a-breadcrumb-item>An Application</a-breadcrumb-item>
    </a-breadcrumb>
    <a-button class="operate-button" type="text" size="large">
      <template #icon>
        <img width="66%" src="@/assets/images/cloud_download.svg" />
      </template>
    </a-button>
  </div>

  <a-empty :image="simpleImage" description="empty" :imageStyle="{ width: '100px', margin: '0 auto' }" />
</template>

<script lang="ts" setup>
import { h } from 'vue';
import { CloudDownloadOutlined } from '@ant-design/icons-vue';
import { Empty } from 'ant-design-vue';
const simpleImage = Empty.PRESENTED_IMAGE_SIMPLE;

</script>

<style scoped>
.result-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 20px;
}

.operate-button {
  display: flex;
  justify-content: center;
  align-items: center;
}
</style>