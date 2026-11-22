#!/usr/bin/env bash
# Tests verify-deployed-pod.sh with a stub kubectl (ADR-002 D-11, T-1 to T-10,
# plus pre-check, unreadable-Deployment and restart cases). Needs bash and jq.
# Run from anywhere: bash .github/scripts/verify-deployed-pod_test.sh

set -uo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script=$here/verify-deployed-pod.sh
command -v jq >/dev/null || { echo "jq is required" >&2; exit 2; }

root=$(mktemp -d) || exit 2
trap 'rm -rf "$root"' EXIT

# --- stub kubectl ---------------------------------------------------------
# Serves fixtures from $FIXTURES and records every call in $FIXTURES/calls.
mkdir "$root/bin"
cat >"$root/bin/kubectl" <<'STUB'
#!/usr/bin/env bash
echo "$*" >>"$FIXTURES/calls"
args=" $* "
has() { [[ "$args" == *" $1 "* ]]; }
if ! has --request-timeout=20s || ! has "-n seashell"; then
  echo "stub kubectl: missing --request-timeout=20s or -n seashell: $*" >&2
  exit 99
fi
if has get && has deployment/gogear-api && has "-o json"; then
  if [ -f "$FIXTURES/deployment.err" ]; then cat "$FIXTURES/deployment.err" >&2; exit 1; fi
  cat "$FIXTURES/deployment.json"
elif has get && has pods && has "-o json"; then
  cat "$FIXTURES/pods.json"
elif has get && has pods && has "-o wide"; then
  echo "NAME READY STATUS (stub)"
elif has describe; then
  cat "$FIXTURES/describe.txt"
elif has logs; then
  if [ -f "$FIXTURES/logs.rc" ]; then cat "$FIXTURES/logs.err" >&2; exit "$(cat "$FIXTURES/logs.rc")"; fi
  cat "$FIXTURES/logs.txt"
else
  echo "stub kubectl: unexpected call: $*" >&2
  exit 98
fi
STUB
chmod +x "$root/bin/kubectl"

# --- fixture builders -----------------------------------------------------
hex() { printf '%s%0*d' "$1" $((64 - ${#1})) 0; } # 64-hex digest from a prefix
REPO=docker.io/bateau/gogear-api
D_NEW=sha256:$(hex 5b444c61) # digest of this release (v4.9.5)
D_OLD=sha256:$(hex 5978fe2a) # digest of the previous release (v4.9.4)
D_OTHER=sha256:$(hex deadbeef)
IMG_NEW=$REPO:v4.9.5@$D_NEW
IMG_OLD=$REPO:v4.9.4@$D_OLD

# deployment <image> [generation observed updated ready replicas]
deployment() {
  jq -n --arg image "$1" --argjson gen "${2:-3}" --argjson obs "${3:-3}" \
    --argjson upd "${4:-1}" --argjson ready "${5:-1}" --argjson repl "${6:-1}" '
    {metadata: {name: "gogear-api", generation: $gen},
     spec: {template: {spec: {containers: [{name: "sidecar", image: "x"}, {name: "gogear-api", image: $image}]}}},
     status: {observedGeneration: $obs, updatedReplicas: $upd, readyReplicas: $ready, replicas: $repl}}'
}

# pod <name> <hash> <image> <phase> <ready> <restarts> <imageID> [owners-json]
pod() {
  local owners=${8:-}
  [ -n "$owners" ] || owners=$(jq -n --arg n "gogear-api-$2" '[{kind: "ReplicaSet", name: $n, controller: true}]')
  jq -n --arg name "$1" --arg hash "$2" --arg image "$3" --arg phase "$4" \
    --argjson ready "$5" --argjson rc "$6" --arg iid "$7" --argjson owners "$owners" '
    {metadata: {name: $name, labels: {app: "gogear-api", "pod-template-hash": $hash}, ownerReferences: $owners},
     spec: {containers: [{name: "sidecar", image: "x"}, {name: "gogear-api", image: $image}]},
     status: {phase: $phase, containerStatuses: [
       {name: "sidecar", ready: false, restartCount: 7, imageID: "docker.io/other/sidecar@sha256:0"},
       {name: "gogear-api", ready: $ready, restartCount: $rc, imageID: $iid}]}}'
}

pods() { jq -s '{items: .}'; }

LOG_OK='{"level":"info","message":"login","username":"alice-secret"}
{"level":"error","message":"boom","error":"db locked","username":"alice-secret"}
gogear-api migrations: version=10 dirty=false'

# --- harness --------------------------------------------------------------
pass_count=0
fail_count=0
n=0

# new_case: fresh fixtures with the consistent T-1 state.
new_case() {
  n=$((n + 1))
  case_dir=$root/case$n
  mkdir -p "$case_dir/fx" "$case_dir/work/migrations"
  touch "$case_dir/work/migrations/000009_a.up.sql" "$case_dir/work/migrations/000009_a.down.sql" \
    "$case_dir/work/migrations/000010_b.up.sql" "$case_dir/work/migrations/000010_b.down.sql"
  fx=$case_dir/fx
  deployment "$IMG_NEW" >"$fx/deployment.json"
  pod gogear-api-abc-1 abc "$IMG_NEW" Running true 0 "$REPO@$D_NEW" | pods >"$fx/pods.json"
  echo "Name: gogear-api-abc-1" >"$fx/describe.txt"
  printf '%s\n' "$LOG_OK" >"$fx/logs.txt"
  expected_image=$IMG_NEW
  build_digest=$D_NEW
}

# run_case: run the script in the case's work dir with the stub first on PATH.
run_case() {
  (cd "$case_dir/work" && FIXTURES=$fx PATH="$root/bin:$PATH" VERIFY_WAIT_SECONDS=0 \
    EXPECTED_IMAGE=$expected_image BUILD_DIGEST=$build_digest \
    bash "$script") >"$case_dir/out" 2>&1
  rc=$?
}

ok=1
check() { # description, condition result
  if [ "$2" != 0 ]; then
    ok=0
    echo "    FAILED: $1"
  fi
}
want_rc() { [ "$rc" = "$1" ]; check "exit code $rc, want $1" $?; }
want_line() { grep -Eq -- "$1" "$case_dir/out"; check "output has /$1/" $?; }
want_no() { ! grep -Eq -- "$1" "$case_dir/out"; check "output lacks /$1/" $?; }
want_count() { [ "$(grep -Ec -- "$1" "$case_dir/out")" = "$2" ]; check "output has $2 of /$1/" $?; }
want_calls() { [ "$(grep -Ec -- "$1" "$fx/calls" 2>/dev/null)" = "$2" ]; check "$2 kubectl calls matching /$1/" $?; }

finish() { # case name
  # Privacy: nothing but level, message and error of a log line may be printed.
  want_no 'username|alice-secret'
  if [ "$ok" = 1 ]; then
    pass_count=$((pass_count + 1))
    echo "ok   $1 (exit $rc)"
  else
    fail_count=$((fail_count + 1))
    echo "FAIL $1 (exit $rc); output:"
    sed 's/^/    | /' "$case_dir/out"
  fi
  ok=1
}

# --- cases ----------------------------------------------------------------

# T-1 normal
new_case; run_case
want_rc 0
want_line '^R-1 PASS'; want_line '^I-1 PASS'; want_line '^P-1 PASS'; want_line '^I-2 PASS'
want_line '^I-3 PASS'; want_line '^H-1 PASS'; want_line '^I-4 PASS'; want_line '^M-1 PASS'
want_line '^imageID digest equals build digest'
want_line '^Verified: tag v4.9.5, digest sha256:5b444c61'
want_count 'Diagnostics' 0
want_calls 'get deployment/gogear-api .*-o json' 1
want_calls 'get pods .*-o json' 1
finish "T-1 normal"

# T-2 content-identical release (run 45048563144)
new_case
pod gogear-api-abc-1 abc "$IMG_NEW" Running true 0 "$REPO@$D_OLD" | pods >"$fx/pods.json"
run_case
want_rc 0
want_line '^I-4 PASS'
want_line "^imageID digest $D_OLD differs from build digest; the runtime names identical content by an earlier digest"
want_no '^[A-Z]-[0-9] FAIL'
finish "T-2 content-identical release"

# T-3 wrong revision: the live pod is the previous ReplicaSet's
new_case
pod gogear-api-old-1 old "$IMG_OLD" Running true 0 "$REPO@$D_OLD" | pods >"$fx/pods.json"
printf '%s\n' "$LOG_OK" | sed 's/version=10/version=9/' >"$fx/logs.txt"
run_case
want_rc 1
want_line '^I-1 PASS'; want_line '^I-2 FAIL'
want_line '^M-1 '
finish "T-3 wrong revision"

# T-4 rollout not caught up
new_case
deployment "$IMG_NEW" 4 3 >"$fx/deployment.json"
run_case
want_rc 1
want_line '^R-1 FAIL'
want_line '^I-2 PASS'
finish "T-4 rollout not caught up"

# T-5 spec digest mismatch (template and pod name another digest)
new_case
deployment "$REPO:v4.9.5@$D_OTHER" >"$fx/deployment.json"
pod gogear-api-abc-1 abc "$REPO:v4.9.5@$D_OTHER" Running true 0 "$REPO@$D_NEW" | pods >"$fx/pods.json"
run_case
want_rc 1
want_line '^I-1 FAIL'; want_line '^I-2 FAIL'
finish "T-5 spec digest mismatch"

# T-6 hand-made pod: no ReplicaSet controller owner
new_case
pod gogear-api-abc-1 abc "$IMG_NEW" Running true 0 "$REPO@$D_NEW" '[]' | pods >"$fx/pods.json"
run_case
want_rc 1
want_line '^I-3 FAIL'; want_line '^I-2 PASS'
finish "T-6 hand-made pod"

# T-7 foreign image
new_case
pod gogear-api-abc-1 abc "$IMG_NEW" Running true 0 "docker.io/other/x@$D_NEW" | pods >"$fx/pods.json"
run_case
want_rc 1
want_line '^I-4 FAIL'; want_line '^I-2 PASS'
finish "T-7 foreign image"

# T-8 migration lag with identity failure
new_case
pod gogear-api-abc-1 abc "$REPO:v4.9.5@$D_OTHER" Running true 0 "$REPO@$D_NEW" | pods >"$fx/pods.json"
printf '%s\n' "$LOG_OK" | sed 's/version=10/version=9/' >"$fx/logs.txt"
run_case
want_rc 1
want_line '^I-2 FAIL'; want_line '^M-1 FAIL'
want_count '^::group::Diagnostics' 1
want_line '^error boom db locked$' # filtered error line reaches the output
want_no 'needs get on pods/log'
finish "T-8 migration lag with identity failure"

# T-9 no single pod: only a Failed one
new_case
pod gogear-api-abc-1 abc "$IMG_NEW" Failed false 0 "$REPO@$D_NEW" | pods >"$fx/pods.json"
run_case
want_rc 1
want_line '^P-1 FAIL'
for id in I-2 I-3 H-1 I-4 M-1; do want_line "^$id NOT RUN \(no single live pod\)"; done
want_line '^R-1 PASS'; want_line '^I-1 PASS'
want_calls ' logs ' 1 # only the diagnostics read the log, M-1 did not run
finish "T-9 no single pod"

# T-10 unreadable log
new_case
echo 1 >"$fx/logs.rc"
echo 'Error from server (Forbidden): pods "gogear-api-abc-1" is forbidden: cannot get resource "pods/log"' >"$fx/logs.err"
run_case
want_rc 1
want_line '^M-1 FAIL: could not read pod logs .*Forbidden.*The deploy service account needs get on pods/log\.'
want_line '^I-2 PASS'
finish "T-10 unreadable log"

# T-11 pre-checks fail fast, before any kubectl call
new_case
build_digest=sha256:abc
run_case
want_rc 1
want_line "Build digest 'sha256:abc' is malformed"
[ ! -e "$fx/calls" ]; check "no kubectl call" $?
finish "T-11 malformed build digest"

new_case
expected_image=$IMG_OLD
run_case
want_rc 1
want_line 'does not end with @'
[ ! -e "$fx/calls" ]; check "no kubectl call" $?
finish "T-11b expected image without the build digest"

# T-12 Deployment unreadable: R-1 and I-1 FAIL, pod checks still run
new_case
echo 'Error from server (Forbidden): deployments.apps "gogear-api" is forbidden' >"$fx/deployment.err"
run_case
want_rc 1
want_line '^R-1 FAIL: could not read Deployment'; want_line '^I-1 FAIL'
want_line '^I-2 PASS'; want_line '^M-1 PASS'
finish "T-12 unreadable Deployment"

# T-13 restarted container: H-1 FAIL, previous-container diagnostics
new_case
pod gogear-api-abc-1 abc "$IMG_NEW" Running true 2 "$REPO@$D_NEW" | pods >"$fx/pods.json"
run_case
want_rc 1
want_line '^H-1 FAIL: .*restartCount=2'
want_line 'previous container, filtered'
want_calls ' logs .*--previous' 1
finish "T-13 restarted container"

echo
echo "$pass_count passed, $fail_count failed"
[ "$fail_count" = 0 ]
