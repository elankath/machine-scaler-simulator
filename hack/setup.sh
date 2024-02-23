#!/bin/bash
set -euo pipefail

declare PROJECT # these are mandatory cli flags to be provided by the user
declare LANDSCAPE_NAME="dev"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
PROJECT_DIR="$(cd "$(dirname "${SCRIPT_DIR}")" &>/dev/null && pwd)"
LAUNCH_ENV_FILE="launch.env"
LAUNCH_ENV_PATH="$PROJECT_DIR/$LAUNCH_ENV_FILE"
K8S_SRC_TAR_URL="https://github.com/kubernetes/kubernetes/archive/refs/tags/v1.29.1.tar.gz"
KUBE_SOURCE_DIR="$HOME/go/src/github.com/kubernetes/kubernetes"



function create_usage() {
  usage=$(printf '%s\n' "
    Usage: $(basename $0) [Options]
    Options:
      -p | --project                    <project-name>                      (Required) Name of the Gardener Project
      -l | --landscape                  <landscape-name>                    (Optional) Short Name of the landscape. Defaults to dev
    ")
  echo "${usage}"
}

function parse_flags() {
  while test $# -gt 0; do
    case "$1" in
    --project | -p)
      shift
      PROJECT="$1"
      ;;
    --landscape | -l)
        shift
        LANDSCAPE_NAME="$1"
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
  if [[ -z "${PROJECT}" ]]; then
    echo -e "Garden project name has not been passed. Please provide project name either by specifying --project or -p argument"
    exit 1
  fi
}

validate_go_version() {
  local goVer
  goVer=$(go version)
  if [[ ! "$goVer" == *"go1.22"* ]]; then
    echo -e "Go 1.22 required to run scalesim"
    exit 2
  fi
}

main() {
  parse_flags "$@"
  validate_args
  validate_go_version
  local GOOS GOARCH binaryAssetsDir kubeSchedulerBinaryUrl launchEnv kubeSchedulerGoMainFile landscapeFullName

  GOOS=$(go env GOOS)
  GOARCH=$(go env GOARCH)
  printf "GOOS=%s, GOARCH=%s\n" $GOOS $GOARCH
  printf "Installing air for live reload...\n"
  go install github.com/cosmtrek/air@latest
  printf "Installing setup-envtest...\n"
  go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
  envTestSetupCmd="setup-envtest --os $GOOS --arch $GOARCH use  -p path"
  printf "Executing: %s\n" "$envTestSetupCmd"
  binaryAssetsDir=$(eval "$envTestSetupCmd")

  if [[ ! -d "$KUBE_SOURCE_DIR" ]]; then
    printf "No k8s sources at %s\n" "$KUBE_SOURCE_DIR"
    pushd "/tmp" > /dev/null
    printf "Downloading k8s source tarball from %s\n" "$K8S_SRC_TAR_URL"
    curl -kL $K8S_SRC_TAR_URL -o /tmp/kubernetes.tar.gz
    tar -zxf kubernetes.tar.gz
    # TODO: make sure to create the directory under GOPATH if not present.
    mv /tmp/kubernetes-1.29.1/ $KUBE_SOURCE_DIR
    popd > /dev/null

#    pushd "$KUBE_SOURCE_DIR" > /dev/null
#    echo "Executing go mod tidy on $KUBE_SOURCE_DIR..."
#    go mod tidy
#    popd > /dev/null
  fi


  kubeSchedulerGoMainFile="$KUBE_SOURCE_DIR/cmd/kube-scheduler/scheduler.go"
  if [[ ! -f  $kubeSchedulerGoMainFile ]]; then
    echo -e "No kube-scheduler Go main file at: $kubeSchedulerGoMainFile"
    exit 3
  fi

  if [[ ! -f "$binaryAssetsDir/kube-scheduler" ]]; then
    echo -e "No kube-scheduler binary in: $binaryAssetsDir"
    echo "Building kube-scheduler..."
    pushd "$KUBE_SOURCE_DIR" > /dev/null
    go build -v -o /tmp/kube-scheduler cmd/kube-scheduler/scheduler.go
    chmod +w "$binaryAssetsDir"
    cp -v /tmp/kube-scheduler "$binaryAssetsDir"
    ls -al "$binaryAssetsDir/kube-scheduler"
    popd > /dev/null
  fi

  if [[ ! -f "$binaryAssetsDir/kube-controller-manager" ]]; then
    echo -e "No kube-controller-manager binary in: $binaryAssetsDir"
    echo "Building kube-controller-manager..."
    pushd "$KUBE_SOURCE_DIR" > /dev/null
    go build -v -o /tmp/kube-controller-manager cmd/kube-controller-manager/controller-manager.go
    chmod +w "$binaryAssetsDir"
    cp -v /tmp/kube-controller-manager "$binaryAssetsDir"
    popd > /dev/null
    ls -al "$binaryAssetsDir/kube-controller-manager"
  fi


#  kubeSchedulerBinaryUrl="https://dl.k8s.io/v1.29.1/bin/$GOOS/$GOARCH/kube-scheduler"
#  chmod u+w "$binaryAssetsDir"
#  pushd "$binaryAssetsDir" > /dev/null
#  printf "Downloading kube-scheduler binary from %s into %s\n" "$kubeSchedulerBinaryUrl" "$BINARY_ASSETS_DIR"
#  curl -kLO "$kubeSchedulerBinaryUrl"
#  chmod +x kube-scheduler

  landscapeFullName="sap-landscape-${LANDSCAPE_NAME}"
  loginCmd="gardenctl target --garden $landscapeFullName  --project $PROJECT"
  printf "Logging via gardenctl: %s\n" "$loginCmd"
  eval "$loginCmd"
  eval "$(gardenctl kubectl-env bash)"

  printf "BINARY_ASSETS_DIR=\"%s\"
GARDEN_PROJECT_NAME=\"%s\"
GARDEN_LANDSCAPE_NAME=\"%s\"
GARDENCTL_KUBECONFIG=\"%s\"
KUBE_SOURCE_DIR=\"%s\"" "$binaryAssetsDir" "$PROJECT" "$landscapeFullName" "$KUBECONFIG" "$KUBE_SOURCE_DIR"> "$LAUNCH_ENV_PATH"



  printf "Wrote env to %s\n" "$LAUNCH_ENV_PATH"

  echo
  echo "NOTE: COPY & EXECUTE THIS->> set -o allexport && source launch.env && set +o allexport"
}


USAGE=$(create_usage)
main "$@"