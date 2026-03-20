package rcon_client

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

// this a better (ofc lol) implementation from my python rcon connection pool
type ConnectionPool struct {
	uri        string
	password   string
	idle       chan *ControlledClient
	mu         sync.Mutex
	allocated  int
	maxSize    int
	staleAfter time.Duration
	logger     *log.Logger
}

func NewPool(uri, password string, maxSize int, staleAfter time.Duration) *ConnectionPool {
	return &ConnectionPool{
		uri:        uri,
		password:   password,
		idle:       make(chan *ControlledClient, maxSize),
		maxSize:    maxSize,
		staleAfter: staleAfter,
		logger: log.New(
			log.Default().Writer(),
			"[RconConnectionPool] ",
			log.Default().Flags(),
		),
	}
}

func (p *ConnectionPool) newClient() (*ControlledClient, error) {
	p.logger.Printf("creating new client...")
	client, err := New(p.uri)
	if err != nil {
		return nil, err
	}
	ok, err := client.Authenticate(p.password)
	if err != nil {
		client.Close()
		return nil, err
	}
	if !ok {
		client.Close()
		return nil, errors.New("authentication failed")
	}
	p.logger.Printf("new client created and authenticated")
	return client, nil
}

func (p *ConnectionPool) isStale(client *ControlledClient) bool {
	return time.Now().Unix()-client.LastUsed() > int64(p.staleAfter.Seconds())
}

func (p *ConnectionPool) Get(ctx context.Context) (*ControlledClient, error) {
	p.logger.Printf("Get() called [allocated=%d, idle=%d]", p.allocated, len(p.idle))
	for {
		// 1. Non-blocking try from idle pool
		select {
		case client := <-p.idle:
			if p.isStale(client) {
				p.logger.Printf("pulled stale client from idle, discarding [allocated=%d]", p.allocated)
				client.Close()
				p.mu.Lock()
				p.allocated--
				p.mu.Unlock()
				continue
			}
			p.logger.Printf("reusing idle client [allocated=%d, idle=%d]", p.allocated, len(p.idle))
			return client, nil
		default:
		}

		// 2. Idle empty — try to allocate a new slot
		p.mu.Lock()
		if p.allocated < p.maxSize {
			p.allocated++
			inUse := p.allocated - len(p.idle)
			p.logger.Printf("allocating new client [allocated=%d, in-use=%d]", p.allocated, inUse)
			p.mu.Unlock()
			client, err := p.newClient()
			if err != nil {
				p.mu.Lock()
				p.allocated--
				p.mu.Unlock()
				p.logger.Printf("client creation failed, slot released [allocated=%d]: %v", p.allocated, err)
				return nil, err
			}
			return client, nil
		}
		p.mu.Unlock()

		// 3. Pool at capacity — block until a client is returned or ctx is done
		p.logger.Printf("pool at capacity [allocated=%d, maxSize=%d], waiting...", p.allocated, p.maxSize)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case client := <-p.idle:
			if p.isStale(client) {
				p.logger.Printf("pulled stale client while waiting, discarding, retrying [allocated=%d]", p.allocated)
				client.Close()
				p.mu.Lock()
				p.allocated--
				p.mu.Unlock()
				continue
			}
			p.logger.Printf("got idle client after waiting [allocated=%d, idle=%d]", p.allocated, len(p.idle))
			return client, nil
		}
	}
}

func (p *ConnectionPool) Release(client *ControlledClient) {
	select {
	case p.idle <- client:
		p.logger.Printf("client released to idle [allocated=%d, idle=%d]", p.allocated, len(p.idle))
	default:
		// Idle channel is sized to maxSize so this shouldn't normally happen
		p.logger.Printf("idle pool full on release, closing excess client [allocated=%d]", p.allocated)
		client.Close()
		p.mu.Lock()
		p.allocated--
		p.mu.Unlock()
	}
}

func (p *ConnectionPool) Discard(client *ControlledClient) {
	client.Close()
	p.mu.Lock()
	p.allocated--
	p.mu.Unlock()
	p.logger.Printf("client discarded [allocated=%d, idle=%d]", p.allocated, len(p.idle))
}

func (p *ConnectionPool) WithClient(ctx context.Context, fn func(*ControlledClient) error) error {
	client, err := p.Get(ctx)
	if err != nil {
		return err
	}
	if err := fn(client); err != nil {
		p.Discard(client)
		return err
	}
	p.Release(client)
	return nil
}

func (p *ConnectionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logger.Printf("closing pool [allocated=%d, idle=%d]", p.allocated, len(p.idle))
	for {
		select {
		case client := <-p.idle:
			client.Close()
			p.allocated--
		default:
			p.logger.Printf("pool drained [remaining in-use=%d]", p.allocated)
			return
		}
	}
}
