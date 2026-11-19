<script setup lang="ts">
import{ref,onMounted}from'vue';import{api}from'../services/api';import{isLicenseStatus,type LicenseStatus}from'../services/plus';import Icon from'./Icon.vue';
const status=ref<LicenseStatus>();onMounted(async()=>{try{const v=await api('/engine/v1/license');if(isLicenseStatus(v))status.value=v}catch{}});
</script>
<template><section class="panel"><div class="panel-heading"><div class="section-icon-title"><span class="summary-icon"><Icon name="shield"/></span><div><h2>授权与购买</h2><p>{{status?status.activated?'Air2Mi Plus 已激活':'激活 Plus，解锁更多 AirPlay 2 接收实例':'查看授权状态、设备码和购买入口'}}</p></div></div><span class="badge" :class="status?.activated?'teal':'neutral'">{{status?status.activated?'已激活':'未激活':'待查询'}}</span></div><RouterLink class="button secondary" to="/plus"><Icon name="shield" :size="17"/>管理 Air2Mi Plus<Icon name="chevron" :size="15"/></RouterLink></section></template>
