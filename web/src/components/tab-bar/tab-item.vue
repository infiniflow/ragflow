<script lang="ts" setup>
import { PropType, computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useTabBarStore } from '@/store';
import { DEFAULT_ROUTE_NAME, REDIRECT_ROUTE_NAME } from '@/router/constants';

export interface TagProps {
  title: string
  name: string
  fullPath: string
  query?: any
  ignoreCache?: boolean
}

const props = defineProps({
  itemData: {
    type: Object as PropType<TagProps>,
    default() {
      return [];
    },
  },
  index: {
    type: Number,
    default: 0,
  },
});

enum Eaction {
  reload = 'reload',
  current = 'current',
  left = 'left',
  right = 'right',
  others = 'others',
  all = 'all',
}

const router = useRouter();
const route = useRoute();
const tabBarStore = useTabBarStore();

const goto = (tag: TagProps) => {
  router.push({ ...tag });
};
const tagList = computed(() => {
  return tabBarStore.getTabList;
});

const disabledReload = computed(() => {
  return props.itemData.fullPath !== route.fullPath;
});

const disabledCurrent = computed(() => {
  return props.index === 0;
});

const disabledLeft = computed(() => {
  return [0, 1].includes(props.index);
});

const disabledRight = computed(() => {
  return props.index === tagList.value.length - 1;
});

const tagClose = (tag: TagProps, idx: number) => {
  tabBarStore.deleteTag(idx, tag);
  if (props.itemData.fullPath === route.fullPath) {
    const latest = tagList.value[idx - 1]; // 获取队列的前一个tab
    router.push({ ...latest });
  }
};

const findCurrentRouteIndex = () => {
  return tagList.value.findIndex(el => el.fullPath === route.fullPath);
};
const actionSelect = async (value: { key: string }) => {
  const { key } = value;
  const { itemData, index } = props;
  const copyTagList = [...tagList.value];
  if (key === Eaction.current) {
    tagClose(itemData, index);
  } else if (key === Eaction.left) {
    const currentRouteIdx = findCurrentRouteIndex();
    copyTagList.splice(1, props.index - 1);

    tabBarStore.freshTabList(copyTagList);
    if (currentRouteIdx < index) {
      router.push({ name: itemData.name });
    }
  } else if (key === Eaction.right) {
    const currentRouteIdx = findCurrentRouteIndex();
    copyTagList.splice(props.index + 1);

    tabBarStore.freshTabList(copyTagList);
    if (currentRouteIdx > index) {
      router.push({ name: itemData.name });
    }
  } else if (key === Eaction.others) {
    const filterList = tagList.value.filter((el: any, idx: number) => {
      return idx === 0 || idx === props.index;
    });
    tabBarStore.freshTabList(filterList);
    router.push({ name: itemData.name });
  } else if (key === Eaction.reload) {
    tabBarStore.deleteCache(itemData);
    await router.push({
      name: REDIRECT_ROUTE_NAME,
      params: {
        path: route.fullPath,
      },
    });
    tabBarStore.addCache(itemData.name);
  } else {
    tabBarStore.resetTabList();
    router.push({ name: DEFAULT_ROUTE_NAME });
  }
};
</script>

<template>
  <a-dropdown
    :trigger="['contextmenu']"
  >
    <a-tag
      closable
      :color="itemData.fullPath === $route.fullPath ? 'processing' : 'default'"
      @close="tagClose(itemData, index)"
      @click="goto(itemData)"
    >
      {{ itemData.title }}
    </a-tag>
    <template #overlay>
      <a-menu @click="actionSelect">
        <a-menu-item :key="Eaction.reload" :disabled="disabledReload">
          <span>重新加载</span>
        </a-menu-item>
        <a-menu-item :key="Eaction.current" :disabled="disabledCurrent">
          <span>关闭当前标签页</span>
        </a-menu-item>
        <a-menu-item :key="Eaction.left" :disabled="disabledLeft">
          <span>关闭左侧标签页</span>
        </a-menu-item>
        <a-menu-item :key="Eaction.right" :disabled="disabledRight">
          <span>关闭右侧标签页</span>
        </a-menu-item>
        <a-menu-item :key="Eaction.others">
          <span>关闭其它标签页</span>
        </a-menu-item>
        <a-menu-item :key="Eaction.all">
          <span>关闭全部标签页</span>
        </a-menu-item>
      </a-menu>
    </template>
  </a-dropdown>
</template>
