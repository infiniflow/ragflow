<script lang="ts" setup>
const router = useRouter();
const back = () => {
  // warning： Go to the node that has the permission
  router.push({ name: 'welcome' });
};
</script>

<template>
  <div class="content">
    <a-result class="result" status="404" title="404" sub-title="not found" />
    <div class="operation-row">
      <a-button key="back" type="primary" @click="back">
        back
      </a-button>
    </div>
  </div>
</template>

<style scoped lang="less">
  .content {
    position: absolute;
    top: 30%;
    left: 50%;
    margin-left: -95px;
    margin-top: -121px;
    text-align: center;
  }
</style>
