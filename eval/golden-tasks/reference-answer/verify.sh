#!/bin/sh
# Acceptance oracle for the reference-answer task. Exit 0 = solved.
# Run from the task working directory (where the runner produced its work).
grep -qx 'FIXED' answer.txt 2>/dev/null
