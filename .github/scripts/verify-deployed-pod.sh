#!/usr/bin/env bash
# Prove that the pod serving gogear-api is the one this release deployed
# (ADR-002 D-11). Reads the Deployment once and the app=gogear-api pods once,
# reports every check as PASS, FAIL or NOT RUN, and exits 1 if any check is
# not PASS.
#
# Inputs (environment):
#   EXPECTED_IMAGE      exactly the k8s-deploy `images:` string,
#                       docker.io/bateau/gogear-api:<tag>@<build digest>
#   BUILD_DIGEST        sha256:<64 hex>, the digest the build pushed
#   VERIFY_WAIT_SECONDS seconds to wait before reading the cluster (default 180)
#
# Run from the root of the checked-out release tag: the expected migration
# version is the highest number in migrations/*.up.sql.

set -uo pipefail

NAMESPACE=seashell
DEPLOYMENT=gogear-api
CONTAINER=gogear-api
POD_LABEL=app=gogear-api
IMAGE_ID_RE='^(docker\.io/)?bateau/gogear-api@sha256:[0-9a-f]{64}$'

die() {
  echo "::error::$1"
  exit 1
}

EXPECTED_IMAGE=${EXPECTED_IMAGE:-}
BUILD_DIGEST=${BUILD_DIGEST:-}
wait_seconds=${VERIFY_WAIT_SECONDS:-180}

# Pre-checks: nothing else can be checked without them.
[[ "$BUILD_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]] || die "Build digest '$BUILD_DIGEST' is malformed"
[ -n "$EXPECTED_IMAGE" ] || die "EXPECTED_IMAGE is empty"
[[ "$EXPECTED_IMAGE" == *"@$BUILD_DIGEST" ]] || die "Expected image '$EXPECTED_IMAGE' does not end with @$BUILD_DIGEST"
[[ "$wait_seconds" =~ ^[0-9]+$ ]] || die "VERIFY_WAIT_SECONDS '$wait_seconds' is not a number"

expected=
for f in migrations/*.up.sql; do
  [ -e "$f" ] || continue
  n=${f##*/}
  n=$((10#${n%%_*}))
  if [ -z "$expected" ] || [ "$n" -gt "$expected" ]; then expected=$n; fi
done
[ -n "$expected" ] || die "No migrations/*.up.sql found in the checked-out tag"

k() { kubectl --request-timeout=20s -n "$NAMESPACE" "$@"; }

tmp=$(mktemp -d) || die "mktemp failed"
trap 'rm -rf "$tmp"' EXIT

failed=0
pass() { echo "$1 PASS: $2"; }
fail() {
  echo "$1 FAIL: $2"
  failed=1
}
not_run() {
  echo "$1 NOT RUN ($2)"
  failed=1
}

# First line of a kubectl error file, bounded.
first_error() {
  { grep -m1 -iE '^error' "$1" || head -n 1 "$1"; } 2>/dev/null | cut -c1-200
}

# Select the container by name, never by index.
spec_image() { # json-on-stdin; path prefix as $1
  jq -r --arg c "$CONTAINER" "first($1.containers[]? | select(.name == \$c) | .image) // \"\""
}

# Keep only what cannot identify a user: error-level lines of the JSON app
# log reduced to level, message and error (never the other fields, which can
# carry request or user data), non-JSON crash output, and the migrations
# line. The repository is public.
safe_logs() {
  local raw
  raw=$(cat)
  {
    jq -R -r 'fromjson? | select(type == "object" and ((.level // "") | IN("error", "dpanic", "panic", "fatal")))
              | "\(.level) \(.message // "") \(.error // "")"' <<<"$raw" || true
    grep -v '^{' <<<"$raw" | grep -E 'panic:|fatal error:|Failed to|gogear-api migrations:' || true
  } | cut -c1-300 | tail -n 20
}

diagnostics() {
  echo "::group::Diagnostics"
  k get pods -l "$POD_LABEL" -o wide || true
  local p rc
  if [ -n "$pods_json" ]; then
    for p in $(jq -r '.items[].metadata.name' <<<"$pods_json" | head -n 5); do
      echo "--- describe $p"
      k describe "pod/$p" | tail -n 60 || true
      echo "--- error-level log lines $p (current, filtered)"
      k logs "$p" --tail=500 2>/dev/null | safe_logs || true
      rc=$(jq -r --arg p "$p" --arg c "$CONTAINER" \
        'first(.items[] | select(.metadata.name == $p) | .status.containerStatuses[]? | select(.name == $c) | .restartCount) // 0' \
        <<<"$pods_json")
      if [ "$rc" != "0" ]; then
        echo "--- error-level log lines $p (previous container, filtered)"
        k logs "$p" --previous --tail=500 2>/dev/null | safe_logs || true
      fi
    done
  fi
  echo "::endgroup::"
}

echo "Waiting ${wait_seconds} s before checking the cluster (image $EXPECTED_IMAGE, expected migration version $expected)"
sleep "$wait_seconds"

# One Deployment read and one pods read; every check works from these.
deploy_json=
deploy_err=
if out=$(k get "deployment/$DEPLOYMENT" -o json 2>"$tmp/err") && jq -e 'type == "object"' <<<"$out" >/dev/null 2>&1; then
  deploy_json=$out
else
  deploy_err=$(first_error "$tmp/err")
  deploy_err=${deploy_err:-unreadable response}
fi
pods_json=
pods_err=
if out=$(k get pods -l "$POD_LABEL" -o json 2>"$tmp/err") && jq -e 'type == "object" and (.items | type == "array")' <<<"$out" >/dev/null 2>&1; then
  pods_json=$out
else
  pods_err=$(first_error "$tmp/err")
  pods_err=${pods_err:-unreadable response}
fi

# R-1 and I-1 always run.
if [ -z "$deploy_json" ]; then
  fail R-1 "could not read Deployment $DEPLOYMENT: $deploy_err"
  fail I-1 "could not read Deployment $DEPLOYMENT: $deploy_err"
else
  r1=$(jq -r '"observedGeneration=\(.status.observedGeneration) generation=\(.metadata.generation) updatedReplicas=\(.status.updatedReplicas) readyReplicas=\(.status.readyReplicas) replicas=\(.status.replicas)"' <<<"$deploy_json")
  if jq -e '.status.observedGeneration != null and .status.observedGeneration == .metadata.generation
            and ([.status.updatedReplicas, .status.readyReplicas, .status.replicas] | all(. == 1))' \
    <<<"$deploy_json" >/dev/null; then
    pass R-1 "$r1"
  else
    fail R-1 "Deployment is not rolled out at its current generation: $r1"
  fi
  tmpl_image=$(spec_image .spec.template.spec <<<"$deploy_json")
  if [ "$tmpl_image" = "$EXPECTED_IMAGE" ]; then
    pass I-1 "template image $tmpl_image"
  else
    fail I-1 "template image '${tmpl_image:-<none>}' != expected '$EXPECTED_IMAGE'"
  fi
fi

# P-1 decides whether the pod checks can run.
pod_ok=0
live=
if [ -z "$pods_json" ]; then
  fail P-1 "could not read pods: $pods_err"
  skip_reason="pods could not be read"
else
  live=$(jq -c '[.items[]
                 | select(.metadata.deletionTimestamp == null)
                 | select(.status.phase != "Failed" and .status.phase != "Succeeded")]' <<<"$pods_json")
  count=$(jq 'length' <<<"$live")
  if [ "$count" = "1" ]; then
    pod_ok=1
    pass P-1 "exactly one live pod"
  else
    fail P-1 "expected exactly one live gogear-api pod, found $count"
    skip_reason="no single live pod"
  fi
fi

if [ "$pod_ok" != "1" ]; then
  for id in I-2 I-3 H-1 I-4 M-1; do not_run "$id" "$skip_reason"; done
else
  pod_json=$(jq -c '.[0]' <<<"$live")
  pod=$(jq -r '.metadata.name' <<<"$pod_json")

  # I-2
  pod_image=$(spec_image .spec <<<"$pod_json")
  if [ "$pod_image" = "$EXPECTED_IMAGE" ]; then
    pass I-2 "pod $pod spec image $pod_image"
  else
    fail I-2 "pod $pod spec image '${pod_image:-<none>}' != expected '$EXPECTED_IMAGE'"
  fi

  # I-3
  hash=$(jq -r '.metadata.labels["pod-template-hash"] // ""' <<<"$pod_json")
  owner=$(jq -r '[.metadata.ownerReferences[]? | select(.controller == true and .kind == "ReplicaSet") | .name] | first // ""' <<<"$pod_json")
  if [ -n "$hash" ] && [ "$owner" = "$DEPLOYMENT-$hash" ]; then
    pass I-3 "pod $pod is owned by ReplicaSet $owner"
  else
    fail I-3 "pod $pod controller ReplicaSet '${owner:-<none>}', pod-template-hash '${hash:-<missing>}'; expected ReplicaSet $DEPLOYMENT-<pod-template-hash>"
  fi

  # H-1 and I-4 read the container status selected by name.
  read -r phase ready restarts raw_image_id < <(jq -r --arg c "$CONTAINER" '
    first(.status.containerStatuses[]? | select(.name == $c)) as $s
    | "\(.status.phase // "-") \($s.ready // false) \($s.restartCount // "missing") \($s.imageID // "-")"' <<<"$pod_json")
  if [ "$phase" = "Running" ] && [ "$ready" = "true" ] && [ "$restarts" = "0" ]; then
    pass H-1 "phase=$phase ready=$ready restartCount=$restarts"
  else
    fail H-1 "phase=$phase ready=$ready restartCount=$restarts (want Running, ready, 0 restarts)"
  fi

  image_id=$raw_image_id
  [ "$image_id" = "-" ] && image_id=
  image_id=${image_id#*://}
  if [[ "$image_id" =~ $IMAGE_ID_RE ]]; then
    pass I-4 "imageID=$raw_image_id"
    id_digest=${image_id##*@}
    if [ "$id_digest" = "$BUILD_DIGEST" ]; then
      echo "imageID digest equals build digest"
    else
      echo "imageID digest $id_digest differs from build digest; the runtime names identical content by an earlier digest; identity is proven by I-1 to I-3"
    fi
  else
    fail I-4 "imageID '$raw_image_id' is not from bateau/gogear-api"
  fi

  # M-1: read the log first so a failed read (for example missing RBAC on
  # pods/log) is not reported as a missing migrations line.
  want="gogear-api migrations: version=$expected dirty=false"
  log_rc=0
  pod_log=$(k logs "$pod" 2>"$tmp/err") || log_rc=$?
  if [ "$log_rc" -ne 0 ]; then
    log_err=$(first_error "$tmp/err")
    log_err=${log_err:-no error output}
    hint="See the diagnostics below."
    case "$log_err" in
      *Forbidden*) hint="The deploy service account needs get on pods/log." ;;
    esac
    fail M-1 "could not read pod logs (kubectl exit $log_rc): $log_err. $hint"
  elif grep -qF -- "$want" <<<"$pod_log"; then
    pass M-1 "$want"
  else
    fail M-1 "pod log lacks '$want'"
  fi
fi

if [ "$failed" != "0" ]; then
  diagnostics
  echo "::error::Deploy verification failed; see the check results above"
  exit 1
fi

t=${EXPECTED_IMAGE%@*}
echo "Verified: tag ${t##*:}, digest $BUILD_DIGEST, pod $pod, migration version $expected"
