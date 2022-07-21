package discovery

import (
	"sync"

	"github.com/atomyze-ru/hlf-sdk-go/api"
	"github.com/atomyze-ru/hlf-sdk-go/api/config"
)

// implementation of tlsConfigMapper interface
var _ tlsConfigMapper = (*EndpointsMapper)(nil)

// EndpointsMapper - if tls is enabled with gossip maps provided from cfg TLS certs to discovered peers
type EndpointsMapper struct {
	addressEndpoint map[string]*api.HostAddress
	lock            sync.RWMutex
}

func NewEndpointsMapper(endpoints []config.Endpoint) *EndpointsMapper {
	addressEndpointMap := make(map[string]*api.HostAddress)

	for _, e := range endpoints {
		var hostAddress api.HostAddress
		hostAddress.TlsConfig = e.TlsConfig

		hostAddress.Address = e.Address
		if e.AddressOverride != "" {
			hostAddress.Address = e.AddressOverride
		}

		addressEndpointMap[e.Address] = &hostAddress
	}

	return &EndpointsMapper{
		addressEndpoint: addressEndpointMap,
		lock:            sync.RWMutex{},
	}
}

// TlsConfigForAddress - get tls config for provided address
// if config wasn't provided on startup time return disabled tls
func (m *EndpointsMapper) TlsConfigForAddress(address string) config.TlsConfig {
	m.lock.RLock()
	defer m.lock.RUnlock()

	v, ok := m.addressEndpoint[address]
	if ok {
		return v.TlsConfig
	}

	return config.TlsConfig{
		Enabled: false,
	}
}

func (m *EndpointsMapper) TlsEndpointForAddress(address string) string {
	m.lock.RLock()
	defer m.lock.RUnlock()

	v, ok := m.addressEndpoint[address]
	if ok {
		return v.Address
	}

	return address
}

/*
	decorators over api.ChaincodeDiscoverer/ChannelDiscoverer
	adds TLS settings(if they were provided in cfg) for discovered peers
*/

type chaincodeDiscovererTLSDecorator struct {
	target    api.ChaincodeDiscoverer
	tlsMapper tlsConfigMapper
}

func newChaincodeDiscovererTLSDecorator(
	target api.ChaincodeDiscoverer,
	tlsMapper tlsConfigMapper,
) *chaincodeDiscovererTLSDecorator {
	return &chaincodeDiscovererTLSDecorator{
		target:    target,
		tlsMapper: tlsMapper,
	}
}

func (d *chaincodeDiscovererTLSDecorator) Endorsers() []*api.HostEndpoint {
	return addTLSSettings(d.target.Endorsers(), d.tlsMapper)
}

func (d *chaincodeDiscovererTLSDecorator) Orderers() []*api.HostEndpoint {
	return addTLSSettings(d.target.Orderers(), d.tlsMapper)
}

func (d *chaincodeDiscovererTLSDecorator) ChaincodeVersion() string {
	return d.target.ChaincodeVersion()
}

func (d *chaincodeDiscovererTLSDecorator) ChaincodeName() string {
	return d.target.ChaincodeName()
}

func (d *chaincodeDiscovererTLSDecorator) ChannelName() string {
	return d.target.ChannelName()
}

/* */
type channelDiscovererTLSDecorator struct {
	target    api.ChannelDiscoverer
	tlsMapper tlsConfigMapper
}

func newChannelDiscovererTLSDecorator(
	target api.ChannelDiscoverer,
	tlsMapper tlsConfigMapper,
) *channelDiscovererTLSDecorator {
	return &channelDiscovererTLSDecorator{
		target:    target,
		tlsMapper: tlsMapper,
	}
}

func (d *channelDiscovererTLSDecorator) Orderers() []*api.HostEndpoint {
	return addTLSSettings(d.target.Orderers(), d.tlsMapper)
}

func (d *channelDiscovererTLSDecorator) ChannelName() string {
	return d.target.ChannelName()
}

func addTLSSettings(endpoints []*api.HostEndpoint, tlsMapper tlsConfigMapper) []*api.HostEndpoint {
	for i := range endpoints {
		for j := range endpoints[i].HostAddresses {
			tlsCfg := tlsMapper.TlsConfigForAddress(endpoints[i].HostAddresses[j].Address)
			endpoints[i].HostAddresses[j].TlsConfig = tlsCfg

			endpoints[i].HostAddresses[j].Address = tlsMapper.TlsEndpointForAddress(endpoints[i].HostAddresses[j].Address)
		}
	}
	return endpoints
}

/* */
type localPeersDiscovererTLSDecorator struct {
	target    api.LocalPeersDiscoverer
	tlsMapper tlsConfigMapper
}

func newLocalPeersDiscovererTLSDecorator(
	target api.LocalPeersDiscoverer,
	tlsMapper tlsConfigMapper,
) *localPeersDiscovererTLSDecorator {
	return &localPeersDiscovererTLSDecorator{
		target:    target,
		tlsMapper: tlsMapper,
	}
}

func (d *localPeersDiscovererTLSDecorator) Peers() []*api.HostEndpoint {
	return addTLSSettings(d.target.Peers(), d.tlsMapper)
}