# Notes

This document serves as a placeholder for all things that needs to be considered when designing the recommendation engine.
The idea is to capture the learnings from several production (Canary, Live, Dev, Staging) issues and document them here.


## Resource consumption by system pods
Reference: `Live#4618`
Gardener deploys the following system pods on every node:
```
| 2024-03-28 14:16:36 | {"log":"\"Successfully bound pod to node\" pod=\"monitoring/fluentd-7lwxq\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:35 | {"log":"\"Successfully bound pod to node\" pod=\"monitoring/promtail-large-zt8xf\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:35 | {"log":"\"Successfully bound pod to node\" pod=\"kube-system/csi-driver-node-file-7qzzh\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:35 | {"log":"\"Successfully bound pod to node\" pod=\"kube-system/csi-driver-node-disk-7x8hm\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:35 | {"log":"\"Successfully bound pod to node\" pod=\"kube-system/cloud-node-manager-bfkxz\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:35 | {"log":"\"Successfully bound pod to node\" pod=\"kube-system/calico-node-zwnzj\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:35 | {"log":"\"Successfully bound pod to node\" pod=\"kube-system/kube-proxy-hana-l-v2-v1.26.11-9kf2d\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:35 | {"log":"\"Successfully bound pod to node\" pod=\"kube-system/egress-filter-applier-zsfb2\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:35 | {"log":"\"Successfully bound pod to node\" pod=\"hana-host-config/hana-host-config-9ksxk\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:35 | {"log":"\"Successfully bound pod to node\" pod=\"default/trace-file-collector-zf4fh\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:35 | {"log":"\"Successfully bound pod to node\" pod=\"kube-system/node-exporter-28qjl\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:35 | {"log":"\"Successfully bound pod to node\" pod=\"kube-system/network-problem-detector-pod-gd59m\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:35 | {"log":"\"Successfully bound pod to node\" pod=\"kube-system/apiserver-proxy-rd84l\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:34 | {"log":"\"Successfully bound pod to node\" pod=\"kube-system/node-problem-detector-4fzv5\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:34 | {"log":"\"Successfully bound pod to node\" pod=\"hana-basis-exporter/hana-basis-exporter-6ghkz\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:34 | {"log":"\"Successfully bound pod to node\" pod=\"default/object-store-d6qr8\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
| 2024-03-28 14:16:34 | {"log":"\"Successfully bound pod to node\" pod=\"kube-system/network-problem-detector-host-26p8f\" node=\"node-1\" evaluatedNodes=1 feasibleNodes=1","pid":"1","severity":"INFO","source":"schedule_one.go:252"}
```
Points to note:
* These system pods consume node resources
* VPA can vertically scale these system pods which then increases the resource consumption by the system pods. This could potentially cause eviction of non-system pods.

`Cluster-Autoscaler` does not take into account the resource requirements for the system pods (as these are not really deployed yet on the new node). Also if it is `scale-from-zero` case then there is no information regarding reserved resource quantity inside `NodeTemplate` that
the CA can use when computing `allocatable` resources for a new node and then using it to find the node fitment for unscheduled pod(s). This results in a node being launched by CA to schedule the unscheduled pods. In the live issue each pod had massive memory requirements which 
were very close to the max allocatable `memory` for the node. So any slight increase in resource consumption by the system pods will result in yet another eviction of the customer pods. This cycle will repeat.

