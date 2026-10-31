#!/usr/bin/env bash
# Build the movies-cast image on a ZimaOS host and put it into the host-local
# registry, where the ZimaOS installer pulls it from (ZIMAOS-KNOWLEDGE "Weg 3").
#
#   deploy/zimaos/build-on-host.sh <ssh-user>@<host> [version] [--switch]   (-version prints <version>+<commit>)
#
# Steps, each with its witness:
#   1. check.sh (vet, tests, browser tests, public-data scan; CHECK_ARGS=--no-e2e
#      skips the browser tests), then a static build here (CGO_ENABLED=0); sha256 printed
#   2. binary + Dockerfile to <host>:/DATA/AppData/movies-cast-build/ — next to,
#      never inside, /DATA/AppData/movies-cast (a ZimaOS uninstall deletes that)
#   3. sha256 compared on the host before the build
#   4. docker build, push to 127.0.0.1:5000, rmi, pull back (the installer's path)
#   5. only with --switch: as root (sudo), switch-image.sh points the installed
#      compose (/var/lib/casaos/apps/movies-cast) at this image, with a backup in
#      /DATA/AppData/movies-cast-build/compose-backups and proof that no other
#      line changed (it prints counts and the image lines, never other lines:
#      the installed compose holds the real values); then the container is
#      recreated and -version in it must name this build. Back to a release:
#      the tile's "Manage" -> image and tag, or the backup.
#
# SSHPASS_FILE may name a file with the ssh password (sshpass -f); otherwise
# plain ssh (keys). The script never prints it.
#
# sudo reads the password from SSHPASS_FILE on stdin; the root script travels as
# a base64 argument, never on that stdin. Without SSHPASS_FILE: sudo -n.
set -euo pipefail

switch=0
args=()
for a in "$@"; do
  if [[ "$a" == "--switch" ]]; then switch=1; else args+=("$a"); fi
done
target="${args[0]:?usage: build-on-host.sh <ssh-user>@<host> [version] [--switch]}"
version="${args[1]:-0.1.0}"
build_dir="/DATA/AppData/movies-cast-build"
image="127.0.0.1:5000/movies-cast:local"
repo="$(cd "$(dirname "$0")/../.." && pwd)"
out="$(mktemp -d)"
wt="$(mktemp -d)"
trap 'rm -rf "$out"; git -C "$repo" worktree remove --force "$wt" >/dev/null 2>&1 || rm -rf "$wt"' EXIT

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
# the test NAS from an uncommitted tree, and nothing said which code it was (2026-10-29).
# check.sh and go build run in a worktree of HEAD, so uncommitted files (another
# session's, say) are neither tested nor shipped; they are listed.
dirty="$(cd "$repo" && git status --porcelain --untracked-files=normal)"
if [[ -n "$dirty" ]]; then
  echo "not in this build (uncommitted):" >&2
  echo "$dirty" >&2
fi
version="$version+$(cd "$repo" && git rev-parse --short=12 HEAD)"
git -C "$repo" worktree add --detach "$wt" HEAD >/dev/null
[[ -d "$repo/e2e/node_modules" ]] && ln -s "$repo/e2e/node_modules" "$wt/e2e/node_modules"

echo "== 1/5 check.sh + static build ($version, worktree of HEAD)"
# shellcheck disable=SC2086 # CHECK_ARGS is empty or one flag
"$wt/check.sh" ${CHECK_ARGS:-}
(cd "$wt" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$out/movies-cast" ./cmd/movies-cast)
cp "$wt/Dockerfile" "$out/Dockerfile"
cp "$wt/deploy/zimaos/switch-image.sh" "$out/switch-image.sh"
chmod 0755 "$out/movies-cast"
chmod 0644 "$out/Dockerfile" "$out/switch-image.sh"
local_sum="$(sha256sum "$out/movies-cast" | cut -d' ' -f1)"
echo "binary sha256 $local_sum"
"$out/movies-cast" -version

echo "== 2/5 docker usable as $target?"
"${ssh_cmd[@]}" "$target" "$remote_docker docker info --format '{{.ServerVersion}} {{.Architecture}}'"

echo "== 3/5 upload to $target:$build_dir/"
"${ssh_cmd[@]}" "$target" "mkdir -p $build_dir"
"${scp_cmd[@]}" "$out/movies-cast" "$out/Dockerfile" "$out/switch-image.sh" "$target:$build_dir/"
local_sums="$(cd "$out" && sha256sum movies-cast Dockerfile switch-image.sh)"
remote_sums="$("${ssh_cmd[@]}" "$target" "cd $build_dir && sha256sum movies-cast Dockerfile switch-image.sh")"
if [[ "$remote_sums" != "$local_sums" ]]; then
  echo "sha256 mismatch on host:" >&2
  echo "$remote_sums" >&2
  exit 1
fi
echo "sha256 on host matches (binary, Dockerfile, switch-image.sh)"

echo "== 4/5 build, push, pull back"
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

if [[ "$switch" != 1 ]]; then
  echo "== 5/5 switch skipped (without --switch the install keeps its image)"
  exit 0
fi
echo "== 5/5 switch the install to $image and recreate"
app_dir="/var/lib/casaos/apps/movies-cast"
root_script="exec </dev/null
set -e
export DOCKER_CONFIG=/tmp/dc-root; mkdir -p \"\$DOCKER_CONFIG\"
sh $build_dir/switch-image.sh $app_dir/docker-compose.yml $image $build_dir/compose-backups
cd $app_dir
docker compose up -d --force-recreate movies-cast > $build_dir/switch.log 2>&1 || { tail -20 $build_dir/switch.log; exit 1; }
s=none
for i in \$(seq 1 30); do
  s=\$(docker inspect -f '{{.State.Health.Status}}' movies-cast 2>/dev/null || echo none)
  [ \"\$s\" = healthy ] && break
  sleep 2
done
echo \"health \$s after \$i tries\"
v=\$(docker exec movies-cast /movies-cast -version)
echo \"running: \$v\"
[ \"\$s\" = healthy ] && [ \"\$v\" = 'movies-cast $version' ]"
b64="$(printf '%s' "$root_script" | base64 -w0)"
if [[ -n "${SSHPASS_FILE:-}" ]]; then
  "${ssh_cmd[@]}" "$target" "sudo -S -p '' sh -c \"\$(printf %s $b64 | base64 -d)\"" < "$SSHPASS_FILE"
else
  "${ssh_cmd[@]}" "$target" "sudo -n sh -c \"\$(printf %s $b64 | base64 -d)\"" < /dev/null
fi
echo "switched: $target runs $version"
