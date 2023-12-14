<script lang="ts" setup>
import bannerImage from '@/assets/images/login-banner.png';

const carouselItem = computed(() => [
  {
    slogan: '开箱即用的模板',
    subSlogan: '丰富的的页面模板，覆盖大多数典型业务场景',
    image: bannerImage,
  },
  {
    slogan: '内置了常见问题的解决方案',
    subSlogan: '路由配置，状态管理应有尽有',
    image: bannerImage,
  },
]);
</script>

<template>
  <div class="banner">
    <div class="banner-inner">
      <a-carousel class="carousel" animation-name="fade" arrows autoplay>
        <template #prevArrow>
          <div class="custom-slick-arrow" style="left: 10px">
            <left-circle-outlined />
          </div>
        </template>
        <template #nextArrow>
          <div class="custom-slick-arrow" style="right: 10px">
            <right-circle-outlined />
          </div>
        </template>
        <div v-for="item in carouselItem" :key="item.slogan">
          <div :key="item.slogan" class="carousel-item" style="height: 100vh;">
            <div class="carousel-title">
              {{ item.slogan }}
            </div>
            <div class="carousel-sub-title">
              {{ item.subSlogan }}
            </div>
            <img class="carousel-image" :src="item.image">
          </div>
        </div>
      </a-carousel>
    </div>
  </div>
</template>

<style lang="less" scoped>
.ant-carousel :deep(.slick-arrow.custom-slick-arrow) {
  width: 25px;
  height: 25px;
  font-size: 25px;
  color: #fff;
  background-color: rgba(31, 45, 61, 0.11);
  opacity: 0.3;
  z-index: 1;
}
.ant-carousel :deep(.custom-slick-arrow:before) {
  display: none;
}
.ant-carousel :deep(.custom-slick-arrow:hover) {
  opacity: 0.5;
}

.banner {
  display: flex;
  align-items: center;
  justify-content: center;

  &-inner {
    flex: 1;
    height: 100%;
    width: 100%;
  }
}

.carousel {
  height: 100%;

  &-item {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100%;
  }

  &-title {
    color: rgb(247, 248, 250);
    font-weight: 500;
    font-size: 20px;
    line-height: 28px;
  }

  &-sub-title {
    margin-top: 8px;
    color: rgb(134, 144, 156);
    font-size: 14px;
    line-height: 22px;
  }

  &-image {
    width: 320px;
    margin-top: 30px;
  }
}
</style>
