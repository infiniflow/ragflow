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

package admin

import (
	"testing"

	"ragflow/internal/common"
)

func TestDefaultSuperuserPassword(t *testing.T) {
	tests := []struct {
		name       string
		canonical  string
		adminAlias string
		want       string
	}{
		{name: "unset keeps historical default", want: "admin"},
		{name: "docker alias", adminAlias: "from-dotenv", want: "from-dotenv"},
		{name: "canonical name", canonical: "from-config", want: "from-config"},
		{name: "canonical wins when both are set", canonical: "from-config", adminAlias: "from-dotenv", want: "from-config"},
		{name: "blank alias is unset", adminAlias: "   ", want: "admin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(common.EnvDefaultSuperuserPassword, tt.canonical)
			t.Setenv(common.EnvAdminDefaultPassword, tt.adminAlias)
			if got := defaultSuperuserPassword(); got != tt.want {
				t.Fatalf("defaultSuperuserPassword() = %q, want %q", got, tt.want)
			}
		})
	}
}
