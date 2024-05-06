package recommender

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elankath/scaler-simulator/pricing"
	"github.com/samber/lo"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/rand"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elankath/scaler-simulator/simutil"
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
const resourceNameFormat = "%s-simrun-%s"

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

type Recommender struct {
	engine                 scalesim.Engine
	scenarioName           string
	shootName              string
	strategyWeights        StrategyWeights
	logWriter              http.ResponseWriter
	state                  simulationState
	podOrder               string
	instanceTypeCostRatios map[string]float64
}

type nodeScore struct {
	wasteRatio       float64
	unscheduledRatio float64
	costRatio        float64
	cumulativeScore  float64
}

type runResult struct {
	nodePoolName    string
	zone            string
	instanceType    string
	nodeScore       nodeScore
	unscheduledPods []corev1.Pod
	nodeToPods      map[string][]types.NamespacedName
	err             error
}

type simRunRef struct {
	key   string
	value string
}

func (s simRunRef) asMap() map[string]string {
	return map[string]string{
		s.key: s.value,
	}
}

type simulationState struct {
	existingNodes   []corev1.Node
	unscheduledPods []corev1.Pod
	scheduledPods   []corev1.Pod
	// eligibleNodePools holds the available node capacity per node pool.
	eligibleNodePools map[string]scalesim.NodePool
}

func (s *simulationState) updateEligibleNodePools(recommendation *Recommendation) {
	np, ok := s.eligibleNodePools[recommendation.nodePoolName]
	if !ok {
		return
	}
	np.Current += recommendation.incrementBy
	if np.Current == np.Max {
		delete(s.eligibleNodePools, recommendation.nodePoolName)
	} else {
		s.eligibleNodePools[recommendation.nodePoolName] = np
	}
}

func NewRecommender(engine scalesim.Engine, scenarioName, shootName, podOrder string, strategyWeights StrategyWeights, logWriter http.ResponseWriter) *Recommender {
	return &Recommender{
		engine:                 engine,
		scenarioName:           scenarioName,
		shootName:              shootName,
		strategyWeights:        strategyWeights,
		logWriter:              logWriter,
		instanceTypeCostRatios: make(map[string]float64),
		podOrder:               podOrder,
	}
}

func (r *Recommender) Run(ctx context.Context) ([]Recommendation, error) {
	var (
		recommendations []Recommendation
		runNumber       int
	)

	shoot, err := r.getShoot()
	if err != nil {
		webutil.InternalError(r.logWriter, err)
		return recommendations, err
	}

	r.computeCostRatiosForInstanceTypes(shoot.Spec.Provider.Workers)
	if err = r.initializeSimulationState(ctx, shoot); err != nil {
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
		simRunStartTime := time.Now()
		recommendation, winnerRunResult, err := r.runSimulation(ctx, runNumber)
		webutil.Log(r.logWriter, fmt.Sprintf("scale-up recommender run #%d completed in %f seconds", runNumber, time.Since(simRunStartTime).Seconds()))
		if err != nil {
			webutil.Log(r.logWriter, fmt.Sprintf("runSimulation for shoot %s failed, err: %v", shoot.Name, err))
			break
		}

		if recommendation == nil {
			webutil.Log(r.logWriter, fmt.Sprintf("scale-up recommender run #%d, no winner could be identified. This will happen when no pods could be assgined. No more runs are required, exiting early", runNumber))
			break
		}
		if err := r.syncWinningResult(ctx, recommendation, winnerRunResult); err != nil {
			return nil, err
		}
		webutil.Log(r.logWriter, fmt.Sprintf("For scale-up recommender run #%d, winning score is: %v", runNumber, recommendation))
		recommendations = append(recommendations, *recommendation)
	}
	return recommendations, nil
}

func (r *Recommender) getShoot() (*v1beta1.Shoot, error) {
	shoot, err := r.engine.ShootAccess(r.shootName).GetShootObj()
	if err != nil {
		return nil, err
	}
	return shoot, nil
}

func (r *Recommender) computeCostRatiosForInstanceTypes(workerPools []v1beta1.Worker) {
	totalCost := lo.Reduce[v1beta1.Worker, float64](workerPools, func(totalCost float64, pool v1beta1.Worker, _ int) float64 {
		return totalCost + pricing.GetPricing(pool.Machine.Type)
	}, 0.0)
	for _, pool := range workerPools {
		price := pricing.GetPricing(pool.Machine.Type)
		r.instanceTypeCostRatios[pool.Machine.Type] = price / totalCost
	}
}

func (r *Recommender) initializeSimulationState(ctx context.Context, shoot *v1beta1.Shoot) error {
	unscheduledPods, err := r.engine.VirtualClusterAccess().ListPods(ctx)
	if err != nil {
		return err
	}
	r.state.unscheduledPods = unscheduledPods
	return r.initializeEligibleNodePools(ctx, shoot)
}

func (r *Recommender) initializeEligibleNodePools(ctx context.Context, shoot *v1beta1.Shoot) error {
	eligibleNodePools := make(map[string]scalesim.NodePool, len(shoot.Spec.Provider.Workers))
	for _, worker := range shoot.Spec.Provider.Workers {
		nodes, err := r.engine.VirtualClusterAccess().ListNodesInNodePool(ctx, worker.Name)
		if err != nil {
			return err
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
	r.state.eligibleNodePools = eligibleNodePools
	return nil
}

// TODO: sync existing nodes and pods deployed on them. DO NOT TAINT THESE NODES.
// eg:- 1 node(A) existing in zone a. Any node can only fit 2 pods.
// deployment 6 replicas, tsc zone, minDomains 3
// 1 pod will get assigned to A. 5 pending. 3 Nodes will be scale up. (1-a, 1-b, 1-c)
// if you count existing nodes and pods, then only 2 nodes are needed.
func (r *Recommender) runSimulation(ctx context.Context, runNum int) (*Recommendation, *runResult, error) {
	/*
		    1. initializeEligibleNodePools
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
	resultCh := make(chan runResult, len(r.state.eligibleNodePools))
	r.triggerNodePoolSimulations(ctx, resultCh, runNum)

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
		return nil, nil, errs
	}

	recommendation, winnerRunResult := getWinner(results)
	return &recommendation, &winnerRunResult, nil
}

func (r *Recommender) syncWinningResult(ctx context.Context, recommendation *Recommendation, winningRunResult *runResult) error {
	startTime := time.Now()
	defer func() {
		webutil.Log(r.logWriter, fmt.Sprintf("syncWinningResult for nodePool: %s completed in %f seconds", recommendation.nodePoolName, time.Since(startTime).Seconds()))
	}()
	winningNodeName, scheduledPodNames, err := r.syncClusterWithWinningResult(ctx, winningRunResult)
	if err != nil {
		return err
	}
	return r.syncRecommenderStateWithWinningResult(ctx, recommendation, winningNodeName, scheduledPodNames)
}

func (r *Recommender) syncClusterWithWinningResult(ctx context.Context, winningRunResult *runResult) (string, []string, error) {
	node, err := r.constructNodeFromExistingNodeOfInstanceType(winningRunResult.instanceType, winningRunResult.nodePoolName, winningRunResult.zone, false, nil)
	if err != nil {
		return "", nil, err
	}
	var originalPods []corev1.Pod
	var scheduledPods []corev1.Pod
	for _, simPodObjectKeys := range winningRunResult.nodeToPods {
		for _, simPodObjectKey := range simPodObjectKeys {
			podName := toOriginalResourceName(simPodObjectKey.Name)
			pod, err := r.engine.VirtualClusterAccess().GetPod(ctx, types.NamespacedName{Name: podName, Namespace: simPodObjectKey.Namespace})
			if err != nil {
				return "", nil, err
			}
			originalPods = append(originalPods, *pod)
			podCopy := pod.DeepCopy()
			podCopy.Spec.NodeName = node.Name
			podCopy.ObjectMeta.ResourceVersion = ""
			podCopy.ObjectMeta.CreationTimestamp = metav1.Time{}
			scheduledPods = append(scheduledPods, *podCopy)
		}
	}
	if err := r.engine.VirtualClusterAccess().DeletePods(ctx, originalPods...); err != nil {
		return "", nil, err
	}
	if err = r.engine.VirtualClusterAccess().AddPods(ctx, scheduledPods...); err != nil {
		return "", nil, err
	}
	if err = r.engine.VirtualClusterAccess().AddNodes(ctx, node); err != nil {
		return "", nil, err
	}
	if err = r.engine.VirtualClusterAccess().RemoveTaintFromVirtualNode(ctx, node.Name, "node.kubernetes.io/not-ready"); err != nil {
		return "", nil, err
	}
	return node.Name, simutil.PodNames(scheduledPods), nil
}

func (r *Recommender) syncRecommenderStateWithWinningResult(ctx context.Context, recommendation *Recommendation, winningNodeName string, scheduledPodNames []string) error {
	winnerNode, err := r.engine.VirtualClusterAccess().GetNode(ctx, types.NamespacedName{Name: winningNodeName, Namespace: "default"})
	if err != nil {
		return err
	}
	r.state.existingNodes = append(r.state.existingNodes, *winnerNode)
	scheduledPods, err := r.engine.VirtualClusterAccess().ListPodsMatchingPodNames(ctx, "default", scheduledPodNames)
	if err != nil {
		return err
	}
	for _, pod := range scheduledPods {
		r.state.scheduledPods = append(r.state.scheduledPods, pod)
		r.state.unscheduledPods = slices.DeleteFunc(r.state.unscheduledPods, func(p corev1.Pod) bool {
			return p.Name == pod.Name
		})
	}
	r.state.updateEligibleNodePools(recommendation)
	return nil
}

func (r *Recommender) triggerNodePoolSimulations(ctx context.Context, resultCh chan runResult, runNum int) {
	wg := &sync.WaitGroup{}
	webutil.Log(r.logWriter, fmt.Sprintf("Starting simulation runs for %v nodePools", maps.Keys(r.state.eligibleNodePools)))
	for _, nodePool := range r.state.eligibleNodePools {
		wg.Add(1)
		runRef := simRunRef{
			key:   "app.kubernetes.io/simulation-run",
			value: nodePool.Name + "-" + strconv.Itoa(runNum),
		}
		r.runSimulationForNodePool(ctx, wg, nodePool, resultCh, runRef)
	}
	wg.Wait()
	close(resultCh)
}

func (r *Recommender) runSimulationForNodePool(ctx context.Context, wg *sync.WaitGroup, nodePool scalesim.NodePool, resultCh chan runResult, runRef simRunRef) {
	simRunStartTime := time.Now()
	defer func() {
		webutil.Log(r.logWriter, fmt.Sprintf("Simulation run: %s for nodePool: %s completed in %f seconds", runRef.value, nodePool.Name, time.Since(simRunStartTime).Seconds()))
	}()
	defer wg.Done()
	defer func() {
		if err := r.cleanUpNodePoolSimRun(ctx, runRef); err != nil {
			// In the productive code, there will not be any real KAPI and ETCD. Fake API server will never return an error as everything will be in memory.
			// For now, we are only logging this error as in the POC code since the caller of recommender will re-initialize the virtual cluster.
			webutil.Log(r.logWriter, "Error cleaning up simulation run: "+runRef.value+" err: "+err.Error())
		}
	}()

	var (
		node *corev1.Node
		err  error
	)
	slog.Info("Simulation Run", "run-no", runRef.value, "node-pool", nodePool.Name, "num-zones", len(nodePool.Zones))
	for _, zone := range nodePool.Zones {
		webutil.Log(r.logWriter, fmt.Sprintf("Starting simulation run: %s for nodePool: %s in zone: %s", runRef.value, nodePool.Name, zone))
		if node != nil {
			if err = r.resetNodePoolSimRun(ctx, node.Name, runRef); err != nil {
				resultCh <- createErrorResult(err)
				return
			}
		}
		node, err = r.constructNodeFromExistingNodeOfInstanceType(nodePool.MachineType, nodePool.Name, zone, true, &runRef)
		if err != nil {
			resultCh <- createErrorResult(err)
			return
		}
		if err = r.engine.VirtualClusterAccess().AddNodes(ctx, node); err != nil {
			resultCh <- createErrorResult(err)
			return
		}
		if err = r.engine.VirtualClusterAccess().RemoveTaintFromVirtualNodes(ctx, "node.kubernetes.io/not-ready"); err != nil {
			return
		}
		deployTime := time.Now()
		unscheduledPods, err := r.createAndDeployUnscheduledPods(ctx, runRef)
		if err != nil {
			return
		}
		// in production code FAKE KAPI will not return any error. This is only for POC code where an envtest KAPI is used.
		_, _, err = simutil.WaitForAndRecordPodSchedulingEvents(ctx, r.engine.VirtualClusterAccess(), r.logWriter, deployTime, unscheduledPods, 10*time.Second)
		if err != nil {
			resultCh <- createErrorResult(err)
			return
		}
		simRunCandidatePods, err := r.engine.VirtualClusterAccess().ListPodsMatchingLabels(ctx, runRef.asMap())
		if err != nil {
			resultCh <- createErrorResult(err)
			return
		}
		ns := r.computeNodeScore(node, simRunCandidatePods)
		resultCh <- r.computeRunResult(ctx, nodePool.Name, nodePool.MachineType, zone, runRef, ns)
	}
}

func (r *Recommender) resetNodePoolSimRun(ctx context.Context, nodeName string, runRef simRunRef) error {
	// remove the node with the nodeName
	if err := r.engine.VirtualClusterAccess().DeleteNode(ctx, nodeName); err != nil {
		return err
	}
	// remove the pods with nodeName
	pods, err := r.engine.VirtualClusterAccess().ListPodsMatchingLabels(ctx, runRef.asMap())
	if err != nil {
		return err
	}
	// TODO(rishabh-11): This is incorrect. We should not delete only those pods with matching nodeName
	podsToDelete := make([]corev1.Pod, 0, len(pods))
	for _, pod := range pods {
		if pod.Spec.NodeName == nodeName {
			podsToDelete = append(podsToDelete, pod)
		}
	}
	return r.engine.VirtualClusterAccess().DeletePods(ctx, podsToDelete...)
}

func (r *Recommender) constructNodeFromExistingNodeOfInstanceType(instanceType, poolName, zone string, forSimRun bool, runRef *simRunRef) (*corev1.Node, error) {
	referenceNode, err := r.engine.VirtualClusterAccess().GetReferenceNode(instanceType)
	if err != nil {
		return nil, err
	}
	nodeNamePrefix, err := simutil.GenerateRandomString(4)
	if err != nil {
		return nil, err
	}
	nodeLabels := referenceNode.Labels
	nodeLabels["topology.kubernetes.io/zone"] = zone
	var nodeName string
	taints := make([]corev1.Taint, 0, 1)
	if forSimRun {
		nodeName = nodeNamePrefix + "-simrun-" + runRef.value
		nodeLabels[runRef.key] = runRef.value
		taints = append(taints, corev1.Taint{Key: runRef.key, Value: runRef.value, Effect: corev1.TaintEffectNoSchedule})
	} else {
		nodeName = nodeNamePrefix + "-" + poolName
	}
	nodeLabels["kubernetes.io/hostname"] = nodeName
	delete(nodeLabels, "app.kubernetes.io/existing-node")

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeName,
			Namespace: "default",
			Labels:    nodeLabels,
		},
		Spec: corev1.NodeSpec{
			Taints: taints,
		},
		Status: corev1.NodeStatus{
			Allocatable: referenceNode.Status.Allocatable,
			Capacity:    referenceNode.Status.Capacity,
			Phase:       corev1.NodeRunning,
		},
	}
	return node, nil
}

func (r *Recommender) createAndDeployUnscheduledPods(ctx context.Context, runRef simRunRef) ([]corev1.Pod, error) {
	unscheduledPodList := make([]corev1.Pod, 0, len(r.state.unscheduledPods))
	for _, pod := range r.state.unscheduledPods {
		podCopy := pod.DeepCopy()
		podCopy.Name = fromOriginalResourceName(podCopy.Name, runRef.value)
		podCopy.Labels[runRef.key] = runRef.value
		podCopy.ObjectMeta.UID = ""
		podCopy.ObjectMeta.ResourceVersion = ""
		podCopy.ObjectMeta.CreationTimestamp = metav1.Time{}
		podCopy.Spec.Tolerations = []corev1.Toleration{
			{Key: runRef.key, Value: runRef.value, Effect: corev1.TaintEffectNoSchedule, Operator: corev1.TolerationOpEqual},
		}
		podCopy.Spec.SchedulerName = virtualcluster.BinPackingSchedulerName
		unscheduledPodList = append(unscheduledPodList, *podCopy)
	}
	return unscheduledPodList, r.engine.VirtualClusterAccess().AddPods(ctx, unscheduledPodList...)
}

func (r *Recommender) computeRunResult(ctx context.Context, nodePoolName, instanceType, zone string, runRef simRunRef, score nodeScore) runResult {
	pods, err := r.engine.VirtualClusterAccess().ListPodsMatchingLabels(ctx, runRef.asMap())
	if err != nil {
		return runResult{err: err}
	}
	unscheduledPods := make([]corev1.Pod, 0, len(pods))
	nodeToPods := make(map[string][]types.NamespacedName)
	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			unscheduledPods = append(unscheduledPods, pod)
		} else {
			nodeToPods[pod.Spec.NodeName] = append(nodeToPods[pod.Spec.NodeName], client.ObjectKeyFromObject(&pod))
		}
	}
	return runResult{
		nodePoolName:    nodePoolName,
		zone:            zone,
		instanceType:    instanceType,
		nodeScore:       score,
		unscheduledPods: unscheduledPods,
		nodeToPods:      nodeToPods,
	}
}

func (r *Recommender) computeNodeScore(scaledNode *corev1.Node, candidatePods []corev1.Pod) nodeScore {
	costRatio := r.strategyWeights.LeastCost * r.instanceTypeCostRatios[scaledNode.Labels["node.kubernetes.io/instance-type"]]
	wasteRatio := r.strategyWeights.LeastWaste * computeWasteRatio(scaledNode, candidatePods)
	unscheduledRatio := computeUnscheduledRatio(candidatePods)
	cumulativeScore := wasteRatio + unscheduledRatio*costRatio
	return nodeScore{
		wasteRatio:       wasteRatio,
		unscheduledRatio: unscheduledRatio,
		costRatio:        costRatio,
		cumulativeScore:  cumulativeScore,
	}
}

func (r *Recommender) cleanUpNodePoolSimRun(ctx context.Context, runRef simRunRef) error {
	labels := runRef.asMap()
	err := r.engine.VirtualClusterAccess().DeletePodsWithMatchingLabels(ctx, labels)
	err = r.engine.VirtualClusterAccess().DeleteNodesWithMatchingLabels(ctx, labels)
	return err
}

func getWinner(results []runResult) (Recommendation, runResult) {
	var winner runResult
	minScore := math.MaxFloat64
	var winningRunResults []runResult
	for _, v := range results {
		if v.nodeScore.cumulativeScore < minScore {
			winner = v
			minScore = v.nodeScore.cumulativeScore
		}
	}
	for _, v := range results {
		if v.nodeScore.cumulativeScore == minScore {
			winningRunResults = append(winningRunResults, v)
		}
	}
	rand.Seed(uint64(time.Now().UnixNano()))
	winningIndex := rand.Intn(len(winningRunResults))
	winner = winningRunResults[winningIndex]
	return Recommendation{
		zone:         winner.zone,
		nodePoolName: winner.nodePoolName,
		incrementBy:  int32(1),
		instanceType: winner.instanceType,
	}, winner
}

func createErrorResult(err error) runResult {
	return runResult{
		err: err,
	}
}

func (r *Recommender) logError(err error) {
	webutil.Log(r.logWriter, "Execution of scenario: "+r.scenarioName+" completed with error: "+err.Error())
}

func computeUnscheduledRatio(candidatePods []corev1.Pod) float64 {
	var totalAssignedPods int
	for _, pod := range candidatePods {
		if pod.Spec.NodeName != "" {
			totalAssignedPods++
		}
	}
	return float64(len(candidatePods)-totalAssignedPods) / float64(len(candidatePods))
}

func computeWasteRatio(node *corev1.Node, candidatePods []corev1.Pod) float64 {
	var (
		targetNodeAssignedPods []corev1.Pod
		totalMemoryConsumed    int64
	)
	for _, pod := range candidatePods {
		if pod.Spec.NodeName == node.Name {
			targetNodeAssignedPods = append(targetNodeAssignedPods, pod)
			for _, container := range pod.Spec.Containers {
				totalMemoryConsumed += container.Resources.Requests.Memory().MilliValue()
			}
			slog.Info("NodPodAssignment: ", "pod", pod.Name, "node", pod.Spec.NodeName, "memory", pod.Spec.Containers[0].Resources.Requests.Memory().MilliValue())
		}
	}
	totalMemoryCapacity := node.Status.Capacity.Memory().MilliValue()
	return float64(totalMemoryCapacity-totalMemoryConsumed) / float64(totalMemoryCapacity)
}

func fromOriginalResourceName(name, suffix string) string {
	return fmt.Sprintf(resourceNameFormat, name, suffix)
}

func toOriginalResourceName(simResName string) string {
	return strings.Split(simResName, "-simrun-")[0]
}
