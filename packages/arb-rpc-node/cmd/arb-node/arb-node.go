/*
 * Copyright 2020-2021, Offchain Labs, Inc.
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

package main

import (
	"context"
	"fmt"
	golog "log"
	"math/big"
	"net/http"
	_ "net/http/pprof"
	"strings"
	"time"

	gosundheit "github.com/AppsFlyer/go-sundheit"
	"github.com/AppsFlyer/go-sundheit/checks"
	healthhttp "github.com/AppsFlyer/go-sundheit/http"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/pkg/errors"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"

	"github.com/offchainlabs/arbitrum/packages/arb-node-core/arbmetrics"
	"github.com/offchainlabs/arbitrum/packages/arb-node-core/cmdhelp"
	"github.com/offchainlabs/arbitrum/packages/arb-node-core/ethbridge"
	"github.com/offchainlabs/arbitrum/packages/arb-node-core/monitor"
	"github.com/offchainlabs/arbitrum/packages/arb-rpc-node/aggregator"
	"github.com/offchainlabs/arbitrum/packages/arb-rpc-node/batcher"
	"github.com/offchainlabs/arbitrum/packages/arb-rpc-node/rpc"
	"github.com/offchainlabs/arbitrum/packages/arb-rpc-node/txdb"
	"github.com/offchainlabs/arbitrum/packages/arb-rpc-node/web3"
	"github.com/offchainlabs/arbitrum/packages/arb-util/broadcastclient"
	"github.com/offchainlabs/arbitrum/packages/arb-util/broadcaster"
	"github.com/offchainlabs/arbitrum/packages/arb-util/common"
	"github.com/offchainlabs/arbitrum/packages/arb-util/configuration"
	"github.com/offchainlabs/arbitrum/packages/arb-util/core"
	"github.com/offchainlabs/arbitrum/packages/arb-util/ethutils"
	"github.com/offchainlabs/arbitrum/packages/arb-util/healthcheck"
)

var logger zerolog.Logger

var pprofMux *http.ServeMux

func init() {
	pprofMux = http.DefaultServeMux
	http.DefaultServeMux = http.NewServeMux()
}

func main() {
	// Enable line numbers in logging
	golog.SetFlags(golog.LstdFlags | golog.Lshortfile)

	// Print stack trace when `.Error().Stack().Err(err).` is added to zerolog call
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Print line number that log was created on
	logger = log.With().Caller().Stack().Str("component", "arb-node").Logger()

	if err := startup(); err != nil {
		logger.Error().Err(err).Msg("Error running node")
	}
}

func printSampleUsage() {
	fmt.Printf("\n")
	fmt.Printf("Sample usage:                  arb-node --conf=<filename> \n")
	fmt.Printf("          or:  forwarder node: arb-node --l1.url=<L1 RPC> [optional arguments]\n\n")
	fmt.Printf("          or: aggregator node: arb-node --l1.url=<L1 RPC> --node.type=aggregator [optional arguments] %s\n", cmdhelp.WalletArgsString)
	fmt.Printf("          or:       sequencer: arb-node --l1.url=<L1 RPC> --node.type=sequencer [optional arguments] %s\n", cmdhelp.WalletArgsString)
}

type NodeMetrics struct {
	OpenedDB           metrics.Gauge
	LoadedDB           metrics.Gauge
	StartedInboxReader metrics.Gauge
	StartedBatcher     metrics.Gauge
	StartedRPC         metrics.Gauge
}

func NewNodeMetrics() *NodeMetrics {
	return &NodeMetrics{
		OpenedDB:           metrics.NewGauge(),
		LoadedDB:           metrics.NewGauge(),
		StartedInboxReader: metrics.NewGauge(),
		StartedBatcher:     metrics.NewGauge(),
		StartedRPC:         metrics.NewGauge(),
	}
}

func (m *NodeMetrics) Register(r metrics.Registry) error {
	if err := r.Register("opened_db", m.OpenedDB); err != nil {
		return err
	}
	if err := r.Register("loaded_db", m.LoadedDB); err != nil {
		return err
	}
	return nil
}

type NodeInitializationHealth struct {
	metrics *NodeMetrics
}

func (c NodeInitializationHealth) Execute(context.Context) (interface{}, error) {
	if c.metrics.OpenedDB.Value() != 1 {
		return nil, errors.New("db not opened")
	}
	if c.metrics.LoadedDB.Value() != 1 {
		return nil, errors.New("db not finished loading")
	}
	if c.metrics.StartedInboxReader.Value() != 1 {
		return nil, errors.New("inbox reader not started")
	}
	if c.metrics.StartedBatcher.Value() != 1 {
		return nil, errors.New("batcher not started")
	}
	if c.metrics.StartedRPC.Value() != 1 {
		return nil, errors.New("rpc not started")
	}
	return nil, nil
}

func (c NodeInitializationHealth) Name() string {
	return "node-initialization"
}

type HealthConfiguration struct {
	MaxInboxSyncDiff         int64
	MaxMessagesSyncDiff      int64
	MaxLogsProcessedSyncDiff int64
	MaxL1BlockDiff           int64
	MaxL2BlockDiff           int64
}

func startup() error {
	ctx, cancelFunc, cancelChan := cmdhelp.CreateLaunchContext()
	defer cancelFunc()

	healthConfig := HealthConfiguration{}

	config, walletConfig, l1URL, l1ChainId, err := configuration.ParseNode(ctx)
	if err != nil || len(config.Persistent.GlobalConfig) == 0 || len(config.L1.URL) == 0 ||
		len(config.Rollup.Address) == 0 || len(config.BridgeUtilsAddress) == 0 ||
		((config.Node.Type != "sequencer") && len(config.Node.Sequencer.Lockout.Redis) != 0) ||
		((len(config.Node.Sequencer.Lockout.Redis) == 0) != (len(config.Node.Sequencer.Lockout.SelfRPCURL) == 0)) {
		printSampleUsage()
		if err != nil && !strings.Contains(err.Error(), "help requested") {
			fmt.Printf("%s\n", err.Error())
		}

		return nil
	}

	badConfig := false
	if config.BridgeUtilsAddress == "" {
		badConfig = true
		fmt.Println("Missing --bridge-utils-address")
	}
	if config.Persistent.Chain == "" {
		badConfig = true
		fmt.Println("Missing --persistent.chain")
	}
	if config.Rollup.Address == "" {
		badConfig = true
		fmt.Println("Missing --rollup.address")
	}
	if config.Node.ChainID == 0 {
		badConfig = true
		fmt.Println("Missing --rollup.chain-id")
	}
	if config.Rollup.Machine.Filename == "" {
		badConfig = true
		fmt.Println("Missing --rollup.machine.filename")
	}

	var rpcMode web3.RpcMode
	if config.Node.Type == "forwarder" {
		if config.Node.Forwarder.Target == "" {
			badConfig = true
			fmt.Println("Forwarder node needs --node.forwarder.target")
		}

		if config.Node.Forwarder.RpcMode == "full" {
			rpcMode = web3.NormalMode
		} else if config.Node.Forwarder.RpcMode == "non-mutating" {
			rpcMode = web3.NonMutatingMode
		} else if config.Node.Forwarder.RpcMode == "forwarding-only" {
			rpcMode = web3.ForwardingOnlyMode
		} else {
			badConfig = true
			fmt.Printf("Unrecognized RPC mode %s", config.Node.Forwarder.RpcMode)
		}
	} else if config.Node.Type == "aggregator" {
		if config.Node.Aggregator.InboxAddress == "" {
			badConfig = true
			fmt.Println("Aggregator node needs --node.aggregator.inbox-address")
		}
	} else if config.Node.Type == "sequencer" {
		// Sequencer always waits
		config.WaitToCatchUp = true
	} else {
		badConfig = true
		fmt.Printf("Unrecognized node type %s", config.Node.Type)
	}

	if config.WaitToCatchUp && !config.Metrics {
		badConfig = true
		fmt.Println("wait to catch up needs --metrics")
	}

	if badConfig {
		return nil
	}

	defer logger.Log().Msg("Cleanly shutting down node")

	if err := cmdhelp.ParseLogFlags(&config.Log.RPC, &config.Log.Core); err != nil {
		return err
	}

	if config.PProfEnable {
		go func() {
			err := http.ListenAndServe("localhost:8081", pprofMux)
			log.Error().Err(err).Msg("profiling server failed")
		}()
	}

	metricsConfig := arbmetrics.NewMetricsConfig(config.MetricsServer)
	nodeMetrics := NewNodeMetrics()
	if err := metricsConfig.Register(nodeMetrics); err != nil {
		return err
	}

	l2ChainId := new(big.Int).SetUint64(config.Node.ChainID)
	rollupAddress := common.HexToAddress(config.Rollup.Address)
	logger.Info().
		Hex("chainaddress", rollupAddress.Bytes()).
		Hex("chainid", l2ChainId.Bytes()).
		Str("type", config.Node.Type).
		Int64("fromBlock", config.Rollup.FromBlock).
		Msg("Launching arbitrum node")

	healthCheck := healthcheck.New(metrics.NewPrefixedChildRegistry(metricsConfig.Registry, "health/"))
	if err := healthCheck.RegisterCheck(NodeInitializationHealth{metrics: nodeMetrics}); err != nil {
		return err
	}

	syncedCheck := healthcheck.New(metrics.NewPrefixedChildRegistry(metricsConfig.Registry, "sync_health/"))
	if err := syncedCheck.RegisterCheck(&checks.CustomCheck{
		CheckName: "ready",
		CheckFunc: func(ctx context.Context) (details interface{}, err error) {
			if !healthCheck.IsHealthy() {
				return nil, errors.New("core system not ready")
			}
			return nil, nil
		},
	}); err != nil {
		return err
	}

	go func() {
		mux := http.NewServeMux()
		// register a health endpoint

		mux.Handle("/health/ready", healthhttp.HandleHealthJSON(healthCheck))
		mux.Handle("/admin/synced/ready", healthhttp.HandleHealthJSON(syncedCheck))

		// serve HTTP
		if err := http.ListenAndServe(":8080", mux); err != nil {
			log.Error().Err(err).Msg("healthcheck server failed")
		}
	}()

	mon, err := monitor.NewMonitor(config.GetNodeDatabasePath(), &config.Core)
	if err != nil {
		return errors.Wrap(err, "error opening monitor")
	}
	defer mon.Close()

	nodeMetrics.OpenedDB.Update(1)
	if err := mon.StartCore(config.Rollup.Machine.Filename); err != nil {
		return err
	}
	nodeMetrics.LoadedDB.Update(1)

	if err := metricsConfig.Register(mon.Metrics); err != nil {
		return err
	}

	if err := syncedCheck.RegisterCheck(monitor.InboxSynced{
		Metrics: mon.Metrics,
		MaxDiff: healthConfig.MaxInboxSyncDiff,
	}); err != nil {
		return err
	}
	if err := syncedCheck.RegisterCheck(core.MessagesSynced{
		Metrics: mon.Metrics.Core,
		MaxDiff: healthConfig.MaxMessagesSyncDiff,
	}); err != nil {
		return err
	}

	nodeStore := mon.Storage.GetNodeStore()
	db, txDBErrChan, err := txdb.New(ctx, mon.Core, nodeStore, 100*time.Millisecond, &config.Node.Cache)
	if err != nil {
		return errors.Wrap(err, "error opening txdb")
	}
	defer db.Close()

	if err := metricsConfig.RegisterWithPrefix(db.Metrics, "txdb"); err != nil {
		return err
	}

	if err := syncedCheck.RegisterCheck(txdb.Synced{
		CoreMetrics: mon.Metrics.Core,
		Metrics:     db.Metrics,
		MaxDiff:     healthConfig.MaxLogsProcessedSyncDiff,
	}); err != nil {
		return err
	}

	l1Health := ethutils.L1Ready{
		Url:          l1URL,
		MaxBlockDiff: uint64(healthConfig.MaxL1BlockDiff),
	}
	if err := healthCheck.RegisterCheck(l1Health); err != nil {
		return err
	}

	var sequencerFeed chan broadcaster.BroadcastFeedMessage
	if len(config.Feed.Input.URLs) == 0 {
		logger.Warn().Msg("Missing --feed.url so not subscribing to feed")
	} else {
		sequencerFeed = make(chan broadcaster.BroadcastFeedMessage, 1)
		for _, url := range config.Feed.Input.URLs {
			broadcastClient := broadcastclient.NewBroadcastClient(url, nil, config.Feed.Input.Timeout)
			broadcastClient.ConnectInBackground(ctx, sequencerFeed)
		}
	}
	var inboxReader *monitor.InboxReader
	for {
		l1Client, err := ethutils.NewRPCEthClient(l1URL)
		if err != nil {
			return err
		}
		inboxReader, err = mon.StartInboxReader(ctx, l1Client, common.HexToAddress(config.Rollup.Address), config.Rollup.FromBlock, common.HexToAddress(config.BridgeUtilsAddress), sequencerFeed)
		if err == nil {
			break
		}
		logger.Warn().Err(err).
			Str("url", config.L1.URL).
			Str("rollup", config.Rollup.Address).
			Str("bridgeUtils", config.BridgeUtilsAddress).
			Int64("fromBlock", config.Rollup.FromBlock).
			Msg("failed to start inbox reader, waiting and retrying")

		select {
		case <-ctx.Done():
			return errors.New("ctx cancelled StartInboxReader retry loop")
		case <-time.After(5 * time.Second):
		}
	}

	nodeMetrics.StartedInboxReader.Update(1)

	var dataSigner func([]byte) ([]byte, error)
	var batcherMode rpc.BatcherMode
	var batcherHealthcheck gosundheit.Check
	if config.Node.Type == "forwarder" {
		logger.Info().Str("forwardTxURL", config.Node.Forwarder.Target).Msg("Arbitrum node starting in forwarder mode")
		batcherMode = rpc.ForwarderBatcherMode{Config: config.Node.Forwarder}
		batcherHealthcheck = batcher.ForwarderHealth{
			Url:          config.Node.Forwarder.Target,
			MaxBlockDiff: healthConfig.MaxL2BlockDiff,
			TxDBMetrics:  db.Metrics,
		}
	} else {
		var auth *bind.TransactOpts
		auth, dataSigner, err = cmdhelp.GetKeystore(config, walletConfig, l1ChainId, true)
		if err != nil {
			return errors.Wrap(err, "error running GetKeystore")
		}

		logger.Info().Hex("from", auth.From.Bytes()).Msg("Arbitrum node submitting batches")
		l1Client, err := ethutils.NewRPCEthClient(l1URL)
		if err != nil {
			return err
		}
		if err := ethbridge.WaitForBalance(
			ctx,
			l1Client,
			common.Address{},
			common.NewAddressFromEth(auth.From),
		); err != nil {
			return errors.Wrap(err, "error waiting for balance")
		}

		if config.Node.Type == "sequencer" {
			batcherMode = rpc.SequencerBatcherMode{
				Auth:        auth,
				Core:        mon.Core,
				InboxReader: inboxReader,
			}
		} else {
			inboxAddress := common.HexToAddress(config.Node.Aggregator.InboxAddress)
			if config.Node.Aggregator.Stateful {
				batcherMode = rpc.StatefulBatcherMode{Auth: auth, InboxAddress: inboxAddress}
			} else {
				batcherMode = rpc.StatelessBatcherMode{Auth: auth, InboxAddress: inboxAddress}
			}
			batcherHealthcheck = batcher.BatcherHealth{}
		}
	}

	if config.WaitToCatchUp {
		for {
			if syncedCheck.IsHealthy() {
				break
			}
			time.Sleep(time.Second * 5)
		}
	}

	var batch batcher.TransactionBatcher
	errChan := make(chan error, 1)
	for {
		l1Client, err := ethutils.NewRPCEthClient(l1URL)
		if err != nil {
			return err
		}
		batch, err = rpc.SetupBatcher(
			ctx,
			l1Client,
			rollupAddress,
			l2ChainId,
			db,
			time.Duration(config.Node.Aggregator.MaxBatchTime)*time.Second,
			batcherMode,
			dataSigner,
			config,
			walletConfig,
		)
		lockoutConf := config.Node.Sequencer.Lockout
		if err == nil {
			seqBatcher, ok := batch.(*batcher.SequencerBatcher)
			if lockoutConf.Redis != "" {
				// Setup the lockout. This will take care of the initial delayed sequence.
				batch, err = rpc.SetupLockout(ctx, seqBatcher, mon.Core, inboxReader, lockoutConf, errChan)
				batcherHealthcheck = batcher.LockoutSequencerHealth{}
			} else if ok {
				// Ensure we sequence delayed messages before opening the RPC.
				err = seqBatcher.SequenceDelayedMessages(ctx, false)
				batcherHealthcheck = batcher.SequencerHealth{}
			}
		}
		if err == nil {
			go batch.Start(ctx)
			break
		}
		logger.Warn().Err(err).Msg("failed to setup batcher, waiting and retrying")

		select {
		case <-ctx.Done():
			return errors.New("ctx cancelled setup batcher")
		case <-time.After(5 * time.Second):
		}
	}

	nodeMetrics.StartedBatcher.Update(1)

	if err := healthCheck.RegisterCheck(batcherHealthcheck); err != nil {
		return err
	}

	srv := aggregator.NewServer(batch, rollupAddress, l2ChainId, db)
	web3Server, err := web3.GenerateWeb3Server(srv, nil, rpcMode, nil)
	if err != nil {
		return err
	}
	go func() {
		err := rpc.LaunchPublicServer(ctx, web3Server, config.Node.RPC, config.Node.WS)
		if err != nil {
			errChan <- err
		}
	}()
	nodeMetrics.StartedRPC.Update(1)

	select {
	case err := <-txDBErrChan:
		return err
	case err := <-errChan:
		return err
	case <-cancelChan:
		return nil
	}
}
