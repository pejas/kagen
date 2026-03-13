# Maintainer Checklist

Use this checklist whenever a change affects a documented workflow or repository contract.

- Update `README.md` when install steps, command behaviour, or developer verification steps change.
- Update `docs/ARCHITECTURE.md` when responsibilities, transport boundaries, or runtime ownership move.
- Update `docs/CONVENTIONS.md` when contributor rules, validation expectations, or tooling contracts change.
- Update `docs/E2E.md` when the E2E boundary changes.
- Add or update direct tests for user-facing correctness and security contracts.
- Keep `make build`, `make test`, and `make lint` green locally before proposing the change.
- Update CI if the required validation contract changes.
- Re-check Cobra help text for any command whose behaviour or flags changed.
