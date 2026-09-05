import json
import unittest
from unittest.mock import patch

import ci_image_smoke as smoke

IMAGE = "ghcr.io/fixture/unimap@sha256:" + "a" * 64
COMMIT = "b" * 40


class ImageSmokeTest(unittest.TestCase):
    def check_case(self, failure=None):
        calls = []

        def fake(*args, **kwargs):
            calls.append(args)
            if args[0] == "create":
                self.assertIn("none", args)
                self.assertNotIn("--publish", args)
                self.assertNotIn("--volume", args)
                return "created-container"
            if args[0] == "inspect":
                if "{{.State.Running}}" in args:
                    return "false" if failure == "early-exit" else "true"
                return "1" if failure == "shutdown" else "0"
            if args[0] == "exec":
                if args[2] == "wget":
                    if failure == "unready":
                        return ""
                    if failure == "bad-json":
                        return "not JSON"
                    commit = "c" * 40 if failure == "commit" else COMMIT
                    return json.dumps({"status": "ok", "version": f"master (commit={commit}, built=fixture)"})
                if args[2] == "id":
                    return "0" if failure == "root" else "1000"
                return "644" if failure == "mode" else "600"
            return ""

        with patch.object(smoke, "docker", side_effect=fake), patch.object(smoke.time, "sleep"), patch.object(smoke.time, "monotonic", side_effect=[0, 1, 100]):
            if failure:
                with self.assertRaises(RuntimeError):
                    smoke.verify(IMAGE, COMMIT)
            else:
                result = smoke.verify(IMAGE, COMMIT)
                self.assertEqual(result["readiness"], "ok")
        self.assertEqual(calls[-1], ("rm", "--force", "created-container"))
        self.assertEqual(calls[0], ("pull", IMAGE))

    def test_success(self):
        self.check_case()

    def test_failure_paths_cleanup(self):
        for failure in ["early-exit", "unready", "bad-json", "commit", "root", "mode", "shutdown"]:
            with self.subTest(failure=failure):
                self.check_case(failure)

    def test_unpinned_image_rejected_before_docker(self):
        with patch.object(smoke, "docker") as docker:
            with self.assertRaises(ValueError):
                smoke.verify("ghcr.io/fixture/unimap:latest", COMMIT)
            docker.assert_not_called()


if __name__ == "__main__":
    unittest.main()
