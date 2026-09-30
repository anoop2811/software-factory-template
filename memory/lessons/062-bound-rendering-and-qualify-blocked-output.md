# Bound rendering and qualify blocked output

Input limits do not bound a rendered report when each row repeats metadata.
Bound aggregate rows and the encoded sink, then include HTML escaping expansion
before allocating the page. Embedded JSON must also remain inert to the HTML
tokenizer; escaping only closing script tags leaves comment/script openers.
The contract remains in docs/adr/0088-go-native-metrics.md:74 and
docs/adr/0088-go-native-metrics.md:96.

Cancellation tests must prove the operation started. Keep a stdout pipe open,
read the first byte, then signal without draining the remaining output. A child
that merely starts successfully is not evidence that a blocked write cancels.
Reuse the existing borrowed-descriptor mechanism instead of creating an
unjoinable writer goroutine.

Provenance: observed 2026-09-30 through compiled metrics acceptance regressions
in `acceptance/native_metrics_safety_test.go`: generated-output cases failed
before bounds were added; blocked stdout exceeded the three-second deadline;
the inert-event case emitted literal `<!--<script>` before full less-than
escaping. The subsequent publication-safety command reported `ok 11.538s`.
