# Releasing MyGo MCP

This repository releases one Go module, `github.com/auron-labs/mygo-mcp`.
Release Please owns release PRs, the changelog, version tracking, and GitHub
releases with `vX.X.X` tags. The initial release is configured as **v0.1.0**.

## First release

1. Commit and merge the local release configuration into `main`. Include a
   releasable Conventional Commit, such as `feat: add headless MyGo MCP harness`.
   Prefer squash merges with Conventional Commit titles.
2. The [release workflow](../.github/workflows/release-please.yml) runs after pushes
   to `main` and opens or updates a release PR when it finds releasable commits.
3. Review the proposed version and generated `CHANGELOG.md`. Run the normal
   [repository checks](../mise.toml) on the release PR's branch:

   ```sh
   mise run check
   mise run test
   mise run build
   ```

4. Merge the release PR after validation. The next workflow run creates the
   `v0.1.0` tag and GitHub release. Consumers can then install it with
   `go get github.com/auron-labs/mygo-mcp@v0.1.0`.

The [manifest](../.release-please-manifest.json) starts empty because no version
has been released. `initial-version` in the [configuration](../release-please-config.json)
selects the first release without pretending that it already exists. Release
Please fills the manifest when proposing that release and uses the released
version for later bumps. The annotated MCP server version in `server.go` is
updated in the same release PR.

## Subsequent releases

Continue merging Conventional Commits into `main`: `fix:` requests a patch,
`feat:` requests a minor, and `feat!:` or a `BREAKING CHANGE:` footer requests a
major. Review and validate each generated release PR before merging it.

## GitHub prerequisites

The workflow uses the repository's `GITHUB_TOKEN` with contents and pull-request
write permissions. Repository or organization policy must allow GitHub Actions
to create pull requests. No custom token or registry credential is configured.

Pull requests created with `GITHUB_TOKEN` do not automatically trigger the normal
pull-request CI workflow. Validate the release branch locally with the commands
above, or use the existing CI workflow's **Run workflow** control and select that
branch. Required branch checks must pass before merging.

Go modules are distributed through version tags; this library has no binary
packaging or registry-upload job. Private repositories still require consumer
Git authentication and `GOPRIVATE` configuration. This setup creates no tags or
releases until the workflow runs on GitHub and a release PR is merged.

See the upstream [manifest configuration](https://github.com/googleapis/release-please/blob/main/docs/manifest-releaser.md)
and [action documentation](https://github.com/googleapis/release-please-action#readme)
for the release mechanism and token behavior.
