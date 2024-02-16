package service

import (
	"fmt"
	"github.com/elankath/scaler-simulator/utils"
	"log/slog"
	"net/http"
	"strconv"

	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/scenarios"
)

type engine struct {
	virtualAccess scalesim.VirtualClusterAccess
	shootAccess   scalesim.ShootAccess
	mux           *http.ServeMux
}

var _ scalesim.Engine = (*engine)(nil)

func NewEngine(virtualAccess scalesim.VirtualClusterAccess, shootAccess scalesim.ShootAccess) (scalesim.Engine, error) {
	mux := http.NewServeMux()
	addRoutes(mux, virtualAccess, shootAccess)
	return &engine{
		virtualAccess: virtualAccess,
		shootAccess:   shootAccess,
		mux:           mux,
	}, nil
}

func (e *engine) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	e.mux.ServeHTTP(writer, request)
}

func (e *engine) VirtualClusterAccess() scalesim.VirtualClusterAccess {
	return e.virtualAccess
}

func (e *engine) ShootAccess() scalesim.ShootAccess {
	return e.shootAccess
}

func addRoutes(mux *http.ServeMux, virtualAccess scalesim.VirtualClusterAccess, shootAccess scalesim.ShootAccess) {
	mux.Handle("DELETE /api/virtual-cluster", handleClearVirtualCluster(virtualAccess))
	mux.Handle("POST /api/sync-shoot-nodes", handleSyncShootNodes(virtualAccess, shootAccess))
	mux.Handle("POST /scenario/{id}/{podCount}", handleScenarios(virtualAccess, shootAccess))
	mux.Handle("POST /scenario/C", handleScenarioC(virtualAccess, shootAccess))
}

func handleScenarioC(virtualAccess scalesim.VirtualClusterAccess, shootAccess scalesim.ShootAccess) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		utils.SetupSSEWriter(w)
		podCountAstr := r.URL.Query().Get("podCountA")
		podCountBstr := r.URL.Query().Get("podCountB")
		podCountA, err := strconv.Atoi(podCountAstr)
		if err != nil {
			slog.Error("Error converting podCountA to int", "error", err)
			return
		}
		podCountB, err := strconv.Atoi(podCountBstr)
		if err != nil {
			slog.Error("Error converting podCountB to int", "error", err)
			return
		}
		utils.LogEvent(w, fmt.Sprintf("handling scenario C\n"))

		scenarios.NewScenarioC(r.Context(), virtualAccess, shootAccess, w, podCountA, podCountB)
	})
}
func handleScenarios(virtualAccess scalesim.VirtualClusterAccess, shootAccess scalesim.ShootAccess) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		utils.SetupSSEWriter(w)
		id := r.PathValue("id")
		podCount, err := strconv.Atoi(r.PathValue("podCount"))
		if err != nil {
			podCount = 3
		}
		utils.LogEvent(w, fmt.Sprintf("handling scenario with id=%v\n", id))

		switch id {
		case "A":
			scenarios.NewScenarioA(r.Context(), virtualAccess, shootAccess, w, podCount)
		case "B":
			scenarios.NewScenarioB(r.Context(), virtualAccess, shootAccess, w, podCount)
		}
	})

}

func handleSyncShootNodes(virtualAccess scalesim.VirtualClusterAccess, shootAccess scalesim.ShootAccess) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			utils.SetupSSEWriter(w)
			nodes, err := shootAccess.GetNodes()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			err = virtualAccess.AddNodes(r.Context(), nodes)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			slog.Info("added nodes to virtual cluster.", "num-nodes", len(nodes))
		},
	)
}

func handleClearVirtualCluster(virtualAccess scalesim.VirtualClusterAccess) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			utils.SetupSSEWriter(w)
			err := virtualAccess.ClearAll(r.Context())
			_, _ = fmt.Fprintf(w, "Cleared virtual cluster objects")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		},
	)
}
