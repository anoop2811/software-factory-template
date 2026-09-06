#!/bin/bash
set -euo pipefail

# Installer artifact acceptance for docs/adr/0045-maven-adoption.md:7.
# Copies the current template and stubs its post-install selftest/prereq checks
# and npm dependency installation to keep this artifact matrix bounded/offline.
# Maven builds, dependency installation, and attestation need separate real runs.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

python3 - "$TEMPLATE_ROOT" <<'PYTHON'
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tempfile

source = Path(sys.argv[1])
for required in ("scripts/factory-init.sh", "packs/java/pack.yaml"):
    if not (source / required).is_file():
        sys.exit("java-build-tool: required template input missing: " + required)

environment = os.environ.copy()
for key in list(environment):
    if key.startswith("GIT_") or key.startswith("FACTORY_"):
        environment.pop(key)
environment["GIT_CONFIG_NOSYSTEM"] = "1"
environment["GIT_CONFIG_GLOBAL"] = os.devnull
answers = "\n".join([
    "Matrix", "matrix", "@example", "example", "", "", "",
    "inherit", "standard", "n", "25", "y", "",
])
pom = (
    '<?xml version="1.0"?>\r\n'
    '<!-- Keep this adopter-owned comment and CRLF bytes. -->\r\n'
    '<project xmlns="http://maven.apache.org/POM/4.0.0">\r\n'
    '  <modelVersion>4.0.0</modelVersion>\r\n'
    '  <groupId>example</groupId><artifactId>matrix</artifactId><version>1</version>\r\n'
    '</project>\r\n'
).encode()


def require(condition, message, output):
    if not condition:
        print(output, file=sys.stderr)
        raise AssertionError(message)


with tempfile.TemporaryDirectory(prefix="factory-java-build-tool-") as temporary:
    workspace = Path(temporary)
    template = workspace / "template"
    tool_bin = workspace / "bin"
    tool_bin.mkdir()
    npm = tool_bin / "npm"
    npm.write_text("#!/bin/bash\nexit 0\n")
    npm.chmod(0o755)
    environment["PATH"] = str(tool_bin) + os.pathsep + environment["PATH"]
    shutil.copytree(source, template, ignore=shutil.ignore_patterns(
        ".git", ".factory", "node_modules", ".venv", "__pycache__",
        "java-build-tool.sh", "optional-packs.sh",
    ))
    for name in ("scripts/selftest/run.sh", "scripts/prereq-check.sh"):
        stub = template / name
        stub.write_text("#!/bin/bash\nexit 0\n")
        stub.chmod(0o755)

    def install(name, hints, expected, extra=()):
        target = workspace / name
        target.mkdir()
        if "pom" in hints:
            (target / "pom.xml").write_bytes(pom)
        for wrapper in ("mvnw", "gradlew"):
            if wrapper in hints:
                path = target / wrapper
                path.write_text("#!/bin/bash\nexit 0\n")
                path.chmod(0o644 if "nonexec" in hints else 0o755)
        original_files = {p.relative_to(target): p.read_bytes() for p in target.rglob("*") if p.is_file()}
        process = subprocess.Popen(
            ["bash", str(template / "scripts/factory-init.sh"), str(target), "--pack", "java", *extra],
            cwd=workspace, env=environment, stdin=subprocess.PIPE,
            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
            start_new_session=True,
        )
        try:
            output, _ = process.communicate(answers, timeout=60)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.communicate()
            raise AssertionError(name + " exceeded 60 seconds") from None
        for path, contents in original_files.items():
            require((target / path).read_bytes() == contents,
                    name + " changed adopter-owned " + str(path), output)
        if expected == "error":
            require(process.returncode != 0, name + " accepted a non-executable Maven wrapper", output)
            require(re.search(r"mvnw.*(executable|chmod)|chmod.*mvnw", output, re.I),
                    name + " omitted actionable wrapper guidance", output)
            after_files = {p.relative_to(target) for p in target.rglob("*") if p.is_file()}
            require(after_files == set(original_files), name + " copied files before rejecting the wrapper", output)
        else:
            require(process.returncode == 0, name + " installer failed", output)
            config = (target / "factory.yaml").read_text()
            check = next((line for line in config.splitlines() if line.startswith("check_command:")), "")
            makefile = (target / "Makefile.java.pack").read_text()
            workflow = (target / ".github/workflows/java-pack.yml").read_text()
            require("include Makefile.java.pack" in (target / "Makefile").read_text(),
                    name + " did not include its Java Makefile", output)
            require("./scripts/hooks/junit5-only-check.sh" in check,
                    name + " omitted the JUnit dialect gate", output)
            require("__JAVA_VERSION__" not in workflow and "25" in workflow,
                    name + " did not substitute the JDK version", output)
            if expected == "gradle":
                require("./gradlew" in check and "./gradlew" in workflow,
                        name + " lost the Gradle selection", output)
                require((target / "quality.gradle").is_file(), name + " omitted Gradle quality config", output)
            else:
                require(expected + " -B verify" in check, name + " selected the wrong Maven check: " + check, output)
                require((target / "quality-maven.xml").is_file(), name + " omitted native Maven quality config", output)
                require(not (target / "quality.gradle").exists(), name + " installed Gradle quality config into Maven", output)
                require("gradle" not in (check + makefile + workflow).lower(),
                        name + " generated Gradle commands for Maven", output)
                require(expected in makefile and expected + " -B" in workflow and "cache: maven" in workflow,
                        name + " generated mismatched Maven Makefile/workflow commands", output)
                require(not re.search(r"__[A-Z_]+__", makefile + workflow),
                        name + " left an unsubstituted build token", output)
                require("quality-maven.xml" in output,
                        name + " omitted Maven quality integration guidance", output)
        print("  ok: " + name, flush=True)

    install("pom-only", {"pom"}, "mvn")
    install("maven-wrapper", {"mvnw"}, "./mvnw")
    install("mixed-auto-gradle", {"pom", "mvnw", "gradlew"}, "gradle")
    install("mixed-explicit-maven", {"pom", "mvnw", "gradlew"}, "./mvnw", ("--java-build-tool", "maven"))
    install("empty-default-gradle", set(), "gradle")
    install("nonexecutable-maven-wrapper", {"pom", "mvnw", "nonexec"}, "error")

print("java-build-tool: 6 passed, 0 failed")
PYTHON
