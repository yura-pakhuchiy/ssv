package commons

import (
	"context"
	"github.com/bloxapp/ssv/network"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"time"
)

// WaitMinPeersCtx represents the context needed for WaitForMinPeers
type WaitMinPeersCtx struct {
	Ctx    context.Context
	Logger *zap.Logger
	Net    network.Network
}

// WaitForMinPeers waits until min peers joined the validator's topic
func WaitForMinPeers(ctx WaitMinPeersCtx, validatorPk []byte, min int, start, limit time.Duration, stopAtLimit bool) error {
	interval := start
	for {
		ok, n := haveMinPeers(ctx.Logger, ctx.Net, validatorPk, min)
		if ok {
			ctx.Logger.Info("found enough peers",
				zap.Int("current peer count", n))
			break
		}
		ctx.Logger.Info("waiting for min peers",
			zap.Int("current peer count", n))

		time.Sleep(interval)

		select {
		case <-ctx.Ctx.Done():
			return errors.New("timed out")
		default:
			interval *= 2
			if stopAtLimit && interval == limit {
				return errors.New("could not find peers")
			}
			interval %= limit
			if interval == 0 {
				interval = start
			}
		}
	}
	return nil
}

// haveMinPeers checks that there are at least <count> connected peers
func haveMinPeers(logger *zap.Logger, net network.Network, validatorPk []byte, count int) (bool, int) {
	peers, err := net.AllPeers(validatorPk)
	if err != nil {
		logger.Error("failed fetching peers", zap.Error(err))
		return false, 0
	}
	nPeers := len(peers)
	return nPeers >= count, nPeers
}
