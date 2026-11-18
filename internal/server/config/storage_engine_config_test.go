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
	"testing"

	"github.com/spf13/viper"
)

func TestParseS3ConfigRegionAliases(t *testing.T) {
	for _, test := range []struct {
		name   string
		values map[string]interface{}
		want   string
	}{
		{name: "region name", values: map[string]interface{}{"region_name": "eu-west-1"}, want: "eu-west-1"},
		{name: "region fallback", values: map[string]interface{}{"region": "ap-southeast-2"}, want: "ap-southeast-2"},
		{name: "region name precedence", values: map[string]interface{}{"region_name": "eu-central-1", "region": "us-east-1"}, want: "eu-central-1"},
		{name: "empty region name", values: map[string]interface{}{"region_name": "", "region": "us-west-2"}, want: "us-west-2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := viper.New()
			v.Set("s3", test.values)
			cfg := &Config{}
			cfg.parseS3Config(v)
			if got := cfg.GetS3Config().Region; got != test.want {
				t.Fatalf("S3 region = %q, want %q", got, test.want)
			}
		})
	}
}
