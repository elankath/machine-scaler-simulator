### Case 1
#### WorkerGroup Configuration
worker pool 1 (single zone): min=1, max=2

#### Action
Real cluster has one node 
2 pods deployed where one pod fits per node

#### Expectations
- Should schedule one pod on the first node
- Should create a new node and assign the second pod to this node


### Case 2
worker pool 1 (single zone): min=1, max=4
existing nodes: 1

deployment with 6 replicas - 2 schedules, 4 not scheduled



### Case 3
worker pool 1 (single zone): min=1, max=2
worker pool 2 (single zone): min=1, max=2
existing nodes: wp1-1 wp2-0

deployment with 6 replicas - 2 schedules, 4 not scheduled




## Preemption
Do we need to worry about this?
