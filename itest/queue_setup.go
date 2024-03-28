package e2etest

import (
	"fmt"
	"testing"

	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/babylonchain/staking-indexer/config"
	"github.com/babylonchain/staking-indexer/consumer"
)

func setupTestQueueConsumer(t *testing.T, cfg *config.QueueConfig) (*consumer.QueueConsumer, error) {
	amqpURI := fmt.Sprintf("amqp://%s:%s@%s", cfg.User, cfg.Password, cfg.Url)
	conn, err := amqp091.Dial(amqpURI)
	require.NoError(t, err)
	defer conn.Close()

	// Start the actual queue processing in our codebase
	queues, err := consumer.NewQueueConsumer(cfg, zap.NewNop())
	require.NoError(t, err)

	return queues, nil
}
