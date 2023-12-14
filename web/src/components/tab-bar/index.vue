<script lang="ts" setup>
import { computed, onUnmounted, ref } from 'vue';
import type { RouteLocationNormalized } from 'vue-router';
import type { Affix } from 'ant-design-vue';
import tabItem from './tab-item.vue';
import {
  listenerRouteChange,
  removeRouteListener,
} from '@/utils/route-listener';
import { useTabBarStore } from '@/store';

const tabBarStore = useTabBarStore();

const affixRef = ref<InstanceType<typeof Affix>>();
const tagList = computed(() => {
  return tabBarStore.getTabList;
});

listenerRouteChange((route: RouteLocationNormalized) => {
  if (
    !route.meta.noAffix
      && !tagList.value.some(tag => tag.fullPath === route.fullPath)
  ) {
    tabBarStore.updateTabList(route);
  }
}, true);

onUnmounted(() => {
  removeRouteListener();
});
</script>

<template>
  <div class="tab-bar-container">
    <a-affix ref="affixRef">
      <div class="tab-bar-box">
        <div class="tab-bar-scroll">
          <div class="tags-wrap">
            <tab-item
              v-for="(tag, index) in tagList"
              :key="tag.fullPath"
              :index="index"
              :item-data="tag"
            />
          </div>
        </div>
        <div class="tag-bar-operation" />
      </div>
    </a-affix>
  </div>
</template>

<style scoped lang="less">
  .tab-bar-container {
    position: relative;
    background-color: #fff;
    .tab-bar-box {
      display: flex;
      padding: 0 0 0 20px;
      background-color: #fff;
      border-bottom: 1px solid rgb(229,230,235);
      .tab-bar-scroll {
        height: 32px;
        flex: 1;
        overflow: hidden;
        .tags-wrap {
          padding: 7px 0;
          height: 48px;
          white-space: nowrap;
          overflow-x: auto;

          :deep(.ant-tag) {
            cursor: pointer;
            &:first-child {
              .ant-tag-close-icon {
                display: none;
              }
            }
          }
        }
      }
    }

    .tag-bar-operation {
      width: 100px;
      height: 32px;
    }
  }
</style>
