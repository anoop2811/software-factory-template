# Java pack with Maven

Factory-init selects Maven for `pom.xml` or `mvnw` when no `gradlew` is present.
Use `--java-build-tool maven` in a mixed repository to override detection. An
executable `mvnw` is preferred; without a wrapper, commands use `mvn` from PATH.
`java_build_tool` in factory.yaml records the install choice. To change tools,
rerun init with the explicit option and review the generated diff.

The initial `check_command`, Makefile, and CI run Maven's `verify` lifecycle
and the JUnit dialect gate. They respect the tests and plugins already in your
POM. Installing the pack does not add quality plugins to your POM automatically.

## Arm the quality plugins

Merge the entries in `quality-maven.xml` into `build/plugins` in your shared
parent POM. Do not replace an existing build block or put these only in
`pluginManagement`, which would not activate them. Combine settings when a
plugin is already present; retain your compiler release, annotation processors,
and other project-specific configuration. In a multi-module build, children
must inherit from that parent for its plugins to apply.

Run Maven with JDK 21 or newer for Error Prone. The snippet uses forked javac
with the required module exports, so it needs no overwrite of `.mvn/jvm.config`.
If you use Lombok, MapStruct, or another annotation processor, keep it in the
compiler's `annotationProcessorPaths` alongside Error Prone. See the
[Error Prone installation instructions](https://errorprone.info/docs/installation).

After merging the snippet:

```bash
mvn -B spotless:apply
mvn -B verify
```

Use `./mvnw` in place of `mvn` if you have a wrapper. Spotless runs in validate,
Error Prone runs during compilation, and SpotBugs with find-sec-bugs runs in
verify. Your existing Surefire and Failsafe settings still select and execute
tests. CI runs the same lifecycle plus OSV-Scanner. Check the Maven output for
these plugins before claiming those gates are armed.

PIT is explicit: set `targetClasses` and `targetTests` to your protected packages
and run `make mutate`. The snippet includes PIT's JUnit 5 bridge. Start with
one module using Maven's `-pl`/`-am` options as appropriate for your reactor;
mutation results are not inferred from a passing `verify` run.

## Avoid the Surefire selection trap

`-Dtest` replaces Surefire's configured includes/excludes. An exclusion-only
value such as `-Dtest=!SomeSlowTest` can therefore select classes outside the
normal unit-test naming patterns, including an `E2EIT` intended for Failsafe
after packaging. Prefer persistent exclusions in Surefire's POM configuration.
For a one-off selection, state the normal positive patterns too:

```bash
mvn -B test '-Dtest=Test*,*Test,*Tests,*TestCase,!SomeSlowTest'
```

Adjust these patterns if your project uses different unit-test naming. In a
reactor, `-Dsurefire.failIfNoSpecifiedTests=false` allows modules with no tests
matching this explicit selection; use it only when those empty selections are
intentional. Integration tests belong in the configured Failsafe lifecycle,
normally reached with `mvn verify`. These are Maven's
[test parameter semantics](https://maven.apache.org/surefire/maven-surefire-plugin/test-mojo.html#test)
and [default includes](https://maven.apache.org/surefire/maven-surefire-plugin/examples/inclusion-exclusion.html).

The Java pack is beta following Duke42's real adoption with local Maven
adaptations, reported in [issue #66](https://github.com/anoop2811/software-factory-template/issues/66).
That adoption does not establish compatibility with every Maven project.
The native Maven assets and pinned version sources are specified in
`docs/adr/0045-maven-adoption.md` in the template repository.
