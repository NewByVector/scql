// Copyright 2023 Ant Group Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewConfig(t *testing.T) {
	r := require.New(t)

	// when
	cfg, err := NewConfig("../testdata/config_test.yml")
	r.NoError(err)

	// then
	expectedCfg := &Config{
		IntraServer: ServerConfig{
			Host:     "127.0.0.1",
			Port:     8081,
			Protocol: "https",
			CertFile: "cert_file",
			KeyFile:  "key_file",
		},
		InterServer: ServerConfig{
			Port: 8082,
		},
		InterTimeout:                 5 * time.Second,
		IntraHost:                    "localhost:8081",
		LogLevel:                     "debug",
		PartyCode:                    "alice",
		Engines:                      []string{"alice.com"},
		PartyInfoFile:                "../testdata/party_info.json",
		PrivatePemPath:               "../testdata/private_key_alice.pem",
		SessionExpireTime:            1000 * time.Second,
		SessionCheckInterval:         10 * time.Second,
		ExchangeJobInfoRetryTimes:    2,
		ExchangeJobInfoRetryInterval: 10 * time.Second,
		Engine: EngineConfig{
			ClientTimeout: 120 * time.Second,
			Protocol:      "http",
			ContentType:   "application/json",
		},
		Storage: StorageConf{
			Type:            "mysql",
			ConnStr:         "root:testpass@tcp(mysql:3306)/scdb?charset=utf8mb4&parseTime=True&loc=Local&interpolateParams=true",
			MaxIdleConns:    10,
			MaxOpenConns:    100,
			ConnMaxIdleTime: time.Minute * 2,
			ConnMaxLifetime: time.Minute * 5,
		},
	}
	r.Equal(cfg, expectedCfg)
}
