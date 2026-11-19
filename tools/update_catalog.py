#!/usr/bin/env python3
"""Sign an update catalog offline and verify it against the app trust anchor."""
import argparse,base64,json
from pathlib import Path
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey,Ed25519PublicKey
ROOT=Path(__file__).resolve().parents[1]
def b64(raw):return base64.urlsafe_b64encode(raw).decode().rstrip('=')
def decode(value):
 raw=base64.b64decode(value+'='*(-len(value)%4),altchars=b'-_',validate=True)
 if b64(raw)!=value:raise ValueError('Noncanonical base64url')
 return raw
def verify(path,component):
 envelope=json.loads(Path(path).read_bytes())
 if envelope['format']!='air2mi-update-v1' or envelope['key_id']!='air2mi-update-v1':raise ValueError('Wrong envelope')
 payload=decode(envelope['payload']);signature=decode(envelope['signature'])
 key=base64.b64decode((ROOT/'engine/development-trust.pub').read_text().strip())
 Ed25519PublicKey.from_public_bytes(key).verify(signature,payload);catalog=json.loads(payload)
 if catalog['schema_version']!=1 or catalog['app_id']!='air2mi' or catalog['component']!=component or catalog['channel']!='stable' or type(catalog['published']) is not bool:raise ValueError('Wrong catalog claims')
 return catalog
if __name__=='__main__':
 parser=argparse.ArgumentParser();sub=parser.add_subparsers(dest='action',required=True)
 sign=sub.add_parser('sign');sign.add_argument('--payload',type=Path,required=True);sign.add_argument('--key',type=Path,required=True);sign.add_argument('--output',type=Path,required=True)
 args=parser.parse_args()
 payload=args.payload.read_bytes()
 value=json.loads(payload)
 key=Ed25519PrivateKey.from_private_bytes(args.key.read_bytes())
 envelope=dict(format='air2mi-update-v1',key_id='air2mi-update-v1',payload=b64(payload),signature=b64(key.sign(payload)))
 args.output.write_text(json.dumps(envelope,indent=2)+'\n')
 verify(args.output,value['component'])
