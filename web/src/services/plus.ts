export interface LicenseStatus {
    activated: boolean;
    can_clear?: boolean;
    max_instances: number | null;
    max_outputs: number | null;
    device_id: string;
    reason: string;
    issued_at?: number;
    license_id?: string;
}
export interface PlusOffer {
    schema_version: 1;
    status?: string;
    app_id: string;
    product_id: string;
    enabled: boolean;
    purchasable: boolean;
    purchase_url?: string;
    price_minor?: number;
    original_price_minor?: number | null;
    currency?: string;
    title?: string;
    description?: string;
    button_text?: string;
    badge?: string;
    features?: {
        id: string;
        title: string;
        description?: string | null;
    }[];
    updated_at?: string;
}
export function isPlusOffer(value: unknown): value is PlusOffer {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
    const offer = value as Record<string, unknown>;
    if (offer.schema_version !== 1 || offer.app_id !== 'air2mi' ||
        (offer.product_id !== 'plus' && offer.product_id !== 'air2mi_plus') ||
        typeof offer.enabled !== 'boolean' || typeof offer.purchasable !== 'boolean' ||
        typeof offer.currency !== 'string' || !/^[A-Z]{3}$/.test(offer.currency)) return false;
    for (const key of ['title', 'description', 'button_text', 'purchase_url', 'badge', 'updated_at']) {
        const field = offer[key];
        if (field !== undefined && field !== null && typeof field !== 'string') return false;
    }
    for (const key of ['price_minor', 'original_price_minor']) {
        const field = offer[key];
        if (field !== undefined && field !== null &&
            (typeof field !== 'number' || !Number.isSafeInteger(field) || field < 0)) return false;
    }
    if (offer.features !== undefined && offer.features !== null) {
        if (!Array.isArray(offer.features)) return false;
        for (const feature of offer.features) {
            if (!feature || typeof feature !== 'object' ||
                typeof feature.id !== 'string' || typeof feature.title !== 'string' ||
                (feature.description !== undefined && feature.description !== null &&
                    typeof feature.description !== 'string')) return false;
        }
    }
    return true;
}
export function isLicenseStatus(value: unknown): value is LicenseStatus { const v = value as LicenseStatus; return !!v && typeof v.activated === 'boolean' && typeof v.device_id === 'string' && /^(?:AMD1|AVD1)-[A-F0-9-]+$/.test(v.device_id) && (v.max_instances === null || Number.isSafeInteger(v.max_instances)); }
export function normalizedCDK(value: string) { return value.trim().toUpperCase(); }
export function isCDK(value: string) { return /^A2M-(?:[A-F0-9]{5}-){7}[A-F0-9]{5}$/.test(normalizedCDK(value)); }
export function purchaseURL(offer: PlusOffer | undefined, fresh = true) { if (!fresh || !isPlusOffer(offer) || offer.enabled !== true || offer.purchasable !== true || !offer.button_text?.trim())
    return ''; try {
    const url = new URL(offer.purchase_url || '');
    return url.protocol === 'https:' && !url.username && !url.password ? url.href : '';
}
catch {
    return '';
} }
export function offerPrice(minor: unknown, currency: unknown) { if (typeof minor !== 'number' || !Number.isSafeInteger(minor) || minor < 0 || typeof currency !== 'string' || !/^[A-Z]{3}$/.test(currency))
    return ''; try {
    return new Intl.NumberFormat('zh-CN', { style: 'currency', currency }).format(minor / 100);
}
catch {
    return '';
} }
const errors: Record<string, string> = { INVALID_LICENSE_CODE: '激活码格式不正确，请检查后重新输入。', LICENSE_APP_MISMATCH: '这张激活码不适用于 Air2Mi。', LICENSE_REVOKED: '该授权已被撤销，内核已锁定多实例功能。', LICENSE_NOT_FOUND: '未找到对应授权，请检查激活码或稍后重试。', DEVICE_ID_MISMATCH: '激活码与本机设备不匹配，请联系销售渠道处理。', LICENSE_BOUND_TO_OTHER_DEVICE: '该激活码已绑定其他设备，请联系销售渠道解绑。', INVALID_DEVICE_ID: '内核设备码异常，请查看诊断或更新内核。', NETWORK_OR_INVALID_RESPONSE: '暂时无法连接授权服务。已有有效授权会保留。', SERVICE_UNAVAILABLE: '授权服务暂时维护中，请稍后再试。', INTERNAL_ERROR: '授权服务暂时发生错误，请稍后重试。', RATE_LIMITED: '请求过于频繁，请等待后再试。', OFFER_NOT_CONFIGURED: '购买入口暂未上架。已有激活码仍可直接激活。', OFFER_UNAVAILABLE: '暂时无法获取购买信息，请稍后刷新。', OFFER_INVALID_RESPONSE: '购买信息格式异常，购买入口暂不可用。' };
export function licenseMessage(code: string) { return errors[code] || '操作未完成，请稍后重试；可在诊断中查看错误码。'; }
