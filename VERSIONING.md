# Versioning

Wasmbench is currently unreleased. Do not infer a release version from a local
build, changelog entry or passing test suite.

Use semantic version tags (`vMAJOR.MINOR.PATCH`) for reviewed releases and
prerelease suffixes for development milestones. Before 1.0, minor versions may
contain breaking API changes; document them explicitly in CHANGELOG.md.

CLI versions and evidence-contract versions are separate. Protocol, metric,
analysis, exporter and archive version identifiers define recorded semantics.
Change those identifiers when their meaning or compatibility changes; a release
tag never makes old evidence compatible with a changed contract.

Before tagging a release:

1. Review Unreleased changes, compatibility and licenses/attributions.
2. Run the ordinary tests and required native acceptance tests for claimed targets.
3. Verify packaged tools, exact replay, exported data and rendered reports.
4. Move the reviewed changelog entries into a dated version section.
5. Tag the reviewed commit and publish only verified artifacts and checksums.

Tags, pushes, release creation and official benchmark publication are separate
actions. No release or official dataset is created by committing source. Native
acceptance and dedicated-host qualification must be evidenced, not inferred from
cross-compilation or synthetic tests.
