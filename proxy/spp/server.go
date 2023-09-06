package spp

import (
	"context"
	"io"
	"spp/ray"
	"time"

	core "github.com/v2fly/v2ray-core/v5"
	"github.com/v2fly/v2ray-core/v5/common"
	"github.com/v2fly/v2ray-core/v5/common/buf"
	"github.com/v2fly/v2ray-core/v5/common/log"
	"github.com/v2fly/v2ray-core/v5/common/net"
	"github.com/v2fly/v2ray-core/v5/common/net/packetaddr"
	"github.com/v2fly/v2ray-core/v5/common/protocol"
	udp_proto "github.com/v2fly/v2ray-core/v5/common/protocol/udp"
	"github.com/v2fly/v2ray-core/v5/common/session"
	"github.com/v2fly/v2ray-core/v5/common/signal"
	"github.com/v2fly/v2ray-core/v5/common/task"
	"github.com/v2fly/v2ray-core/v5/features"
	"github.com/v2fly/v2ray-core/v5/features/policy"
	"github.com/v2fly/v2ray-core/v5/features/routing"
	"github.com/v2fly/v2ray-core/v5/transport/internet"
	"github.com/v2fly/v2ray-core/v5/transport/internet/udp"
)

// Server is a spp proxy server
type Server struct {
	config        *ServerConfig
	policyManager policy.Manager
}

// NewServer creates a new Server object.
func NewServer(ctx context.Context, config *ServerConfig) (*Server, error) {
	v := core.MustFromContext(ctx)
	s := &Server{
		config:        config,
		policyManager: v.GetFeature(policy.ManagerType()).(policy.Manager),
	}
	return s, nil
}

func (s *Server) policy() policy.Session {
	config := s.config
	p := s.policyManager.ForLevel(config.UserLevel)
	return p
}

// Network implements proxy.Inbound.
func (s *Server) Network() []net.Network {
	list := []net.Network{net.Network_TCP}
	return list
}

// Process implements proxy.Inbound.
func (s *Server) Process(ctx context.Context, network net.Network, conn internet.Connection, dispatcher routing.Dispatcher) error {
	if inbound := session.InboundFromContext(ctx); inbound != nil {
		inbound.User = &protocol.MemoryUser{
			Level: s.config.UserLevel,
		}
	}

	if network != net.Network_TCP {
		return newError("spp currently supports TCP only")
	}

	plcy := s.policy()
	if err := conn.SetReadDeadline(time.Now().Add(plcy.Timeouts.Handshake)); err != nil {
		newError("failed to set deadline").Base(err).WriteToLog(session.ExportIDToError(ctx))
	}

	ray, err := ray.NewFromConn(conn, []byte(s.config.Username), []byte(s.config.Password))
	if err != nil {
		return newError("handshake failed").Base(err).AtError()
	}

	if err := ray.SetDeadline(time.Time{}); err != nil {
		newError("failed to clear deadline after handshake").Base(err).WriteToLog(session.ExportIDToError(ctx))
	}

	ctx, cancel := context.WithCancel(ctx)
	timer := signal.CancelAfterInactivity(ctx, cancel, s.policy().Timeouts.ConnectionIdle)

	plcy = s.policy()
	ctx = policy.ContextWithBufferPolicy(ctx, plcy.Buffer)
	link, err := dispatcher.Dispatch(ctx, dest)
	if err != nil {
		return err
	}

	requestDone := func() error {
		defer timer.SetTimeout(plcy.Timeouts.DownlinkOnly)
		if err := buf.Copy(buf.NewReader(reader), link.Writer, buf.UpdateActivity(timer)); err != nil {
			return newError("failed to transport all TCP request").Base(err)
		}

		return nil
	}

	responseDone := func() error {
		defer timer.SetTimeout(plcy.Timeouts.UplinkOnly)

		v2writer := buf.NewWriter(writer)
		if err := buf.Copy(link.Reader, v2writer, buf.UpdateActivity(timer)); err != nil {
			return newError("failed to transport all TCP response").Base(err)
		}

		return nil
	}

	requestDonePost := task.OnSuccess(requestDone, task.Close(link.Writer))
	if err := task.Run(ctx, requestDonePost, responseDone); err != nil {
		common.Interrupt(link.Reader)
		common.Interrupt(link.Writer)
		return newError("connection ends").Base(err)
	}

	return nil

}
