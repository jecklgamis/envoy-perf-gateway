import os
import tempfile


def atomic_write(path, content):
    """Write content to path via a temp file + os.replace().

    os.replace() is an atomic rename on POSIX filesystems, so Envoy's
    inotify-based watch never observes a missing or partially-written file -
    unlike a plain unlink()+move() (delete then create), which has a window
    where the file doesn't exist and can trigger a reload against nothing
    or a truncated read.
    """
    directory = os.path.dirname(os.path.abspath(path))
    fd, tmp_path = tempfile.mkstemp(dir=directory, prefix=".tmp-")
    try:
        with os.fdopen(fd, "w") as f:
            f.write(content)
        os.replace(tmp_path, path)
    except Exception:
        if os.path.exists(tmp_path):
            os.unlink(tmp_path)
        raise
