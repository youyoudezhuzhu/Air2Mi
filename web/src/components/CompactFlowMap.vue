<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Icon from './Icon.vue'

type Node = { id: string; name: string; phase: string; status: string }
const props = defineProps<{ inputs: Node[]; speakers: Node[] }>()
const picked = ref<string>()
// Keep both sides on one vertical scale, even when only one side has devices.
const rowPitch = 36
const height = computed(() => Math.max(3, props.inputs.length, props.speakers.length) * rowPitch)

function bank(source: Node[], side: 'inputs' | 'speakers', label: string) {
  const middle = height.value / 2
  const x = side === 'inputs' ? 25 : 75
  const nodes = source.map((node, i) => {
    const y = middle + (i - (source.length - 1) / 2) * rowPitch
    const path = side === 'inputs'
      ? `M${x} ${y} C62 ${y} 70 ${middle} 100 ${middle}`
      : `M0 ${middle} C30 ${middle} 38 ${y} ${x} ${y}`
    return { ...node, key: side + '/' + node.id, y, path }
  })
  return { side, label, x, nodes }
}

const banks = computed(() => [
  bank(props.inputs, 'inputs', 'AirPlay 接收器'),
  bank(props.speakers, 'speakers', '妙播音箱'),
])
const selected = computed(() => banks.value
  .flatMap(group => group.nodes)
  .find(node => node.key === picked.value))
watch(selected, node => { if (!node) picked.value = undefined })

function toggle(key: string) {
  picked.value = picked.value === key ? undefined : key
}
</script>

<template>
  <div class="compact-flow">
    <figure class="dot-map" aria-label="接收器经 River 到音箱的音频流">
      <header class="map-labels"><h3 v-for="group in banks" :key="group.side">{{ group.label }}</h3></header>
      <div class="map-diagram" :style="{ height: height + 'px' }">
        <template v-for="group in banks" :key="group.side">
          <div v-if="group.side === 'speakers'" class="map-hub"><Icon name="river" :size="23"/>River Engine</div>
          <div class="dot-bank" :aria-label="group.label + '光点'" role="group">
            <span v-if="!group.nodes.length" class="bank-empty">{{ group.side === 'inputs' ? '暂无接收器' : '暂无音箱' }}</span>
            <svg aria-hidden="true" :viewBox="`0 0 100 ${height}`" preserveAspectRatio="none">
              <path v-for="node in group.nodes" :key="node.key" class="stream-edge" :class="node.phase"
                :d="node.path" vector-effect="non-scaling-stroke"/>
            </svg>
            <button v-for="node in group.nodes" :key="node.key" class="map-point" type="button"
              :style="{ left: group.x + '%', top: node.y + 'px' }"
              :aria-label="node.name + '，' + node.status" :aria-pressed="selected?.key === node.key"
              @click="toggle(node.key)"><i aria-hidden="true"/></button>
          </div>
        </template>
      </div>
      <figcaption aria-live="polite" :class="{ 'near-point': selected && height > 360 }"
        :style="selected && height > 360 ? { top: selected.y + 'px' } : undefined">
        <template v-if="selected"><strong>{{ selected.name }}</strong><span>{{ selected.status }}</span></template>
        <span v-else-if="inputs.length || speakers.length">轻点光点查看设备名称</span>
      </figcaption>
    </figure>
  </div>
</template>
