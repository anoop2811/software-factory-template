# Diagnostic probes must stay inside their fixtures

A review probe intended to replace a temporary scratch directory instead renamed
the live checkout. Nested shell quoting turned an intended quoted heredoc into
an unquoted inner heredoc. That shell expanded $PWD and an unset fixture variable
inside the Python source before the fixture was created. The resulting script
contained the live checkout path and an empty symlink target.

Use a single tool-executed quoted heredoc or a structured file patch, without a
second shell parsing layer. Inspect generated fixture scripts before executing
filesystem mutations. Bind every mutation to a newly created temporary root;
never infer isolation from the variable names or the enclosing Python program.

Provenance: observed during the native doctor security review on 2026-09-29.
The review agent acknowledged the unsafe command construction. The parent
confirmed the original directory identity, restored it from the .renamed path,
removed only the identified empty symlink and probe KEEP directory, and confirmed
the probe session had exited. This probe was discarded as review evidence; it
was an orchestration mistake, not a product defect. The existing shell-quoting
and verification rules remain authoritative.
