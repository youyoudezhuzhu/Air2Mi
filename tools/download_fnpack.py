#!/usr/bin/env python3
import hashlib,json,urllib.request
from pathlib import Path
lock=json.loads(Path('packaging/fnos/toolchain.json').read_text());b=urllib.request.urlopen(lock['url'],timeout=60).read();assert b[:4]==b'\x7fELF';assert hashlib.sha256(b).hexdigest()==lock['sha256'];p=Path('build/fnpack');p.parent.mkdir(exist_ok=True);p.write_bytes(b);p.chmod(0o700)
