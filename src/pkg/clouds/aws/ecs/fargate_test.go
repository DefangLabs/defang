package ecs

import (
	"math"
	"testing"
)

func TestFixupFargateCPU(t *testing.T) {
	tests := []struct {
		vcpu    float64
		wantCPU CpuUnits
	}{
		{0.0, 256},
		{0.26, 512},
		{111.0, 16384},
	}

	for _, tt := range tests {
		if gotCPU := fixupFargateCPU(tt.vcpu); gotCPU != tt.wantCPU {
			t.Errorf("fixupFargateCPU(%v) = %v, want %v", tt.vcpu, gotCPU, tt.wantCPU)
		}
	}
}

func TestFixupFargateMemory(t *testing.T) {
	tests := []struct {
		cpu     CpuUnits
		memMiB  float64
		wantMem MemoryMiB
	}{
		{256, 0, 512},
		{256, 1023, 1024},
		{256, 1024, 1024},
		{256, 1025, 2048},
	}

	for _, tt := range tests {
		if gotMem := fixupFargateMemory(tt.cpu, tt.memMiB); gotMem != tt.wantMem {
			t.Errorf("fixupFargateMemory(%v, %v) = %v, want %v", tt.cpu, tt.memMiB, gotMem, tt.wantMem)
		}
	}
}

func TestMakeMinMaxCeil(t *testing.T) {
	tests := []struct {
		value float64
		min   uint
		max   uint
		ceil  uint
		want  uint
	}{
		{math.NaN(), 5, 100, 10, 5},
		{0, 5, 100, 10, 5},
		{6, 5, 100, 10, 10},
		{1, 1, 100, 10, 1},
		{1.1, 1, 100, 10, 10},
		{89, 1, 100, 10, 90},
		{90, 1, 100, 10, 90},
		{91, 1, 100, 10, 100},
	}

	for _, tt := range tests {
		if got := makeMinMaxCeil(tt.value, tt.min, tt.max, tt.ceil); got != tt.want {
			t.Errorf("makeMinMaxCeil(%v, %v, %v, %v) = %v, want %v", tt.value, tt.min, tt.max, tt.ceil, got, tt.want)
		}
	}
}

func TestFixupFargateConfig(t *testing.T) {
	tests := []struct {
		vcpu    float64
		memMiB  float64
		wantCPU CpuUnits
		wantMem MemoryMiB
	}{
		{0.0, 0.0, 256, 512},
		{0.25, 0, 256, 512},
		{0.0, 1024, 256, 1024},
		{0.26, 0, 512, 1024},
		{0.26, 1024, 512, 1024},
		{111.0, 1024, 16384, 32768},
	}

	for _, tt := range tests {
		gotCPU, gotMem := FixupFargateConfig(tt.vcpu, tt.memMiB)
		if gotCPU != tt.wantCPU || gotMem != tt.wantMem {
			t.Errorf("FixupFargateConfig(%v, %v) = %v, %v, want %v, %v", tt.vcpu, tt.memMiB, gotCPU, gotMem, tt.wantCPU, tt.wantMem)
		}
	}
}
