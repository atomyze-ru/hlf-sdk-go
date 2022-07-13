package client

import (
	"context"
	"math/rand"

	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric/msp"
)

type Orderers struct {
	orderers []*Orderer
}

func NewOrderers(orderers ...*Orderer) *Orderers {
	return &Orderers{
		orderers: orderers,
	}
}

func (o *Orderers) Broadcast(ctx context.Context, envelope *common.Envelope) (*orderer.BroadcastResponse, error) {
	var errResult error

	for _, i := range rand.Perm(len(o.orderers)) {
		broadcastResponse, err := o.orderers[i].Broadcast(ctx, envelope)
		if err != nil {
			errResult = err
		} else {
			return broadcastResponse, nil
		}
	}

	return nil, errResult
}

func (o *Orderers) Deliver(ctx context.Context, envelope *common.Envelope) (*common.Block, error) {
	var errResult error

	for _, i := range rand.Perm(len(o.orderers)) {
		block, err := o.orderers[i].Deliver(ctx, envelope)
		if err != nil {
			errResult = err
		} else {
			return block, nil
		}
	}

	return nil, errResult
}

func (o *Orderers) GetConfigBlock(ctx context.Context, signer msp.SigningIdentity, channelName string) (*common.Block, error) {
	var errResult error

	for _, i := range rand.Perm(len(o.orderers)) {
		configBlock, err := o.orderers[i].GetConfigBlock(ctx, signer, channelName)
		if err != nil {
			errResult = err
		} else {
			return configBlock, nil
		}
	}

	return nil, errResult
}
