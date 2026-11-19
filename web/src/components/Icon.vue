<script setup lang="ts">
withDefaults(defineProps<{name:string;size?:number;scanning?:boolean}>(),{size:20});
const paths:Record<string,string>={
 key:'M15 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0M11 11v11m0-5h4m-4 4h3',
 device:'M7 2h10v22H7zM11 20h2',
 external:'M14 3h7v7M21 3 10 14M10 5H3v17h17v-7',
 home:'M3 10.5 12 3l9 7.5M5 9.5V21h5v-7h4v7h5V9.5',
 inputs:'M4 4h16v12H4zM9 20h6M12 16v4M7 8h10M7 12h5',
 speaker:'M6 3h12v18H6zM12 6.5h.01M15.5 14a3.5 3.5 0 1 1-7 0 3.5 3.5 0 0 1 7 0',
 river:'M3 7h4c5 0 5 10 10 10h4M3 17h4c5 0 5-10 10-10h4M18 4l3 3-3 3M18 14l3 3-3 3',
 settings:'M9.5 3h5l.6 3 2.1 1.2L20 6l2.5 4.3-2.3 2 .1 2.4 2.2 2L20 21l-2.8-1.1-2.1 1.2-.6 2.9h-5l-.6-2.9-2.1-1.2L4 21l-2.5-4.3 2.2-2 .1-2.4-2.3-2L4 6l2.8 1.2L8.9 6zM15.5 13.5a3.5 3.5 0 1 1-7 0 3.5 3.5 0 0 1 7 0',
 plus:'M12 5v14M5 12h14',play:'M8 5l11 7-11 7z',stop:'M6 6h12v12H6z',pause:'M8 5v14M16 5v14',
 edit:'m15 4 5 5M4 20l5-1L21 7l-5-5L4 14z',trash:'M4 6h16M9 6V3h6v3M6 6l1 15h10l1-15M10 10v7M14 10v7',
 chevron:'m9 5 7 7-7 7',close:'m6 6 12 12M18 6 6 18',check:'m5 12 4 4L19 6',warning:'m12 3 10 18H2zM12 9v5M12 17h.01',
 info:'M12 11v6M12 7h.01M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',
 refresh:'M20 8a8.5 8.5 0 1 0 .3 7M20 3v5h-5',download:'M12 3v12m-5-5 5 5 5-5M4 16v5h16v-5',
 upload:'M12 16V4m-5 5 5-5 5 5M4 16v5h16v-5',search:'M15 15l6 6M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0',
 shield:'M12 3 3 6v6c0 5 9 10 9 10s9-5 9-10V6zM8 12l3 3 5-6',
 network:'M9 3h6v6H9zM2 17h6v5H2zM16 17h6v5h-6zM12 9v4M5 17v-4h14v4',
 copy:'M8 8h13v13H8zM16 8V3H3v13h5',sound:'m3 9 5 0 5-5v16l-5-5H3zM17 8c3 2 3 6 0 8M20 5c5 4 5 10 0 14',
 clock:'M12 6v6l4 2M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0',
 arrow:'M4 12h16m-6-6 6 6-6 6',file:'M6 2h8l5 5v15H6zM14 2v6h5M9 13h7M9 17h7',
};
</script>
<template><svg :width="size" :height="size" :viewBox="name==='scan'?'0 0 24 24':'0 0 24 26'" fill="none" stroke="currentColor" stroke-width="1.65" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" class="icon">
  <g v-if="name==='scan'">
    <circle cx="12" cy="12" r="9"/>
    <circle cx="12" cy="12" r="5" opacity=".55"/>
    <g class="scan-sweep" :class="{ 'is-scanning': scanning }">
      <path v-if="scanning" d="M12 12V3a9 9 0 0 1 6.36 2.64Z" fill="currentColor" stroke="none" opacity=".24"/>
      <path d="M12 12l6.36-6.36"/>
    </g>
    <circle cx="12" cy="12" r="1" fill="currentColor" stroke="none"/>
  </g>
  <path v-else :d="paths[name]||paths.info"/>
</svg></template>
<style scoped>
.scan-sweep {
  transform-box: view-box;
  transform-origin: center;
}

.scan-sweep.is-scanning {
  animation: radar-sweep 1.55s linear infinite;
}

@keyframes radar-sweep {
  to { transform: rotate(360deg); }
}
</style>
