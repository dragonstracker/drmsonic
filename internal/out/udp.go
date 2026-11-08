package out

import (
	"errors"
	"net"
	"strings"
)

const datagramPayload = 188 * 7

type UDPSender struct {
	conn *net.UDPConn
}

func DialUDP(target string) (*UDPSender, error) {
	addr, err := net.ResolveUDPAddr("udp", NormalizeUDP(target))
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}
	return &UDPSender{conn: conn}, nil
}

func NormalizeUDP(target string) string {
	target = strings.TrimSpace(target)
	target = strings.TrimPrefix(target, "udp://")
	target = strings.TrimPrefix(target, "udp/")
	if i := strings.IndexAny(target, "?#"); i >= 0 {
		target = target[:i]
	}
	return target
}

func (u *UDPSender) Write(ts []byte) error {
	for len(ts) > 0 {
		n := datagramPayload
		if len(ts) < n {
			n = len(ts)
		}
		if _, err := u.conn.Write(ts[:n]); err != nil {
			return err
		}
		ts = ts[n:]
	}
	return nil
}

func (u *UDPSender) Close() error {
	if u == nil || u.conn == nil {
		return nil
	}
	return u.conn.Close()
}

var ErrNoTarget = errors.New("udp output requires a host:port target")
