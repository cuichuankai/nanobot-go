package bus

import (
	"log"
	"sync"
)

// MessageBus decouples chat channels from the agent core.
type MessageBus struct {
	inbound             chan InboundMessage
	outbound            chan OutboundMessage
	outboundSubscribers map[string][]func(OutboundMessage)
	subscribersMu       sync.RWMutex
	stopChan            chan struct{}
}

// NewMessageBus creates a new MessageBus.
func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:             make(chan InboundMessage, 100),
		outbound:            make(chan OutboundMessage, 100),
		outboundSubscribers: make(map[string][]func(OutboundMessage)),
		stopChan:            make(chan struct{}),
	}
}

// PublishInbound publishes a message from a channel to the agent.
func (b *MessageBus) PublishInbound(msg InboundMessage) {
	b.inbound <- msg
}

// ConsumeInbound returns a channel to consume inbound messages.
func (b *MessageBus) ConsumeInbound() <-chan InboundMessage {
	return b.inbound
}

// PublishOutbound publishes a response from the agent to channels.
func (b *MessageBus) PublishOutbound(msg OutboundMessage) {
	b.outbound <- msg
}

// SubscribeOutbound subscribes to outbound messages for a specific channel.
func (b *MessageBus) SubscribeOutbound(channel string, callback func(OutboundMessage)) {
	b.subscribersMu.Lock()
	defer b.subscribersMu.Unlock()
	b.outboundSubscribers[channel] = append(b.outboundSubscribers[channel], callback)
}

// DispatchOutbound starts dispatching outbound messages to subscribers.
// This should be run in a goroutine.
func (b *MessageBus) DispatchOutbound() {
	for {
		select {
		case msg := <-b.outbound:
			b.subscribersMu.RLock()
			subscribers, ok := b.outboundSubscribers[msg.Channel]
			b.subscribersMu.RUnlock()

			if ok {
				for _, cb := range subscribers {
					go func(callback func(OutboundMessage), message OutboundMessage) {
						defer func() {
							if r := recover(); r != nil {
								log.Printf("Error in outbound subscriber callback: %v", r)
							}
						}()
						callback(message)
					}(cb, msg)
				}
			}
		case <-b.stopChan:
			return
		}
	}
}

// Stop stops the dispatcher loop.
func (b *MessageBus) Stop() {
	close(b.stopChan)
}
