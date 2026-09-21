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

package config

import (
	"reflect"
	"testing"

	"github.com/spf13/viper"
)

func TestParseAPIServerConfigTrustedProxies(t *testing.T) {
	for _, test := range []struct {
		name   string
		values map[string]interface{}
		want   []string
	}{
		// nil means "not configured": the engine falls back to the loopback default.
		{name: "unset", values: map[string]interface{}{"http_port": 9380}, want: nil},
		{name: "explicit list", values: map[string]interface{}{"trusted_proxies": []string{"10.0.0.0/8", "192.168.1.5"}}, want: []string{"10.0.0.0/8", "192.168.1.5"}},
		// An explicit empty list must survive as empty (trust nobody), not collapse to nil.
		{name: "explicit empty", values: map[string]interface{}{"trusted_proxies": []string{}}, want: []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := viper.New()
			v.Set("ragflow", test.values)
			cfg := &Config{}
			if err := cfg.ParseAPIServerConfig(v); err != nil {
				t.Fatalf("ParseAPIServerConfig: %v", err)
			}
			if got := cfg.GetAPIServerConfig().TrustedProxies; !reflect.DeepEqual(got, test.want) {
				t.Fatalf("TrustedProxies = %#v, want %#v", got, test.want)
			}
		})
	}
}
