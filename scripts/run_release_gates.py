"""Run shared release gates with a portable total deadline and child cleanup."""

import os
import signal
import subprocess
import sys
import time

GATES = ("ci", "test-statistical")
BUDGET_SECONDS = 25 * 60


def run_gates():
    deadline = time.monotonic() + BUDGET_SECONDS
    for recipe in GATES:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise TimeoutError("Release checks exceeded their 25-minute total budget")
        process = subprocess.Popen(["just", recipe], start_new_session=True)
        try:
            result = process.wait(timeout=remaining)
        except BaseException:
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                pass
            # The parent can exit while a descendant ignores SIGTERM.
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            if process.poll() is None:
                process.wait()
            raise
        if result:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            raise subprocess.CalledProcessError(result, ["just", recipe])


if __name__ == "__main__":
    try:
        run_gates()
    except (OSError, TimeoutError, subprocess.SubprocessError) as error:
        sys.exit(f"error: release checks failed: {error}")
