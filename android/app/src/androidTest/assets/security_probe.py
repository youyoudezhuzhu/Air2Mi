"""Run only in the isolated instrumentation process. No real license is used."""
def run():
    import base64,json,os,shutil,tempfile,time
    from pathlib import Path
    from unittest.mock import patch
    from types import SimpleNamespace
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
    from cryptography.hazmat.bindings._rust import openssl
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM
    import engine_runtime.licensing as public
    import engine_runtime._license_guard as guard
    import _air2mi_integrity as integrity
    from a2m_kernel.registry import EngineRegistry
    from a2m_kernel.model import InputInstance
    def reject(action):
        try:action()
        except ValueError:return
        raise AssertionError('unsigned authorization was accepted')
    result={}
    with tempfile.TemporaryDirectory(prefix='a2m-security-') as tmp:
        directory=Path(tmp)
        guard.enforce(directory,2,['a','b'])
        reject(lambda:guard.enforce(directory,3,[]))
        reject(lambda:guard.enforce(directory,1,['a','b','c']))
        result['free_limits']=True
        try:guard._Verifier.require=lambda *args:None
        except (TypeError,AttributeError):pass
        else:raise AssertionError('mutable native verifier type')
        result['immutable_verifier']=True
        original=guard.enforce
        with patch.object(public,'verify_credential',return_value={'activated':True}),patch.object(public.Entitlements,'require',return_value=None),patch.object(public,'FREE_LIMIT',999),patch.object(guard,'enforce',lambda *args:None):
            registry=EngineRegistry(directory/'registry.json')
            registry.add_input(InputInstance('a','a'));registry.add_input(InputInstance('b','b'))
            reject(lambda:registry.add_input(InputInstance('c','c')))
        result['c_import_registry_gate']=True
        encode=lambda value:base64.urlsafe_b64encode(value).decode().rstrip('=')
        claim=dict(version=1,license_id='unsigned-security-test',product_id='air2mi_plus',device_id=public.device_id(),license_type='permanent',issued_at=int(time.time()))
        envelope=dict(format='air2mi-license-v1',key_id='air2mi-v1',payload=encode(json.dumps(claim).encode()),signature=encode(bytes(64)))
        store=public.Entitlements(directory);nonce=os.urandom(12)
        store.path.write_bytes(nonce+AESGCM(store._key(create=True)).encrypt(nonce,json.dumps(envelope).encode(),b'air2mi-license-v1'))
        assert not hasattr(guard,'_pinned_signature_verify') and not hasattr(guard,'_pinned_claim_loads')
        with patch.object(guard,'_pinned_signature_verify',lambda *args:None,create=True),patch.object(guard,'_pinned_claim_loads',lambda *args:claim,create=True),patch.object(Ed25519PublicKey,'from_public_bytes',return_value=SimpleNamespace(verify=lambda *args:None)),patch.object(openssl.ed25519,'from_public_bytes',return_value=SimpleNamespace(verify=lambda *args:None)),patch('json.loads',return_value=envelope):
            reject(lambda:original(directory,3,[]))
        result['forged_encrypted_credential_rejected']=True
        result['crypto_and_parser_replacement_rejected']=True
        root=Path(integrity.__file__).resolve().parent
        # Every attempt uses a new path: the existing gate memoizes successful
        # initialization and is not a continuous integrity monitor.
        for kind in ('binary','shadow','local'):
            copy=directory/kind;shutil.copytree(root,copy)
            if kind=='binary':
                file=next((copy/'engine_runtime').glob('_license_guard*.so'));file.write_bytes(file.read_bytes()+b'changed')
                (copy/'BUILD.json').write_text('{"sha256":"rewritten"}')
            else:
                file=copy/('local/shadow.py' if kind=='local' else 'shadow.py');file.parent.mkdir(exist_ok=True);file.write_text('pass')
            try:integrity.validate(copy)
            except RuntimeError:pass
            else:raise AssertionError('modified module tree accepted: '+kind)
        result['native_tamper_and_shadow_rejected']=True
    result['real_cdk_used']=False
    return result
