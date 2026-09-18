import os
import tempfile


def atomic_write_bytes(path, content):
    """Write bytes to path via temp file + os.replace().

    Done from inside the container (not across a Docker Desktop bind mount)
    so the rename generates a real inotify event Envoy's watched_directory
    will see.
    """
    directory = os.path.dirname(os.path.abspath(path))
    fd, tmp_path = tempfile.mkstemp(dir=directory, prefix=".tmp-")
    try:
        with os.fdopen(fd, "wb") as f:
            f.write(content)
        os.replace(tmp_path, path)
    except Exception:
        if os.path.exists(tmp_path):
            os.unlink(tmp_path)
        raise
