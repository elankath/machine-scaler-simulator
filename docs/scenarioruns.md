
### <u>ScaleUp</u>
#### Case 1 (case-up-3)

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
```

#### Case 2 (case-up-2)

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
```

#### Case 3 (case-up-3)

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

#### Case 4 (case-up-4)

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
```

####  Case 5 

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
```

### Conclusion:- 

We use the multidimensional scorer algorithm along with kube-scheduler and the following parameters 
`podDeploymentOrder = descending`
`LeastWasteWeight = 1.0`
`LeastCostWeight = 1.0`

### <u>ScaleDown</u>

### Case 1 (scenario-a)

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

```
(single zone workerpool with each workerpool in different zone)
PodA: 5GB -> 10 replicas  
TSC: topology-key = `zone`, maxSkew=1, whenUnsatisfiable=`DoNotSchedule`  
PodB: 12GB -> 2 replicas  
TSC: topology-key = `node`, maxSkew = 1, whenUnsatisfiable=`DoNotSchedule`  
NG1 -> m5.Large -> Zone-A (8gb)  (Max : 12)
NG2 -> m5.xLarge -> Zone-B (16 gb)  (Max : 5) 
NG3 -> m5.2xLarge -> Zone-C (32 gb) (Max : 5)

p1-kpvlb p1-4zgd6 p1-m945v p1-gbbxb p2-k4pz6 p2-fcr4z p2-fv9d5 p2-n42m8 p3-98llf
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