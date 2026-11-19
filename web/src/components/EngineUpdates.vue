<script setup lang="ts">
import {computed,onBeforeUnmount,ref} from 'vue';
import {api,downloadAppUpdate} from '../services/api';
import {useEngine} from '../stores/engine';
import {confirmAction} from '../services/confirm';
import Icon from './Icon.vue';
const props=withDefaults(defineProps<{component?:'app'|'engine'}>(),{component:'engine'});
type Status={configured:boolean;component?:string;phase:string;available:boolean;reason:string;current_version?:string;app_version?:string;release_id?:string;package_version?:string;kernel_version?:string;kind?:string;downloaded?:number;total?:number};
const engine=useEngine(),status=ref<Status>(),busy=ref(false),error=ref('');
let timer:ReturnType<typeof setTimeout>|undefined,closed=false;
const title=computed(()=>props.component==='app'?'应用更新':'内核安装与更新');
const path=computed(()=>props.component==='app'?{check:'/app-updates',status:'/app-update-status',begin:'/app-update-online'}:{check:'/updates',status:'/update-status',begin:'/update-online'});
const current=computed(()=>props.component==='app'?engine.snapshot?.app_version:(engine.snapshot?.engine_manifest?.package_version||engine.snapshot?.engine_manifest?.kernel_version));
const next=computed(()=>props.component==='app'?status.value?.app_version:status.value?.package_version);
const active=computed(()=>['downloading','installing'].includes(status.value?.phase||''));
const progress=computed(()=>status.value?.total?Math.min(100,Math.floor(100*(status.value.downloaded||0)/status.value.total)):0);
async function poll(){try{status.value=await api(path.value.status);if(status.value?.phase==='installed')await engine.refresh()}catch(e){error.value=String(e)}if(!closed&&active.value)timer=setTimeout(poll,document.hidden?3000:800)}
async function check(){busy.value=true;error.value='';try{status.value=await api(path.value.check);if(active.value)void poll()}catch(e){error.value=String(e)}finally{busy.value=false}}
async function download(){if(!status.value?.release_id||!await confirmAction(title.value,status.value.kind==='engine_components'?'下载并验证内核组件。正在使用的内核将在重启应用后切换。':status.value.kind==='fpk'?'下载并校验 FPK，再通过飞牛应用中心覆盖安装。':status.value.kind==='apk'?'下载后由 Android 系统确认覆盖安装。原生内核更新也需要安装 APK，配置会保留。':'更新前会停止当前投放，健康检查失败会回滚程序文件，保留身份和关联。'))return;
 busy.value=true;error.value='';try{status.value=await api(path.value.begin,'POST',{release_id:status.value.release_id});void poll()}catch(e){error.value=String(e)}finally{busy.value=false}}
async function install(){if(status.value?.kind==='fpk'){
 busy.value=true;error.value='';try{await downloadAppUpdate('Air2Mi-'+status.value.app_version+'.fpk')}catch(e){error.value=String(e)}finally{busy.value=false}
 }else{const bridge=(window as any).Air2MiNative;if(bridge?.installUpdate)bridge.installUpdate(props.component);else error.value='请在安卓应用中确认系统安装'}}
onBeforeUnmount(()=>{closed=true;if(timer)clearTimeout(timer)});
</script>
<template><section class="panel online-updates" :data-update-component="component"><div class="panel-heading"><div class="section-icon-title"><span class="summary-icon neutral"><Icon name="download"/></span><div><h2>{{title}}</h2><p>当前版本 {{current||'未安装'}}</p></div></div><button class="secondary compact" :disabled="busy||active||(component==='engine'&&engine.busy)" @click="check">{{busy?'处理中…':'在线检查更新'}}</button></div><p v-if="error" class="notice error" role="alert">{{error}}</p><p role="status">{{status?.reason||'检查后显示可下载的版本。'}}</p><div v-if="next" class="version-row"><div><small>可用版本</small><b>{{next}}</b></div><div v-if="component==='engine'&&status?.kernel_version"><small>Engine</small><b>{{status.kernel_version}}</b></div></div><div v-if="active" class="update-progress"><progress :value="progress" max="100" :aria-label="title+'下载进度'"/><span>{{status?.phase==='installing'?'正在安装':progress+'%'}}</span></div><div class="actions"><button v-if="status?.available" :disabled="busy||(component==='engine'&&engine.busy)" @click="download"><Icon name="download" :size="17"/>{{['engine_zip','engine_components'].includes(status.kind||'')?'下载并安装内核':'下载'+(component==='app'?'应用':'内核更新 APK')}}</button><button v-if="status?.phase==='install_ready'" :disabled="busy" @click="install"><Icon name="shield" :size="17"/>{{status.kind==='fpk'?'保存 FPK，前往应用中心安装':'确认系统安装'}}</button></div><div v-if="component==='engine' && $slots.offline" class="offline-import">
  <slot name="offline" :updating="busy||active"/>
</div></section></template>
<style scoped>.update-progress{display:flex;align-items:center;gap:12px;margin:16px 0}.update-progress progress{flex:1;min-width:0;accent-color:var(--accent,#0078d4)}.online-updates .actions:empty{display:none}.offline-import{border-top:1px solid var(--line);margin-top:16px;padding-top:16px}</style>
