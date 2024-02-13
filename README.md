# scaler-simulator
Simulator that determines which worker pool must be scaled to host unschedulable pods


## Goals

1. Advice which worker pool must be extended to host the unschedulable pod(s).
1. Simulate scaling the adviced worker pool and pod scheduling on scaled nodes and compare real-world against simulation.
 

## Setup

1. Ensure you are using Go version above `1.21`. Use `go version` to check your version.
1. Run `./hack/setup.sh`
   1. This will generate a `launch.env` file in the project dir
1. Source the `launch.env` file using command below (only necessary once in term session)
   1. `set -o allexport && source launch.env && set +o allexport`
1. Run the simulation server: `go run cmd/simserver/main.go`


### Executing within Goland/Intellij IDE

1. Install the [EnvFile](https://plugins.jetbrains.com/plugin/7861-envfile) plugin.
1. There is a run configuration already checked-in at `.idea/.idea/runConfigurations/LaunchSimServer.xml`
   1. This will automatically source the generated `.env` leveraging the plugin
   2. You should be able to execute using `Run > LaunchSimServer`
