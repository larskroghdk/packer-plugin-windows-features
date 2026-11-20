# 🤝 Contributing

Read [AGENTS.md](AGENTS.md) and [INVARIANTS.md](INVARIANTS.md). Keep changes small and focused on feature servicing. Describe the concrete before/after behavior and how it was verified in each pull request.

Run the [local checks](docs/testing.md) before submitting. Add meaningful regression coverage for behavior changes, keeping fake-guest responses consistent with the PowerShell JSON contract. Use Windows PowerShell 5.1 for guest tests. Do not run servicing tests on your workstation or an unapproved VM.

Change `Config` and regenerate the HCL2 spec together. Document options, platform limits and examples. Report JSON incompatible changes require a schema-version increment. Avoid additional runtime dependencies unless the existing SDK, standard library and supported Windows cmdlets cannot solve the problem clearly.

Never include credentials, signing material, sensitive reports or machine-specific variable files. Report vulnerabilities privately to the repository owner; include the affected behavior and a minimal reproduction without secrets. Safety fixes must consider transitive dependencies and active WinRM communication.

Use semantic commit subjects such as `fix: verify compliant requests after reboot` or `docs: explain source requirements`. Maintainers own version tags, release review and publication. No contribution workflow grants permission to publish or alter external infrastructure.
