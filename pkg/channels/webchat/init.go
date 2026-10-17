package webchat

import (
	"github.com/kiry163/mole/pkg/bus"
	"github.com/kiry163/mole/pkg/channels"
	"github.com/kiry163/mole/pkg/config"
)

func init() {
	channels.RegisterFactory(
		config.ChannelWebChat,
		func(channelName, channelType string, cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
			bc := cfg.Channels[channelName]
			decoded, err := bc.GetDecoded()
			if err != nil {
				return nil, err
			}
			c, ok := decoded.(*config.WebChatSettings)
			if !ok {
				return nil, channels.ErrSendFailed
			}
			ch, err := NewWebChatChannel(bc, c, b)
			if err != nil {
				return nil, err
			}
			if channelName != config.ChannelWebChat {
				ch.SetName(channelName)
			}
			return ch, nil
		},
	)
}
