package ecs

import (
	"fmt"
	"math"
)

type CpuUnits = uint
type MemoryMiB = uint

func makeMinMaxCeil(value float64, minValue, maxValue, step uint) uint {
	if value <= float64(minValue) || math.IsNaN(value) {
		return minValue
	} else if value >= float64(maxValue) {
		return maxValue
	}
	return uint(math.Ceil(value/float64(step))) * step
}

func fixupFargateCPU(vCpu float64) CpuUnits {
	return 1 << makeMinMaxCeil(math.Log2(vCpu)+10, 8, 14, 1) // 256…16384
}

func fixupFargateMemory(cpu CpuUnits, memoryMiB float64) MemoryMiB {
	switch cpu {
	case 256: // 0.25 vCPU
		return makeMinMaxCeil(memoryMiB, 512, 2048, 1024)
	case 512: // 0.5 vCPU
		return makeMinMaxCeil(memoryMiB, 1024, 4096, 1024)
	case 1024: // 1 vCPU
		return makeMinMaxCeil(memoryMiB, 2048, 8192, 1024)
	case 2048: // 2 vCPU
		return makeMinMaxCeil(memoryMiB, 4096, 16384, 1024)
	case 4096: // 4 vCPU
		return makeMinMaxCeil(memoryMiB, 8192, 30720, 1024)
	case 8192: // 8 vCPU
		return makeMinMaxCeil(memoryMiB, 16384, 61440, 4096)
	case 16384: // 16 vCPU
		return makeMinMaxCeil(memoryMiB, 32768, 122880, 4096)
	default:
		panic(fmt.Sprintf("Unsupported value for cpu: %v", cpu))
	}
}

func FixupFargateConfig(vCpu, memoryMiB float64) (cpu CpuUnits, memory MemoryMiB) {
	for cpu = fixupFargateCPU(vCpu); ; cpu *= 2 {
		memory = fixupFargateMemory(cpu, memoryMiB)
		if float64(memory) >= memoryMiB {
			return
		}
	}
}
