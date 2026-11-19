// SHA-256("air2mi.device.display.v1")[:16], matching the licensing handoff.
const displayMask = '3A69EA0374CC555A6EC103FCDF39B012';

export function displayDeviceCode(deviceId: string): string {
    const normalized = deviceId.trim().toUpperCase();
    if (/^AVD1-(?:[A-F0-9]{4}-){7}[A-F0-9]{4}$/.test(normalized)) return normalized;
    if (!/^AMD1-(?:[A-F0-9]{4}-){7}[A-F0-9]{4}$/.test(normalized)) return '';
    const realHex = normalized.slice(5).replaceAll('-', '');
    const bytes: string[] = [];
    for (let offset = 0; offset < realHex.length; offset += 2) {
        const value = Number.parseInt(realHex.slice(offset, offset + 2), 16) ^
            Number.parseInt(displayMask.slice(offset, offset + 2), 16);
        bytes.push(value.toString(16).padStart(2, '0').toUpperCase());
    }
    return 'AVD1-' + bytes.join('').match(/.{4}/g)!.join('-');
}
