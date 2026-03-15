package rcon_client

import (
	"errors"
	"sync"
	"time"

	"github.com/UltimateForm/tcprcon/pkg/common_rcon"
	"github.com/UltimateForm/tcprcon/pkg/packet"
	"github.com/UltimateForm/tcprcon/pkg/rcon"
)

type ControlledClient struct {
	*rcon.Client
	// unix
	lastUsed int64
	mu       sync.Mutex
}

func (src *ControlledClient) LastUsed() int64 {
	src.mu.Lock()
	defer src.mu.Unlock()
	return src.lastUsed
}

func New(uri string) (*ControlledClient, error) {
	baseClient, err := rcon.New(uri)
	if err != nil {
		return nil, err
	}
	return &ControlledClient{
		Client:   baseClient,
		lastUsed: time.Now().Unix(),
	}, nil
}

func (src *ControlledClient) Authenticate(password string) (bool, error) {
	src.mu.Lock()
	defer src.mu.Unlock()
	return common_rcon.Authenticate(src, password)
}

func (src *ControlledClient) Execute(cmd string) (string, error) {
	src.mu.Lock()
	defer src.mu.Unlock()
	defer func() {
		src.lastUsed = time.Now().Unix()
	}()

	writeId := src.Id()

	// Execute command
	execPacket := packet.New(writeId, packet.SERVERDATA_EXECCOMMAND, []byte(cmd))
	_, err := src.Write(execPacket.Serialize())
	if err != nil {
		return "", errors.Join(errors.New("failed to write command"), err)
	}

	deadline := time.Now().Add(time.Second * 30)
	for {
		if time.Now().After(deadline) {
			return "", errors.New("timed out waiting for message with expected id")
		}
		src.SetReadDeadline(time.Now().Add(10 * time.Second))

		responsePkt, err := packet.ReadWithId(src, writeId)
		if errors.Is(err, packet.ErrPacketIdMismatch) {
			continue
		}
		if err != nil {
			return "", errors.Join(errors.New("failed to read response"), err)
		}
		return responsePkt.BodyStr(), nil
	}
}
