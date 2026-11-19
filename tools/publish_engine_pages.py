"""Build a Pages artifact from the three files attached to an Engine release."""
import base64
import hashlib
import json
import os
import shutil
import tarfile
from pathlib import Path
from zipfile import ZipFile

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat

def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()

def encode(value):
    return base64.urlsafe_b64encode(value).rstrip(b'=').decode()

def build():
    seed = os.environ.get('UPDATE_KEY_B64')
    if not seed:
        raise ValueError('Configure AIR2MI_UPDATE_KEY_B64 with the existing catalog signing key')
    key = Ed25519PrivateKey.from_private_bytes(base64.b64decode(seed, validate=True))
    public = key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw)
    if public != base64.b64decode(Path(__file__).with_name('update-trust.pub').read_bytes()):
        raise ValueError('Signing key does not match installed clients')
    sequence = int(os.environ['UPDATE_SEQUENCE'])
    if sequence <= 2026111401:
        raise ValueError('Update sequence must exceed the last pre-migration catalog')
    source = Path('assets')
    linux, = source.glob('*.run')
    android, = source.glob('*-components.zip')
    receipts = list(source.glob('*.manifest.json'))
    if receipts:
        receipt, = receipts
        meta = json.loads(receipt.read_bytes())
    else:
        sums = (source/'SHA256SUMS').read_text().splitlines()
        for artifact in (linux, android):
            if digest(artifact)+'  '+artifact.name not in sums:
                raise ValueError('Release checksum mismatch')
        with linux.open('rb') as stream:
            for _ in range(256):
                if stream.readline(65536) == b'__A2M_PAYLOAD_BELOW__\n':
                    break
            else:
                raise ValueError('Linux payload marker missing')
            with tarfile.open(fileobj=stream, mode='r|*') as bundle:
                for entry in bundle:
                    if entry.name.removeprefix('./') == 'BUILD.json' and entry.isfile():
                        meta = json.load(bundle.extractfile(entry))
                        break
                else:
                    raise ValueError('Linux build receipt missing')
        meta['sha256'] = digest(linux)
    with ZipFile(android) as archive:
        mobile = json.loads(archive.read('assets/engine-build.json'))
        for name, expected in mobile['files'].items():
            folder = 'assets/' if name.endswith('.zip') else 'jniLibs/arm64-v8a/'
            if hashlib.sha256(archive.read(folder + name)).hexdigest() != expected:
                raise ValueError('Android component hash mismatch')
    for field in ('package_version','kernel_version','source_revision','api_version'):
        if meta[field] != mobile[field]:
            raise ValueError('Mixed Engine builds: '+field)
    if meta['sha256'] != digest(linux):
        raise ValueError('Linux checksum mismatch')
    site = Path('site')
    dl = site/'dl'
    dl.mkdir(parents=True)
    common = {k:meta[k] for k in ('package_version','kernel_version','api_version')}
    common['source_commit'] = meta['source_revision']
    manifest = dict(common, platform='linux', architecture='x64', build_id=meta['build_id'],
                    sha256=meta['sha256'],update_sequence=sequence)
    raw = json.dumps(manifest).encode()
    package = dl/('linux-'+meta['sha256']+'.zip')
    with ZipFile(package,'w') as archive:
        archive.writestr('manifest.json',raw)
        archive.writestr('manifest.sig',key.sign(raw))
        archive.write(linux,'engine.run')
    mobile_file=dl/('android-'+digest(android)+'.zip')
    shutil.copyfile(android,mobile_file)
    targets=[]
    for file, fields in [(package,dict(platform='linux',architecture='x64',kind='engine_zip',engine_sha256=meta['sha256'])),
                         (mobile_file,dict(platform='android',architecture='arm64-v8a',kind='engine_components'))]:
        targets.append(dict(common,**fields,size=file.stat().st_size,sha256=digest(file),
                            url='https://youyoudezhuzhu.github.io/Air2Mi/dl/'+file.name))
    def catalog(component, published, entries):
        payload=json.dumps(dict(schema_version=1,app_id='air2mi',component=component,channel='stable',
                                published=published,sequence=sequence,targets=entries)).encode()
        return json.dumps(dict(format='air2mi-update-v1',key_id='air2mi-update-v1',
                               payload=encode(payload),signature=encode(key.sign(payload))))
    updates=site/'updates'
    updates.mkdir()
    (updates/'engine-stable.json').write_text(catalog('engine',True,targets))
    (updates/'app-stable.json').write_text(catalog('app',False,[]))
    (site/'index.html').write_text('<meta charset=utf-8><title>Air2Mi</title><h1>Air2Mi 内核下载</h1>'
        +f'<p><a href="dl/{package.name}">飞牛 x64 内核更新包</a></p>'
        +f'<p><a href="dl/{mobile_file.name}">Android ARM64 内核组件</a></p>')

if __name__ == '__main__':
    build()
