package recommender

import (
	"context"
	"errors"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/webutil"
	"github.com/gardener/gardener/pkg/apis/core/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

/*
	for {
		unscheduledPods = determine unscheduled pods
		if noUnscheduledPods then exit early
		- runSimulation
 		  - Start a go-routine for each of candidate nodePool which are eligible
				- eligibility: max is not yet reached for that nodePool
              For each go-routine:
                Setup:
                    - create a unique label that will get added to all nodes and pods
                	- copy previous winner nodes and a taint.
                	- copy the deployed pods with node names assigned and add toleration to the taint.
	            - scale up one node, add a taint and only copy of pods will have toleration to that taint.
                - copy of unscheduled pods, add a toleration for this taint.
                - wait for pods to be scheduled
                - compute node score.
	}
*/

type StrategyWeights struct {
	LeastWaste float64
	LeastCost  float64
}

type Recommendation map[string]int

type Recommender struct {
	engine          scalesim.Engine
	shootNodes      []corev1.Node
	scenarioName    string
	shootName       string
	strategyWeights StrategyWeights
	logWriter       http.ResponseWriter
}

type runResult struct {
	result          scalesim.NodeRunResult
	unscheduledPods []corev1.Pod
	err             error
}

func NewRecommender(engine scalesim.Engine, shootNodes []corev1.Node, scenarioName, shootName string, strategyWeights StrategyWeights, logWriter http.ResponseWriter) *Recommender {
	return &Recommender{
		engine:          engine,
		shootNodes:      shootNodes,
		scenarioName:    scenarioName,
		shootName:       shootName,
		strategyWeights: strategyWeights,
		logWriter:       logWriter,
	}
}

func (r *Recommender) Run(ctx context.Context) (Recommendation, error) {
	recommendation := make(Recommendation)
	unscheduledPods, err := r.engine.VirtualClusterAccess().ListPods(ctx)
	if err != nil {
		return recommendation, err
	}
	var runNumber int
	shoot, err := r.getShoot()
	if err != nil {
		webutil.InternalError(r.logWriter, err)
		return recommendation, err
	}
	var winningNodeResult *scalesim.NodeRunResult
	for {
		runNumber++
		webutil.Log(r.logWriter, fmt.Sprintf("scale-up recommender run #%d started...", runNumber))
		if len(unscheduledPods) == 0 {
			webutil.Log(r.logWriter, "All pods are scheduled. Exiting the loop...")
			break
		}
		winningNodeResult, unscheduledPods, err = r.runSimulation(ctx, shoot, runNumber)
		if err != nil {
			webutil.Log(r.logWriter, fmt.Sprintf("Unable to get eligible node pools for shoot %s, err: %v", shoot.Name, err))
			break
		}
		if winningNodeResult == nil {
			webutil.Log(r.logWriter, fmt.Sprintf("scale-up recommender run #%d, no winner could be identified. This will happen when no pods could be assgined. No more runs are required, exiting early", runNumber))
			break
		}
		webutil.Log(r.logWriter, fmt.Sprintf("For scale-up recommender run #%d, winning score is: %v", runNumber, winningNodeResult))
	}

	return recommendation, nil
}

func (r *Recommender) getShoot() (*v1beta1.Shoot, error) {
	shoot, err := r.engine.ShootAccess(r.shootName).GetShootObj()
	if err != nil {
		return nil, err
	}
	return shoot, nil
}

// TODO: sync existing nodes and pods deployed on them. DO NOT TAINT THESE NODES.
// eg:- 1 node(A) existing in zone a. Any node can only fit 2 pods.
// deployment 6 replicas, tsc zone, minDomains 3
// 1 pod will get assigned to A. 5 pending. 3 Nodes will be scale up. (1-a, 1-b, 1-c)
// if you count existing nodes and pods, then only 2 nodes are needed.

func (r *Recommender) runSimulation(ctx context.Context, shoot *v1beta1.Shoot, runNum int) (*scalesim.NodeRunResult, []corev1.Pod, error) {
	/*
		    1. getEligibleNodePools
			2. For each nodePool, start a go routine. Each go routine will return a node score.
			3. Collect the scores and return

			Inside each go routine:-
				1. Setup:-
					 - create a unique label that will get added to all nodes and pods (for helping in clean up)
				     - copy all nodes and add a taint.
		             - copy the deployed pods with node names assigned and add a toleration to the taint.
				2. For each zone in the nodePool:-
					- scale up one node
					- wait for assignment of pods (5 sec delay),
					- calculate the score.
			    	- Reset the state
			    3. Compute the winning score for this nodePool and push to the result channel
	*/
	eligibleNodePools, err := r.getEligibleNodePools(ctx, shoot)
	if err != nil {
		return nil, nil, err
	}
	var results []runResult

	resultCh := make(chan runResult, len(eligibleNodePools))
	go r.triggerNodePoolSimulations(ctx, eligibleNodePools, resultCh, runNum)

	// label, taint, result chan, error chan, close chan
	var errs error
	for result := range resultCh {
		if result.err != nil {
			_ = errors.Join(errs, err)
		} else {
			results = append(results, result)
		}
	}
	if errs != nil {
		return nil, nil, err
	}
	winningResult := getWinner(results)
	return &winningResult.result, winningResult.unscheduledPods, nil
}

func (r *Recommender) triggerNodePoolSimulations(ctx context.Context, nodePools []scalesim.NodePool, resultCh chan runResult, runNum int) {
	wg := &sync.WaitGroup{}
	for _, nodePool := range nodePools {
		wg.Add(1)
		go r.runSimulationForNodePool(ctx, wg, nodePool, resultCh, runNum)
	}
	wg.Wait()
	close(resultCh)
}

func (r *Recommender) runSimulationForNodePool(ctx context.Context, wg *sync.WaitGroup, nodePool scalesim.NodePool, resultCh chan runResult, runNum int) {
	defer wg.Done()

	labelKey := "app.kubernetes.io/simulation-run"
	labelValue := nodePool.Name + "-" + strconv.Itoa(runNum)
	if err := r.setupNodePoolSimRun(ctx, labelKey, labelValue); err != nil {
		resultCh <- createErrorResult(err)
		return
	}
	for _, zone := range nodePool.Zones {
		node, err := r.constructNodeFromExistingNodeOfInstanceType(ctx, nodePool.MachineType, zone, labelKey, labelValue)
		if err != nil {
			resultCh <- createErrorResult(err)
			return
		}
		if err = r.engine.VirtualClusterAccess().AddNodes(ctx, node); err != nil {
			resultCh <- createErrorResult(err)
			return
		}
		waitTillSchedulingComplete()
	}
}

func (r *Recommender) constructNodeFromExistingNodeOfInstanceType(ctx context.Context, instanceType, zone, labelKey, labelValue string) (*corev1.Node, error) {
	labels := map[string]string{"node.kubernetes.io/instance-type": instanceType}
	nodes, err := r.engine.VirtualClusterAccess().ListNodesMatchingLabels(ctx, labels)
	if err != nil {
		return nil, err
	}
	// TODO: scale from zero not handled
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no node found in shoot %s for instance type %s", r.shootName, instanceType)
	}

	referenceNode := nodes[0]
	nodeName := string(referenceNode.UID) + "SimRun-" + labelValue
	nodeLabels := referenceNode.Labels
	nodeLabels["topology.kubernetes.io/zone"] = zone
	nodeLabels[labelKey] = labelValue
	nodeLabels["kubernetes.io/hostname"] = nodeName

	delete(nodeLabels, "app.kubernetes.io/existing-node")

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeName,
			Namespace: "default",
			//TODO Change k8s hostname nodeLabels
			Labels: referenceNode.Labels,
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: labelKey, Effect: corev1.TaintEffectNoSchedule},
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: referenceNode.Status.Allocatable,
			Capacity:    referenceNode.Status.Capacity,
			Phase:       corev1.NodeRunning,
		},
	}
	return node, nil
}

func (r *Recommender) setupNodePoolSimRun(ctx context.Context, labelKey, labelValue string) error {
	nodes, err := r.engine.VirtualClusterAccess().ListNodes(ctx)
	if err != nil {
		return err
	}
	var nodeList []*corev1.Node
	for _, node := range nodes {
		nodeCopy := node.DeepCopy()
		nodeCopy.Name = node.Name + "SimRun-" + labelValue
		nodeCopy.Labels[labelKey] = labelValue
		nodeCopy.Spec.Taints = []corev1.Taint{
			{Key: labelKey, Effect: corev1.TaintEffectNoSchedule},
		}
		nodeList = append(nodeList, nodeCopy)
	}
	if err = r.engine.VirtualClusterAccess().AddNodes(ctx, nodeList...); err != nil {
		return err
	}

	pods, err := r.engine.VirtualClusterAccess().ListPods(ctx)
	if err != nil {
		return err
	}

	var podList []corev1.Pod
	for _, pod := range pods {
		podCopy := pod.DeepCopy()
		podCopy.Name = podCopy.Name + "SimRun-" + labelValue
		podCopy.Labels[labelKey] = labelValue
		podCopy.Spec.Tolerations = []corev1.Toleration{
			{Key: labelKey, Effect: corev1.TaintEffectNoSchedule, Operator: corev1.TolerationOpEqual},
		}
		podList = append(podList, *podCopy)
	}
	return r.engine.VirtualClusterAccess().CreatePods(ctx, "bin-packing", podList...)
}

func (r *Recommender) resetNodePoolSimRun(ctx context.Context, labelKey, labelValue string) error {
	labels := map[string]string{labelKey: labelValue}
	err := r.engine.VirtualClusterAccess().DeletePodsWithMatchingLabels(ctx, labels)
	err = r.engine.VirtualClusterAccess().DeleteNodesWithMatchingLabels(ctx, labels)
	return err
}

func (r *Recommender) getEligibleNodePools(ctx context.Context, shoot *v1beta1.Shoot) ([]scalesim.NodePool, error) {
	eligibleNodePools := make([]scalesim.NodePool, 0, len(shoot.Spec.Provider.Workers))
	for _, worker := range shoot.Spec.Provider.Workers {
		nodes, err := r.engine.VirtualClusterAccess().ListNodesInNodePool(ctx, worker.Name)
		if err != nil {
			return nil, err
		}
		if int32(len(nodes)) >= worker.Maximum {
			continue
		}
		nodePool := scalesim.NodePool{
			Name:        worker.Name,
			Zones:       worker.Zones,
			Max:         worker.Maximum,
			Current:     int32(len(nodes)),
			MachineType: worker.Machine.Type,
		}
		eligibleNodePools = append(eligibleNodePools, nodePool)
	}
	return eligibleNodePools, nil
}

func getWinner(results []runResult) runResult {
	var winner runResult
	minScore := math.MaxFloat64
	for _, v := range results {
		if v.result.CumulativeScore < minScore {
			winner = v
			minScore = v.result.CumulativeScore
		}
	}
	return winner
}

func waitTillSchedulingComplete() {
	time.Sleep(5 * time.Second)
}

func createErrorResult(err error) runResult {
	return runResult{
		err: err,
	}
}

func (r *Recommender) logError(err error) {
	webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
}
