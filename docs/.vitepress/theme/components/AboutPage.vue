<script setup>
import { withBase } from 'vitepress'
import SiteHeader from './SiteHeader.vue'
import SiteFooter from './SiteFooter.vue'

const home = withBase('/')
</script>

<template>
  <div class="lerd-landing">
    <SiteHeader :home="home" />

    <main id="top">
      <section class="about"><div class="wrap">
        <div class="label"><b>00</b> / About</div>
        <h1>Why Lerd exists</h1>
        <div class="about-body">
          <figure>
            <img :src="withBase('/assets/about/george.jpg')" width="460" height="460" alt="George Dumitrescu">
            <figcaption><strong>George Dumitrescu</strong>Creator and maintainer</figcaption>
          </figure>
          <div class="about-text">
            <p>I'm George Dumitrescu, a PHP developer who works on Linux. I started Lerd in March 2026 because the tool I wanted for my own desktop did not exist. Laravel Herd gave Mac and Windows users a .test domain, HTTPS and the right PHP version for each project with nothing to set up, and Linux had nothing like it.</p>
            <p>On Linux the choice was Docker with a compose file per project, or system packages installed by hand that drifted from one machine to the next. Lerd takes a different route: rootless Podman containers that run as your own user, installed into your home folder, with no daemon and each project's PHP, Node and services kept apart from the rest of the machine.</p>
            <p>Lerd is free and MIT licensed, and it will stay that way. Frameworks and services live in open stores that anyone can extend, so support for a new tool arrives as a YAML file instead of a release. If Lerd saves you time, you can <a href="https://github.com/sponsors/geodro" target="_blank" rel="noopener">sponsor the work</a> or write to me at <a href="mailto:george@dumitres.co">george@dumitres.co</a>.</p>
          </div>
        </div>
      </div></section>
    </main>

    <SiteFooter :home="home" />
  </div>
</template>
