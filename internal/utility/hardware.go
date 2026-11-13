//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package utility

import (
	"fmt"
	"runtime"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

type HardwareInfo struct {
	Arch     string
	CPUCores int
	Memory   uint64
	OS       string
}

func GetHardwareInfo() (*HardwareInfo, error) {
	logicalCores, err := cpu.Counts(true)
	if err != nil {
		return nil, fmt.Errorf("fail to get logical cores count: %w", err)
	}

	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("fail to get virtual memory info: %w", err)
	}

	return &HardwareInfo{
		Arch:     runtime.GOARCH,
		CPUCores: logicalCores,
		Memory:   vmStat.Total,
		OS:       runtime.GOOS,
	}, nil
}
