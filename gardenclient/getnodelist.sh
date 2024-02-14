#!/usr/bin/env bash
set -x

if [[ -z "${GARDEN_PROJECT_NAME}" ]]; then
  echo "GARDEN_PROJECT_NAME is undefined"
  exit 1
else
  PROJECT_NAME_VAR="${GARDEN_PROJECT_NAME}"
fi

if [[ -z "${GARDEN_SHOOT_NAME}" ]]; then
  echo "GARDEN_SHOOT_NAME is undefined"
  exit 1
else
  SHOOT_NAME_VAR="${GARDEN_SHOOT_NAME}"
fi


gardenctl target --garden sap-landscape-dev --project "${PROJECT_NAME_VAR}" --shoot "${SHOOT_NAME_VAR}" >/dev/null

eval $(gardenctl kubectl-env bash) >/dev/null

kubectl get node -oyaml