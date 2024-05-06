
### <u>ScaleUp</u>
#### Case 1 (case-up-3)

`curl -XPOST 'localhost:8080/scenarios/score4?small=20&large=0&leastWaste=1.0&leastCost=1.0&shoot=case-up-3'`

```
(single zone workerpool)
PodA : 5Gb -> 20 Replicas
NG1 : m5.large -> 8Gb; NG1Max: 12
NG2 : m5.4xlarge -> 64Gb; NG2Max: 4

(no effect of pod order here, as only one pod type)
Result (optimal) -> 2 * NG2 (lower fragmentation)
Result of CA -> 2 * NG2
Result of using ksc(vanilla) -> 12 * NG1 + 1 * NG2
Result of using ksc+algo (lW = 1,lC = 1) -> 8 * NG1 + 1 * NG2 
Result of using ksc+algo (lW = 1,lC = 1.5) -> 11 * NG1 + 1 * NG2

lW = 1, lC = 1
cost ratio * unscheduled ratio + least waste ratio = total
NG2:= (8/9)*(8/20) + 4/64 = 0.418
NG1:= (1/9)*(19/20) + 3/8 = 0.480
```

#### Case 2 (case-up-2)

 `curl -XPOST 'localhost:8080/scenarios/score4?small=10&large=1&leastWaste=1.0&leastCost=1.0&shoot=case-up-2&podOrder=desc'`

```
(single zone workerpool)
PodA : 5Gb -> 10 Replica
PodB : 12Gb -> 1 Replica
NG1 : m5.large -> 8Gb; NG1Max: 12
NG2 : m5.xlarge -> 16Gb; NG2Max: 5
NG3 : m5.4xlarge -> 64Gb; NG3Max: 5

Result (optimal) ->  1 * NG3 + 1 * NG1
Result of CA -> 1 * NG3 + 1 * NG1
Result of using ksc(vanilla) -> 10 * NG1 + 1 * NG2
Result of using ksc+algo (lW = 1,lC = 1, pd = none) -> 1 * NG2 + 1 * NG3 (cannot reach optimal as it does not consider pod order.)
Result of using ksc+algo (lW = 1,lC = 1, pd = desc) -> 1 * NG1 + 1 * NG3 
Result of using ksc+algo (lW = 1,lC = 1.5,pd = none) -> 1 * NG2 + 1 * NG3 
Result of using ksc+algo (lW = 1,lC = 1.5,pd = desc) -> 1 * NG1 + 1 * NG3 

lW = 1, lC = 1
cost ratio * unscheduled ratio + least waste ratio = total

NG1:= (1/11)*(10/11) + 3/8 = 0.457
NG2:= (2/11)*(10/11) + 4/16 = 0.415
NG3:= (8/11)*(1/11) + 7/64 = 0.175

NG3 wins!!

NG1:= (1/11)*(0/1) + 3/8
NG2:= (2/11)*(0/1) + 11/16
NG3:= (8/11)*(0/1) + 59/64

NG1 wins!!
```

#### Case 3 (case-up-3)

`curl -XPOST 'localhost:8080/scenarios/score4?small=11&large=1&leastWaste=1.0&leastCost=1.0&shoot=case-up-3&podOrder=desc'`

```
(single zone workerpool)
PodA : 5Gb -> 11 Replicas
PodB : 12Gb -> 1 Replicas
NG1 : m5.large -> 8Gb; NG1Max: 12
NG2 : m5.4xlarge -> 64Gb; NG2Max: 4

Result (optimal) ->  1 * NG2 + 2 * NG1
Result of CA -> 1 * NG2 + 2 * NG1
Result of using ksc(vanilla) -> 11 * NG1 + 1 * NG2
Result of using ksc+algo (lW = 1,lC = 1, pd = none) -> 2 * NG2
Result of using ksc+algo (lW = 1,lC = 1, pd = desc) -> 2 * NG1 + 1 * NG2
Result of using ksc+algo (lW = 1,lC = 1.5,pd = none) -> 11 * NG1 + 1 * NG2
Result of using ksc+algo (lW = 1,lC = 1.5,pd = desc) ->  11 * NG1 + 1 * NG2
```

#### Case 4 (scenario-4)

`curl -XPOST 'localhost:8080/scenarios/score5?small=10&large=2&leastWaste=1.0&leastCost=1.0&shoot=scenario-4&podOrder=desc&withTSC=true'`

```
(single zone workerpool with each workerpool in different zone)
PodA: 5GB -> 10 replicas  
TSC: topology-key = `zone`, maxSkew=1, whenUnsatisfiable=`DoNotSchedule`  
PodB: 12GB -> 2 replicas  
TSC: topology-key = `node`, maxSkew = 1, whenUnsatisfiable=`DoNotSchedule`  
NG1 -> m5.Large -> Zone-A (8gb)  (Max : 12)
NG2 -> m5.xLarge -> Zone-B (16 gb)  (Max : 5)
NG3 -> m5.4xLarge -> Zone-C (64 gb) (Max : 5)


Result (optimal) ->  3 * NG1, 3 * NG2, 1 * NG3
Result of CA -> 
Result of using ksc(vanilla) ->  4 * NG1 + 4 * NG2 + 1 * NG3 
Result of using ksc+algo (lW = 1,lC = 1, pd = none) -> 3 * NG1 + 4 * NG2 + 1 * NG3 
Result of using ksc+algo (lW = 1,lC = 1, pd = desc) -> 3 * NG1 + 4 * NG2 + 1 * NG3
Result of using ksc+algo (lW = 1,lC = 1.5,pd = none) -> 3 * NG1 + 4 * NG2 + 1 * NG3 
Result of using ksc+algo (lW = 1,lC = 1.5,pd = desc) -> 3 * NG1 + 4 * NG2 + 1 * NG3 

Result of using ksc+newAlgo(least waste(capacity and not allocatable) + cost * unscheduled) ->  3 * NG1, 3 * NG2, 1 * NG3
```

####  Case 5 (case-up-5)

```
PodA : 3Gb -> 15Repl
NG1 : m5.large -> 8GB; NG1Max:  1
NG2 : m5.xlarge -> 16GB; NG2Max : 2
NG3 : m5.2xlarge -> 32Gb; NG3Max: 5
NG4 : m5.4xlarge -> 64Gb; NG4Max: 5

(no effect of pod order here, as only one pod type)

Result(optimal) -> 1 * NG1 + 1 * NG2 + 1 * NG3 
Result of CA -> 1 * NG1, 2 * NG2, 1 * NG3
#run 1
op 1 : 2/8
op 2 : 2/32
op 3 : 19/64
op 4 : 19/64
chosen : op2
Rem pods : 5
Scaleup : 2 * NG2
 
#run 2
op1 : 2/8
op2 : NA
op3 : 17/32
op4 : 49/64
chosen : op1
Rem pods : 3
Scaleup : 1 * NG1
 
#run 3
Scaleup : 1 * NG3

Result of using ksc+algo (lW = 1,lC = 1, pd = none, desc) -> 1 * NG1 + 1 * NG2 + 1 * NG3 
Result of using ksc+algo (lW = 1,lC = 1.5, pd = none, desc) -> 1 * NG1 + 1 * NG2 + 1 * NG3 

Result of using ksc+newAlgo(least waste(capacity and not allocatable) + cost * unscheduled) ->  1 * NG1, 1 * NG2, 1 * NG3

```

### Conclusion:- 

We use the multidimensional scorer algorithm along with kube-scheduler and the following parameters 
`podDeploymentOrder = descending`
`LeastWasteWeight = 1.0`
`LeastCostWeight = 1.0`

### <u>ScaleDown</u>

### Case 1 (scenario-a)

` curl -XPOST 'localhost:8080/scenarios/scaledown/simple?small=10&large=4'`

```
PodA: 2GB -> 10 replicas
PodB: 12GB -> 4 replicas
NG1 -> m5.large -> 7
NG2 -> m5.2xlarge -> 4

Initial pod to node distribution : 4 * NG1 for podAs, 4 * NG2 for podBs (running default scheduler)

Result (optimal) : 2 * NG1, 4 * NG2
Result of scaledown algo : 2 * NG1, 4 * NG2
```

### Case 2 (case-up-4)

`curl -XPOST 'localhost:8080/scenarios/scaledown/tsc?small=10&large=2'`

```
(single zone workerpool with each workerpool in different zone)
PodA: 5GB -> 10 replicas  
TSC: topology-key = `zone`, maxSkew=1, whenUnsatisfiable=`DoNotSchedule`  
PodB: 12GB -> 2 replicas  
TSC: topology-key = `node`, maxSkew = 1, whenUnsatisfiable=`DoNotSchedule`  
NG1 -> m5.Large -> Zone-A (8gb)  (Max : 12)
NG2 -> m5.xLarge -> Zone-B (16 gb)  (Max : 5) 
NG3 -> m5.2xLarge -> Zone-C (32 gb) (Max : 5)

Initial pod to node distribution : 4 * NG1, 4 * NG2, 1 * NG3 (running default scheduler)
Result (optimal) : 3 * NG1, 3 * NG2, 1 * NG3 
Result of scaledown algo : 3 * NG1, 3 * NG2, 1 * NG3 
```

### Case 3 (scenario-a)

```
PodA: 2GB -> 7 replicas
NG1 -> m5.large -> 7
NG2 -> m5.xlarge -> 1
Result (optimal) : 1 * NG2
Result of scaledown algo : 3 * NG1
```