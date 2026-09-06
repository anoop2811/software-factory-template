# ADR-0045: select Java build tooling without replacing the adopter POM

Status: accepted for implementation, 2026-09-06.

Issue #66 reports a real Maven adoption receiving Gradle commands and files:
https://github.com/anoop2811/software-factory-template/issues/66 (read 2026-09-06).

Factory-init selects Gradle when gradlew exists; otherwise pom.xml or mvnw
selects Maven; an unrecognized or empty Java project retains the Gradle default.
An explicit --java-build-tool maven|gradle overrides detection for mixed builds.
Maven selects ./mvnw when present and executable, otherwise mvn when absent;
a present non-executable wrapper produces an actionable error before install.

The Java pack shares one JUnit dialect gate and language identity. Maven has an
overlay containing Makefile targets, a native CI workflow, and quality-maven.xml.
Maven installs do not receive quality.gradle. The selected build tool and
commands are shown during init; the initial check_command runs Maven verify
and the dialect gate. Existing POMs remain unchanged.

The quality snippet contains build/plugins entries for Spotless, Error Prone,
SpotBugs with find-sec-bugs, and PIT with its JUnit 5 bridge. Adopters merge
these entries into the shared parent POM, retaining existing configuration and
annotation processors. Spotless, compilation checks, and SpotBugs then bind
into verify; PIT is explicitly invoked after test-compile. Until merged, Maven
verify checks the existing project lifecycle; documentation and init identify
quality-plugin integration as remaining setup, not an enforced guarantee.

Surefire documentation must explain that -Dtest replaces default selection,
including the risk of selecting failsafe-only classes with exclusion-only
filters. The example spells out Surefire default positive patterns before
excluding a named slow test; reactor users may need the documented
surefire.failIfNoSpecifiedTests setting. No adopter-specific exclusion is seeded.

Sources read 2026-09-06: Maven Surefire test-mojo.html,
https://maven.apache.org/surefire/maven-surefire-plugin/test-mojo.html ;
https://errorprone.info/docs/installation ;
https://github.com/diffplug/spotless/blob/main/plugin-maven/README.md ;
https://spotbugs.github.io/spotbugs-maven-plugin/examples/violationChecking.html ;
https://pitest.org/quickstart/maven/ .

New Maven pins checked against upstream release APIs on 2026-09-06:

| Component | Version | Release date | Source |
|---|---|---|---|
| Spotless Maven | 3.10.2 | 2026-09-04 | https://github.com/diffplug/spotless/releases/tag/maven/3.10.2 |
| palantir-java-format | 2.97.0 | 2026-08-06 | https://github.com/palantir/palantir-java-format/releases/tag/2.97.0 |
| Maven Compiler | 3.16.0 | 2026-09-02 | https://github.com/apache/maven-compiler-plugin/releases/tag/maven-compiler-plugin-3.16.0 |
| Error Prone | 2.50.0 | 2026-06-10 | https://github.com/google/error-prone/releases/tag/v2.50.0 |
| SpotBugs Maven | 4.10.4.1 | 2026-09-05 | https://github.com/spotbugs/spotbugs-maven-plugin/releases/tag/spotbugs-maven-plugin-4.10.4.1 |
| find-sec-bugs | 1.14.0 | 2025-06-17 | https://github.com/find-sec-bugs/find-sec-bugs/releases/tag/version-1.14.0 |
| PIT | 1.30.0 | 2026-08-27 | https://github.com/hcoles/pitest/releases/tag/1.30.0 |
| PIT JUnit 5 bridge | 1.2.2 | 2025-02-24 | https://github.com/pitest/pitest-junit5-plugin/releases/tag/1.2.2 |
| Maven validation tool | 3.9.16 | 2026-05-17 | https://github.com/apache/maven/releases/tag/maven-3.9.16 |

Acceptance: wrapperless and wrapped Maven installs select matching commands;
Gradle and mixed builds retain deliberate selection; POM bytes are preserved;
generated workflow and Makefile contain no Gradle command for Maven; invalid
wrapper selection fails before writing files; real Maven loads the snippet
and executes quality checks, with break/fix proof of an enforced violation.

New workflow pins checked through upstream release APIs on 2026-09-06:
checkout v7.0.1 (2026-07-20), setup-java v6.0.0 (2026-08-24),
setup-go v7.0.0 (2026-07-16), actionlint v1.7.12 (2026-03-30),
OSV-Scanner action v2.5.1 (2026-08-17). Sources are the respective
https://github.com/actions/checkout/releases ,
https://github.com/actions/setup-java/releases ,
https://github.com/actions/setup-go/releases ,
https://github.com/rhysd/actionlint/releases , and
https://github.com/google/osv-scanner-action/releases .

Template CI runs the real quality acceptance suite on JDK 25 with Maven
3.9.16, checking its upstream SHA512 before extracting the binary archive.
The test fixture uses JUnit 5.14.4 (2026-04-26) and Surefire 3.6.0
(2026-09-03), checked against the upstream release pages:
https://github.com/junit-team/junit-framework/releases/tag/r5.14.4 and
https://github.com/apache/maven-surefire/releases/tag/surefire-3.6.0 .
