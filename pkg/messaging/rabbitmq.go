// pkg/messaging/rabbitmq.go
package messaging

import (
	"context"
	"encoding/json"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQClient struct {
	conn *amqp.Connection
	ch *amqp.Channel
	uri string
	connRetry chan struct{}
	closed bool
}

func NewRabbitMQClient(uri string) (*RabbitMQClient, error) {
	client := &RabbitMQClient{
		uri: uri,
		connRetry: make(chan struct{}, 1),
		closed: false,
	}

	if err := client.connect(); err != nil {
		return nil, err
	}

	go client.reconnectMonitor()

	return client, nil
}

func (c *RabbitMQClient) connect() error {
	// init connection
	conn, err := amqp.Dial(c.uri)

	if err != nil {
        return err
    }

	ch, err := conn.Channel()
    if err != nil {
        conn.Close()
        return err
    }

	//store connection to client
	c.conn = conn
	c.ch = ch

	//connection monitoring, waiting for connection to close
	go func() {
		<-c.conn.NotifyClose(make(chan *amqp.Error))
		//reconnect if not intentionally closed.
		if !c.closed {
			c.connRetry <- struct{}{}
		}
	}()

	return nil
}

// In case of lost connections - attempts to reconnect to RabbitMQ
// waits for signal on connRetry channel (will signal whenever connections drop)
// TODO: implement max-reconnect values (at ~3-5?)
func (c *RabbitMQClient) reconnectMonitor() {
	for {
		select {
		case <-c.connRetry:
			// don't reconnect client-intentional closings
			if c.closed {
				return
			}

			log.Println("RabbitMQ connection lost. Attempting to reconnect...")

			for {
				if err := c.connect(); err != nil {
					log.Printf("Failed to reconnect to RabbitMQ: %v. Retrying in 5 seconds...", err)
					time.Sleep(5* time.Second)
					continue
				}
				log.Println("Succesfully reconnected to RabbitMQ")
				break
			}
		}
	}
}

// create exchanges/queues/bindings
// mostly topical exchanges as we are looking to send messages to a group of queues, not individual workers.
func (c *RabbitMQClient) SetupInfrastructure() error {
	// Declare exchanges - name, type ("topic"), durability, autodelete, internal, no-wait, other args
	exchanges := []struct {
		name string
		kind string
		durable bool
		autoDelete bool
	}{
		{"biomarker.file.events", "topic", true, false},
		{"biomarker.analysis.events", "topic", true, false},
		{"biomarker.result.events", "topic", true, false},
	}

	for _, e := range exchanges {
		if err := c.ch.ExchangeDeclare(
			e.name,
			e.kind,
			e.durable,
			e.autoDelete,
			false,
			false,
			nil,
		); err != nil {
			return err
		}
	}
	// Declare Queues - name, durability, delete when unused, exclusive, no-wait, Other args
	queues := []struct {
		name string
		durable bool
		autoDelete bool
	}{
		{"file.detected", true, false},
		{"analysis.requested", true, false},
		{"analysis.completed", true, false},
	}

	for _, q := range queues {
		if _, err := c.ch.QueueDeclare(
			q.name,
			q.durable,
			q.autoDelete,
			false,
			false,
			nil,
		); err != nil {
			return err
		}
	}
	// Bind queues to exchanges using routing keys - which queue connects to which exchange (using what pattern), no wait, extraArgs
	bindings := []struct {
		queue string
		exchange string
		routingKey string
	}{
		{"file.detected", "biomarker.file.events", "file.detected.*"},
		{"analysis.requested", "biomarker.analysis.events", "analysis.requested.*"},
		{"analysis.completed", "biomarker.result.events", "analysis.completed.*"},
	}

	for _, b := range bindings {
		if err := c.ch.QueueBind(
			b.queue,
			b.routingKey,
			b.exchange,
			false,
			nil,
		); err != nil {
			return err
		}
	}

	return nil
}

// publish events to an exchange
func (c *RabbitMQClient) PublishEvent(ctx context.Context, exchange, routingKey string, event interface{}) error {
	// convert event to JSON
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	//publishing
	// exchange name, routing key, mandatory, immediate, Publishing Notes
	return c.ch.PublishWithContext(ctx,
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			DeliveryMode: amqp.Presistent,
			Body: body,
			Timestamp: time.Now(),
		},
	)
}

// subscribes to messages from a queue
func (c *RabbitMQClient) Subscribe(queue string, handler func([]byte) error) error {
	// start consuming from specified queue
	// queue name, consumer tag, auto-acknowledge, exclusive, no-local, no-wait, extraArgs
	msgs, err := c.ch.Consume(
		queue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}
	//spin up goroutine to process method (non-blocking)
	go func() {
		for msg := range msgs {
			err := handler(msg.Body)
			// if an error occurs, reject the message and requeue it
			if err != nil {
				log.Printf("Error handling message: %v", err)
				// reject multiple? , requeue?
				msg.Nack(false, true)
			} else {
				msg.Ack(false)
			}
		}
	}()

	return nil
}

func (c *RabbitMQClient) Close() error {
    c.closed = true
    
    if c.ch != nil {
        c.ch.Close()
    }
    
    if c.conn != nil {
        return c.conn.Close()
    }
    
    return nil
}