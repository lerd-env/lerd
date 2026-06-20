import DefaultTheme from 'vitepress/theme'
import Layout from './Layout.vue'
import 'asciinema-player/dist/bundle/asciinema-player.css'
import './style.css'
import './landing.css'

export default {
  extends: DefaultTheme,
  Layout,
}
