package gardenclient

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	gardencore "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/serutil"
)

type shootAccess struct {
	projectName string
	shootName   string
	getShootCmd *exec.Cmd
	getNodesCmd *exec.Cmd
	////shoot
	//shoot *gardencore.Shoot
	////Nodes
	//nodeList []corev1.Node
}

func (s *shootAccess) ProjectName() string {
	return s.projectName
}

func (s *shootAccess) ShootName() string {
	return s.shootName
}

var _ scalesim.ShootAccess = (*shootAccess)(nil)

func InitShootAccess(projectName, shootName string) scalesim.ShootAccess {
	cmdEnv := os.Environ()
	cmdEnv = append(cmdEnv, fmt.Sprintf("KUBECONFIG=%s", os.Getenv("GARDENCTL_KUBECONFIG")))
	cmdEnv = append(cmdEnv, "GCTL_SESSION_ID=scalesim")

	shellCmd := fmt.Sprintf("gardenctl target --garden sap-landscape-dev --project %s >&2 &&  eval $(gardenctl kubectl-env bash) && kubectl get shoot %s -oyaml", projectName, shootName)
	getShootCmd := exec.Command("bash", "-l", "-c", shellCmd)
	getShootCmd.Env = cmdEnv

	shellCmd = fmt.Sprintf("gardenctl target --garden sap-landscape-dev --project %s --shoot %s >&2 &&  eval $(gardenctl kubectl-env bash) && kubectl get node -oyaml", projectName, shootName)
	getNodesCmd := exec.Command("bash", "-l", "-c", shellCmd)
	getNodesCmd.Env = cmdEnv

	return &shootAccess{
		projectName: projectName,
		shootName:   shootName,
		getShootCmd: getShootCmd,
		getNodesCmd: getNodesCmd,
	}
}

func (s *shootAccess) GetShootObj() (*gardencore.Shoot, error) {
	slog.Info("shootAccess.GetShootObj().", "command", s.getShootCmd.String())
	cmdOutput, err := s.getShootCmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		slog.Error("cannot get shoot obj", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
		return nil, err
	}
	return serutil.DecodeShoot(cmdOutput)
}

func (s *shootAccess) GetNodes() ([]corev1.Node, error) {
	slog.Info("shootAccess.GetNodes().", "command", s.getNodesCmd.String())
	cmdOutput, err := s.getNodesCmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		slog.Error("cannot get shoot nodes", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
		return nil, err
	}
	return serutil.DecodeNodeList(cmdOutput)
}
