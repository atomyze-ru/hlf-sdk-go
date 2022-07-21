package client

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/hyperledger/fabric/msp"
	"go.uber.org/zap"

	"github.com/atomyze-ru/hlf-sdk-go/api"
	"github.com/atomyze-ru/hlf-sdk-go/api/config"
	"github.com/atomyze-ru/hlf-sdk-go/client/grpc"
	"github.com/atomyze-ru/hlf-sdk-go/crypto"
	"github.com/atomyze-ru/hlf-sdk-go/crypto/ecdsa"
	"github.com/atomyze-ru/hlf-sdk-go/discovery"
)

// implementation of api.Core interface
var _ api.Core = (*core)(nil)

type core struct {
	ctx               context.Context
	logger            *zap.Logger
	config            *config.Config
	identity          msp.SigningIdentity
	peerPool          api.PeerPool
	orderer           api.Orderer
	discoveryProvider api.DiscoveryProvider
	channels          map[string]api.Channel
	channelMx         sync.Mutex
	chaincodeMx       sync.Mutex
	cs                api.CryptoSuite
	fabricV2          bool
}

func New(identity api.Identity, opts ...CoreOpt) (api.Core, error) {
	if identity == nil {
		return nil, errors.New("identity wasn't provided")
	}

	coreImp := &core{
		channels: make(map[string]api.Channel),
	}

	for _, option := range opts {
		if err := option(coreImp); err != nil {
			return nil, fmt.Errorf(`apply option: %w`, err)
		}
	}

	if coreImp.ctx == nil {
		coreImp.ctx = context.Background()
	}

	if coreImp.logger == nil {
		coreImp.logger = DefaultLogger
	}

	var err error
	if coreImp.cs == nil {
		coreImp.cs, err = crypto.GetSuite(ecdsa.DefaultConfig.Type, ecdsa.DefaultConfig.Options)
		if err != nil {
			return nil, fmt.Errorf(`initialize crypto suite: %w`, err)
		}
	}

	coreImp.identity = identity.GetSigningIdentity(coreImp.cs)

	// if peerPool is empty, set it from config
	if coreImp.peerPool == nil {
		coreImp.logger.Info("initializing peer pool")

		if coreImp.config == nil {
			return nil, api.ErrEmptyConfig
		}

		coreImp.peerPool = NewPeerPool(coreImp.ctx, coreImp.logger)
		for _, mspConfig := range coreImp.config.MSP {
			for _, peerConfig := range mspConfig.Endorsers {
				var p api.Peer
				p, err = NewPeer(coreImp.ctx, peerConfig, coreImp.identity, coreImp.logger)
				if err != nil {
					return nil, fmt.Errorf("initialize endorsers for MSP: %s: %w", mspConfig.Name, err)
				}

				if err = coreImp.peerPool.Add(mspConfig.Name, p, api.StrategyGRPC(api.DefaultDuration)); err != nil {
					return nil, fmt.Errorf(`add peer to pool: %w`, err)
				}
			}
		}
	}

	if coreImp.discoveryProvider == nil && coreImp.config != nil {
		mapper := discovery.NewEndpointsMapper(coreImp.config.EndpointsMap)

		switch coreImp.config.Discovery.Type {
		case string(discovery.LocalConfigServiceDiscoveryType):
			coreImp.logger.Info("local discovery provider", zap.Reflect(`options`, coreImp.config.Discovery.Options))

			coreImp.discoveryProvider, err = discovery.NewLocalConfigProvider(coreImp.config.Discovery.Options, mapper)
			if err != nil {
				return nil, fmt.Errorf(`initialize discovery provider: %w`, err)
			}

		case string(discovery.GossipServiceDiscoveryType):
			if coreImp.config.Discovery.Connection == nil {
				return nil, fmt.Errorf(`discovery connection config wasn't provided. configure 'discovery.connection': %w`, err)
			}

			coreImp.logger.Info("gossip discovery provider", zap.Reflect(`connection`, coreImp.config.Discovery.Connection))

			identitySigner := func(msg []byte) ([]byte, error) {
				return coreImp.CurrentIdentity().Sign(msg)
			}

			clientIdentity, err := coreImp.CurrentIdentity().Serialize()
			if err != nil {
				return nil, fmt.Errorf(`serialize current identity: %w`, err)
			}

			// add tls settings from mapper if they were provided
			conn := mapper.MapConnection(coreImp.config.Discovery.Connection.Host)
			coreImp.config.Discovery.Connection.Tls = conn.TlsConfig
			coreImp.config.Discovery.Connection.Host = conn.Address

			coreImp.discoveryProvider, err = discovery.NewGossipDiscoveryProvider(
				coreImp.ctx,
				*coreImp.config.Discovery.Connection,
				coreImp.logger,
				identitySigner,
				clientIdentity,
				mapper,
			)
			if err != nil {
				return nil, fmt.Errorf(`initialize discovery provider: %w`, err)
			}

			// discovery initialized, add local peers to the pool
			lDiscoverer, err := coreImp.discoveryProvider.LocalPeers(coreImp.ctx)
			if err != nil {
				return nil, fmt.Errorf(`fetch local peers from discovery provider connection=%s: %w`,
					coreImp.config.Discovery.Connection.Host, err)
			}

			peers := lDiscoverer.Peers()

			for _, lp := range peers {
				mspID := lp.MspID

				for _, lpAddresses := range lp.HostAddresses {
					peerCfg := config.ConnectionConfig{
						Host: lpAddresses.Address,
						Tls:  lpAddresses.TlsConfig,
					}

					p, err := NewPeer(coreImp.ctx, peerCfg, coreImp.identity, coreImp.logger)
					if err != nil {
						return nil, fmt.Errorf(`initialize endorsers for MSP: %s: %w`, mspID, err)
					}

					if err = coreImp.peerPool.Add(mspID, p, api.StrategyGRPC(api.DefaultDuration)); err != nil {
						return nil, fmt.Errorf(`add peer to pool: %w`, err)
					}
				}
			}
		default:
			return nil, fmt.Errorf("unknown discovery type=%v. available: %v, %v",
				coreImp.config.Discovery.Type,
				discovery.LocalConfigServiceDiscoveryType,
				discovery.GossipServiceDiscoveryType,
			)
		}
	}

	if coreImp.orderer == nil && coreImp.config != nil {
		coreImp.logger.Info("initializing orderer")
		if len(coreImp.config.Orderers) > 0 {
			ordConn, err := grpc.ConnectionFromConfigs(coreImp.ctx, coreImp.logger, coreImp.config.Orderers...)
			if err != nil {
				return nil, fmt.Errorf(`initialize orderer connection: %w`, err)
			}

			coreImp.orderer, err = NewOrdererFromGRPC(ordConn)
			if err != nil {
				return nil, fmt.Errorf(`initialize orderer: %w`, err)
			}
		}
	}

	//// use chaincode fetcher for Go chaincodes by default
	//if coreImp.fetcher == nil {
	//	coreImp.fetcher = fetcher.NewLocal(&golang.Platform{})
	//}

	return coreImp, nil
}

func (c *core) CurrentIdentity() msp.SigningIdentity {
	return c.identity
}

func (c *core) CryptoSuite() api.CryptoSuite {
	return c.cs
}

func (c *core) PeerPool() api.PeerPool {
	return c.peerPool
}

func (c *core) FabricV2() bool {
	return c.fabricV2
}

func (c *core) CurrentMspPeers() []api.Peer {
	allPeers := c.peerPool.GetPeers()

	if peers, ok := allPeers[c.identity.GetMSPIdentifier()]; !ok {
		return []api.Peer{}
	} else {
		return peers
	}
}

func (c *core) Channel(name string) api.Channel {
	logger := c.logger.Named(`channel`).With(zap.String(`channel`, name))
	c.channelMx.Lock()
	defer c.channelMx.Unlock()

	ch, ok := c.channels[name]
	if ok {
		return ch
	}

	var ord api.Orderer

	logger.Debug(`channel instance doesn't exist, initiating new`)
	discChannel, err := c.discoveryProvider.Channel(c.ctx, name)
	if err != nil {
		logger.Error(`Failed channel discovery. We'll use default orderer`, zap.Error(err))
	} else {
		// if custom orderers are enabled
		if len(discChannel.Orderers()) > 0 {
			// convert api.HostEndpoint-> grpc config.ConnectionConfig
			var grpcConnCfgs []config.ConnectionConfig
			orderers := discChannel.Orderers()

			for _, orderer := range orderers {
				if len(orderer.HostAddresses) > 0 {
					for _, hostAddr := range orderer.HostAddresses {
						grpcCfg := config.ConnectionConfig{
							Host: hostAddr.Address,
							Tls:  hostAddr.TlsConfig,
						}
						grpcConnCfgs = append(grpcConnCfgs, grpcCfg)
					}
				}
			}
			// we can have many orderers and here we establish connection with internal round-robin balancer
			ordConn, err := grpc.ConnectionFromConfigs(c.ctx, c.logger, grpcConnCfgs...)
			if err != nil {
				logger.Error(`Failed to initialize custom GRPC connection for orderer`, zap.String(`channel`, name), zap.Error(err))
			}
			if ord, err = NewOrdererFromGRPC(ordConn); err != nil {
				logger.Error(`Failed to construct orderer from GRPC connection`)
			}
		}
	}

	// using default orderer
	if ord == nil {
		ord = c.orderer
	}

	ch = NewChannel(c.identity.GetMSPIdentifier(), name, c.peerPool, ord, c.discoveryProvider, c.identity, c.fabricV2, c.logger)
	c.channels[name] = ch
	return ch
}
