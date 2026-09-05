"""Start the exact published image in an isolated, disposable container."""
import argparse
import json
import re
import subprocess
import time


def docker(*args, timeout=60, check=True):
    result = subprocess.run(["docker", *args], capture_output=True, text=True, timeout=timeout)
    if result.returncode:
        if check:
            raise RuntimeError(f"docker {args[0]} failed (exit {result.returncode})")
        return ""
    return result.stdout.strip()


def verify(image, commit):
    if not re.fullmatch(r"ghcr\.io/[a-z0-9_.-]+/[a-z0-9_.-]+@sha256:[a-f0-9]{64}", image):
        raise ValueError("image must be pinned to a GHCR digest")
    if not re.fullmatch(r"[a-f0-9]{40}", commit):
        raise ValueError("expected commit must be a full SHA")
    docker("pull", image, timeout=180)
    container = docker("create", "--network", "none", "--env",
                       "UNIMAP_CONFIG_PATH=/app/runtime-config/config.yaml", image)
    try:
        docker("start", container)
        deadline = time.monotonic() + 90
        while time.monotonic() < deadline:
            if docker("inspect", "--format", "{{.State.Running}}", container) != "true":
                raise RuntimeError("container exited before readiness")
            body = docker("exec", container, "wget", "-q", "-T", "5", "-O", "-",
                          "http://127.0.0.1:8448/health/ready", check=False)
            try:
                ready = json.loads(body)
            except (ValueError, TypeError):
                ready = {}
            if isinstance(ready, dict) and ready.get("status") == "ok":
                if f"commit={commit}," not in ready.get("version", ""):
                    raise RuntimeError("readiness reports a different commit")
                break
            time.sleep(2)
        else:
            raise RuntimeError("readiness did not become healthy within 90 seconds")
        if docker("exec", container, "id", "-u") == "0":
            raise RuntimeError("image runs as root")
        mode = docker("exec", container, "stat", "-c", "%a", "/app/runtime-config/config.yaml")
        if mode != "600":
            raise RuntimeError("runtime configuration was not initialized with mode 600")
        docker("stop", "--time", "40", container, timeout=50)
        if docker("inspect", "--format", "{{.State.ExitCode}}", container) != "0":
            raise RuntimeError("container did not shut down gracefully")
        return {"image": image, "commit": commit, "readiness": "ok", "shutdown": "ok"}
    finally:
        # Only remove the container ID created by this invocation; no host mounts.
        docker("rm", "--force", container)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("image")
    parser.add_argument("commit")
    args = parser.parse_args()
    print(json.dumps(verify(args.image, args.commit), sort_keys=True))
