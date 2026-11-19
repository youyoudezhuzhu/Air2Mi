"""Initialize the user-imported Engine; keep installed code immutable in-process."""
import hashlib,importlib.util,json,os,shutil,stat,sys,tempfile,threading,zipfile
from pathlib import Path, PurePosixPath
_lock=threading.RLock()

def digest_stream(source):
    digest=hashlib.sha256()
    for chunk in iter(lambda:source.read(1024*1024),b''):digest.update(chunk)
    return digest.hexdigest()

def unpack(component_dir):
    components=Path(component_dir).resolve();metadata=json.loads((components/'engine-build.json').read_text())
    native='engine-native.zip' in metadata.get('files',{});archive=components/('engine-native.zip' if native else 'engine-source.zip')
    with archive.open('rb') as source:digest=digest_stream(source)
    if digest!=metadata['files'].get(archive.name):raise ValueError('Engine archive SHA256 mismatch')
    root=components/('python-'+digest)
    with _lock:
        if root.is_symlink():raise ValueError('Engine extraction root is a symlink')
        with zipfile.ZipFile(archive) as source:
            entries={}
            for info in source.infolist():
                name=info.filename;path=PurePosixPath(name)
                if (path.is_absolute() or '\\' in name or
                        any(p in ('','.','..') for p in name.rstrip('/').split('/')) or
                        name.rstrip('/') in entries or stat.S_ISLNK(info.external_attr>>16)):
                    raise ValueError('unsafe Engine archive entry')
                entries[name.rstrip('/')]=info
            if not root.exists():
                temporary=Path(tempfile.mkdtemp(prefix='.engine-',dir=components))
                try:
                    source.extractall(temporary)
                    verify_tree(temporary,source,entries)
                    os.replace(temporary,root)
                finally:
                    if temporary.exists():shutil.rmtree(temporary)
            else:verify_tree(root,source,entries)
    return root,native

def verify_tree(root,archive,entries):
    if not root.is_dir():raise ValueError('Engine extraction is not a directory')
    for path in root.rglob('*'):
        if path.is_symlink() or (path.is_file() and path.relative_to(root).as_posix() not in entries):
            raise ValueError('Engine extracted files changed')
    for name,info in entries.items():
        file=root/name
        if info.is_dir():
            if not file.is_dir():raise ValueError('Engine directory missing')
        else:
            if not file.is_file() or file.stat().st_size!=info.file_size:raise ValueError('Engine file missing or changed')
            with archive.open(info) as expected,file.open('rb') as actual:
                if digest_stream(expected)!=digest_stream(actual):raise ValueError('Engine file checksum mismatch')

def load_anchor(root):
    anchor=root/'_air2mi_integrity.so'
    if anchor.is_symlink() or not anchor.is_file():raise ValueError('Native integrity anchor missing')
    loaded=sys.modules.get('_air2mi_integrity')
    if loaded is not None:
        if Path(getattr(loaded,'__file__','')).resolve()!=anchor.resolve():raise ValueError('Engine build changed; restart the App process')
        return loaded
    spec=importlib.util.spec_from_file_location('_air2mi_integrity',anchor)
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
    sys.modules['_air2mi_integrity']=module
    return module

def initialize(config,data_dir,component_dir,native_dir,android_host):
    root,native=unpack(component_dir);components=Path(component_dir).resolve()
    if native:load_anchor(root).validate(root,native_dir)
    os.environ['AIR2MI_BUILD_MANIFEST']=str(components/'engine-build.json');sys.dont_write_bytecode=True
    for entry in (str(root),str(root/'kernel')):
        if entry not in sys.path:sys.path.insert(0,entry)
    from engine_runtime.platform import configure
    configure(kind='android',data_root=data_dir,native_dir=native_dir,android_host=android_host,embedded=True)
    from engine_runtime.configuration import configure_host
    from a2m_kernel.host import EngineHost
    cfg=json.loads(config) if config and config!='{}' else configure_host(data_dir,native_dir,False,android_host.getBindAddress())
    host=EngineHost();host.initialize(cfg,data_dir,platform='android',native_dir=native_dir,android_host=android_host,embedded=True)
    return host
