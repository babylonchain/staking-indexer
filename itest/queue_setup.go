package e2etest

import (
	"fmt"
	"testing"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/consumer"
	queueclient "github.com/babylonchain/staking-indexer/queue/client"
)

func setupTestQueueConsumer(t *testing.T, cfg *config.QueueConfig) (*consumer.QueueConsumer, error) {
	amqpURI := fmt.Sprintf("amqp://%s:%s@%s", cfg.User, cfg.Password, cfg.Url)
	conn, err := amqp091.Dial(amqpURI)
	require.NoError(t, err)
	defer conn.Close()
	purgeQueues(conn, []string{
		queueclient.ActiveStakingQueueName,
		queueclient.UnbondingStakingQueueName,
		queueclient.WithdrawStakingQueueName,
	})

	// Start the actual queue processing in our codebase
	queues, err := consumer.NewQueueConsumer(cfg, zap.NewNop())
	require.NoError(t, err)

	return queues, nil
}

// purgeQueues purges all messages from the given list of queues.
func purgeQueues(conn *amqp091.Connection, queues []string) error {
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open a channel in test: %w", err)
	}
	defer ch.Close()

	for _, queue := range queues {
		_, err := ch.QueuePurge(queue, false)
		if err != nil {
			return fmt.Errorf("failed to purge queue in test %s: %w", queue, err)
		}
	}

	return nil
}
