#!/usr/bin/env bash
# Build the package for testers: a ZimaOS compose that pulls the public image
# from ghcr.io, and the guides (de, en) with the test list.
#
#   deploy/testers/make-package.sh <version>          build image + package
#   deploy/testers/make-package.sh <version> --push   also push the image (needs
#                                                     gh auth with write:packages)
#
# Out: deploy/testers/out/movies-cast-<version>/ and movies-cast-<version>.zip
#
# Only a committed tree is built (like build-on-host.sh); the image carries
# <version>+<commit> in -version and the commit as an OCI label.
set -euo pipefail

version="${1:?usage: make-package.sh <version> [--push]}"
push=0
[[ "${2:-}" == "--push" ]] && push=1
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "version must be x.y.z: $version" >&2; exit 1; }

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
image="ghcr.io/chicohaager/movies-cast:$version"
name="movies-cast-$version"
out="$here/out/$name"

dirty="$(cd "$repo" && git status --porcelain --untracked-files=normal -- cmd internal go.mod go.sum Dockerfile deploy/zimaos deploy/testers)"
if [[ -n "$dirty" ]]; then
  echo "uncommitted changes, commit first:" >&2
  echo "$dirty" >&2
  exit 1
fi
commit="$(cd "$repo" && git rev-parse --short=12 HEAD)"

echo "== 1/5 check.sh"
# shellcheck disable=SC2086 # CHECK_ARGS is empty or one flag
"$repo/check.sh" ${CHECK_ARGS:-}

echo "== 2/5 static build ($version+$commit)"
ctx="$(mktemp -d)"
trap 'rm -rf "$ctx"' EXIT
(cd "$repo" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$version+$commit" -o "$ctx/movies-cast" ./cmd/movies-cast)
cp "$repo/Dockerfile" "$ctx/Dockerfile"
"$ctx/movies-cast" -version

echo "== 3/5 image $image"
docker build -q --platform linux/amd64 \
  --label "org.opencontainers.image.title=movies-cast" \
  --label "org.opencontainers.image.version=$version" \
  --label "org.opencontainers.image.revision=$commit" \
  --label "org.opencontainers.image.description=Play IceWhale Movies on a DLNA TV, controlled from the phone (ZimaOS app)" \
  -t "$image" "$ctx"
docker run --rm --user 65534:65534 --cap-drop ALL --security-opt no-new-privileges:true --read-only "$image" -version

echo "== 4/5 push"
if [[ "$push" == 1 ]]; then
  docker push -q "$image"
  # A push can succeed into a private package (GitHub's default): testers then
  # get 401. Ask the registry the way an anonymous client does.
  anon_repo="${image#ghcr.io/}"; anon_repo="${anon_repo%%:*}"
  anon="$(curl -s -o /dev/null -w '%{http_code}' "https://ghcr.io/token?scope=repository:$anon_repo:pull")"
  if [[ "$anon" != 200 ]]; then
    echo "the package is not public (anonymous pull token: HTTP $anon)." >&2
    echo "GitHub: Package settings -> Change visibility -> Public, then run again." >&2
    exit 1
  fi
  echo "anonymous pull allowed (token HTTP 200)"
  # The test of what a tester gets: drop the local copy and pull it back.
  docker rmi -f "$image" >/dev/null
  docker pull -q "$image"
  docker run --rm "$image" -version
else
  echo "skipped (without --push the package points at an image that may not exist yet)"
fi

echo "== 5/5 package $out"
rm -rf "${out:?}"
mkdir -p "$out"
python3 "$here/render.py" "$repo" "$out" "$version" "$image"
(cd "$out" && sha256sum -- * > SHA256SUMS)
(cd "$here/out" && rm -f "$name.zip" && zip -qr "$name.zip" "$name")
# What leaves is the zip: scan an unpacked copy (deploy/testers/out is
# git-ignored, so a scan of the tree does not see it).
scanner="$HOME/.claude/bin/check-public-data.py"
if [[ -f "$scanner" ]]; then
  scan="$(mktemp -d)"
  unzip -q "$here/out/$name.zip" -d "$scan"
  python3 "$scanner" "$scan" || { rm -rf "$scan"; echo "private data in the package" >&2; exit 1; }
  rm -rf "$scan"
fi
ls -l "$out" "$here/out/$name.zip"
