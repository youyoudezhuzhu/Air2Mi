import { describe, it, expect } from 'vitest';
import { isLicenseStatus, isPlusOffer, isCDK, normalizedCDK, purchaseURL, offerPrice, licenseMessage, type PlusOffer } from '../plus';
const offer: PlusOffer = { schema_version: 1, app_id: 'air2mi', product_id: 'plus', enabled: true, purchasable: true, purchase_url: 'https://example.com/buy', button_text: '购买', price_minor: 1990, currency: 'CNY' };
describe('Plus metadata cannot grant authorization', () => {
    it('rejects unsupported schemas and malformed fields without crashing purchase rendering', () => {
        expect(isPlusOffer(offer)).toBe(true);
        for (const fields of [
            { schema_version: 2 }, { schema_version: undefined }, { currency: 'invalid' },
            { button_text: 123 }, { title: {} }, { enabled: 'true' },
            { price_minor: -1 }, { price_minor: 1.5 }, { features: {} },
            { features: [{ id: 'permanent', title: {} }] },
        ]) {
            const malformed = { ...offer, ...fields };
            expect(isPlusOffer(malformed)).toBe(false);
            expect(purchaseURL(malformed as PlusOffer)).toBe('');
        }
    });
    it('requires actual kernel status rather than service active flags', () => { expect(isLicenseStatus({ status: 'active', ok: true })).toBe(false); expect(isLicenseStatus({ activated: false, device_id: 'AMD1-1234-ABCD', max_instances: 2, max_outputs: 2 })).toBe(true); });
    it('normalizes and validates a complete CDK without storing it', () => { const value = 'A2M-' + Array(8).fill('ABCDE').join('-'); expect(isCDK('  ' + value.toLowerCase() + ' ')).toBe(true); expect(normalizedCDK(' ' + value + ' ')).toBe(value); expect(isCDK(value.replace('A2M-', 'PM-'))).toBe(false); expect(isCDK('A2M-ABCDE')).toBe(false); });
    it('requires explicit true flags and fresh product data for purchasing', () => { expect(purchaseURL(offer)).toBe(offer.purchase_url); expect(purchaseURL(offer, false)).toBe(''); for (const change of [{ enabled: false }, { purchasable: false }, { app_id: 'pushmi' }, { button_text: '' }, { enabled: 'true' }])
        expect(purchaseURL({ ...offer, ...change } as PlusOffer)).toBe(''); });
    it('rejects insecure and credential-bearing purchase URLs', () => { for (const purchase_url of ['http://example.com', 'javascript:alert(1)', 'file:///tmp/payment', 'https://user:secret@example.com', 'not a url'])
        expect(purchaseURL({ ...offer, purchase_url })).toBe(''); });
    it('formats only actual valid monetary values without fabricated prices', () => { expect(offerPrice(1990, 'CNY')).toContain('19.90'); for (const value of [undefined, null, '1990', -1, Infinity, 1.5])
        expect(offerPrice(value, 'CNY')).toBe(''); expect(offerPrice(1990, 'invalid')).toBe(''); });
    it('distinguishes unconfigured offers from temporary errors and revocation', () => { expect(licenseMessage('OFFER_NOT_CONFIGURED')).toContain('暂未上架'); expect(licenseMessage('NETWORK_OR_INVALID_RESPONSE')).toContain('保留'); expect(licenseMessage('LICENSE_REVOKED')).toContain('撤销'); });
});
