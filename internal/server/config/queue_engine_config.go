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
	"fmt"

	"github.com/spf13/viper"
)

type QueueEngineConfig struct {
	NATS NATSConfig `mapstructure:"nats"`
}

// NATSConfig NATS queue configuration
type NATSConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

func (c *Config) ParseQueueEngineConfig(v *viper.Viper) error {
	queueEngineType := c.general.QueueEngine
	var err error
	switch queueEngineType {
	case "nats":
		err = c.parseNATSConfig(v)
	default:
		return fmt.Errorf("queue engine type %s is not supported", queueEngineType)
	}

	return err
}

func (c *Config) parseNATSConfig(v *viper.Viper) error {
	// Default NATS config
	c.queueEngine.NATS.Host = "localhost"
	c.queueEngine.NATS.Port = 4222

	if !v.IsSet("nats") {
		return nil
	}
	sub := v.Sub("nats")
	if sub == nil {
		return nil
	}

	if sub.IsSet("host") {
		c.queueEngine.NATS.Host = sub.GetString("host")
	}

	if sub.IsSet("port") {
		c.queueEngine.NATS.Port = sub.GetInt("port")
	}

	return nil
}

func (c *Config) GetNATSConfig() NATSConfig {
	return c.queueEngine.NATS
}

func (n NATSConfig) ExportConfigs() map[string]interface{} {
	var natsConfigs map[string]interface{}
	natsConfigs = make(map[string]interface{})
	natsConfigs["host"] = n.Host
	natsConfigs["port"] = n.Port
	return natsConfigs
}
