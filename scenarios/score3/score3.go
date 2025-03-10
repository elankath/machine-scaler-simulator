package score3

//
//
//import (
//	"fmt"
//	"net/http"
//	"time"
//
//	scalesim "github.com/elankath/scaler-simulator"
//	"github.com/elankath/scaler-simulator/simutil"
//	"github.com/elankath/scaler-simulator/webutil"
//)
//
//var shootName = "scenario-c2"
//var scenarioName = "score3"
//
//type scenarioScore3 struct {
//	engine scalesim.Engine
//}
//
//func New(engine scalesim.Engine) scalesim.Scenario {
//	return &scenarioScore3{
//		engine: engine,
//	}
//}
//
//// Scenario C will first scale up nodes in all worker pools of scenario-c shoot to MAX
//// Then deploy Pods small and large according to count.
//// Then wait till all Pods are scheduled or till timeout.
//func (s *scenarioScore3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
//	webutil.Log(w, "Commencing scenario: "+s.Name()+"...")
//	webutil.Log(w, "Clearing virtual cluster..")
//	err := s.engine.VirtualClusterAccess().ClearAll(r.Context())
//	if err != nil {
//		webutil.InternalError(w, err)
//		return
//	}
//	webutil.Log(w, fmt.Sprintf("Synchronizing virtual nodes with nodes of shoot: %s ...", shootName))
//	err = s.engine.SyncVirtualNodesWithShoot(r.Context(), shootName)
//	if err != nil {
//		webutil.InternalError(w, err)
//		return
//	}
//	shoot, err := s.engine.ShootAccess(shootName).GetShootObj()
//	if err != nil {
//		webutil.InternalError(w, err)
//		return
//	}
//
//	smallCount := webutil.GetIntQueryParam(r, "small", 10)
//	largeCount := webutil.GetIntQueryParam(r, "large", 1)
//
//	podSpecPath := "scenarios/score3/podLarge.yaml"
//	webutil.Log(w, fmt.Sprintf("Deploying podSpec %s with count %d...", podSpecPath, largeCount))
//	err = s.engine.VirtualClusterAccess().CreatePodsFromYaml(r.Context(), podSpecPath, largeCount)
//	if err != nil {
//		webutil.InternalError(w, err)
//		return
//	}
//
//	podSpecPath = "scenarios/score3/podSmall.yaml"
//	if err != nil {
//		webutil.InternalError(w, err)
//		return
//	}
//	webutil.Log(w, fmt.Sprintf("Deploying podSpec %s with count %d...", podSpecPath, smallCount))
//	err = s.engine.VirtualClusterAccess().CreatePodsFromYaml(r.Context(), podSpecPath, smallCount)
//	if err != nil {
//		webutil.InternalError(w, err)
//		return
//	}
//
//	podListForRun, err := s.engine.VirtualClusterAccess().ListPods(r.Context())
//	if err != nil {
//		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//		return
//	}
//
//	runCounter := 0
//	for {
//		runCounter++
//		unscheduledPodCount, err := simutil.GetUnscheduledPodCount(r.Context(), s.engine.VirtualClusterAccess())
//		if err != nil {
//			webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//			return
//		}
//		if unscheduledPodCount == 0 {
//			webutil.Log(w, "All pods are scheduled. Exiting the loop...")
//			break
//		}
//		webutil.Log(w, fmt.Sprintf("Scenario-Run#%d: Scaling workerpool by one, one by one...", runCounter))
//		nodeScores := scalesim.NodeRunResults(make(map[string]scalesim.NodeRunResult))
//		for _, pool := range shoot.Spec.Provider.Workers {
//			webutil.Logf(w, "Scaling workerpool %s...", pool.Name)
//			scaledNode, err := simutil.CreateNodeInWorkerGroup(r.Context(), s.engine.VirtualClusterAccess(), &pool)
//			if err != nil {
//				webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//				return
//			}
//			err = s.engine.VirtualClusterAccess().RemoveAllTaintsFromVirtualNodes(r.Context())
//			if err != nil {
//				webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//				return
//			}
//			webutil.Log(w, "Waiting for 5 seconds before calculating node score...")
//			time.Sleep(5 * time.Second)
//			allPods, err := s.engine.VirtualClusterAccess().ListPods(r.Context())
//			if err != nil {
//				webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//				return
//			}
//
//			podListForRun = simutil.GetMatchingPods(allPods, podListForRun)
//			nodeScore, err := simutil.ComputeNodeRunResult(scaledNode, podListForRun, shoot.Spec.Provider.Workers)
//			if err != nil {
//				webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//				return
//			}
//			if nodeScore.NumAssignedPodsToNode == 0 {
//				webutil.Log(w, "No pods are scheduled on the candidate scaled node "+scaledNode.Name+". This node group will not be considered in this run")
//				continue
//			}
//			nodeScores[scaledNode.Name] = nodeScore
//			webutil.Log(w, fmt.Sprintf("Node score for %s: %v", scaledNode.Name, nodeScores[scaledNode.Name]))
//			webutil.Log(w, "Deleting scaled node and clearing pod assignments...")
//			podListForRun, err = simutil.DeleteNodeAndResetPods(r.Context(), s.engine.VirtualClusterAccess(), scaledNode.Name, podListForRun)
//			if err != nil {
//				webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//				return
//			}
//		}
//		if nodeScores.GetTotalAssignedPods() == 0 {
//			webutil.Log(w, fmt.Sprintf("No pods are scheduled on the scaled nodes on Run#%d. Exiting the loop...", runCounter))
//			webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error")
//			break
//		}
//		webutil.Log(w, fmt.Sprintf("%+v", nodeScores))
//		winnerNodeScore := nodeScores.GetWinner()
//		webutil.Log(w, "Winning Score: "+winnerNodeScore.String())
//		scaledNode, err := simutil.CreateNodeInWorkerGroup(r.Context(), s.engine.VirtualClusterAccess(), winnerNodeScore.Pool)
//		if err != nil {
//			webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//			return
//		}
//		err = s.engine.VirtualClusterAccess().RemoveAllTaintsFromVirtualNodes(r.Context())
//		if err != nil {
//			webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//			return
//		}
//		webutil.Log(w, "Waiting for 3 seconds for pod assignments to winning scalednode: "+scaledNode.Name)
//		time.Sleep(3 * time.Second)
//
//		assignedPods, err := simutil.GetPodsAssignedToNode(r.Context(), s.engine.VirtualClusterAccess(), scaledNode.Name)
//		if err != nil {
//			webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error")
//			return
//		}
//		if len(assignedPods) == 0 {
//			webutil.Log(w, "No pods are assigned to the winning scaled node "+scaledNode.Name)
//			webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error")
//			return
//		}
//		podListForRun = simutil.DeleteAssignedPods(podListForRun, assignedPods)
//	}
//
//	//numCreatedNodes, err := s.engine.ScaleAllWorkerPoolsTillMax(r.Context(), s.Name(), shoot, w)
//	//if err != nil {
//	//	webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//	//	webutil.InternalError(w, err)
//	//	slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
//	//	return
//	//}
//	//webutil.Log(w, fmt.Sprintf("Created %d total virtual nodes", numCreatedNodes))
//
//	//timeoutSecs := 5 * time.Second
//	//webutil.Logf(w, "Waiting till there are no unschedulable pods or timeout of %.2f secs", timeoutSecs.Seconds())
//	//err = simutil.WaitTillNoUnscheduledPodsOrTimeout(r.Context(), s.engine.VirtualClusterAccess(), timeoutSecs, scaleStartTime)
//	//if err != nil { // TODO: too much repetition move this to scenarios as utility function
//	//	webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//	//	slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
//	//}
//	//
//	//
//	//timeoutSecs = 30 * time.Second
//	//webutil.Logf(w, "Waiting till there are no unschedulable pods or timeout of %.2f secs", timeoutSecs.Seconds())
//	//err = simutil.WaitTillNoUnscheduledPodsOrTimeout(r.Context(), s.engine.VirtualClusterAccess(), timeoutSecs, scaleStartTime)
//	//if err != nil { // TODO: too much repetition move this to scenarios as utility function
//	//	webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//	//	slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
//	//}
//
//	//webutil.Log(w, "Trimming virtual cluster...")
//	//err = s.engine.VirtualClusterAccess().TrimCluster(r.Context())
//	//if err != nil {
//	//	webutil.InternalError(w, err)
//	//	return
//	//}
//	//
//	//nodePodAssignments, err := simutil.GetNodePodAssignments(r.Context(), s.engine.VirtualClusterAccess())
//	//if err != nil {
//	//	webutil.InternalError(w, err)
//	//	return
//	//}
//	//
//	//recommendation, err := simutil.GetScalerRecommendation(r.Context(), s.engine.VirtualClusterAccess(), nodePodAssignments)
//	//if err != nil {
//	//	webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//	//	slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
//	//	return
//	//}
//	//
//	//err = simutil.PrintScheduledPodEvents(r.Context(), s.engine.VirtualClusterAccess(), scaleStartTime, w)
//	//if err != nil {
//	//	webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
//	//	slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
//	//	return
//	//}
//
//	webutil.Log(w, fmt.Sprintf("Scenario-%s Completed!", s.Name()))
//	//webutil.LogNodePodAssignments(w, s.Name(), nodePodAssignments)
//	//slog.Info("Execution of scenario " + s.Name() + " completed!")
//	//webutil.Log(w, "Recommendation for Scaleup: "+recommendation.String())
//
//}
//
//var _ scalesim.Scenario = (*scenarioScore3)(nil)
//
//func (s scenarioScore3) Description() string {
//	return "Scale 2 Worker Pool with machine type m5.large, m5.2xlarge with replicas of small and large pods"
//}
//
//func (s scenarioScore3) ShootName() string {
//	return shootName
//}
//
//func (s scenarioScore3) Name() string {
//	return scenarioName
//}
