#!/usr/bin/env python3
"""Convert Air2Mi display codes for trusted support. Codes are sensitive."""
import argparse
import hashlib
import re


def convert(code, encode=False):
    source, target = ('AMD1', 'AVD1') if encode else ('AVD1', 'AMD1')
    value = code.strip().upper()
    if not re.fullmatch(source + r'-(?:[A-F0-9]{4}-){7}[A-F0-9]{4}', value):
        raise ValueError('invalid device code format')
    data = bytes.fromhex(value[5:].replace('-', ''))
    mask = hashlib.sha256(b'air2mi.device.display.v1').digest()[:16]
    result = bytes(a ^ b for a, b in zip(data, mask)).hex().upper()
    return target + '-' + '-'.join(result[i:i + 4] for i in range(0, 32, 4))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('code', help='AVD1 display code; use --encode for AMD1')
    parser.add_argument('--encode', action='store_true')
    args = parser.parse_args()
    try:
        print(convert(args.code, encode=args.encode))
    except ValueError as error:
        parser.error(str(error))
