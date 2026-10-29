#!/usr/bin/env bash
# Build the movies-cast image on a ZimaOS host and put it into the host-local
# registry, where the ZimaOS installer pulls it from (ZIMAOS-KNOWLEDGE "Weg 3").
#
#   deploy/zimaos/build-on-host.sh <ssh-user>@<host> [version]   (-version prints <version>+<commit>)
#
# Steps, each with its witness:
#   1. go vet + go test, then a static build here (CGO_ENABLED=0); sha256 printed
#   2. binary + Dockerfile to <host>:/DATA/AppData/movies-cast-build/ — next to,
#      never inside, /DATA/AppData/movies-cast (a ZimaOS uninstall deletes that)
#   3. sha256 compared on the host before the build
#   4. docker build, push to 127.0.0.1:5000, rmi, pull back (the installer's path)
#
# SSHPASS_FILE may name a file with the ssh password (sshpass -f); otherwise
# plain ssh (keys). The script never prints it.
#
# Update of a running install afterwards, on the host:
#   cd /var/lib/casaos/apps/movies-cast && sudo docker compose up -d --force-recreate movies-cast
set -euo pipefail

target="${1:?usage: build-on-host.sh <ssh-user>@<host> [version]}"
version="${2:-0.1.0}"
build_dir="/DATA/AppData/movies-cast-build"
image="127.0.0.1:5000/movies-cast:local"
repo="$(cd "$(dirname "$0")/../.." && pwd)"
out="$(mktemp -d)"
trap 'rm -rf "$out"' EXIT

ssh_cmd=(ssh -o BatchMode=no)
scp_cmd=(scp -q)
if [[ -n "${SSHPASS_FILE:-}" ]]; then
  ssh_cmd=(sshpass -f "$SSHPASS_FILE" ssh)
  scp_cmd=(sshpass -f "$SSHPASS_FILE" scp -q)
fi
# DOCKER_CONFIG: on ZimaOS a non-root shell must not use /DATA/.docker (KB §2.3/§3.6)
# shellcheck disable=SC2016 # $USER must expand on the host, not here
remote_docker='export DOCKER_CONFIG=/tmp/dc-$USER; mkdir -p "$DOCKER_CONFIG";'

# Only a committed state is built, and -version names its commit: 0.20.2 ran on
# .143 from an uncommitted tree, and nothing said which code it was (2026-10-29).
dirty="$(cd "$repo" && git status --porcelain --untracked-files=normal -- cmd internal go.mod go.sum Dockerfile)"
if [[ -n "$dirty" ]]; then
  echo "uncommitted changes, commit first:" >&2
  echo "$dirty" >&2
  exit 1
fi
version="$version+$(cd "$repo" && git rev-parse --short=12 HEAD)"

echo "== 1/4 test + static build ($version)"
(cd "$repo" && go vet ./... && go test -count=1 ./... >/dev/null)
(cd "$repo" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$out/movies-cast" ./cmd/movies-cast)
cp "$repo/Dockerfile" "$out/Dockerfile"
chmod 0755 "$out/movies-cast"
chmod 0644 "$out/Dockerfile"
local_sum="$(sha256sum "$out/movies-cast" | cut -d' ' -f1)"
echo "binary sha256 $local_sum"
"$out/movies-cast" -version

echo "== 2/4 docker usable as $target?"
"${ssh_cmd[@]}" "$target" "$remote_docker docker info --format '{{.ServerVersion}} {{.Architecture}}'"

echo "== 3/4 upload to $target:$build_dir/"
"${ssh_cmd[@]}" "$target" "mkdir -p $build_dir"
"${scp_cmd[@]}" "$out/movies-cast" "$out/Dockerfile" "$target:$build_dir/"
remote_sum="$("${ssh_cmd[@]}" "$target" "sha256sum $build_dir/movies-cast | cut -d' ' -f1")"
if [[ "$remote_sum" != "$local_sum" ]]; then
  echo "sha256 mismatch on host: $remote_sum" >&2
  exit 1
fi
echo "sha256 on host matches"

echo "== 4/4 build, push, pull back"
"${ssh_cmd[@]}" "$target" "$remote_docker
  set -e
  cd $build_dir
  # No pipe into tail: it would hand back tail's exit code instead of docker's.
  step() { if \"\$@\" > step.log 2>&1; then tail -1 step.log; else tail -20 step.log; echo \"FAILED: \$*\"; exit 1; fi; }
  step docker build -t $image .
  curl -s -m5 -o /dev/null -w 'registry /v2/: HTTP %{http_code}\n' http://127.0.0.1:5000/v2/
  step docker push $image
  step docker rmi $image
  step docker pull $image
  curl -s http://127.0.0.1:5000/v2/movies-cast/tags/list; echo
  # Same restrictions as the compose: a plain docker run passed while the
  # container under cap_drop ALL could not exec a 0700 binary.
  docker run --rm --user 65534:65534 --cap-drop ALL --security-opt no-new-privileges:true --read-only $image -version"
