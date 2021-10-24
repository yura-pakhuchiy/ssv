package exporter

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/bloxapp/eth2-key-manager/core"
	"github.com/bloxapp/ssv/beacon"
	"github.com/bloxapp/ssv/eth1"
	"github.com/bloxapp/ssv/exporter/api"
	"github.com/bloxapp/ssv/exporter/ibft"
	"github.com/bloxapp/ssv/exporter/storage"
	"github.com/bloxapp/ssv/ibft/proto"
	"github.com/bloxapp/ssv/monitoring/metrics"
	"github.com/bloxapp/ssv/network"
	"github.com/bloxapp/ssv/storage/basedb"
	"github.com/bloxapp/ssv/storage/collections"
	"github.com/bloxapp/ssv/utils/tasks"
	"github.com/bloxapp/ssv/validator"
	validatorstorage "github.com/bloxapp/ssv/validator/storage"
	"github.com/herumi/bls-eth-go-binary/bls"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"sync"
	"time"
)

const (
	mainQueueInterval            = 100 * time.Millisecond
	readerQueuesInterval         = 10 * time.Millisecond
	metaDataReaderQueuesInterval = 5 * time.Second
	metaDataBatchSize            = 25
)

var (
	syncWhitelist []string
)

// Exporter represents the main interface of this package
type Exporter interface {
	Start() error
	StartEth1(syncOffset *eth1.SyncOffset) error
}

// Options contains options to create the node
type Options struct {
	Ctx context.Context

	Logger     *zap.Logger
	ETHNetwork *core.Network

	Eth1Client eth1.Client
	Beacon     beacon.Beacon

	Network network.Network

	DB basedb.IDb

	WS                              api.WebSocketServer
	WsAPIPort                       int
	IbftSyncEnabled                 bool
	CleanRegistryData               bool
	ValidatorMetaDataUpdateInterval time.Duration
}

// exporter is the internal implementation of Exporter interface
type exporter struct {
	ctx              context.Context
	storage          storage.Storage
	validatorStorage validatorstorage.ICollection
	ibftStorage      collections.Iibft
	logger           *zap.Logger
	network          network.Network
	eth1Client       eth1.Client
	beacon           beacon.Beacon

	ws           api.WebSocketServer
	commitReader ibft.Reader

	readersMut     sync.RWMutex
	decidedReaders map[string]ibft.Reader
	netReaders     map[string]ibft.Reader

	wsAPIPort                       int
	ibftSyncEnabled                 bool
	validatorMetaDataUpdateInterval time.Duration

	mainQueue            tasks.Queue
	decidedReadersQueue  tasks.Queue
	networkReadersQueue  tasks.Queue
	metaDataReadersQueue tasks.Queue
}

// New creates a new Exporter instance
func New(opts Options) Exporter {
	ibftStorage := collections.NewIbft(opts.DB, opts.Logger, "attestation")
	validatorStorage := validatorstorage.NewCollection(
		validatorstorage.CollectionOptions{
			DB:     opts.DB,
			Logger: opts.Logger,
		},
	)
	e := exporter{
		ctx:                  opts.Ctx,
		storage:              storage.NewExporterStorage(opts.DB, opts.Logger),
		ibftStorage:          &ibftStorage,
		validatorStorage:     validatorStorage,
		logger:               opts.Logger.With(zap.String("component", "exporter/node")),
		network:              opts.Network,
		eth1Client:           opts.Eth1Client,
		beacon:               opts.Beacon,
		mainQueue:            tasks.NewExecutionQueue(mainQueueInterval),
		decidedReadersQueue:  tasks.NewExecutionQueue(readerQueuesInterval),
		networkReadersQueue:  tasks.NewExecutionQueue(readerQueuesInterval),
		metaDataReadersQueue: tasks.NewExecutionQueue(metaDataReaderQueuesInterval),
		ws:                   opts.WS,
		readersMut:           sync.RWMutex{},
		decidedReaders:       map[string]ibft.Reader{},
		netReaders:           map[string]ibft.Reader{},
		commitReader: ibft.NewCommitReader(ibft.CommitReaderOptions{
			Logger:           opts.Logger,
			Network:          opts.Network,
			ValidatorStorage: validatorStorage,
			IbftStorage:      &ibftStorage,
			Out:              opts.WS.OutboundSubject(),
		}),
		wsAPIPort:                       opts.WsAPIPort,
		ibftSyncEnabled:                 opts.IbftSyncEnabled,
		validatorMetaDataUpdateInterval: opts.ValidatorMetaDataUpdateInterval,
	}

	if err := e.init(opts); err != nil {
		e.logger.Panic("failed to init", zap.Error(err))
	}

	return &e
}

func (exp *exporter) init(opts Options) error {
	if opts.CleanRegistryData {
		if err := exp.validatorStorage.CleanAllShares(); err != nil {
			return errors.Wrap(err, "could not clean existing shares")
		}
		if err := exp.storage.Clean(); err != nil {
			return errors.Wrap(err, "could not clean existing data")
		}
		exp.logger.Debug("manage to cleanup registry data")
	}
	return nil
}

// Start starts the Controller dispatcher for syncing data nd listen to messages
func (exp *exporter) Start() error {
	exp.logger.Info("starting node")

	go exp.metaDataReadersQueue.Start()
	if err := exp.warmupValidatorsMetaData(); err != nil {
		exp.logger.Error("failed to warmup validators metadata", zap.Error(err))
	}
	go exp.continuouslyUpdateValidatorMetaData()

	go exp.mainQueue.Start()
	go exp.decidedReadersQueue.Start()
	go exp.networkReadersQueue.Start()

	if exp.ws == nil {
		return nil
	}

	exp.ws.UseQueryHandler(exp.handleQueryRequests)

	go exp.triggerAllValidators()

	go func() {
		if err := exp.commitReader.Start(); err != nil {
			exp.logger.Error("could not start commit reader", zap.Error(err))
		}
	}()

	go exp.startMainTopic()

	go exp.reportOperators()

	return exp.ws.Start(fmt.Sprintf(":%d", exp.wsAPIPort))
}

// HealthCheck returns a list of issues regards the state of the exporter node
func (exp *exporter) HealthCheck() []string {
	return metrics.ProcessAgents(exp.healthAgents())
}

func (exp *exporter) healthAgents() []metrics.HealthCheckAgent {
	var agents []metrics.HealthCheckAgent
	if agent, ok := exp.eth1Client.(metrics.HealthCheckAgent); ok {
		agents = append(agents, agent)
	}
	if agent, ok := exp.beacon.(metrics.HealthCheckAgent); ok {
		agents = append(agents, agent)
	}
	return agents
}

// startMainTopic starts to listen to main topic
func (exp *exporter) startMainTopic() {
	if err := tasks.Retry(exp.network.SubscribeToMainTopic, 3); err != nil {
		exp.logger.Error("failed to subscribe to main topic", zap.Error(err))
	}
}

// handleQueryRequests waits for incoming messages and
func (exp *exporter) handleQueryRequests(nm *api.NetworkMessage) {
	if nm.Err != nil {
		nm.Msg = api.Message{Type: api.TypeError, Data: []string{"could not parse network message"}}
	}
	exp.logger.Debug("got incoming export request",
		zap.String("type", string(nm.Msg.Type)))
	switch nm.Msg.Type {
	case api.TypeOperator:
		handleOperatorsQuery(exp.logger, exp.storage, nm)
	case api.TypeValidator:
		handleValidatorsQuery(exp.logger, exp.storage, nm)
	case api.TypeDecided:
		handleDecidedQuery(exp.logger, exp.storage, exp.ibftStorage, nm)
	case api.TypeError:
		handleErrorQuery(exp.logger, nm)
	default:
		handleUnknownQuery(exp.logger, nm)
	}
}

// StartEth1 starts the eth1 events sync and streaming
func (exp *exporter) StartEth1(syncOffset *eth1.SyncOffset) error {
	exp.logger.Info("starting node -> eth1")

	// sync events
	syncErr := eth1.SyncEth1Events(exp.logger, exp.eth1Client, exp.storage, syncOffset, exp.handleEth1Event)
	if syncErr != nil {
		return errors.Wrap(syncErr, "failed to sync eth1 contract events")
	}
	exp.logger.Info("managed to sync contract events")

	// register for contract events that will arrive from eth1Client
	eth1EventChan, err := exp.eth1Client.EventsSubject().Register("Eth1ExporterObserver")
	if err != nil {
		return errors.Wrap(err, "could not register for eth1 events subject")
	}
	errCn := exp.listenToEth1Events(eth1EventChan)
	go func() {
		// log errors while processing events
		for err := range errCn {
			exp.logger.Warn("could not handle eth1 event", zap.Error(err))
		}
	}()
	// start events stream
	err = exp.eth1Client.Start()
	if err != nil {
		return errors.Wrap(err, "could not start eth1 client")
	}
	return nil
}

func (exp *exporter) triggerAllValidators() {
	shares, err := exp.validatorStorage.GetAllValidatorsShare()
	if err != nil {
		exp.logger.Error("could not get validators shares", zap.Error(err))
		return
	}
	exp.logger.Debug("triggering validators", zap.Int("count", len(shares)))
	for _, share := range shares {
		if err = exp.triggerValidator(share.PublicKey); err != nil {
			exp.logger.Error("failed to trigger ibft sync", zap.Error(err),
				zap.String("pubKey", share.PublicKey.SerializeToHexStr()))
		}
	}
}

func (exp *exporter) shouldProcessValidator(pubkey string) bool {
	for _, pk := range syncWhitelist {
		if pubkey == pk {
			return true
		}
	}
	return exp.ibftSyncEnabled
}

func (exp *exporter) triggerValidator(validatorPubKey *bls.PublicKey) error {
	if validatorPubKey == nil {
		return errors.New("empty validator pubkey")
	}
	pubkey := validatorPubKey.SerializeToHexStr()
	if !exp.shouldProcessValidator(pubkey) {
		return nil
	}
	validatorShare, found, err := exp.validatorStorage.GetValidatorShare(validatorPubKey.Serialize())
	if !found {
		return errors.New("could not find validator share")
	}
	if err != nil {
		return errors.Wrap(err, "could not get validator share")
	}
	exp.logger.Debug("validator was triggered", zap.String("pubKey", pubkey))

	exp.mainQueue.QueueDistinct(func() error {
		return exp.setup(validatorShare)
	}, fmt.Sprintf("ibft:setup/%s", pubkey))

	return nil
}

func (exp *exporter) setup(validatorShare *validatorstorage.Share) error {
	pubKey := validatorShare.PublicKey.SerializeToHexStr()
	logger := exp.logger.With(zap.String("pubKey", pubKey))
	validator.ReportValidatorStatus(pubKey, validatorShare.Metadata, exp.logger)
	decidedReader := exp.createDecidedReader(validatorShare)

	// start network reader
	networkReader := exp.createNetworkReader(validatorShare.PublicKey)
	exp.networkReadersQueue.QueueDistinct(networkReader.Start, pubKey)

	// sync decided
	if err := tasks.Retry(func() error {
		if err := decidedReader.Sync(); err != nil {
			logger.Error("could not sync validator", zap.Error(err))
			return err
		}
		return nil
	}, 3); err != nil {
		logger.Error("could not setup validator, sync failed", zap.Error(err))
		return err
	}
	logger.Debug("sync is done, starting to read network messages")
	exp.decidedReadersQueue.QueueDistinct(decidedReader.Start, pubKey)
	return nil
}

func (exp *exporter) createDecidedReader(validatorShare *validatorstorage.Share) ibft.SyncRead {
	exp.readersMut.Lock()
	defer exp.readersMut.Unlock()

	pk := validatorShare.PublicKey.SerializeToHexStr()
	if _, ok := exp.decidedReaders[pk]; !ok {
		exp.decidedReaders[pk] = ibft.NewDecidedReader(ibft.DecidedReaderOptions{
			Logger:         exp.logger,
			Storage:        exp.ibftStorage,
			Network:        exp.network,
			Config:         proto.DefaultConsensusParams(),
			ValidatorShare: validatorShare,
			Out:            exp.ws.OutboundSubject(),
		})
	}

	return exp.decidedReaders[pk].(ibft.SyncRead)
}

func (exp *exporter) getDecidedReader(pk string) ibft.Reader {
	exp.readersMut.RLock()
	defer exp.readersMut.RUnlock()

	return exp.decidedReaders[pk]
}

func (exp *exporter) createNetworkReader(validatorPubKey *bls.PublicKey) ibft.Reader {
	exp.readersMut.Lock()
	defer exp.readersMut.Unlock()

	pk := validatorPubKey.SerializeToHexStr()
	if _, ok := exp.netReaders[pk]; !ok {
		exp.netReaders[pk] = ibft.NewNetworkReader(ibft.IncomingMsgsReaderOptions{
			Logger:  exp.logger,
			Network: exp.network,
			Config:  proto.DefaultConsensusParams(),
			PK:      validatorPubKey,
		})
	}

	return exp.netReaders[pk]
}

func (exp *exporter) reportOperators() {
	// TODO: change api maybe, limited to 1000 operators
	operators, err := exp.storage.ListOperators(0, 1000)
	if err != nil {
		exp.logger.Error("could not get operators", zap.Error(err))
	}
	exp.logger.Debug("reporting operators", zap.Int("count", len(operators)))
	for _, op := range operators {
		pkHash := fmt.Sprintf("%x", sha256.Sum256([]byte(op.PublicKey)))
		metricOperatorIndex.WithLabelValues(pkHash, op.Name).Set(float64(op.Index))
		exp.logger.Debug("report operator", zap.String("pkHash", pkHash),
			zap.String("name", op.Name), zap.Int64("index", op.Index))
	}
}
