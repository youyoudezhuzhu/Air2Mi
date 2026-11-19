<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { api, ApiError, copyText } from '../services/api';
import { confirmAction } from '../services/confirm';
import { displayDeviceCode } from '../services/device-display';
import { useEngine } from '../stores/engine';
import {
    isLicenseStatus, isPlusOffer, isCDK, normalizedCDK, purchaseURL,
    offerPrice, licenseMessage, type LicenseStatus, type PlusOffer,
} from '../services/plus';
import Modal from '../components/Modal.vue';

const engine = useEngine();
const license = ref<LicenseStatus>();
const offer = ref<PlusOffer>();
const available = ref(false);
const loading = ref(true);
const offerLoading = ref(false);
const busy = ref(false);
const error = ref('');
const notice = ref('');
const offerError = ref('');
const code = ref('');
const codeDialog = ref(false);
const codeError = ref('');
const success = ref('');
const clock = ref(Date.now());
const retryAt = ref(0);
const offerAt = ref(0);
const controller = new AbortController();
let timer: ReturnType<typeof setInterval> | undefined;

const deviceCode = computed(() => license.value ? displayDeviceCode(license.value.device_id) : '');
const retrySeconds = computed(() => Math.max(0, Math.ceil((retryAt.value - clock.value) / 1000)));
const locked = computed(() => busy.value || loading.value || retrySeconds.value > 0);
const freshOffer = computed(() => !!offerAt.value && clock.value - offerAt.value < 60000 && !offerError.value);
const purchase = computed(() => purchaseURL(offer.value, freshOffer.value));
const price = computed(() => offerPrice(offer.value?.price_minor, offer.value?.currency));
const originalPrice = computed(() => offer.value?.status === 'discount'
    ? offerPrice(offer.value.original_price_minor, offer.value.currency) : '');
const status = computed(() => {
    if (loading.value) return '正在检查授权';
    if (!available.value) return engine.snapshot?.engine_running ? '暂时无法获取' : '内核未运行';
    return license.value?.activated ? '已永久激活' : '免费版';
});

function errorMessage(cause: unknown) {
    return licenseMessage(cause instanceof ApiError ? cause.code : 'REQUEST_FAILED');
}

async function refresh() {
    loading.value = true;
    error.value = '';
    try {
        const value = await api('/engine/v1/license', 'GET', undefined, controller.signal);
        if (!isLicenseStatus(value)) throw new ApiError('INVALID_LICENSE_RESPONSE', 502);
        license.value = value;
        available.value = true;
    } catch {
        if (!controller.signal.aborted) {
            available.value = false;
            error.value = engine.snapshot?.engine_running ? '暂时无法获取授权状态，请重试。' : '请先启动内核。';
        }
    } finally {
        loading.value = false;
    }
}

async function loadOffer() {
    if (offerLoading.value) return;
    offerLoading.value = true;
    offerError.value = '';
    try {
        const value = await api('/plus/offers', 'GET', undefined, controller.signal);
        if (!isPlusOffer(value)) throw new ApiError('OFFER_INVALID_RESPONSE', 502);
        offer.value = value;
        offerAt.value = Date.now();
    } catch (cause) {
        if (!controller.signal.aborted) {
            offerError.value = errorMessage(cause);
            offerAt.value = 0;
        }
    } finally {
        offerLoading.value = false;
    }
}

async function requestActivation() {
    if (locked.value || !available.value) return;
    if (license.value?.activated && !await confirmAction('重复激活确认',
        '当前设备已激活 Air2Mi Plus。重复输入激活码或恢复授权不会叠加权益，且可能产生绑定冲突。确认仍要继续激活吗？', '继续输入激活码')) return;
    codeError.value = '';
    codeDialog.value = true;
}

async function requestPurchase(event: MouseEvent) {
    if (!license.value?.activated) return;
    event.preventDefault();
    const url = purchase.value;
    if (url && await confirmAction('重复购买确认',
        '当前设备已激活 Air2Mi Plus。重复购买不会叠加权益，且可能产生额外费用。确认仍要继续购买吗？', '继续购买')) {
        window.open(url, '_blank', 'noopener,noreferrer');
    }
}

function closeCode() {
    codeDialog.value = false;
    code.value = '';
    codeError.value = '';
}

async function pasteCode() {
    try {
        code.value = (await navigator.clipboard.readText()).slice(0, 80);
        codeError.value = '';
    } catch {
        codeError.value = '无法读取剪贴板，请手动粘贴。';
    }
}

async function submitCode() {
    if (!isCDK(code.value)) {
        codeError.value = '激活码格式不正确，请检查后重新输入。';
        return;
    }
    const submitted = normalizedCDK(code.value);
    closeCode();
    await action('activate', submitted);
}

async function requestClear() {
    if (locked.value || !license.value?.can_clear) return;
    if (await confirmAction('清除本机授权',
        '仅移除此设备保存的凭据，服务端权益不变。当前推送将停止，之后可重新恢复授权。', '清除凭据', '保留授权')) {
        await action('clear');
    }
}

async function action(name: 'activate' | 'restore' | 'clear', licenseCode?: string) {
    if (locked.value || !available.value) return;
    busy.value = true;
    error.value = '';
    notice.value = '';
    try {
        const value = await api('/engine/v1/license/' + name, 'POST',
            name === 'activate' ? { license_code: licenseCode } : {}, controller.signal);
        if (!isLicenseStatus(value)) throw new ApiError('INVALID_LICENSE_RESPONSE', 502);
        license.value = value;
        available.value = true;
        if (name === 'clear') notice.value = '本机授权凭据已清除。';
        else if (value.activated) {
            if (engine.snapshot?.state?.kernel?.state === 'stopped') {
                await api('/lifecycle/restart', 'POST', {}, controller.signal);
            }
            success.value = name === 'activate' ? '激活成功' : '授权已恢复';
        } else notice.value = '此设备尚未激活。';
        await engine.refresh();
    } catch (cause) {
        if (controller.signal.aborted) return;
        const message = errorMessage(cause);
        if (cause instanceof ApiError && cause.retryAfter) retryAt.value = Date.now() + cause.retryAfter * 1000;
        await refresh();
        error.value = message;
        await engine.refresh();
    } finally {
        busy.value = false;
    }
}

async function copyDevice() {
    if (!deviceCode.value) return;
    try {
        await copyText(deviceCode.value);
        notice.value = '设备识别码已复制。';
        error.value = '';
    } catch {
        error.value = '无法复制，请手动选择设备识别码。';
    }
}

function visibility() {
    if (document.hidden) return;
    if (!busy.value && !loading.value) void refresh();
    if (!freshOffer.value) void loadOffer();
}

watch(() => engine.snapshot?.engine_running, (running, previous) => {
    if (running && !previous && !busy.value && !loading.value) void refresh();
    else if (!running) available.value = false;
});
onMounted(() => {
    void refresh();
    void loadOffer();
    timer = setInterval(() => { clock.value = Date.now(); }, 1000);
    document.addEventListener('visibilitychange', visibility);
});
onBeforeUnmount(() => {
    controller.abort();
    if (timer) clearInterval(timer);
    document.removeEventListener('visibilitychange', visibility);
    code.value = '';
});
</script>

<template>
    <div class="plus-page">
        <header class="page-header"><h1>Air2Mi Plus</h1></header>

        <section class="panel plus-purchase" aria-label="商品信息">
            <p v-if="offerLoading" role="status">正在获取购买信息…</p>
            <template v-else-if="offerError">
                <h2>暂时无法获取购买信息</h2>
                <p class="field-help">{{ offerError }}</p>
                <button class="text" @click="loadOffer">重试</button>
            </template>
            <template v-else-if="offer">
                <h2>{{ offer.title }}</h2>
                <p v-if="offer.description" class="field-help">{{ offer.description }}</p>
                <div class="plus-price">
                    <strong>{{ price || '—' }}</strong>
                    <del v-if="originalPrice">{{ originalPrice }}</del>
                    <span v-if="offer.badge" class="badge teal">{{ offer.badge }}</span>
                </div>
                <p v-if="!freshOffer" class="field-help">价格信息来自缓存，可重试刷新</p>
                <ul v-if="offer.features?.length" class="plus-feature-list">
                    <li v-for="feature in offer.features" :key="feature.id">
                        <span>{{ feature.title }}</span>
                        <small v-if="feature.description">{{ feature.description }}</small>
                    </li>
                </ul>
                <a v-if="purchase" class="button plus-buy" :href="purchase" target="_blank"
                    rel="noopener noreferrer" @click="requestPurchase">{{ offer.button_text }}</a>
                <p v-else class="field-help">当前暂不可购买</p>
                <button class="text" @click="loadOffer">刷新购买信息</button>
            </template>
        </section>

        <section v-if="available && license?.activated" class="panel plus-activated" aria-label="已激活状态">
            <h2>已永久激活</h2>
            <p>多 AirPlay 2 实例数量限制已解除，其他功能不变。</p>
            <p v-if="license.license_id" class="field-help mono"><span>授权编号</span> {{ license.license_id }}</p>
            <button v-if="license.can_clear" class="text" :disabled="locked" @click="requestClear">清除本机授权凭据</button>
        </section>

        <section class="panel" aria-label="设备识别码">
            <h2>当前设备识别码</h2>
            <p class="mono plus-device-code">{{ deviceCode || 'AVD1-UNAVAILABLE' }}</p>
            <div class="actions">
                <button class="text" :disabled="!deviceCode" @click="copyDevice">复制设备识别码</button>
                <button class="text" :disabled="locked" @click="refresh">刷新</button>
            </div>
        </section>

        <section class="panel" aria-label="激活详情">
            <h2>激活详情</h2>
            <p><span>状态</span> {{ status }}</p>
            <p v-if="error" class="notice error" role="alert">{{ error }}</p>
            <p v-if="notice" class="field-help" role="status">{{ notice }}</p>
            <p v-if="retrySeconds" class="field-help" role="status">请在 {{ retrySeconds }} 秒后重试。</p>
            <div class="actions plus-activation-actions">
                <button :disabled="locked || !available" @click="requestActivation">输入激活码</button>
                <button class="text" :disabled="locked || !available" @click="action('restore')">
                    {{ busy ? '检查中…' : '恢复授权' }}
                </button>
            </div>
        </section>
    </div>

    <Modal :open="codeDialog" title="输入激活码" @close="closeCode">
        <form class="plus-cdk" @submit.prevent="submitCode">
            <label for="plus-cdk">CDK</label>
            <input id="plus-cdk" v-model="code" type="password" autocomplete="off" spellcheck="false"
                maxlength="80" autofocus />
            <div class="actions">
                <button type="button" class="text" @click="pasteCode">粘贴</button>
                <button type="button" class="text" @click="code = ''">清除</button>
            </div>
            <p class="field-help">设备识别码 {{ deviceCode }}</p>
            <p class="field-help plus-code-format">CDK 格式 A2M-XXXXX-XXXXX-XXXXX-XXXXX-XXXXX-XXXXX-XXXXX-XXXXX</p>
            <p v-if="codeError" class="notice error" role="alert">{{ codeError }}</p>
            <div class="dialog-actions">
                <button type="button" class="secondary" @click="closeCode">关闭</button>
                <button :disabled="busy">{{ busy ? '激活中…' : '激活' }}</button>
            </div>
        </form>
    </Modal>

    <Modal :open="!!success" title="Plus 授权成功" @close="success = ''">
        <div class="plus-success-dialog">
            <h2>{{ success }}</h2>
            <p>已永久激活，多 AirPlay 2 实例数量限制已解除</p>
            <p v-if="license?.license_id" class="field-help mono">授权编号 {{ license.license_id }}</p>
            <button @click="success = ''">完成</button>
        </div>
    </Modal>
</template>
