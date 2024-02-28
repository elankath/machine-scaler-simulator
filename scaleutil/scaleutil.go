package scaleutil

import (
	"fmt"
	scalesim "github.com/elankath/scaler-simulator"
	"github.com/elankath/scaler-simulator/webutil"
	"log/slog"
	"net/http"
	"strings"
)

// ParseRecommendationsAndScaleUp parses the recommendations and scales up the actual cluster
func ParseRecommendationsAndScaleUp(s scalesim.ShootAccess, recommendations scalesim.ScalerRecommendations, w http.ResponseWriter) error {
	slog.Info("Parsing recommendations and scaling up", "recommendations", recommendations)
	mcds, err := s.GetMachineDeployments()
	if err != nil {
		slog.Error("Error getting machine deployments", "error", err)
		return err
	}
	for key, increment := range recommendations {
		strs := strings.Split(key, "/")
		pool := strs[0]
		zone := strs[1]
		for _, mcd := range mcds {
			slog.Info(fmt.Sprintf("Pool: %s Zone: %s", pool, zone))
			if mcd.Spec.Template.Spec.NodeTemplateSpec.Labels["topology.ebs.csi.aws.com/zone"] == zone && mcd.Spec.Template.Spec.NodeTemplateSpec.Labels["worker.gardener.cloud/pool"] == pool {
				desiredReplicas := mcd.Spec.Replicas + (int32)(increment)
				slog.Info("Scaling up", "mcd", mcd.Name, "zone", zone, "replicas", desiredReplicas, "increment", increment)
				webutil.Log(w, fmt.Sprintf("Scaling up %s mcd to %d replicas", mcd.Name, desiredReplicas))
				err := s.ScaleMachineDeployment(mcd.Name, desiredReplicas)
				if err != nil {
					slog.Error("Error scaling up", "mcd", mcd.Name, "zone", zone, "replicas", desiredReplicas, "error", err)
					return err
				}
				break
			}
		}
	}
	return nil
}
