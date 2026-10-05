# Releasing the Viettel Cloud Go SDK

All SDK packages belong to the single module
`github.com/viettelcloud-oss/sdks/go` and are released together.

Because the module lives in the repository's `go/` subdirectory, release tags
must use the subdirectory prefix:

```text
go/v1.2.3
```

Releases are cut by the `release` Jenkins pipeline
([`.jenkins/release.yml`](../.jenkins/release.yml)), which runs
semantic-release with the plugin configuration in [`.releaserc`](../.releaserc).
It derives the version from the conventional-commit titles since the last tag,
so the version is a consequence of what was merged rather than a choice made at
release time. `tagFormat` there is `go/v${version}`, which is what keeps the
subdirectory prefix above.

`commit-analyzer` adds one `releaseRules` entry on top of its defaults, so
`revert:` can ship on its own instead of requiring an unrelated `fix:` to ride
along:

| Commit | Bump |
|---|---|
| `feat(go)!: …`, or a `BREAKING CHANGE:` footer | major |
| `feat: …` | minor |
| `fix: …`, `perf: …`, `revert: …` | patch |
| anything else (`chore`, `ci`, `docs`, `refactor`, `test`, …) | no release |

Mark a breaking change with `!` — there is no `break:` type here.

## Preparing a release

1. Make sure the bindings are current — regenerate from the live spec and check
   that nothing changes:

   ```bash
   make all FETCH_SPEC=1
   git status --short
   ```

   If it reports changes, merge the regeneration as its own change before
   releasing — a release should not be the commit that also rewrites the
   generated code.

2. Update `SpecVersion` in `core/version.go` if the upstream spec moved.
   It tracks the backend's OpenAPI `info.version`, not this repository's
   commits, so nothing can infer it:

   ```bash
   make spec-version   # prints upstream info.version next to core.SpecVersion
   ```

   `Version` is *not* updated by hand — semantic-release rewrites it during the
   release (see the `@semantic-release/exec` step in `.releaserc`).

3. Confirm the module is healthy:

   ```bash
   make verify
   ```

## Cutting the release

4. Run the `release` pipeline in Jenkins. It defaults to `option: --dry-run`;
   before calculating a release, the pipeline runs `make verify` again on the
   exact checkout it will tag. Read the log and confirm the version it computed
   and the notes it generated.

5. Re-run it with `option` cleared. semantic-release then:

   - bumps `Version` in `go/core/version.go`,
   - commits it as `chore(release): bump version to go/vX.Y.Z`,
   - pushes the `go/vX.Y.Z` tag and creates the GitLab release.

   There is no `CHANGELOG.md`; the generated notes live on the GitLab release.

6. Record the upstream spec version in the release notes alongside the SDK
   version, since the two move independently:

   ```text
   SDK version: 1.2.3
   Upstream OpenAPI info.version: <core.SpecVersion>
   ```

Never tag service packages independently; the root module provides one
coherent dependency version for `core` and all services.

Generated bindings and facade files are committed, so consumers never need
`oapi-codegen` or the upstream spec to use the SDK.
