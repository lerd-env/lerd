<script setup>
import DefaultTheme from 'vitepress/theme-without-fonts'
import { useData } from 'vitepress'
import LandingPage from './components/LandingPage.vue'
import AboutPage from './components/AboutPage.vue'

const { Layout } = DefaultTheme
const { frontmatter } = useData()
</script>

<template>
  <LandingPage v-if="frontmatter.layout === 'landing'" />
  <AboutPage v-else-if="frontmatter.layout === 'about'" />
  <Layout v-else />
</template>
