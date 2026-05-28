package messages

import (
	"fmt"

	"github.com/asynkron/protoactor-go/actor"
)

type IdentityMessage interface {
	Identity() string
}

type WorldLoginReq struct {
	WorldID  int64
	Account  string
	ConnID   string
	ChannelPID *actor.PID
}

func (m WorldLoginReq) Identity() string {
	return fmt.Sprintf("world/%d", m.WorldID)
}

type PlayerLoginReq struct {
	PlayerID int64
	WorldID  int64
	Account  string
	ConnID   string
	ChannelPID *actor.PID
}

func (m PlayerLoginReq) Identity() string {
	return fmt.Sprintf("player/%d", m.PlayerID)
}

type PlayerLoginResp struct {
	PlayerID int64
	WorldID  int64
	ConnID   string
}

type PlayerLoginRejected struct {
	Reason string
}

type WorldLoginRejected struct {
	Reason string
}

type ChannelExpired struct {
	ConnID string
}
