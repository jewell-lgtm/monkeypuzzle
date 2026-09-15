"""Exercise the error pane with a real terminal, without opening herdr UI."""
import errno
import os
from pathlib import Path
import pty
import select
import subprocess
import tempfile
import time

scripts = Path(__file__).resolve().parent.parent / "scripts"
for picker in ['open', 'create', 'adopt', 'inbox']:
    with tempfile.TemporaryDirectory() as tmp:
        master, slave = pty.openpty()
        env = dict(os.environ, HERDR_ENV="1", MP_PLUGIN_BIN=f"{tmp}/missing-mp")
        proc = subprocess.Popen(
            ["bash", str(scripts / "pane.sh"), picker],
            stdin=slave, stdout=slave, stderr=slave, env=env,
        )
        os.close(slave)
        output = b""
        try:
            deadline = time.monotonic() + 5
            while b"Press Enter to close." not in output and time.monotonic() < deadline:
                if select.select([master], [], [], 0.1)[0]:
                    try:
                        output += os.read(master, 65536)
                    except OSError as exc:
                        if exc.errno != errno.EIO:
                            raise
                        break
            assert b"Required command not found:" in output, output
            assert b"MP_PLUGIN_BIN" in output, output
            assert b"Press Enter to close." in output, output
            assert proc.poll() is None, "error pane closed before dismissal"
            os.write(master, b"\n")
            assert proc.wait(timeout=5) == 1
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.wait()
            os.close(master)
