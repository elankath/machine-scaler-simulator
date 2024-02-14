#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
PROJECT_DIR="$(cd "$(dirname "${SCRIPT_DIR}")" &>/dev/null && pwd)"
LAUNCH_ENV_FILE="launch.env"
LAUNCH_ENV_PATH="$PROJECT_DIR/$LAUNCH_ENV_FILE"

#
#
main() {
  local GOOS
  local GOARCH
  local binaryAssetsDir
  local kubeSchedulerBinaryUrl
  local kubeSourceDir

  GOOS=$(go env GOOS)
  GOARCH=$(go env GOARCH)
  printf "GOOS=%s, GOARCH=%s\n" $GOOS $GOARCH
  printf "Installing setup-envtest...\n"
  go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
  envTestSetupCmd="setup-envtest --os $GOOS --arch $GOARCH use  -p path"
  printf "Executing: %s\n" "$envTestSetupCmd"
  binaryAssetsDir=$(eval "$envTestSetupCmd")

  kubeSourceDir="$HOME/go/src/github.com/kubernetes/kubernetes"
  if [[ ! -d "$kubeSourceDir" ]]; then
    printf "Err: Kindly checkout kubernetes source into %s\n" "$kubeSourceDir" >&2
    exit 1
  fi
  
#  kubeSchedulerBinaryUrl="https://dl.k8s.io/v1.29.1/bin/$GOOS/$GOARCH/kube-scheduler"
#  chmod u+w "$binaryAssetsDir"
#  pushd "$binaryAssetsDir" > /dev/null
#  printf "Downloading kube-scheduler binary from %s into %s\n" "$kubeSchedulerBinaryUrl" "$BINARY_ASSETS_DIR"
#  curl -kLO "$kubeSchedulerBinaryUrl"
#  chmod +x kube-scheduler

  printf "BINARY_ASSETS_DIR=\"%s\"\n" "$binaryAssetsDir" > "$LAUNCH_ENV_PATH"
  printf "Wrote env to %s\n" "$LAUNCH_ENV_PATH"
#  setup-envtest --os $(go env GOOS) --arch $(go env GOARCH) use -p path
}

main