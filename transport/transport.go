package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/quic-go/quic-go"
)

// ControlTransport defines the interface for a secure control channel.
type ControlTransport interface {
	AcceptStream(context.Context) (quic.Stream, error)
	OpenStream() (quic.Stream, error)
	CloseWithError(uint64, string) error
	RemoteAddr() net.Addr
}

// DialControl dials a secure control channel using mTLS.
func DialControl(ctx context.Context, addr string, tlsCfg *tls.Config, quicCfg *quic.Config) (*quic.Conn, error) {
	cfg := tlsCfg.Clone()
	if len(cfg.NextProtos) == 0 {
		cfg.NextProtos = []string{"control/v1", "qf/1"}
	}

	conn, err := quic.DialAddr(ctx, addr, cfg, quicCfg)
	if err != nil {
		return nil, fmt.Errorf("transport: DialControl: %w", err)
	}
	return conn, nil
}

// ListenControl starts a secure control channel listener.
func ListenControl(addr string, tlsCfg *tls.Config, quicCfg *quic.Config) (*quic.Listener, error) {
	cfg := tlsCfg.Clone()
	if len(cfg.NextProtos) == 0 {
		cfg.NextProtos = []string{"control/v1", "qf/1"}
	}

	ln, err := quic.ListenAddr(addr, cfg, quicCfg)
	if err != nil {
		return nil, fmt.Errorf("transport: ListenControl: %w", err)
	}
	return ln, nil
}
