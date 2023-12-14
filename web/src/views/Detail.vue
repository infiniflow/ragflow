<script lang="ts" setup>
import { dependencies } from '../../package.json';
</script>

<template>
  <page-container title="Version" sub-title="show current project dependencies">
    <template #content>
      <strong>Content Area</strong>
    </template>
    <template #extra>
      <strong>Extra Area</strong>
    </template>
    <template #extraContent>
      <strong>ExtraContent Area</strong>
    </template>
    <template #tags>
      <a-tag>Tag1</a-tag>
      <a-tag color="pink">
        Tag2
      </a-tag>
    </template>
    <a-card title="Project Version">
      <p v-for="(dep, key) in dependencies" :key="key">
        <strong style="margin-right: 12px">{{ key }}:</strong>
        <a-tag>{{ dep }}</a-tag>
      </p>
      <p v-for="d in new Array(50)" :key="d">
        text block...
      </p>
    </a-card>
  </page-container>
</template>
