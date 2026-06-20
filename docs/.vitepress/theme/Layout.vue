<script setup>
import DefaultTheme from 'vitepress/theme'
import { useData } from 'vitepress'
import LandingPage from './components/LandingPage.vue'

const { Layout } = DefaultTheme
const { frontmatter } = useData()
</script>

<template>
  <LandingPage v-if="frontmatter.layout === 'landing'" />
  <Layout v-else />
</template>
