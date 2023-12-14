<script lang="ts" setup>
import { throttle } from 'lodash-es';
import echarts, { type EChartsOption } from './echarts';

const props = defineProps({
  option: {
    type: Object || null,
    default: null,
  },
});
let chart: echarts.ECharts | null = null;
const chartRef = ref();

function setOption(option: EChartsOption) {
  chart?.setOption(option);
}

function resize() {
  return throttle(() => {
    chart?.resize();
  }, 200);
}

const resizeObserver = new ResizeObserver(() => {
  resize()();
});
onMounted(() => {
  chart = echarts.init(chartRef.value);
  if (props.option) {
    setOption(props.option);
  }

  resizeObserver.observe(chartRef?.value);
});

onBeforeUnmount(() => {
  resizeObserver.unobserve(chartRef?.value);
  resizeObserver.disconnect();
  chart?.dispose();
  chart = null;
});

watch(
  () => props.option,
  (val) => {
    setOption(val);
  },
  {
    deep: true,
  },
);

defineExpose({
  setOption,
});
</script>

<template>
  <div ref="chartRef" class="w-full h-full" />
</template>
