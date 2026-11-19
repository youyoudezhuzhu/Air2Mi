<script setup lang="ts">
import{ref,watch,nextTick,onMounted}from'vue';import Icon from'./Icon.vue';
const props=defineProps<{open:boolean;title:string;wide?:boolean}>();const emit=defineEmits<{close:[]}>();const dialog=ref<HTMLDialogElement>();
async function sync(){await nextTick();if(props.open&&!dialog.value?.open)dialog.value?.showModal();if(!props.open&&dialog.value?.open)dialog.value.close()}
watch(()=>props.open,sync);onMounted(sync);
</script>
<template><dialog ref="dialog" :aria-label="title" :class="{wide}" @cancel.prevent="emit('close')" @click="event=>{if(event.target===dialog)emit('close')}"><div class="dialog-heading"><h2>{{title}}</h2><button type="button" class="icon-button" aria-label="关闭" @click="emit('close')"><Icon name="close" /></button></div><slot /></dialog></template>
