package service

import (
	"fmt"
	"net/http"

	scalesim "github.com/elankath/scaler-simulator"
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

func addRoutes(mux *http.ServeMux, virtualAccess scalesim.VirtualClusterAccess, shootAccess scalesim.ShootAccess) {
	mux.Handle("DELETE /api/virtual-cluster", handleClearVirtualCluster(virtualAccess))
	//mux.Handle("GET /api/bingo", http.HandleFunc())
}

func handleClearVirtualCluster(virtualAccess scalesim.VirtualClusterAccess) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			err := virtualAccess.ClearAll(r.Context())
			_, _ = fmt.Fprintf(w, "Cleared virtual cluster objects")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		},
	)
}
