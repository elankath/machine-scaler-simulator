package gardenclient

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	machinev1alpha1 "github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	"golang.org/x/exp/maps"

	gardencore "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/serutil"
)

type shootAccess struct {
	landscapeName    string
	projectName      string
	shootName        string
	getShootCmd      *exec.Cmd
	getNodesCmd      *exec.Cmd
	getMCDCmd        *exec.Cmd
	getPodsCmd       *exec.Cmd
	getAllPodsCmd    *exec.Cmd
	taintNodesCmd    *exec.Cmd
	untaintNodesCmd  *exec.Cmd
	deleteAllPodsCmd *exec.Cmd
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

func InitShootAccess(landscapeName, projectName, shootName string) scalesim.ShootAccess {
	cmdEnv := os.Environ()
	cmdEnv = append(cmdEnv, fmt.Sprintf("KUBECONFIG=%s", os.Getenv("GARDENCTL_KUBECONFIG")))
	cmdEnv = append(cmdEnv, "GCTL_SESSION_ID=scalesim")

	shellCmd := fmt.Sprintf("gardenctl target --garden %s --project %s >&2 &&  eval $(gardenctl kubectl-env bash) && kubectl get shoot %s -oyaml",
		landscapeName, projectName, shootName)
	getShootCmd := exec.Command("bash", "-l", "-c", shellCmd)
	getShootCmd.Env = cmdEnv

	shellCmd = fmt.Sprintf("gardenctl target --garden %s --project %s --shoot %s >&2 &&  eval $(gardenctl kubectl-env bash) && kubectl get node -oyaml",
		landscapeName, projectName, shootName)
	getNodesCmd := exec.Command("bash", "-l", "-c", shellCmd)
	getNodesCmd.Env = cmdEnv

	shellCmd = fmt.Sprintf("gardenctl target --garden %s --project %s --shoot %s --control-plane >&2 &&  eval $(gardenctl kubectl-env bash) && kubectl get mcd -oyaml",
		landscapeName, projectName, shootName)
	getMCDCmd := exec.Command("bash", "-l", "-c", shellCmd)
	getMCDCmd.Env = cmdEnv

	shellCmd = fmt.Sprintf("gardenctl target --garden %s --project %s --shoot %s  >&2 &&  eval $(gardenctl kubectl-env bash) && kubectl get pod -oyaml",
		landscapeName, projectName, shootName)
	getPodsCmd := exec.Command("bash", "-l", "-c", shellCmd)
	getPodsCmd.Env = cmdEnv

	shellCmd = fmt.Sprintf("gardenctl target --garden %s --project %s --shoot %s  >&2 &&  eval $(gardenctl kubectl-env bash) && kubectl get pod -A -oyaml",
		landscapeName, projectName, shootName)
	getAllPodsCmd := exec.Command("bash", "-l", "-c", shellCmd)
	getAllPodsCmd.Env = cmdEnv

	shellCmd = fmt.Sprintf("gardenctl target --garden %s --project %s --shoot %s  >&2 &&  eval $(gardenctl kubectl-env bash) && kubectl taint nodes --all scaleSim:NoSchedule",
		landscapeName, projectName, shootName)
	taintNodesCmd := exec.Command("bash", "-l", "-c", shellCmd)
	taintNodesCmd.Env = cmdEnv

	shellCmd = fmt.Sprintf("gardenctl target --garden %s --project %s --shoot %s  >&2 &&  eval $(gardenctl kubectl-env bash) && kubectl taint nodes --all scaleSim:NoSchedule-",
		landscapeName, projectName, shootName)
	untaintNodesCmd := exec.Command("bash", "-l", "-c", shellCmd)
	untaintNodesCmd.Env = cmdEnv

	shellCmd = fmt.Sprintf("gardenctl target --garden %s --project %s --shoot %s  >&2 &&  eval $(gardenctl kubectl-env bash) && kubectl delete pods --all",
		landscapeName, projectName, shootName)
	deleteAllPodsCmd := exec.Command("bash", "-l", "-c", shellCmd)
	deleteAllPodsCmd.Env = cmdEnv

	return &shootAccess{
		landscapeName:    landscapeName,
		projectName:      projectName,
		shootName:        shootName,
		getShootCmd:      getShootCmd,
		getNodesCmd:      getNodesCmd,
		getMCDCmd:        getMCDCmd,
		getPodsCmd:       getPodsCmd,
		getAllPodsCmd:    getAllPodsCmd,
		taintNodesCmd:    taintNodesCmd,
		untaintNodesCmd:  untaintNodesCmd,
		deleteAllPodsCmd: deleteAllPodsCmd,
	}
}

func (s *shootAccess) GetShootObj() (*gardencore.Shoot, error) {
	s.clearCommands()
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

func (s *shootAccess) clearCommands() {
	reset(s.getNodesCmd)
	reset(s.getShootCmd)
	reset(s.getMCDCmd)
	reset(s.getPodsCmd)
	reset(s.getAllPodsCmd)
	reset(s.taintNodesCmd)
	reset(s.untaintNodesCmd)
	reset(s.deleteAllPodsCmd)
}

func reset(c *exec.Cmd) {
	c.Process = nil
	c.Stdout = nil
	c.Stderr = nil
	c.ProcessState = nil
	c.SysProcAttr = nil
}
func (s *shootAccess) GetNodes() ([]*corev1.Node, error) {
	s.clearCommands()
	slog.Info("shootAccess.GetNodes().", "command", s.getNodesCmd.String())
	cmdOutput, err := s.getNodesCmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		slog.Error("cannot get shoot nodes", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
		return nil, err
	}
	return serutil.DecodeList[*corev1.Node](cmdOutput)
}

func (s *shootAccess) GetMachineDeployments() ([]*machinev1alpha1.MachineDeployment, error) {
	s.clearCommands()
	slog.Info("shootAccess.GetMachineDeployments().", "command", s.getMCDCmd.String())
	cmdOutput, err := s.getMCDCmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		slog.Error("cannot get mcd", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
		return nil, err
	}
	return serutil.DecodeList[*machinev1alpha1.MachineDeployment](cmdOutput)
}

func (s *shootAccess) ScaleMachineDeployment(machineDeploymentName string, replicas int32) error {
	cmdEnv := os.Environ()
	cmdEnv = append(cmdEnv, fmt.Sprintf("KUBECONFIG=%s", os.Getenv("GARDENCTL_KUBECONFIG")))
	cmdEnv = append(cmdEnv, "GCTL_SESSION_ID=scalesim")

	shellCmd := fmt.Sprintf("gardenctl target --garden %s --project %s --shoot %s --control-plane >&2  && eval $(gardenctl kubectl-env bash) && kubectl scale machinedeployment %s --replicas=%d",
		s.landscapeName, s.projectName, s.shootName, machineDeploymentName, replicas)
	scaleMachineDeployment := exec.Command("bash", "-l", "-c", shellCmd)
	scaleMachineDeployment.Env = cmdEnv
	cmdOutput, err := scaleMachineDeployment.Output()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		slog.Error("cannot scale mcd object", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
		return err
	}
	return nil
}

func (s *shootAccess) getPods() ([]*corev1.Pod, error) {
	s.clearCommands()
	slog.Info("shootAccess.getPods().", "command", s.getPodsCmd.String())
	cmdOutput, err := s.getPodsCmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		slog.Error("cannot get shoot pods in the default namespace", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
		return nil, err
	}
	return serutil.DecodeList[*corev1.Pod](cmdOutput)
}

func (s *shootAccess) getAllPods() ([]*corev1.Pod, error) {
	s.clearCommands()
	slog.Info("shootAccess.getAllPods().", "command", s.getAllPodsCmd.String())
	cmdOutput, err := s.getAllPodsCmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		slog.Error("cannot get all shoot pods", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
		return nil, err
	}
	return serutil.DecodeList[*corev1.Pod](cmdOutput)
}

func (s *shootAccess) GetUnscheduledPods() ([]corev1.Pod, error) {
	pods, err := s.getPods()
	if err != nil {
		return nil, err
	}
	var unscheduledPods []corev1.Pod
	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			unscheduledPods = append(unscheduledPods, *pod)
		}
	}
	return unscheduledPods, nil
}

func (s *shootAccess) CreatePods(filePath string, replicas int) error {
	for i := 0; i < replicas; i++ {
		cmdEnv := os.Environ()
		cmdEnv = append(cmdEnv, fmt.Sprintf("KUBECONFIG=%s", os.Getenv("GARDENCTL_KUBECONFIG")))
		cmdEnv = append(cmdEnv, "GCTL_SESSION_ID=scalesim")
		shellCmd := fmt.Sprintf("gardenctl target --garden %s --project %s --shoot %s  >&2  && eval $(gardenctl kubectl-env bash) && kubectl create -f %s",
			s.landscapeName, s.projectName, s.shootName, filePath)
		scaleMachineDeployment := exec.Command("bash", "-l", "-c", shellCmd)
		scaleMachineDeployment.Env = cmdEnv
		cmdOutput, err := scaleMachineDeployment.Output()
		if err != nil {
			var exitErr *exec.ExitError
			errors.As(err, &exitErr)
			slog.Error("cannot scale mcd object", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
			return err
		}
	}
	return nil
}

func (s *shootAccess) TaintNodes() error {
	s.clearCommands()
	slog.Info("shootAccess.TaintExistingNodes().", "command", s.taintNodesCmd.String())
	cmdOutput, err := s.taintNodesCmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		slog.Error("cannot taint shoot nodes", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
		return err
	}
	return nil
}

func (s *shootAccess) UntaintNodes() error {
	s.clearCommands()
	slog.Info("shootAccess.UntaintExistingNodes().", "command", s.untaintNodesCmd.String())
	cmdOutput, err := s.untaintNodesCmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		slog.Error("cannot untaint shoot nodes", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
		return err
	}
	return nil
}

func (s *shootAccess) DeleteAllPods() error {
	s.clearCommands()
	slog.Info("shootAccess.DeleteAllPods().", "command", s.deleteAllPodsCmd.String())
	cmdOutput, err := s.deleteAllPodsCmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		errors.As(err, &exitErr)
		slog.Error("cannot delete all pods in the shoot", "error", err, "stdout", string(cmdOutput), "stderr", string(exitErr.Stderr))
		return err
	}
	return nil
}

func (s *shootAccess) CleanUp() error {
	err := s.DeleteAllPods()
	if err != nil {
		return err
	}

	return s.UntaintNodes()
}

func (s *shootAccess) GetDSPods() ([]corev1.Pod, error) {
	podsMap := make(map[string]corev1.Pod)

	allPods, err := s.getAllPods()
	if err != nil {
		return nil, err
	}

	for _, pod := range allPods {
		for _, ownerRef := range pod.ObjectMeta.OwnerReferences {
			if ownerRef.Kind == "DaemonSet" {
				if pod.GenerateName != "" {
					podsMap[pod.GenerateName] = *pod
				}
			}
		}
	}

	return maps.Values(podsMap), nil
}
