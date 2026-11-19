#!/usr/bin/env python3
"""Reject Engine payloads even when nested in APK/runtime archives."""
import io,re,sys,zipfile
from pathlib import Path
FORBIDDEN_PARTS={'a2m_kernel','engine_runtime','river','hub'}
FORBIDDEN_NAMES={'engine-source.zip','engine-native.zip','engine-build.json','libair2mi_codec.so','libopenairplay2.so'}
def audit(data,prefix='',depth=0):
    if depth>5:raise AssertionError('unexpected nested runtime archive depth')
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        for name in archive.namelist():
            parts=Path(name).parts;leaf=parts[-1] if parts else ''
            assert not FORBIDDEN_PARTS.intersection(parts),f'Engine module bundled: {prefix}{name}'
            assert leaf not in FORBIDDEN_NAMES and not leaf.startswith(('_air2mi_integrity.','_license_guard.')),f'Engine component bundled: {prefix}{name}'
            if name.endswith('.so') and re.search(rb'/vol[0-9]+/@appdata/', archive.read(name)):
                raise AssertionError(f'Private build path in native library: {prefix}{name}')
            if name == 'assets/chaquopy/app.imy':
                with zipfile.ZipFile(io.BytesIO(archive.read(name))) as app:
                    if any(not item.is_dir() for item in app.infolist()):
                        raise AssertionError(f'Host runtime contains application payload: {prefix}{name}')
            if name.endswith(('.imy','.zip','.jar','.aar')):
                content=archive.read(name)
                if zipfile.is_zipfile(io.BytesIO(content)):audit(content,prefix+name+'!/',depth+1)
def check(apk):
    data=Path(apk).read_bytes();audit(data)
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        names=archive.namelist();assert 'assets/index.html' in names,'Fluent Web UI missing';assert 'assets/air2mi_app_host.py' in names,'Android management adapter missing'
        assert any(name.endswith('/libpython3.11.so') for name in names),'Python host runtime missing';assert 'classes.dex' in names,'Host classes missing'
    return {'engine_bundled':False,'nested_archives_checked':True}
if __name__=='__main__':
    check(sys.argv[1]);print('APK OK: management UI + generic host runtime; no Engine payload')
