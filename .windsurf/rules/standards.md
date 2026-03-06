# VFS Development Standards

All development on this repository MUST follow the comprehensive guidelines documented in `AGENTS.md` at the repository root.

Key requirements include:
- All new code requires tests with testify/suite
- Use mockery v3 with EXPECT() pattern (no manual mocks)
- All PRs must update CHANGELOG.md under [Unreleased]
- Follow Go version compatibility policy (latest and latest-1 minor versions)
- Run golangci-lint before committing
- Use table-driven tests where possible
- Handle all errors explicitly with wrapped context

See AGENTS.md for complete details on:
- Testing requirements and coverage thresholds (80% total, 63% package, 52% file)
- Code quality and style standards
- CHANGELOG and PR process (including section headings and breaking change rules)
- Go version policy and upgrade procedures
- GitHub Actions maintenance (SHA pinning, 10-day rule)
- Module management for monorepo structure
