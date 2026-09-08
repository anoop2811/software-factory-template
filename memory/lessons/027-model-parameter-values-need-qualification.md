# Qualify model parameter values, not only parameter names

OpenRouter endpoint metadata listing `reasoning` did not prove GLM 5.3 Flash
accepted `none`. Retaining the prior model's setting introduced an incompatible
request. Check the selected model's documented value domain when changing models;
request-capture tests prove what is sent, not live provider acceptance.

Provenance: https://docs.z.ai/guides/capabilities/thinking fetched 2026-09-08 UTC;
https://github.com/anoop2811/software-factory-template/actions/runs/34178226454
returned HTTP 400 in 0.130382 seconds. The discarded server response prevents
claiming its exact error message. Decision 62 is the configuration source of truth.
