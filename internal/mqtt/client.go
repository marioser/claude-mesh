// Package mqtt provides a thin, mockable wrapper over paho.mqtt.golang.
// The Client interface is the only surface exposed to the rest of the codebase;
// the concrete pahoClient struct is package-private.
package mqtt

import (
	"context"
	"fmt"
	"sync"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
)

// MessageHandler is the callback invoked when a subscribed message arrives.
type MessageHandler func(topic string, payload []byte)

// Client is the mockable interface for MQTT operations.
// All bridge and publisher code must depend on this interface, not on paho directly.
type Client interface {
	Connect(ctx context.Context) error
	Publish(ctx context.Context, topic string, qos byte, retain bool, payload []byte) error
	Subscribe(ctx context.Context, topic string, qos byte, handler MessageHandler) error
	Disconnect(timeoutMs uint)
}

// OnConnectFunc is called by paho every time the client successfully connects or
// reconnects. The implementation MUST NOT block — spawn a goroutine if needed.
type OnConnectFunc func()

// pahoClient wraps a paho.Client with a mutex around Publish to prevent
// concurrent write races on the underlying TCP connection.
// Pattern mirrors kpiServiceGo (proven in production).
type pahoClient struct {
	mu   sync.Mutex
	paho pahomqtt.Client
}

// NewPahoClient creates a connected-ready paho Client.
// Call Connect before using Publish or Subscribe.
// If onConnect is non-nil it is wired as paho's OnConnectHandler so the caller
// can re-subscribe on every automatic reconnect. The callback runs on paho's
// internal network goroutine — it MUST NOT block.
func NewPahoClient(broker string, clientID string, username, password string, onConnect OnConnectFunc) Client {
	opts := pahomqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID(clientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5e9). // 5s
		SetCleanSession(false)

	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}

	if onConnect != nil {
		opts.SetOnConnectHandler(func(_ pahomqtt.Client) {
			onConnect()
		})
	}

	return &pahoClient{paho: pahomqtt.NewClient(opts)}
}

func (c *pahoClient) Connect(_ context.Context) error {
	token := c.paho.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt connect: %w", err)
	}
	return nil
}

// Publish serializes payload and publishes at the given QoS.
// The mutex prevents concurrent paho writes, matching the kpiServiceGo pattern.
func (c *pahoClient) Publish(_ context.Context, topic string, qos byte, retain bool, payload []byte) error {
	c.mu.Lock()
	token := c.paho.Publish(topic, qos, retain, payload)
	c.mu.Unlock()

	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt publish %s: %w", topic, err)
	}
	return nil
}

func (c *pahoClient) Subscribe(_ context.Context, topic string, qos byte, handler MessageHandler) error {
	token := c.paho.Subscribe(topic, qos, func(_ pahomqtt.Client, msg pahomqtt.Message) {
		handler(msg.Topic(), msg.Payload())
	})
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt subscribe %s: %w", topic, err)
	}
	return nil
}

func (c *pahoClient) Disconnect(timeoutMs uint) {
	c.paho.Disconnect(timeoutMs)
}
