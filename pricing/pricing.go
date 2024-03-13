package pricing

import (
	"errors"
	scalesim "github.com/elankath/scaler-simulator"
	"k8s.io/apimachinery/pkg/util/json"
	"os"
	path "path"
	"runtime"
)

var pricingMap map[string]scalesim.InstancePricing

func LoadInstancePricing() (map[string]scalesim.InstancePricing, error) {
	var allPricing scalesim.AllPricing

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return nil, errors.New("no caller information")
	}
	dirName := path.Dir(filename)
	filePath := path.Join(dirName, "aws_pricing_eu-west-1.json")
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(content, &allPricing)

	pricingMap = make(map[string]scalesim.InstancePricing)
	for _, pricing := range allPricing.Results {
		pricingMap[pricing.InstanceType] = pricing
	}

	return pricingMap, nil
}

func GetPricing(machineType string) float64 {
	if pricingMap == nil {
		LoadInstancePricing()
	}
	return pricingMap[machineType].EDPPrice.Reserved3Year
}
