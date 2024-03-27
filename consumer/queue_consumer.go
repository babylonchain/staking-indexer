package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/queue/client"
	"github.com/babylonchain/staking-indexer/types"
)

type QueueConsumer struct {
	stakingQueue   client.QueueClient
	unbondingQueue client.QueueClient
	withdrawQueue  client.QueueClient
	logger         *zap.Logger
}

func NewQueueConsumer(cfg *config.QueueConfig, logger *zap.Logger) (*QueueConsumer, error) {
	stakingQueue, err := client.NewQueueClient(cfg.Url, cfg.User, cfg.Password, types.ActiveStakingQueueName)
	if err != nil {
		return nil, fmt.Errorf("failed to create staking queue: %w", err)
	}

	unbondingQueue, err := client.NewQueueClient(cfg.Url, cfg.User, cfg.Password, types.UnbondingStakingQueueName)
	if err != nil {
		return nil, fmt.Errorf("failed to create unbonding queue: %w", err)
	}

	withdrawQueue, err := client.NewQueueClient(cfg.Url, cfg.User, cfg.Password, types.WithdrawStakingQueueName)
	if err != nil {
		return nil, fmt.Errorf("failed to create withdraw queue: %w", err)
	}

	return &QueueConsumer{
		stakingQueue:   stakingQueue,
		unbondingQueue: unbondingQueue,
		withdrawQueue:  withdrawQueue,
		logger:         logger.With(zap.String("module", "queue consumer")),
	}, nil
}

func (qc *QueueConsumer) Start() error {
	return nil
}

func (qc *QueueConsumer) PushStakingEvent(ev *types.ActiveStakingEvent) error {
	jsonBytes, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	messageBody := string(jsonBytes)

	qc.logger.Info("pushing staking event", zap.String("tx_hash", ev.StakingTxHex))
	err = qc.stakingQueue.SendMessage(context.TODO(), messageBody)
	if err != nil {
		return fmt.Errorf("failed to push staking event: %w", err)
	}
	qc.logger.Info("successfully pushed staking event", zap.String("tx_hash", ev.StakingTxHex))

	return nil
}

func (qc *QueueConsumer) PushUnbondingEvent(ev *types.UnbondingStakingEvent) error {
	jsonBytes, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	messageBody := string(jsonBytes)

	qc.logger.Info("pushing unbonding event", zap.String("tx_hash", ev.StakingTxHashHex))
	err = qc.unbondingQueue.SendMessage(context.TODO(), messageBody)
	if err != nil {
		return fmt.Errorf("failed to push unbonding event: %w", err)
	}
	qc.logger.Info("successfully pushed unbonding event", zap.String("tx_hash", ev.StakingTxHashHex))

	return nil
}

func (qc *QueueConsumer) PushWithdrawEvent(ev *types.WithdrawStakingEvent) error {
	jsonBytes, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	messageBody := string(jsonBytes)

	qc.logger.Info("pushing withdraw event", zap.String("tx_hash", ev.StakingTxHashHex))
	err = qc.withdrawQueue.SendMessage(context.TODO(), messageBody)
	if err != nil {
		return fmt.Errorf("failed to push withdraw event: %w", err)
	}
	qc.logger.Info("successfully pushed withdraw event", zap.String("tx_hash", ev.StakingTxHashHex))

	return nil
}

func (qc *QueueConsumer) Stop() error {
	qc.stakingQueue.Stop()
	qc.unbondingQueue.Stop()
	qc.withdrawQueue.Stop()

	return nil
}
