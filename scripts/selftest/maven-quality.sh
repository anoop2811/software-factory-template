#!/bin/bash
set -euo pipefail

# Real quality-plugin acceptance: docs/adr/0045-maven-adoption.md:21.
# Requires Maven and a JDK supported by the snippet; no prerequisites are skipped.
# Maven downloads its pinned dependencies into the caller's normal cache.
# Fixture-only pins checked upstream on 2026-09-06:
# JUnit 5.14.4 (2026-04-26): https://github.com/junit-team/junit-framework/releases/tag/r5.14.4
# Surefire 3.6.0 (2026-09-03): https://github.com/apache/maven-surefire/releases/tag/surefire-3.6.0
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
import xml.etree.ElementTree as ET

source = Path(sys.argv[1])
maven = shutil.which(os.environ.get("MAVEN_BINARY", "mvn"))
java_home = os.environ.get("JAVA_HOME", "")
java = str(Path(java_home) / "bin/java") if java_home else shutil.which("java")
javac = str(Path(java_home) / "bin/javac") if java_home else shutil.which("javac")
if not maven:
    sys.exit("maven-quality: Maven is required; set MAVEN_BINARY or add mvn to PATH")
if not java or not javac or not os.access(java, os.X_OK) or not os.access(javac, os.X_OK):
    sys.exit("maven-quality: a full JDK is required; set JAVA_HOME")
snippet = source / "packs/java/maven/quality-maven.xml"
if not snippet.is_file():
    sys.exit("maven-quality: required quality-maven.xml is missing")
workspace = Path(tempfile.mkdtemp(prefix="factory-maven-quality-"))
print("maven-quality: fixture/logs " + str(workspace), flush=True)


def run(name, arguments, failure=None):
    log = workspace / (name + ".log")
    with log.open("w") as stream:
        process = subprocess.Popen(
            [maven, "-B", "--no-transfer-progress", *arguments], cwd=workspace,
            stdout=stream, stderr=subprocess.STDOUT, start_new_session=True,
        )
        try:
            status = process.wait(timeout=600)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()
            raise AssertionError(name + " exceeded 600 seconds; see " + str(log)) from None
    output = log.read_text()
    if failure:
        passed = status != 0 and re.search(failure, output, re.I)
    else:
        passed = status == 0 and "BUILD SUCCESS" in output
    if not passed:
        print("\n".join(output.splitlines()[-55:]), file=sys.stderr)
        raise AssertionError(name + " did not meet its expected result; see " + str(log))
    print("  ok: " + name, flush=True)
    return output


try:
    # Use the shipped build block intact, adding only Surefire to run the
    # fixture's JUnit 5 tests. No quality plugin is disabled or weakened.
    project = ET.fromstring("""<project>
      <modelVersion>4.0.0</modelVersion>
      <groupId>com.example</groupId><artifactId>quality-smoke</artifactId><version>1</version>
      <properties>
        <maven.compiler.release>21</maven.compiler.release>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
      </properties>
      <dependencies><dependency>
        <groupId>org.junit.jupiter</groupId><artifactId>junit-jupiter</artifactId>
        <version>5.14.4</version><scope>test</scope>
      </dependency></dependencies>
    </project>""")
    build = ET.parse(snippet).getroot()
    if build.tag != "build" or build.find("plugins") is None:
        raise AssertionError("quality-maven.xml must contain a native build/plugins block")
    build.find("plugins").append(ET.fromstring("""<plugin>
      <groupId>org.apache.maven.plugins</groupId><artifactId>maven-surefire-plugin</artifactId>
      <version>3.6.0</version>
    </plugin>"""))
    project.append(build)
    ET.ElementTree(project).write(workspace / "pom.xml", encoding="utf-8", xml_declaration=True)
    main_dir = workspace / "src/main/java/com/example"
    test_dir = workspace / "src/test/java/com/example"
    main_dir.mkdir(parents=True)
    test_dir.mkdir(parents=True)
    main = main_dir / "Calculator.java"
    main.write_text("""package com.example;

public final class Calculator {
    public int increment(int value) {
        return value + 1;
    }
}
""")
    (test_dir / "CalculatorTest.java").write_text("""package com.example;

import static org.junit.jupiter.api.Assertions.assertEquals;

import org.junit.jupiter.api.Test;

final class CalculatorTest {
    @Test
    void increments() {
        assertEquals(3, new Calculator().increment(2));
        assertEquals(0, new Calculator().increment(-1));
    }
}
""")
    run("format-fixture", ["spotless:apply"])
    good_source = main.read_text()
    good_output = run("good-verify", ["clean", "verify"])
    if not re.search(r"Tests run: [1-9]", good_output):
        raise AssertionError("good-verify did not execute the JUnit fixture")

    main.write_text(good_source.replace("return value + 1;", "return    value+1;"))
    run("reject-format", ["verify"], r"spotless.*check.*failed|format violations")
    main.write_text(good_source)
    run("restored-format", ["verify"])

    # Error Prone's documented error-severity case: an exception constructed
    # and discarded. https://errorprone.info/bugpattern/DeadException
    bad_exception = main_dir / "BadException.java"
    bad_exception.write_text("""package com.example;

public final class BadException {
    public void broken() {
        new IllegalStateException("forgot to throw");
    }
}
""")
    run("format-dead-exception", ["spotless:apply"])
    run("reject-dead-exception", ["clean", "verify"], r"\[DeadException\]")
    bad_exception.unlink()
    run("restored-error-prone", ["clean", "verify"])

    # Returning a private mutable array is SpotBugs EI_EXPOSE_REP. Format it
    # first and require that detector's name, so a compiler failure cannot pass.
    leaky = main_dir / "Leaky.java"
    leaky.write_text("""package com.example;

public final class Leaky {
    private final int[] values = {1, 2};

    public int[] values() {
        return values;
    }
}
""")
    run("format-exposed-array", ["spotless:apply"])
    run("reject-exposed-array", ["clean", "verify"], r"EI_EXPOSE_REP")
    leaky.unlink()
    run("restored-spotbugs", ["clean", "verify"])

    pit_output = run("pit-junit5", ["test-compile", "pitest:mutationCoverage", "-DoutputFormats=XML"])
    reports = list((workspace / "target/pit-reports").rglob("mutations.xml"))
    mutations = [node for report in reports for node in ET.parse(report).getroot().findall("mutation")]
    if not mutations or not any(node.get("status") == "KILLED" for node in mutations):
        raise AssertionError("PIT did not produce a killed mutation using the JUnit 5 fixture")
    print("maven-quality: 5 passed, 0 failed (verify, format, Error Prone, SpotBugs, PIT)", flush=True)
except Exception:
    print("maven-quality: fixture/logs retained at " + str(workspace), file=sys.stderr)
    raise
else:
    shutil.rmtree(workspace)
PYTHON
