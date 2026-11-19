<script setup lang="ts">
import CompactFlowMap from './CompactFlowMap.vue';
import{computed}from'vue';import{useEngine}from'../stores/engine';import{outputFor,outputLabel,role}from'../types/engine';import Icon from'./Icon.vue';
const e=useEngine();const r=computed(()=>e.snapshot?.state?.river);const primary=computed(()=>e.inputs.find(i=>i.id===r.value?.producer));
const height=computed(()=>Math.max(254, Math.max(e.inputs.length, e.speakers.length) * 84 + 56));const middle=computed(()=>height.value/2);
const y=(index:number,total:number)=>middle.value-(total-1)*42+index*84;
const inputPhase=(id:string)=>r.value?.producer===id&&!r.value?.paused?'flow':r.value?.references?.includes(id)?'standby':e.snapshot?.state?.instances?.[id]?.selected?'selected':'configured';
const outputPhase=(id:string)=>{const output=outputFor(e.snapshot,id);return output?.playback_state==='playing'?'flow':output?.selected?'selected':output?'configured':'unknown'};
const inputNodes = computed(() => e.inputs.map(input => ({
  id: input.id, name: input.name,
  phase: inputPhase(input.id),
  status: input.enabled ? role(input.id, r.value) : '已停用',
})))
const speakerNodes = computed(() => e.speakers.map(speaker => ({
  id: speaker.id, name: speaker.name, phase: outputPhase(speaker.id),
  status: outputLabel(outputFor(e.snapshot, speaker.id)),
})))
</script>
<template><div class="topology-container"><svg class="topology-wide" :viewBox="`0 0 1200 ${height}`" role="img" aria-label="AirPlay 接收器经 River 到妙播音箱的实时音频拓扑"><defs><marker id="top-arrow" markerWidth="7" markerHeight="7" refX="5" refY="3" orient="auto"><path d="M1 1 5 3 1 5" fill="none" stroke="context-stroke" stroke-width="1.3" /></marker></defs><text x="30" y="28" class="svg-heading">AIRPLAY 接收器</text><text x="920" y="28" class="svg-heading">妙播音箱</text><path v-for="(input,index) in e.inputs" :key="'in-'+input.id" :d="`M 300 ${y(index,e.inputs.length)} C 380 ${y(index,e.inputs.length)} 385 ${middle} 480 ${middle}`" class="stream-edge" :class="inputPhase(input.id)" marker-end="url(#top-arrow)"/><path v-for="(speaker,index) in e.speakers" :key="'out-'+speaker.id" :d="`M 720 ${middle} C 810 ${middle} 820 ${y(index,e.speakers.length)} 900 ${y(index,e.speakers.length)}`" class="stream-edge" :class="outputPhase(speaker.id)" marker-end="url(#top-arrow)"/>
<g v-for="(input,index) in e.inputs" :key="input.id" :transform="`translate(24,${y(index,e.inputs.length)-33})`" class="svg-node" :class="inputPhase(input.id)"><title>{{input.name}}</title><rect width="276" height="66" rx="12"/><rect x="14" y="15" width="36" height="36" rx="9" class="svg-icon-bg"/><Icon name="inputs" x="21" y="22" :size="22"/><foreignObject x="65" y="7" width="178" height="26"><div class="diagram-name" :title="input.name">{{input.name}}</div></foreignObject><text x="65" y="47" class="svg-meta">{{input.enabled?role(input.id,r):'已停用'}}</text><circle cx="253" cy="33" r="4" class="svg-dot"/></g>
<g :transform="`translate(480,${middle-87})`" class="svg-hub" :class="{'is-playing':!!r?.producer&&!r.paused}"><rect width="240" height="174" rx="22"/><circle cx="120" cy="45" r="25" class="hub-icon-circle"/><Icon name="river" x="105" y="30" :size="30"/><text x="120" y="91" text-anchor="middle" class="svg-hub-title">River Engine</text><foreignObject x="10" y="96" width="220" height="26"><div class="diagram-name" :title="primary?.name">{{primary?.name || '等待音频输入'}}</div></foreignObject><text x="120" y="145" text-anchor="middle" class="svg-meta">{{r?.paused?'已暂停':primary?'一个主源，共享输出':'由内核管理音频流'}}</text></g>
<g v-for="(speaker,index) in e.speakers" :key="speaker.id" :transform="`translate(900,${y(index,e.speakers.length)-33})`" class="svg-node" :class="outputPhase(speaker.id)"><title>{{speaker.name}}</title><rect width="276" height="66" rx="12"/><rect x="14" y="15" width="36" height="36" rx="9" class="svg-icon-bg"/><Icon name="speaker" x="21" y="22" :size="22"/><foreignObject x="65" y="7" width="178" height="26"><div class="diagram-name" :title="speaker.name">{{speaker.name}}</div></foreignObject><text x="65" y="47" class="svg-meta">{{outputLabel(outputFor(e.snapshot,speaker.id))}}</text><circle cx="253" cy="33" r="4" class="svg-dot"/></g><text v-if="!e.inputs.length" x="165" :y="middle" text-anchor="middle" class="svg-meta">添加一个 AirPlay 接收器</text><text v-if="!e.speakers.length" x="1035" :y="middle" text-anchor="middle" class="svg-meta">搜索你的妙播音箱</text></svg>
<CompactFlowMap :inputs="inputNodes" :speakers="speakerNodes"/>
<div class="topology-legend"><span><i class="legend-flow"></i>正在传输</span><span><i class="legend-ready"></i>备用 / 已选中</span><span><i class="legend-config"></i>配置关联</span><span><i class="legend-unknown"></i>状态未知</span></div></div></template>
