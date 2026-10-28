"""Exercise public build entrypoints without building images or installing agents."""

import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import textwrap
import unittest


ROOT = Path(__file__).resolve().parents[1]
TEMP_ROOT = "/tmp/opencode" if Path("/tmp/opencode").is_dir() else None


def build_env(**values):
    env = {
        key: value for key, value in os.environ.items()
        if key not in ("VERSION", "AGENT_VERSION", "NB_BUILD", "MAKEFLAGS", "MFLAGS")
    }
    return dict(env, **values)


class BuildVersionsTest(unittest.TestCase):
    def test_committed_agent_pin_can_change_without_changing_server_build(self):
        with tempfile.TemporaryDirectory(dir=TEMP_ROOT) as temp:
            root = Path(temp)
            (root / "tools").mkdir()
            shutil.copyfile(ROOT / "tools/agent-version.sh", root / "tools/agent-version.sh")
            (root / "NB_BUILD").write_text("99\n")
            for version in ("2.1.6~nb3", "2.1.6~nb4", "2.1.7~nb1"):
                with self.subTest(version=version):
                    (root / "AGENT_VERSION").write_text(version + "\n")
                    result = subprocess.run(
                        ["bash", str(root / "tools/agent-version.sh")],
                        env=build_env(), text=True, capture_output=True, check=True,
                    )
                    self.assertEqual(version, result.stdout.strip())
                    self.assertEqual("99\n", (root / "NB_BUILD").read_text())

    def test_ci_rejects_invalid_or_missing_agent_pin_before_emitting_versions(self):
        action = (ROOT / ".github/actions/release-context/action.yml").read_text()
        script = textwrap.dedent(action.split("      run: |\n", 1)[1])
        with tempfile.TemporaryDirectory(dir=TEMP_ROOT) as temp:
            root = Path(temp)
            (root / "NB_BUILD").write_text("99\n")
            for value in (None, "", "2.1.6~nb0", "2.1.6~nb3\nextra"):
                with self.subTest(value=value):
                    pin, output = root / "AGENT_VERSION", root / "output"
                    pin.unlink(missing_ok=True)
                    output.unlink(missing_ok=True)
                    if value is not None:
                        pin.write_text(value)
                    result = subprocess.run(
                        ["bash", "-c", script], cwd=root, text=True, capture_output=True,
                        env=build_env(INPUT_TAG="v9.0.0", REF_TYPE="", REF_NAME="", RELEASE_TAG="", FETCH_NOTES="false", NOTES_FOR_PRERELEASE="false", GITHUB_OUTPUT=str(output)),
                    )
                    self.assertNotEqual(0, result.returncode)
                    self.assertFalse(output.exists())

    def test_native_agent_reports_the_pinned_version_with_a_different_server_build(self):
        expected = (ROOT / "AGENT_VERSION").read_text().strip()
        with tempfile.TemporaryDirectory(dir=TEMP_ROOT) as temp:
            subprocess.run(
                ["make", "build", f"BUILD_DIR={temp}", f"GOBIN={temp}"],
                cwd=ROOT / "agent-source-code", env=build_env(NB_BUILD="99"),
                text=True, capture_output=True, check=True, timeout=180,
            )
            binary = Path(temp) / "patchmon-agent"
            result = subprocess.run(
                [str(binary), "--version"], text=True, capture_output=True,
                check=True, timeout=10,
            )
            self.assertEqual("patchmon-agent version " + expected, result.stdout.strip())
            metadata = subprocess.run(
                ["go", "version", "-m", str(binary)], text=True,
                capture_output=True, check=True, timeout=10,
            )
            self.assertIn("pkgversion.Version=" + expected, metadata.stdout)

    def test_legacy_version_override_remains_supported(self):
        for version, expected in (("2.1.7", "2.1.7~nb99"), ("2.1.7~nb2", "2.1.7~nb2")):
            with self.subTest(version=version):
                result = subprocess.run(
                    ["make", "-n", "build", f"VERSION={version}"],
                    cwd=ROOT / "agent-source-code", env=build_env(NB_BUILD="99"),
                    text=True, capture_output=True, check=True,
                )
                self.assertIn(f'pkgversion.Version={expected}"', result.stdout)

    def test_all_agent_targets_use_the_same_pinned_version(self):
        result = subprocess.run(
            ["make", "-n", "build-linux", "build-freebsd", "build-windows"],
            cwd=ROOT / "agent-source-code", env=build_env(NB_BUILD="99"),
            text=True, capture_output=True, check=True,
        )
        expected = (ROOT / "AGENT_VERSION").read_text().strip()
        self.assertEqual(10, result.stdout.count(f'pkgversion.Version={expected}"'))

    def test_ci_agent_matrix_stamps_the_agent_output(self):
        workflow = (ROOT / ".github/workflows/docker.yml").read_text()
        step = workflow.split("      - name: Build all agent binaries\n", 1)[1].split("      - name:", 1)[0]
        output_key = re.search(r"VERSION: \$\{\{ steps.ctx.outputs.([a-z-]+) \}\}", step).group(1)
        script = textwrap.dedent(step.split("        run: |\n", 1)[1])
        with tempfile.TemporaryDirectory(dir=TEMP_ROOT) as temp:
            root = Path(temp)
            (root / "agent-source-code").mkdir()
            stub = root / "bin/go"
            stub.parent.mkdir()
            stub.write_text('#!/bin/sh\nprintf "%s\\n" "$@" >> "$BUILD_LOG"\n')
            stub.chmod(0o755)
            log = root / "build.log"
            outputs = {"version": "9.0.0~nb99", "agent-version": "2.1.6~nb3"}
            subprocess.run(
                ["bash", "-c", script], cwd=root / "agent-source-code", check=True,
                env=build_env(PATH=str(stub.parent) + os.pathsep + os.environ["PATH"], VERSION=outputs[output_key], BUILD_LOG=str(log)),
            )
            commands = log.read_text()
            self.assertEqual(10, commands.count("pkgversion.Version=2.1.6~nb3"))
            self.assertNotIn("9.0.0~nb99", commands)

    def test_ci_server_release_changes_but_agent_release_stays_pinned(self):
        action = (ROOT / ".github/actions/release-context/action.yml").read_text()
        script = textwrap.dedent(action.split("      run: |\n", 1)[1])
        with tempfile.TemporaryDirectory(dir=TEMP_ROOT) as temp:
            root = Path(temp)
            (root / "AGENT_VERSION").write_text("2.1.6~nb3\n")
            (root / "NB_BUILD").write_text("99\n")
            for tag in ("v9.0.0", "v9.0.1-rc.2"):
                with self.subTest(tag=tag):
                    output = root / "output"
                    output.unlink(missing_ok=True)
                    result = subprocess.run(
                        ["bash", "-c", script], cwd=root, text=True, capture_output=True,
                        env=build_env(INPUT_TAG=tag, REF_TYPE="", REF_NAME="", RELEASE_TAG="", FETCH_NOTES="false", NOTES_FOR_PRERELEASE="false", GITHUB_OUTPUT=str(output)),
                    )
                    self.assertEqual(0, result.returncode, result.stderr)
                    values = dict(line.split("=", 1) for line in output.read_text().splitlines())
                    self.assertEqual("2.1.6~nb3", values.get("agent-version"))
                    self.assertEqual(tag[1:] + "~nb99", values["version"])

    def test_local_image_build_passes_agent_pin_not_server_version(self):
        with tempfile.TemporaryDirectory(dir=TEMP_ROOT) as temp:
            root = Path(temp)
            for name in ("docker/build.sh", "tools/agent-version.sh", "AGENT_VERSION", "NB_BUILD"):
                source = ROOT / name
                if source.exists():
                    (root / name).parent.mkdir(parents=True, exist_ok=True)
                    shutil.copyfile(source, root / name)
            stub = root / "bin/make"
            stub.parent.mkdir()
            stub.write_text('#!/bin/sh\nprintf "%s\\n" "$@"\nexit 78\n')
            stub.chmod(0o755)
            for override in (None, "2.1.7~nb1", "", "invalid"):
                with self.subTest(override=override):
                    env = build_env(PATH=str(stub.parent) + os.pathsep + os.environ["PATH"], NB_BUILD="99")
                    if override is not None:
                        env["AGENT_VERSION"] = override
                    result = subprocess.run(
                        ["bash", "docker/build.sh", "--version", "9.0.0"],
                        cwd=root, env=env, text=True, capture_output=True,
                    )
                    if override in ("", "invalid"):
                        self.assertEqual(1, result.returncode)
                        self.assertNotIn("Building agent binaries", result.stdout)
                        continue
                    expected = override or (ROOT / "AGENT_VERSION").read_text().strip()
                    self.assertEqual(78, result.returncode, result.stderr)
                    self.assertIn("Version: 9.0.0~nb99", result.stdout)
                    self.assertIn("AGENT_VERSION=" + expected, result.stdout)
                    self.assertNotIn("VERSION=9.0.0~nb99", result.stdout)

    def test_invalid_agent_version_fails_before_a_build(self):
        for version in ("", "bad", "2.1.7~nb0", "2.1.7~nb2\nextra", "2.1.7'; false '"):
            with self.subTest(version=version):
                result = subprocess.run(
                    ["make", "-n", "build"], cwd=ROOT / "agent-source-code",
                    env=build_env(AGENT_VERSION=version), text=True, capture_output=True,
                )
                self.assertNotEqual(0, result.returncode)
                self.assertNotIn("./cmd/patchmon-agent", result.stdout)

    def test_explicit_agent_version_is_complete_and_independent(self):
        for version in ("2.1.7~nb1", "2.1.7", "2.1.7-rc.1~nb2"):
            with self.subTest(version=version):
                result = subprocess.run(
                    ["make", "-n", "build"], cwd=ROOT / "agent-source-code",
                    env=build_env(AGENT_VERSION=version, NB_BUILD="99"),
                    text=True, capture_output=True, check=True,
                )
                self.assertIn(f'pkgversion.Version={version}"', result.stdout)

    def test_server_build_bump_does_not_change_agent_version(self):
        outputs = []
        for nb_build in ("3", "99"):
            result = subprocess.run(
                ["make", "-n", "build"], cwd=ROOT / "agent-source-code",
                env=build_env(NB_BUILD=nb_build), text=True, capture_output=True, check=True,
            )
            outputs.append(result.stdout)
        self.assertEqual(outputs[0], outputs[1])


if __name__ == "__main__":
    unittest.main()
