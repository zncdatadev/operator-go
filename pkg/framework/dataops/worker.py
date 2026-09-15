import hashlib, json, os, shutil, stat, sys
from pathlib import Path

action, operation = sys.argv[1:3]
source = Path(sys.argv[3] if len(sys.argv) > 3 else '/source')
target = Path(sys.argv[4] if len(sys.argv) > 4 else '/target')
receipt = '.framework-data-operation.json'

def manifest(root):
    result = {}
    for directory, dirs, files in os.walk(root, followlinks=False):
        for name in sorted(dirs + files):
            path = Path(directory) / name
            relative = str(path.relative_to(root))
            if relative == receipt:
                continue
            mode = path.lstat().st_mode
            if stat.S_ISLNK(mode):
                result[relative] = ['link', os.readlink(path)]
            elif stat.S_ISDIR(mode):
                result[relative] = ['directory', stat.S_IMODE(mode)]
            elif stat.S_ISREG(mode):
                digest = hashlib.sha256()
                with path.open('rb') as stream:
                    for chunk in iter(lambda: stream.read(1024 * 1024), b''):
                        digest.update(chunk)
                result[relative] = ['file', stat.S_IMODE(mode), path.stat().st_size, digest.hexdigest()]
            else:
                raise RuntimeError('unsupported special file: ' + relative)
    return result

def report(result):
    encoded = json.dumps(result)
    if len(sys.argv) <= 3:
        Path('/dev/termination-log').write_text(encoded)
    print(encoded)

if action == 'migrate':
    marker = target / receipt
    if marker.exists():
        if json.loads(marker.read_text())['operation'] != operation:
            raise RuntimeError('target belongs to another operation')
    elif list(target.iterdir()):
        raise RuntimeError('migration target is not empty')
    else:
        with marker.open('x') as out:
            json.dump({'operation': operation}, out)
            out.flush()
            os.fsync(out.fileno())
    before = manifest(source)
    for entry in target.iterdir():
        if entry.name == receipt:
            continue
        if entry.is_dir() and not entry.is_symlink():
            shutil.rmtree(entry)
        else:
            entry.unlink()
    # Volume mount roots belong to the provisioner. fsGroup permits payload
    # writes but does not authorize chmod/utime of that root. Copy its children
    # so payload metadata is preserved without replacing mount-root metadata.
    for entry in source.iterdir():
        if entry.name == receipt:
            continue
        destination = target / entry.name
        if entry.is_dir() and not entry.is_symlink():
            shutil.copytree(entry, destination, symlinks=True)
        else:
            shutil.copy2(entry, destination, follow_symlinks=False)
    os.sync()
    after = manifest(source)
    copied = manifest(target)
    if before != after or before != copied:
        raise RuntimeError('source changed or destination SHA256 manifest differs')
    report({'operation': operation, 'verified': 'sha256-tree',
                      'digest': hashlib.sha256(json.dumps(copied, sort_keys=True).encode()).hexdigest()})
elif action == 'destroy':
    for entry in source.iterdir():
        if entry.is_dir() and not entry.is_symlink():
            shutil.rmtree(entry)
        else:
            entry.unlink()
    os.sync()
    if list(source.iterdir()):
        raise RuntimeError('data directory is not empty after destruction')
    report({'operation': operation, 'verified': 'empty-filesystem'})
else:
    raise RuntimeError('unknown operation')
