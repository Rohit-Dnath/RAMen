package server

import "github.com/Rohit-Dnath/RAMen/internal/pubsub"

func (c *conn) cmdSubscribe(args []string) error {
	if len(args) < 2 {
		return c.wrongArgs("subscribe")
	}
	for _, channel := range args[1:] {
		c.submu.Lock()
		if _, already := c.subs[channel]; already {
			c.submu.Unlock()
			continue
		}
		sub := c.s.broker.Subscribe(channel)
		c.subs[channel] = sub
		count := len(c.subs)
		c.submu.Unlock()

		// Confirm the subscription: ["subscribe", channel, count].
		if err := c.writeSubReply("subscribe", channel, count); err != nil {
			return err
		}
		go c.forward(sub)
	}
	return c.flush()
}

func (c *conn) cmdUnsubscribe(args []string) error {
	c.submu.Lock()
	var channels []string
	if len(args) < 2 {
		for ch := range c.subs {
			channels = append(channels, ch)
		}
	} else {
		channels = args[1:]
	}
	c.submu.Unlock()

	if len(channels) == 0 {
		return c.writeSubReply("unsubscribe", "", 0)
	}
	for _, channel := range channels {
		c.submu.Lock()
		sub, ok := c.subs[channel]
		if ok {
			c.s.broker.Unsubscribe(sub) // closes sub.C, ending the forward goroutine
			delete(c.subs, channel)
		}
		count := len(c.subs)
		c.submu.Unlock()
		if err := c.writeSubReply("unsubscribe", channel, count); err != nil {
			return err
		}
	}
	return nil
}

func (c *conn) cmdPublish(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("publish")
	}
	n := c.s.broker.Publish(args[1], args[2])
	return c.writeInt(int64(n))
}

// forward pumps messages from a subscription to the client until the
// subscription's channel is closed (on unsubscribe / disconnect).
func (c *conn) forward(sub *pubsub.Subscription) {
	for msg := range sub.C {
		c.wmu.Lock()
		c.w.WriteArrayHeader(3)
		c.w.WriteBulkString("message")
		c.w.WriteBulkString(msg.Channel)
		c.w.WriteBulkString(msg.Payload)
		c.w.Flush()
		c.wmu.Unlock()
	}
}

// writeSubReply writes the 3-element (un)subscribe acknowledgement.
func (c *conn) writeSubReply(kind, channel string, count int) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	c.w.WriteArrayHeader(3)
	c.w.WriteBulkString(kind)
	if channel == "" {
		c.w.WriteNull()
	} else {
		c.w.WriteBulkString(channel)
	}
	return c.w.WriteInteger(int64(count))
}
