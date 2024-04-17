package recommender

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/elankath/scaler-simulator/virtualcluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
                	- copy previous winner nodes and add a taint.
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

type Recommendation struct {
	zone         string
	nodePoolName string
	incrementBy  int32
	instanceType string
}

type nodePools map[string]scalesim.NodePool

func (n nodePools) update(recommendation Recommendation) {
	np, ok := n[recommendation.nodePoolName]
	if !ok {
		return
	}
	np.Current += recommendation.incrementBy
	if np.Current == np.Max {
		delete(n, recommendation.nodePoolName)
	} else {
		n[recommendation.nodePoolName] = np
	}
}

type Recommender struct {
	engine          scalesim.Engine
	shootNodes      []corev1.Node
	scenarioName    string
	shootName       string
	strategyWeights StrategyWeights
	logWriter       http.ResponseWriter
	state           simulationState
}

type runResult struct {
	nodePoolName     string
	zone             string
	instanceType     string
	WasteRatio       float64
	UnscheduledRatio float64
	CostRatio        float64
	CumulativeScore  float64
	unscheduledPods  []corev1.Pod
	podToNode        map[string]string
	err              error
}

type simulationState struct {
	existingNodes   []corev1.Node
	unscheduledPods []corev1.Pod
	scheduledPods   []corev1.Pod
}

func (s simulationState) update(revisedUnscheduledPods []corev1.Pod) {
	for _, up := range s.unscheduledPods {
		isPodScheduled := !slices.ContainsFunc(revisedUnscheduledPods, func(pod corev1.Pod) bool {
			return pod.Name == up.Name
		})
		if isPodScheduled {
			s.scheduledPods = append(s.scheduledPods, up)
			slices.DeleteFunc(s.unscheduledPods, func(pod corev1.Pod) bool {
				return pod.Name == up.Name
			})
		}
	}
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

func (r *Recommender) Run(ctx context.Context) ([]Recommendation, error) {
	var recommendations []Recommendation
	err := r.initializeSimulationState(ctx)
	if err != nil {
		return recommendations, err
	}
	var runNumber int
	shoot, err := r.getShoot()
	if err != nil {
		webutil.InternalError(r.logWriter, err)
		return recommendations, err
	}
	var recommendation *Recommendation
	nps, err := r.getEligibleNodePools(ctx, shoot)
	if err != nil {
		webutil.InternalError(r.logWriter, err)
		return recommendations, err
	}
	for {
		runNumber++
		webutil.Log(r.logWriter, fmt.Sprintf("scale-up recommender run #%d started...", runNumber))
		if len(r.state.unscheduledPods) == 0 {
			webutil.Log(r.logWriter, "All pods are scheduled. Exiting the loop...")
			break
		}

		recommendation, err = r.runSimulation(ctx, nps, runNumber)
		if err != nil {
			webutil.Log(r.logWriter, fmt.Sprintf("Unable to get eligible node pools for shoot %s, err: %v", shoot.Name, err))
			break
		}
		if recommendation == nil {
			webutil.Log(r.logWriter, fmt.Sprintf("scale-up recommender run #%d, no winner could be identified. This will happen when no pods could be assgined. No more runs are required, exiting early", runNumber))
			break
		}
		webutil.Log(r.logWriter, fmt.Sprintf("For scale-up recommender run #%d, winning score is: %v", runNumber, recommendation))
		nps.update(*recommendation)
		recommendations = append(recommendations, *recommendation)
	}
	return recommendations, nil
}

func (r *Recommender) initializeSimulationState(ctx context.Context) error {
	unscheduledPods, err := r.engine.VirtualClusterAccess().ListPods(ctx)
	if err != nil {
		return err
	}
	r.state.unscheduledPods = unscheduledPods
	return nil
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

func (r *Recommender) runSimulation(ctx context.Context, eligibleNodePools nodePools, runNum int) (*Recommendation, error) {
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

	var results []runResult
	resultCh := make(chan runResult, len(eligibleNodePools))
	go r.triggerNodePoolSimulations(ctx, eligibleNodePools, resultCh, runNum)

	// label, taint, result chan, error chan, close chan
	var errs error
	for result := range resultCh {
		if result.err != nil {
			errs = errors.Join(errs, result.err)
		} else {
			results = append(results, result)
		}
	}
	if errs != nil {
		return nil, errs
	}
	// TODO: Ensure getWinner picks a winner randomly if scores are equal
	recommendation, unscheduledPods := getWinner(results)
	r.state.update(unscheduledPods)
	// create the node and assign the pods using spec.nodeName
	return &recommendation, nil
}

func (r *Recommender) triggerNodePoolSimulations(ctx context.Context, nps nodePools, resultCh chan runResult, runNum int) {
	wg := &sync.WaitGroup{}
	for _, nodePool := range nps {
		wg.Add(len(nodePool.Zones))
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
		result := r.computeNodeScore()
		resultCh <- result
		r.resetNodePoolSimRun()
	}
	err := r.cleanUpNodePoolSimRun(ctx, labelKey, labelValue)
	if err != nil {
		resultCh <- createErrorResult(err)
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

func (r *Recommender) setupNodePoolSimRun(ctx context.Context, key, value string) error {
	var nodeList []*corev1.Node
	resourceNameFormat := "%s-SimRun-%s"
	for _, node := range r.state.existingNodes {
		nodeCopy := node.DeepCopy()
		nodeCopy.Name = fmt.Sprintf(resourceNameFormat, node.Name, value)
		nodeCopy.Labels[key] = value
		nodeCopy.Spec.Taints = []corev1.Taint{
			{Key: key, Effect: corev1.TaintEffectNoSchedule},
		}
		nodeList = append(nodeList, nodeCopy)
	}
	if err := r.engine.VirtualClusterAccess().AddNodes(ctx, nodeList...); err != nil {
		return err
	}

	var podList []corev1.Pod
	for _, pod := range r.state.scheduledPods {
		podCopy := pod.DeepCopy()
		podCopy.Name = fmt.Sprintf(resourceNameFormat, podCopy.Name, value)
		podCopy.Labels[key] = value
		podCopy.Spec.Tolerations = []corev1.Toleration{
			{Key: key, Effect: corev1.TaintEffectNoSchedule, Operator: corev1.TolerationOpEqual},
		}
		podCopy.Spec.SchedulerName = virtualcluster.BinPackingSchedulerName
		podCopy.Spec.NodeName = fmt.Sprintf(resourceNameFormat, pod.Spec.NodeName, value)
		podList = append(podList, *podCopy)
	}
	for _, pod := range r.state.unscheduledPods {
		podCopy := pod.DeepCopy()
		podCopy.Name = fmt.Sprintf(resourceNameFormat, podCopy.Name, value)
		podCopy.Labels[key] = value
		podCopy.Spec.Tolerations = []corev1.Toleration{
			{Key: key, Effect: corev1.TaintEffectNoSchedule, Operator: corev1.TolerationOpEqual},
		}
		podCopy.Spec.SchedulerName = virtualcluster.BinPackingSchedulerName
		podList = append(podList, *podCopy)
	}
	return r.engine.VirtualClusterAccess().AddPods(ctx, podList...)
}

func (r *Recommender) cleanUpNodePoolSimRun(ctx context.Context, labelKey, labelValue string) error {
	labels := map[string]string{labelKey: labelValue}
	err := r.engine.VirtualClusterAccess().DeletePodsWithMatchingLabels(ctx, labels)
	err = r.engine.VirtualClusterAccess().DeleteNodesWithMatchingLabels(ctx, labels)
	return err
}

func (r *Recommender) getEligibleNodePools(ctx context.Context, shoot *v1beta1.Shoot) (nodePools, error) {
	eligibleNodePools := make(nodePools, len(shoot.Spec.Provider.Workers))
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
		eligibleNodePools[worker.Name] = nodePool
	}
	return eligibleNodePools, nil
}

func getWinner(results []runResult) (Recommendation, []corev1.Pod) {
	var winner runResult
	minScore := math.MaxFloat64
	for _, v := range results {
		if v.CumulativeScore < minScore {
			winner = v
			minScore = v.CumulativeScore
		}
	}
	return Recommendation{
		zone:         winner.zone,
		nodePoolName: winner.nodePoolName,
		incrementBy:  int32(1),
		instanceType: winner.instanceType,
	}, winner.unscheduledPods
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

func (r *Recommender) computeNodeScore() runResult {
	return runResult{}
}

func (r *Recommender) resetNodePoolSimRun() {
	return
}
