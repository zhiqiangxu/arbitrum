/*
 * Copyright 2020, Offchain Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package web3

import (
	"fmt"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/offchainlabs/arbitrum/packages/arb-evm/evm"
	"github.com/offchainlabs/arbitrum/packages/arb-rpc-node/batcher"
)

type Arb struct {
	srv     *Server
	counter *prometheus.CounterVec
}

func (a *Arb) GetAggregator() *batcher.AggregatorInfo {
	var ret *ethcommon.Address
	agg := a.srv.srv.Aggregator()
	if agg != nil {
		tmp := agg.ToEthAddress()
		ret = &tmp
	}
	a.counter.WithLabelValues("arb_getAggregator", "true").Inc()
	return &batcher.AggregatorInfo{Address: ret}
}

func (a *Arb) TraceCall(callArgs CallTxArgs, blockNum rpc.BlockNumberOrHash) (string, error) {
	res, debugPrints, err := a.srv.call(callArgs, blockNum)
	if err != nil {
		return "", err
	}
	if res.ResultCode != evm.ReturnCode && res.ResultCode != evm.RevertCode {
		return "", errors.Errorf("call returned error code: %v", res.ResultCode)
	}
	var trace *evm.EVMTrace
	for _, debugPrint := range debugPrints {
		arbosLog, err := evm.NewLogLineFromValue(debugPrint)
		if err != nil {
			return "", err
		}
		fmt.Println("Debug:", arbosLog)
		callTrace, ok := arbosLog.(*evm.EVMTrace)
		if ok {
			trace = callTrace
			break
		}
	}
	if trace == nil {
		return "", errors.New("no call trace produced")
	}
	return trace.String(), nil
}

func (a *Arb) SegmentStartCodeHash(blockNum rpc.BlockNumberOrHash) (hexutil.Bytes, error) {
	snap, err := a.srv.getSnapshotForNumberOrHash(blockNum)
	if err != nil {
		return nil, err
	}
	return snap.SegmentStartCodePointHash().Bytes(), nil
}
