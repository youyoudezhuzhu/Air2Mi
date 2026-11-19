import { describe, expect, it } from 'vitest';
import { displayDeviceCode } from '../device-display';

describe('Air2Mi device display code', () => {
    it('matches the handoff vector and is stable across repeated reads', () => {
        const real = 'AMD1-0000-0000-0000-0000-0000-0000-0000-0000';
        const expected = 'AVD1-3A69-EA03-74CC-555A-6EC1-03FC-DF39-B012';
        expect(displayDeviceCode(real)).toBe(expected);
        expect(displayDeviceCode(real)).toBe(expected);
        expect(displayDeviceCode(real.toLowerCase())).toBe(expected);
        expect(displayDeviceCode(expected)).toBe(expected);
    });

    it('never returns a real device id as a fallback for malformed input', () => {
        for (const value of ['', 'AMD1-UNAVAILABLE', 'AMD1-1234', 'PMD1-0000-0000-0000-0000-0000-0000-0000-0000']) {
            expect(displayDeviceCode(value)).toBe('');
        }
    });
});
