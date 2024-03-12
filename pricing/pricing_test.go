package pricing

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLoadInstancePricing(t *testing.T) {
	results, err := LoadInstancePricing()
	assert.Nil(t, err)
	assert.NotNil(t, results)
	fmt.Printf("%+v", results)
}
