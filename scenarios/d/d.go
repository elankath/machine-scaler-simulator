package d

import (
	"fmt"
	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/webutil"
	"log/slog"
	"net/http"
)

var shootName = "scenario-c"
var scenarioName = "C"

type scenarioD struct {
	engine scalesim.Engine
}

func (s *scenarioD) Description() string {
	return "Scenario D tests scaleup of worker pools when there are unschedulable pods with topology spread constraints for different zones."
}

func (s *scenarioD) Name() string {
	return scenarioName
}

func (s *scenarioD) ShootName() string {
	return shootName
}

func New(engine scalesim.Engine) scalesim.Scenario {
	return &scenarioD{
		engine: engine,
	}
}

// Scenario D tests scaleup of worker pools when there are unschedulable pods with topology spread constraints for different zones.
func (s *scenarioD) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	webutil.Log(w, "Commencing scenario: "+s.Name()+"...")
	webutil.Log(w, "Clearing virtual cluster..")
	err := s.engine.VirtualClusterAccess().ClearAll(r.Context())
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	webutil.Log(w, fmt.Sprintf("Synchronizing virtual nodes with nodes of shoot: %s ...", shootName))
	err = s.engine.SyncVirtualNodesWithShoot(r.Context(), shootName)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	shoot, err := s.engine.ShootAccess(shootName).GetShootObj()
	if err != nil {
		webutil.InternalError(w, err)
		return
	}

	//scaleStartTime := time.Now()
	webutil.Log(w, "Scaling till worker pool max...")
	numCreatedNodes, err := s.engine.ScaleAllWorkerPoolsTillMax(r.Context(), s.Name(), shoot, w)
	if err != nil {
		webutil.InternalError(w, err)
		return
	}
	webutil.Log(w, fmt.Sprintf("Created %d total nodes", numCreatedNodes))
	if err != nil {
		webutil.Log(w, "Execution of scenario: "+s.Name()+" completed with error: "+err.Error())
		slog.Error("Execution of scenario: "+s.Name()+" ran into error", "error", err)
		return
	}
}
