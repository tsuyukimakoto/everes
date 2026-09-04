#!/usr/bin/env bash
set -euo pipefail

project_root="$(cd "$(dirname "$0")/.." && pwd -P)"
cd "$project_root"

usage() {
  cat <<'EOF'
Usage: mise run release

checkとbuildを通した後に新しいversionを確認し、version更新、Developer ID署名、
notarization、commit、tag、mainとtagのpush、GitHub Releaseの完了確認まで実行します。
EOF
}

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  usage
  exit 0
fi
if [ "$#" -ne 0 ]; then
  usage >&2
  exit 2
fi

for command_name in codesign ditto git gh mise perl python3 security spctl xcrun; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "error: required command not found: $command_name" >&2
    exit 1
  fi
done

if [ "$(uname -s)" != "Darwin" ]; then
  echo "error: release must run on macOS for Developer ID signing and notarization" >&2
  exit 1
fi

codesign_identity="${EVERES_CODESIGN_IDENTITY:-7DA45FCCFCD5D3F77D2D4E319EB3E0E116F1C29D}"
codesign_identifier="${EVERES_CODESIGN_IDENTIFIER:-com.tsuyukimakoto.everes}"
notary_profile="${EVERES_NOTARYTOOL_PROFILE:-everes-notary}"

if ! security find-identity -v -p codesigning | grep -F "$codesign_identity" >/dev/null; then
  echo "error: Developer ID Application identity is unavailable: $codesign_identity" >&2
  exit 1
fi
if ! xcrun notarytool history --keychain-profile "$notary_profile" >/dev/null; then
  echo "error: notarytool keychain profile is unavailable: $notary_profile" >&2
  exit 1
fi

if [ "$(git branch --show-current)" != "main" ]; then
  echo "error: release must run on main" >&2
  exit 1
fi
if ! git remote get-url origin >/dev/null 2>&1; then
  echo "error: git remote 'origin' is not configured" >&2
  exit 1
fi
if [ ! -t 0 ]; then
  echo "error: release requires an interactive terminal for version input" >&2
  exit 1
fi

echo "Running pre-release checks..."
mise run check
mise run build
git diff --check

current_version="$(awk -F'"' '/^version: "/ { print $2; exit }' apm.yml)"
if ! [[ "$current_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: invalid current version in apm.yml: $current_version" >&2
  exit 1
fi

version_manifests=(
  apm.yml
  .apm/packages/everes-claude-hook/apm.yml
  .apm/packages/everes-codex-hook/apm.yml
)
for manifest in "${version_manifests[@]}"; do
  manifest_version="$(awk -F'"' '/^version: "/ { print $2; exit }' "$manifest")"
  if [ "$manifest_version" != "$current_version" ]; then
    echo "error: version mismatch: $manifest has $manifest_version, expected $current_version" >&2
    exit 1
  fi
done

mise_version="$(awk -F'"' '/^EVERES_VERSION = "/ { print $2; exit }' mise.toml)"
if [ "$mise_version" != "$current_version" ]; then
  echo "error: version mismatch: mise.toml has $mise_version, expected $current_version" >&2
  exit 1
fi

IFS=. read -r current_major current_minor current_patch <<<"$current_version"
suggested_version="${current_major}.${current_minor}.$((10#$current_patch + 1))"

echo
echo "Current version: v$current_version"
echo "Changes to publish:"
git status --short
echo
read -r -p "New version [v$suggested_version]: " version_input
version_input="${version_input:-$suggested_version}"
new_version="${version_input#v}"

if ! [[ "$new_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: version must be X.Y.Z or vX.Y.Z" >&2
  exit 1
fi
if [ "$new_version" = "$current_version" ]; then
  echo "error: new version must differ from v$current_version" >&2
  exit 1
fi

IFS=. read -r new_major new_minor new_patch <<<"$new_version"
if ((10#$new_major < 10#$current_major)) ||
  ((10#$new_major == 10#$current_major && 10#$new_minor < 10#$current_minor)) ||
  ((10#$new_major == 10#$current_major && 10#$new_minor == 10#$current_minor && 10#$new_patch <= 10#$current_patch)); then
  echo "error: new version must be greater than v$current_version" >&2
  exit 1
fi

release_tag="v$new_version"
if git show-ref --verify --quiet "refs/tags/$release_tag"; then
  echo "error: tag already exists locally: $release_tag" >&2
  exit 1
fi

gh auth status >/dev/null
git fetch origin main --tags

remote_main="$(git rev-parse refs/remotes/origin/main)"
merge_base="$(git merge-base HEAD refs/remotes/origin/main)"
if [ "$merge_base" != "$remote_main" ]; then
  echo "error: local main is behind or has diverged from origin/main" >&2
  echo "Resolve it before releasing." >&2
  exit 1
fi
if git ls-remote --exit-code --tags origin "refs/tags/$release_tag" >/dev/null 2>&1; then
  echo "error: tag already exists on origin: $release_tag" >&2
  exit 1
fi

version_files=(
  apm.yml
  .apm/packages/everes-claude-hook/apm.yml
  .apm/packages/everes-codex-hook/apm.yml
  mise.toml
  README.md
  docs/requirements.md
  install.sh
)

versions_changed=false
release_committed=false
bundled_binary=".apm/skills/everes-task/bin/everes"
signed_binary="build/everes-darwin-arm64"
notary_archive="build/everes-darwin-arm64-notary.zip"
notary_log="build/notarization-log.json"
binary_backup="$(mktemp)"
cp "$bundled_binary" "$binary_backup"
rollback_version_edits() {
  exit_status=$?
  if [ "$exit_status" -ne 0 ] && [ "$versions_changed" = true ] && [ "$release_committed" = false ]; then
    echo "Restoring version strings to v$current_version after failure..." >&2
    OLD_VERSION="$new_version" NEW_VERSION="$current_version" \
      perl -pi -e 's/\Q$ENV{OLD_VERSION}\E/$ENV{NEW_VERSION}/g' "${version_files[@]}"
    install -m 0755 "$binary_backup" "$bundled_binary"
  fi
  rm -f -- "$binary_backup"
  exit "$exit_status"
}
trap rollback_version_edits EXIT

OLD_VERSION="$current_version" NEW_VERSION="$new_version" \
  perl -pi -e 's/\Q$ENV{OLD_VERSION}\E/$ENV{NEW_VERSION}/g' "${version_files[@]}"
versions_changed=true

echo "Validating release v$new_version..."
mise run check
mise run build
git diff --check

if [ "$(./build/everes version)" != "$new_version" ]; then
  echo "error: built CLI version does not match $new_version" >&2
  exit 1
fi

echo "Signing $signed_binary..."
install -m 0755 build/everes "$signed_binary"
codesign \
  --force \
  --sign "$codesign_identity" \
  --identifier "$codesign_identifier" \
  --options runtime \
  --timestamp \
  "$signed_binary"
codesign --verify --strict --verbose=2 "$signed_binary"

rm -f -- "$notary_archive" "$notary_log"
ditto -c -k --keepParent "$signed_binary" "$notary_archive"

echo "Submitting $notary_archive for notarization..."
if ! notary_output="$(xcrun notarytool submit \
  "$notary_archive" \
  --keychain-profile "$notary_profile" \
  --wait \
  --output-format json)"; then
  printf '%s\n' "$notary_output" >&2
  exit 1
fi
printf '%s\n' "$notary_output"

notary_submission_id="$(printf '%s' "$notary_output" | python3 -c 'import json, sys; print(json.load(sys.stdin).get("id", ""))')"
notary_status="$(printf '%s' "$notary_output" | python3 -c 'import json, sys; print(json.load(sys.stdin).get("status", ""))')"
if [ -z "$notary_submission_id" ]; then
  echo "error: notarytool response did not contain a submission ID" >&2
  exit 1
fi

xcrun notarytool log \
  "$notary_submission_id" \
  --keychain-profile "$notary_profile" \
  "$notary_log"
notary_issue_count="$(python3 -c 'import json, sys; print(len(json.load(open(sys.argv[1])).get("issues") or []))' "$notary_log")"
if [ "$notary_issue_count" != "0" ]; then
  echo "error: notarization log contains $notary_issue_count issue(s)" >&2
  echo "Notary log: $notary_log" >&2
  exit 1
fi
if [ "$notary_status" != "Accepted" ]; then
  echo "error: notarization finished with status $notary_status" >&2
  echo "Notary log: $notary_log" >&2
  exit 1
fi

spctl --assess --type execute --verbose=4 "$signed_binary"
install -m 0755 "$signed_binary" "$bundled_binary"
if ! cmp -s "$signed_binary" "$bundled_binary"; then
  echo "error: bundled CLI differs from the signed and notarized binary" >&2
  exit 1
fi

if [ "$("$signed_binary" version)" != "$new_version" ]; then
  echo "error: signed CLI version does not match $new_version" >&2
  exit 1
fi
if [ "$("$bundled_binary" version)" != "$new_version" ]; then
  echo "error: bundled CLI version does not match $new_version" >&2
  exit 1
fi

git add --all
if git diff --cached --quiet; then
  echo "error: there are no changes to release" >&2
  exit 1
fi

git commit -m "Release $release_tag"
release_committed=true
git tag -a "$release_tag" -m "$release_tag"

echo "Publishing main and $release_tag..."
git push --atomic origin main "$release_tag"

release_sha="$(git rev-parse HEAD)"
run_id=""
for _ in $(seq 1 20); do
  run_id="$(gh run list --workflow release --commit "$release_sha" --event push --limit 1 --json databaseId --jq '.[0].databaseId // empty')"
  if [ -n "$run_id" ]; then
    break
  fi
  sleep 2
done
if [ -z "$run_id" ]; then
  echo "error: release workflow run was not found for $release_tag" >&2
  exit 1
fi

gh run watch "$run_id" --exit-status
gh release view "$release_tag"

rm -f -- "$binary_backup"
trap - EXIT
echo "Published $release_tag"
