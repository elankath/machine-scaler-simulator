#!/bin/bash
set -euo pipefail

declare PROJECT SHOOT # these are mandatory cli flags to be provided by the user
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
PROJECT_DIR="$(cd "$(dirname "${SCRIPT_DIR}")" &>/dev/null && pwd)"
LAUNCH_ENV_FILE="launch.env"
LAUNCH_ENV_PATH="$PROJECT_DIR/$LAUNCH_ENV_FILE"


function create_usage() {
  usage=$(printf '%s\n' "
    Usage: $(basename $0) [Options]
    Options:
      -t | --shoot                      <shoot-cluster-name>                (Required) Name of the Gardener Shoot Cluster
      -p | --project                    <project-name>                      (Required) Name of the Gardener Project
    ")
  echo "${usage}"
}

function parse_flags() {
  while test $# -gt 0; do
    case "$1" in
    --shoot | -t)
      shift
      SHOOT="$1"
      ;;
    --project | -p)
      shift
      PROJECT="$1"
      ;;
    --help | -h)
      shift
      echo "${USAGE}"
      exit 0
      ;;
    esac
    shift
  done
}

function validate_args() {
  if [[ -z "${SHOOT}" ]]; then
    echo -e "Garden shoot cluster name has not been passed. Please provide the shoot name either by specifying --shoot or -t argument"
    exit 1
  fi
  if [[ -z "${PROJECT}" ]]; then
    echo -e "Garden project name has not been passed. Please provide project name either by specifying --project or -p argument"
    exit 1
  fi
}

main() {
  parse_flags "$@"
  validate_args
  local GOOS GOARCH binaryAssetsDir kubeSchedulerBinaryUrl kubeSourceDir launchEnv

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

  loginCmd="gardenctl target --garden sap-landscape-dev --project $PROJECT"
  printf "Logging via gardenctl: %s\n" "$loginCmd"
  eval "$loginCmd"
  eval "$(gardenctl kubectl-env bash)"

  printf "BINARY_ASSETS_DIR=\"%s\"
GARDEN_SHOOT_NAME=\"%s\"
GARDEN_PROJECT_NAME=\"%s\"
GARDENCTL_KUBECONFIG=\"%s\"" "$binaryAssetsDir" "$SHOOT" "$PROJECT" "$KUBECONFIG" > "$LAUNCH_ENV_PATH"



  printf "Wrote env to %s\n" "$LAUNCH_ENV_PATH"
}

USAGE=$(create_usage)
main "$@"