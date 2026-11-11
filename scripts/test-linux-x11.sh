#!/usr/bin/env bash
set -euo pipefail

# Never run the opt-in input tests against the caller's desktop. These two
# private servers share only an ephemeral authority file, never xhost + / -ac.
cd "$(dirname "$0")/.."
scope=${KRASIS_X11_TEST_SCOPE:-all}
if [[ $scope != all && $scope != logind ]] || [[ $scope == logind && ${KRASIS_X11_ACTUAL_LIFECYCLE:-0} != 1 ]]; then
  echo 'Use scope all, or logind with KRASIS_X11_ACTUAL_LIFECYCLE=1.' >&2
  exit 2
fi
base=${KRASIS_X11_DISPLAY_BASE:-91}
if [[ ! $base =~ ^[0-9]{1,4}$ ]]; then
  echo 'KRASIS_X11_DISPLAY_BASE must be between 1 and 9998.' >&2
  exit 2
fi
base=$((10#$base))
if (( base < 1 || base > 9998 )); then
  echo 'KRASIS_X11_DISPLAY_BASE must be between 1 and 9998.' >&2
  exit 2
fi
work=$(mktemp -d)
binwork=''
pids=()
login_units=()
cleanup() {
  if (( ${#login_units[@]} )); then
    sudo -n systemctl stop "${login_units[@]}" || true
  fi
  if (( ${#pids[@]} )); then
    kill "${pids[@]}" 2>/dev/null || true
    wait "${pids[@]}" 2>/dev/null || true
  fi
  rm -rf -- "$work"
  if [[ -n $binwork ]]; then
    rm -rf -- "$binwork"
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# Compile once, before starting displays or timed input/lifecycle scenarios.
# Only public executables are shared; identities and X11 cookies stay private.
binwork=$(mktemp -d /tmp/krasis-x11-products-XXXXXXXXXX)
chmod 755 "$binwork"
build_started=$SECONDS
# Match the shipped Linux bundle; race-instrumented test packages still use CGO.
CGO_ENABLED=0 go build -o "$binwork/" ./cmd/krasis-agent ./cmd/krasis-cli
export KRASIS_X11_TEST_BIN_DIR=$binwork
echo "X11 product build: $((SECONDS - build_started))s (one agent + CLI build)"
export XAUTHORITY="$work/Xauthority"
touch "$XAUTHORITY"
chmod 600 "$XAUTHORITY"
for number in "$base" "$((base + 1))"; do
  if [[ -e /tmp/.X${number}-lock || -e /tmp/.X11-unix/X${number} ]]; then
    echo "Refusing to use an existing X server :$number." >&2
    exit 1
  fi
  xauth -f "$XAUTHORITY" add ":$number" . "$(mcookie)"
done
for spec in "$base:1280x720x24" "$((base + 1)):1024x768x24"; do
  number=${spec%%:*}
  Xvfb ":$number" -screen 0 "${spec#*:}" -nolisten tcp -noreset \
    -auth "$XAUTHORITY" > "$work/X$number.log" 2>&1 &
  pids+=("$!")
  ready=false
  for (( attempt=0; attempt<100; attempt++ )); do
    if ! kill -0 "${pids[-1]}" 2>/dev/null; then
      break
    fi
    if timeout 1s xdpyinfo -display ":$number" > /dev/null 2>&1; then
      ready=true
      break
    fi
    sleep 0.1
  done
  if [[ $ready != true ]]; then
    cat "$work/X$number.log" >&2
    echo "Xvfb :$number did not become ready." >&2
    exit 1
  fi
done
export DISPLAY=:$base KRASIS_X11_TEST_OTHER_DISPLAY=:$((base + 1))
export XDG_SESSION_TYPE=x11 KRASIS_X11_TEST=1
unset WAYLAND_DISPLAY
if [[ ${KRASIS_X11_ACTUAL_LIFECYCLE:-0} == 1 ]]; then
  : "${KRASIS_X11_TEST_LOCAL_USER:?Set a dedicated first test user}"
  : "${KRASIS_X11_TEST_PEER_USER:?Set a dedicated second test user}"
  if [[ $KRASIS_X11_TEST_LOCAL_USER == "$KRASIS_X11_TEST_PEER_USER" ]]; then
    echo 'Actual PAM/logind tests require two different users.' >&2
    exit 2
  fi
else
  unset KRASIS_X11_TEST_PEER_USER KRASIS_X11_TEST_LOCAL_USER
fi
# Serialize packages: input/grab and CLIPBOARD tests share a server.
if [[ $scope == all ]]; then
  go test -race -p 1 -count=1 -timeout 3m -v \
    ./internal/x11desktop ./internal/x11registry ./internal/wininput ./internal/linuxclipboard \
    ./internal/launcher ./internal/winlifecycle ./internal/agentinstance
  # Fixture processes use the real product runner, TLS and independent crashguard.
  go test -count=1 -timeout 3m -v ./internal/residentagent \
    -run '^TestLinuxX11CLIInteroperability$'
fi
if [[ ${KRASIS_X11_ACTUAL_LIFECYCLE:-0} == 1 ]]; then
  # Never skip silently if the runner cannot provide real session authority.
  systemctl is-active --quiet systemd-logind
  systemd --version
  # Model logged-in desktop users. Keep their real user managers alive while
  # each scenario creates, locks/terminates, and replaces separate PAM sessions.
  # No input, Xauthority, linger setting or synthetic logind state is involved.
  login_started=$SECONDS
  for user in "$KRASIS_X11_TEST_LOCAL_USER" "$KRASIS_X11_TEST_PEER_USER"; do
    uid=$(id -u "$user")
    manager="user@$uid.service"
    unit="$(basename "$binwork")-login-${#login_units[@]}.service"
    login_units+=("$unit")
    # Old systemd PAM helpers can retain Type=exec's completion fd. Notify
    # from the actual main PID after PAM, then exec sleep without changing PID.
    if ! sudo -n systemd-run --quiet --collect --service-type=notify --unit="$unit" \
      --uid="$user" --property="PAMName=${KRASIS_X11_TEST_PAM_SERVICE:-login}" \
      --property=NotifyAccess=main --property=TimeoutStartSec=8s \
      /usr/bin/systemd-notify --pid=self --ready --exec ';' /usr/bin/sleep infinity \
      || ! systemctl is-active --quiet "$unit"; then
      sudo -n journalctl -b --no-pager -n 100 -u "$unit" -u "$manager" -u systemd-logind.service || true
      exit 1
    fi
  done
  echo "X11 login fixtures: $((SECONDS - login_started))s"
  # Elevate the test runner once; each CLI still runs as its dedicated UID.
  # Preserve only this fixture's display/binary/session selection, not -E.
  go test -exec 'sudo -n --preserve-env=DISPLAY,XAUTHORITY,XDG_SESSION_TYPE,KRASIS_X11_TEST,KRASIS_X11_TEST_BIN_DIR,KRASIS_X11_TEST_OTHER_DISPLAY,KRASIS_X11_TEST_LOCAL_USER,KRASIS_X11_TEST_PEER_USER,KRASIS_X11_TEST_PAM_SERVICE' \
    -count=1 -timeout 8m -v ./internal/residentagent \
    -run '^TestLinuxX11Actual(CLIInteroperability|Lifecycle)$'
fi
