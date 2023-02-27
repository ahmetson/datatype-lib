package network

import "fmt"

type NetworkType string

const (
	ALL NetworkType = "all" // any blockchain
	EVM NetworkType = "evm" // with EVM
	IMX NetworkType = "imx" // without EVM, it's an L2
)

func NewNetworkType(network_type string) (NetworkType, error) {
	new_type := NetworkType(network_type)
	if !new_type.valid() {
		return new_type, fmt.Errorf("unsupported network type")
	}

	return new_type, nil
}

// Whether the given flag is valid Network Flag or not.
func (network_type NetworkType) valid() bool {
	return network_type == ALL || network_type == EVM || network_type == IMX
}