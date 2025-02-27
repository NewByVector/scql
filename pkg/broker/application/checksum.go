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

package application

import (
	"fmt"
	"sync"

	"golang.org/x/exp/slices"

	"github.com/sirupsen/logrus"

	pb "github.com/secretflow/scql/pkg/proto-gen/scql"
)

type Checksum struct {
	TableSchema []byte
	CCL         []byte
}

func NewChecksumFromProto(checksumProto *pb.Checksum) Checksum {
	return Checksum{TableSchema: checksumProto.TableSchema, CCL: checksumProto.Ccl}
}

func (c *Checksum) CompareWith(checksum Checksum) pb.ChecksumCompareResult {
	if !slices.Equal(c.CCL, checksum.CCL) && !slices.Equal(c.TableSchema, checksum.TableSchema) {
		return pb.ChecksumCompareResult_TABLE_CCL_NOT_EQUAL
	}
	if !slices.Equal(c.CCL, checksum.CCL) {
		return pb.ChecksumCompareResult_CCL_NOT_EQUAL
	}
	if !slices.Equal(c.TableSchema, checksum.TableSchema) {
		return pb.ChecksumCompareResult_TABLE_SCHEMA_NOT_EQUAL
	}
	return pb.ChecksumCompareResult_EQUAL
}

type ChecksumStorage struct {
	// key -> party code; value -> Checksum
	// localChecksums keeps checksum calculated by local storage
	// remoteChecksums keeps checksums from their owner party(not include self code)
	localChecksums  sync.Map
	remoteChecksums sync.Map
}

func (s *ChecksumStorage) GetLocal(partyCode string) (Checksum, error) {
	value, ok := s.localChecksums.Load(partyCode)
	if !ok {
		return Checksum{}, fmt.Errorf("failed to find local checksum for party %s", partyCode)
	}
	checksum, ok := value.(Checksum)
	if !ok {
		return Checksum{}, fmt.Errorf("failed to parse local checksum from %+v", value)
	}
	return checksum, nil
}

func (s *ChecksumStorage) GetRemote(partyCode string) (Checksum, error) {
	value, ok := s.remoteChecksums.Load(partyCode)
	if !ok {
		return Checksum{}, fmt.Errorf("failed to find remote checksum for party %s", partyCode)
	}
	checksum, ok := value.(Checksum)
	if !ok {
		return Checksum{}, fmt.Errorf("failed to parse remote checksum from %+v", value)
	}
	return checksum, nil
}

func (s *ChecksumStorage) SaveLocal(partyCode string, sum Checksum) error {
	if len(sum.TableSchema) == 0 {
		return fmt.Errorf("table schema in checksum is empty")
	}
	if sum.CCL == nil || len(sum.CCL) == 0 {
		return fmt.Errorf("ccl in checksum is empty")
	}
	s.localChecksums.Store(partyCode, sum)
	return nil
}

func (s *ChecksumStorage) SaveRemote(partyCode string, pbChecksum *pb.Checksum) error {
	if pbChecksum == nil {
		return fmt.Errorf("checksum in pb is nil")
	}
	sum := NewChecksumFromProto(pbChecksum)
	if len(sum.TableSchema) == 0 {
		return fmt.Errorf("table schema in checksum is empty")
	}
	if sum.CCL == nil || len(sum.CCL) == 0 {
		return fmt.Errorf("ccl in checksum is empty")
	}
	s.remoteChecksums.Store(partyCode, sum)
	return nil
}

func (s *ChecksumStorage) CompareChecksumFor(partyCode string) (pb.ChecksumCompareResult, error) {
	localChecksum, err := s.GetLocal(partyCode)
	if err != nil {
		return 0, err
	}
	remoteChecksum, err := s.GetRemote(partyCode)
	if err != nil {
		return 0, err
	}
	reqChecksumCompareRes := localChecksum.CompareWith(remoteChecksum)
	if reqChecksumCompareRes != pb.ChecksumCompareResult_EQUAL {
		logrus.Infof("compare result of checksum is %d, self checksum: %+v, remote checksum: %+v", reqChecksumCompareRes, localChecksum, remoteChecksum)
	}
	return reqChecksumCompareRes, nil
}
