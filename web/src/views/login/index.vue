<script lang="ts" setup>
import LoginBanner from './components/banner.vue';
import LoginForm from './components/login-form.vue';
</script>

<template>
  <div class="container">
    <div class="logo">
      <img src="https://alicdn.antdv.com/v2/assets/logo.1ef800a8.svg">
      <div class="logo-text">
        Admin
      </div>
    </div>
    <login-banner />
    <div class="content">
      <div class="content-inner">
        <login-form />
      </div>
    </div>
  </div>
</template>

<style lang="less" scoped>
.container {
  display: flex;
  height: 100vh;

  .banner {
    width: 550px;
    background: linear-gradient(163.85deg, #1d2129 0%, #00308f 100%);
  }

  .content {
    position: relative;
    display: flex;
    flex: 1;
    align-items: center;
    justify-content: center;
    padding-bottom: 40px;
  }

  .footer {
    position: absolute;
    right: 0;
    bottom: 0;
    width: 100%;
  }
}

.logo {
  position: fixed;
  top: 24px;
  left: 22px;
  z-index: 1;
  display: inline-flex;
  align-items: center;

  &-text {
    margin-right: 4px;
    margin-left: 4px;
    color: rgb(247, 248, 250);
    font-size: 20px;
  }

  img {
    width: 33px;
    height: 33px;
  }
}
</style>

<style lang="less" scoped>
// responsive
@media (max-width: 992px) {
  .container {
    .banner {
      width: 25%;
    }
  }
}
</style>
